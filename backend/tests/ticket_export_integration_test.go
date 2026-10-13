package tests

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/meddesk/meddesk/backend/internal/tickets"
)

// Export tests use the same synthetic HTTP fixture and TEST_DATABASE_URL gate
// as ticket workflows. No application credentials, .env, or real tickets are read.
func ticketExportRequest(t *testing.T, f *ticketHTTPFixture, browser *ticketBrowser, query url.Values, want int) ([]byte, http.Header) {
	t.Helper()
	request, err := http.NewRequestWithContext(f.ctx, http.MethodGet, f.server.URL+"/api/v1/tickets/export?"+query.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", "http://meddesk.test")
	response, err := browser.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != want {
		t.Fatalf("ticket export status=%d want=%d query=%s response=%s", response.StatusCode, want, query.Encode(), data)
	}
	if want != http.StatusOK {
		if bytes.HasPrefix(data, []byte("PK")) || bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) || response.Header.Get("Content-Disposition") != "" {
			t.Fatal("failed export returned a partial downloadable file")
		}
	}
	return data, response.Header.Clone()
}

func ticketExportCSV(t *testing.T, data []byte) [][]string {
	t.Helper()
	if !bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		t.Fatal("CSV lacks the UTF-8 BOM needed by Excel")
	}
	reader := csv.NewReader(bytes.NewReader(data[3:]))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return records
}

type ticketExportCell struct {
	Reference string `xml:"r,attr"`
	Type      string `xml:"t,attr"`
	Style     string `xml:"s,attr"`
	Value     string `xml:"v"`
	Formula   string `xml:"f"`
	Inline    struct {
		Text string `xml:"t"`
	} `xml:"is"`
}

func (c ticketExportCell) text() string {
	if c.Type == "inlineStr" {
		return c.Inline.Text
	}
	return c.Value
}

type ticketExportSheet struct {
	Rows []struct {
		Cells []ticketExportCell `xml:"c"`
	} `xml:"sheetData>row"`
}

func ticketExportWorkbook(t *testing.T, data []byte) map[string]ticketExportSheet {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("XLSX is not an OOXML archive: %v", err)
	}
	sheets := make(map[string]ticketExportSheet)
	for _, file := range reader.File {
		if !strings.HasPrefix(file.Name, "xl/worksheets/") || !strings.HasSuffix(file.Name, ".xml") {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		var sheet ticketExportSheet
		err = xml.NewDecoder(stream).Decode(&sheet)
		_ = stream.Close()
		if err != nil {
			t.Fatalf("invalid worksheet XML %s: %v", file.Name, err)
		}
		for _, row := range sheet.Rows {
			for _, cell := range row.Cells {
				if cell.Formula != "" {
					t.Fatalf("management export contains executable formula %s", cell.Reference)
				}
			}
		}
		sheets[file.Name] = sheet
	}
	if len(sheets) != 1 {
		t.Fatalf("XLSX requires one report worksheet with summary and tickets, got %d", len(sheets))
	}
	return sheets
}

func ticketExportFindRows(records [][]string, marker string) [][]string {
	var rows [][]string
	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		if number, err := strconv.ParseInt(record[0], 10, 64); err != nil || number < 1 {
			continue
		}
		for _, value := range record {
			if strings.Contains(value, marker) {
				rows = append(rows, record)
				break
			}
		}
	}
	return rows
}

func ticketExportRowsFromSheet(sheet ticketExportSheet) [][]string {
	rows := make([][]string, 0, len(sheet.Rows))
	for _, row := range sheet.Rows {
		values := make([]string, 0, len(row.Cells))
		for _, cell := range row.Cells {
			values = append(values, cell.text())
		}
		rows = append(rows, values)
	}
	return rows
}

func ticketExportAssertDownload(t *testing.T, headers http.Header, format string) {
	t.Helper()
	want := "text/csv"
	if format == "xlsx" {
		want = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	}
	mediaType, _, err := mime.ParseMediaType(headers.Get("Content-Type"))
	if err != nil || mediaType != want {
		t.Fatalf("incorrect export MIME type: %s (%v)", headers.Get("Content-Type"), err)
	}
	disposition, params, err := mime.ParseMediaType(headers.Get("Content-Disposition"))
	if err != nil || disposition != "attachment" || !strings.HasSuffix(params["filename"], "."+format) {
		t.Fatalf("export must download using a format-specific safe filename: %s", headers.Get("Content-Disposition"))
	}
	if strings.ContainsAny(params["filename"], "/\\\r\n") || !strings.Contains(headers.Get("Cache-Control"), "no-store") {
		t.Fatalf("invalid export filename/cache headers: %+v", headers)
	}
	var variesByCookie bool
	for _, value := range headers.Values("Vary") {
		for _, token := range strings.Split(value, ",") {
			variesByCookie = variesByCookie || strings.EqualFold(strings.TrimSpace(token), "Cookie")
		}
	}
	if !variesByCookie {
		t.Fatal("private export response must vary by authenticated Cookie")
	}
}

// Visual QA can opt into preserving these verified synthetic reports. The
// caller creates the private directory; no application data or credentials
// enter these artifacts, and existing files are never overwritten.
func ticketExportPreserveQA(t *testing.T, xlsxData, csvData []byte) {
	t.Helper()
	directory := os.Getenv("MEDDESK_EXPORT_QA_DIR")
	if directory == "" {
		return
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("MEDDESK_EXPORT_QA_DIR must be an existing private directory: %v", err)
	}
	directory, err = os.MkdirTemp(directory, "ticket-export-")
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range []struct {
		filename string
		data     []byte
	}{{"ticket-export-kk.xlsx", xlsxData}, {"ticket-export-kk.csv", csvData}} {
		path := filepath.Join(directory, artifact.filename)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		_, writeError := file.Write(artifact.data)
		closeError := file.Close()
		if writeError != nil || closeError != nil {
			t.Fatalf("unable to preserve synthetic export QA: write=%v close=%v", writeError, closeError)
		}
		t.Logf("Verified synthetic export available for visual QA: %s", path)
	}
}

func ticketExportInsert(t *testing.T, f *ticketHTTPFixture, title, created string) int64 {
	t.Helper()
	var number int64
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO tickets(title,description,requester_id,department_id,created_at) VALUES($1,'Synthetic export boundary',$2::uuid,$3::uuid,$4::timestamptz) RETURNING number`, title, f.employee.user.ID, f.employee.user.DepartmentID, created).Scan(&number); err != nil {
		t.Fatal(err)
	}
	return number
}

func ticketExportClone(values url.Values) url.Values {
	clone := make(url.Values, len(values))
	for key, value := range values {
		clone[key] = append([]string(nil), value...)
	}
	return clone
}

func TestTicketExportHTTPWorkflowAuthorizationAndArtifacts(t *testing.T) {
	f := newTicketHTTPFixture(t)
	marker := "export-" + f.employee.user.ID
	query := url.Values{"format": {"csv"}, "locale": {"kk"}, "q": {marker}, "department_id": {f.employee.user.DepartmentID}}
	ticketExportRequest(t, f, f.anonymous, query, http.StatusUnauthorized)
	ticketExportRequest(t, f, f.employee, query, http.StatusForbidden)

	// Exercise the user's original question across the real HTTP guard and DB.
	unsafeTitle := `=HYPERLINK("https://example.invalid","` + marker + `")`
	issue := ticketFromResponse(t, f.call(t, f.employee, "POST", "/tickets", tickets.CreateInput{Title: unsafeTitle, Description: "Synthetic export workflow", Priority: "HIGH", CategoryID: &f.category, ArticleIDs: []string{}, AttachmentIDs: []string{}}, http.StatusCreated))
	path := "/tickets/" + issue.ID
	issue = ticketFromResponse(t, f.call(t, f.engineer, "PATCH", path, map[string]any{"version": issue.Version, "status": tickets.InProgress, "assignee_id": f.engineer.user.ID}, http.StatusOK))
	if issue.Status != tickets.InProgress || issue.AssigneeID == nil || *issue.AssigneeID != f.engineer.user.ID {
		t.Fatal("IT engineer could not assign/start a received request")
	}
	issue = ticketFromResponse(t, f.call(t, f.engineer, "PATCH", path, map[string]any{"version": issue.Version, "status": tickets.Resolved}, http.StatusOK))
	if issue.Status != tickets.Resolved || issue.ResolvedAt == nil {
		t.Fatal("IT engineer could not resolve a received request")
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE tickets SET created_at='2025-01-15T05:00:00Z',resolved_at='2025-01-15T07:00:00Z' WHERE id=$1::uuid`, issue.ID); err != nil {
		t.Fatal(err)
	}

	// One closed request, one reopened request, and 22 new requests give a
	// 25-ticket selected cohort; historical resolution events must not inflate it.
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO tickets(title,description,requester_id,department_id,category_id,priority,status,assignee_id,created_at,resolved_at)
		SELECT $1||' batch '||n,'Synthetic export description',$2::uuid,$3::uuid,$4::uuid,'HIGH',CASE WHEN n=1 THEN 'CLOSED' WHEN n=2 THEN 'IN_PROGRESS' ELSE 'NEW' END,$5::uuid,
		'2025-01-15T05:00:00Z'::timestamptz,CASE WHEN n=1 THEN '2025-01-15T09:00:00Z'::timestamptz ELSE NULL END FROM generate_series(1,24) n`, marker, f.employee.user.ID, f.employee.user.DepartmentID, f.category, f.engineer.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO ticket_status_history(ticket_id,from_status,to_status,actor_id,reason) SELECT id,'RESOLVED','IN_PROGRESS',$2::uuid,'Synthetic reopened request' FROM tickets WHERE title=$1`, marker+" batch 2", f.engineer.user.ID); err != nil {
		t.Fatal(err)
	}
	for _, excluded := range []struct{ suffix, priority string }{{"other priority", "LOW"}, {"other priority two", "CRITICAL"}} {
		if _, err := f.pool.Exec(f.ctx, `INSERT INTO tickets(title,description,requester_id,department_id,priority,created_at) VALUES($1,'Synthetic unselected request',$2::uuid,$3::uuid,$4,'2025-01-15T05:00:00Z')`, marker+" "+excluded.suffix, f.other.user.ID, f.employee.user.DepartmentID, excluded.priority); err != nil {
			t.Fatal(err)
		}
	}
	query.Set("priority", "HIGH")
	query.Set("category_id", f.category)
	query.Set("assignee_id", f.engineer.user.ID)
	query.Set("from", "2025-01-15")
	query.Set("to", "2025-01-15")
	query.Set("timezone", "Asia/Almaty")

	unsafeRequester, unsafeEngineer, unsafeDepartment := "@requester", "+engineer", "-department "+f.employee.user.DepartmentID
	unsafeCategoryKK, unsafeCategoryRU := "=санат", "+категория"
	unsafeClinic := "+Synthetic clinic"
	var originalClinic string
	if err := f.pool.QueryRow(f.ctx, `SELECT clinic_name FROM application_settings WHERE singleton`).Scan(&originalClinic); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := f.pool.Exec(ctx, `UPDATE application_settings SET clinic_name=$1 WHERE singleton`, originalClinic); err != nil {
			t.Error(err)
		}
	})
	if _, err := f.pool.Exec(f.ctx, `UPDATE application_settings SET clinic_name=$1 WHERE singleton`, unsafeClinic); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE users SET display_name=CASE WHEN id=$1::uuid THEN $3 ELSE $4 END WHERE id IN ($1::uuid,$2::uuid)`, f.employee.user.ID, f.engineer.user.ID, unsafeRequester, unsafeEngineer); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE departments SET name=$2 WHERE id=$1::uuid`, f.employee.user.DepartmentID, unsafeDepartment); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE knowledge_categories SET name_kk=$2,name_ru=$3 WHERE id=$1::uuid`, f.category, unsafeCategoryKK, unsafeCategoryRU); err != nil {
		t.Fatal(err)
	}

	listQuery := ticketExportClone(query)
	listQuery.Del("format")
	listQuery.Del("locale")
	var page tickets.Page
	decodeTicketJSON(t, f.call(t, f.engineer, "GET", "/tickets?"+listQuery.Encode(), nil, http.StatusOK), &page)
	if page.Total != 25 || len(page.Items) != 20 {
		t.Fatalf("expected a 25-ticket filtered list paginated to 20: total=%d rows=%d", page.Total, len(page.Items))
	}

	csvData, headers := ticketExportRequest(t, f, f.engineer, query, http.StatusOK)
	ticketExportAssertDownload(t, headers, "csv")
	records := ticketExportCSV(t, csvData)
	rows := ticketExportFindRows(records, marker)
	if len(rows) != 25 {
		t.Fatalf("CSV exported current page or ignored filters: got %d selected tickets want 25", len(rows))
	}
	for _, row := range rows {
		if len(row) != 12 || row[8] != "15.01.2025 10:00" {
			t.Fatalf("CSV lost management columns or localized date formatting: %+v", row)
		}
		if row[1] == "'"+unsafeTitle && (row[10] != "15.01.2025 12:00" || !ticketExportNumberEqual(row[11], 2)) {
			t.Fatalf("CSV lost resolution time/current duration: %+v", row)
		}
		if row[1] == marker+" batch 2" && (row[10] != "" || row[11] != "") {
			t.Fatalf("CSV reopened request inherited a historic resolution: %+v", row)
		}
	}
	if len(records) == 0 || len(records[0]) != 12 || !strings.Contains(records[0][8], "Asia/Almaty") || bytes.Contains(csvData, []byte("Synthetic export workflow")) || bytes.Contains(csvData, []byte("@example.invalid")) {
		t.Fatal("CSV report lacks timezone labels or includes private descriptions/emails")
	}
	for _, value := range []string{unsafeTitle, unsafeRequester, unsafeEngineer, unsafeDepartment, unsafeCategoryKK} {
		found := false
		for _, row := range rows {
			for _, cell := range row {
				if cell == "'"+value {
					found = true
				}
				if cell == value {
					t.Fatalf("CSV cell remained executable: %q", value)
				}
			}
		}
		if !found {
			t.Fatalf("CSV did not preserve and escape unsafe user data: %q", value)
		}
	}

	query.Set("format", "xlsx")
	xlsxData, headers := ticketExportRequest(t, f, f.admin, query, http.StatusOK)
	ticketExportAssertDownload(t, headers, "xlsx")
	sheets := ticketExportWorkbook(t, xlsxData)
	var ticketSheet ticketExportSheet
	for _, sheet := range sheets {
		if len(ticketExportFindRows(ticketExportRowsFromSheet(sheet), marker)) == 25 {
			ticketSheet = sheet
		}
	}
	if len(ticketSheet.Rows) == 0 {
		t.Fatal("XLSX lost selected tickets or exported only the first page")
	}
	for _, value := range []string{unsafeTitle, unsafeRequester, unsafeEngineer, unsafeDepartment, unsafeCategoryKK, unsafeClinic} {
		found := false
		for _, row := range ticketSheet.Rows {
			for _, cell := range row.Cells {
				if cell.text() == value {
					found = true
					if cell.Type != "inlineStr" || cell.Formula != "" {
						t.Fatalf("XLSX user data is not a plain inline string: %+v", cell)
					}
				}
			}
		}
		if !found {
			t.Fatalf("XLSX changed or lost literal user data: %q", value)
		}
	}
	cells := make(map[string]ticketExportCell)
	for _, row := range ticketSheet.Rows {
		for _, cell := range row.Cells {
			cells[cell.Reference] = cell
		}
	}
	for reference, want := range map[string]float64{"A7": 25, "C7": 23, "E7": 2, "G7": 1, "I7": 3} {
		cell, exists := cells[reference]
		if !exists || cell.Type == "inlineStr" || !ticketExportNumberEqual(cell.Value, want) {
			t.Fatalf("management summary %s did not count the selected current cohort: %+v want %v", reference, cell, want)
		}
	}
	if cells["E3"].text() != "Asia/Almaty" || !strings.Contains(cells["B4"].text(), "2025") {
		t.Fatalf("XLSX lost selected timezone/period context: %+v %+v", cells["E3"], cells["B4"])
	}
	for _, label := range []string{unsafeDepartment, unsafeCategoryKK, unsafeEngineer} {
		if !strings.Contains(cells["B5"].text(), label) {
			t.Fatalf("XLSX filter metadata omitted readable department/category/assignee name %q: %s", label, cells["B5"].text())
		}
	}
	const createdSerial = 45672.0 + 10.0/24 // 15 January 2025, 10:00 Asia/Almaty.
	for _, row := range ticketSheet.Rows {
		if len(row.Cells) == 0 {
			continue
		}
		if number, err := strconv.ParseInt(row.Cells[0].text(), 10, 64); err != nil || number < 1 || len(row.Cells) < 2 || !strings.Contains(row.Cells[1].text(), marker) {
			continue
		}
		var title, created, resolved, duration ticketExportCell
		for _, cell := range row.Cells {
			switch strings.TrimRight(cell.Reference, "0123456789") {
			case "B":
				title = cell
			case "I":
				created = cell
			case "K":
				resolved = cell
			case "L":
				duration = cell
			}
		}
		if created.Type == "inlineStr" || created.Style == "" || !ticketExportNumberEqual(created.Value, createdSerial) {
			t.Fatalf("XLSX created date is not a sortable local datetime: %+v", created)
		}
		switch title.text() {
		case unsafeTitle:
			if resolved.Type == "inlineStr" || resolved.Style == "" || !ticketExportNumberEqual(resolved.Value, createdSerial+2.0/24) || !ticketExportNumberEqual(duration.Value, 2) {
				t.Fatalf("resolved request lost typed date/current duration: %+v %+v", resolved, duration)
			}
		case marker + " batch 1":
			if !ticketExportNumberEqual(duration.Value, 4) {
				t.Fatalf("closed request duration was omitted: %+v", duration)
			}
		default:
			if resolved.text() != "" || duration.text() != "" {
				t.Fatalf("open/reopened request inherited a historical resolution: %+v %+v", resolved, duration)
			}
		}
	}

	// Both staff roles can report all visible requests. Mine is still the
	// requester filter used by the list, rather than the assigned engineer.
	query.Set("format", "csv")
	query.Set("mine", "true")
	mine, _ := ticketExportRequest(t, f, f.engineer, query, http.StatusOK)
	if len(ticketExportFindRows(ticketExportCSV(t, mine), marker)) != 0 {
		t.Fatal("mine export treated assigned requests as engineer-created requests")
	}
	query.Del("mine")
	query.Set("status", tickets.Resolved)
	onlyResolved, _ := ticketExportRequest(t, f, f.engineer, query, http.StatusOK)
	if len(ticketExportFindRows(ticketExportCSV(t, onlyResolved), marker)) != 1 {
		t.Fatal("status export filter omitted or leaked requests")
	}
	query.Del("status")
	query.Set("status", tickets.New)
	query.Set("format", "xlsx")
	openData, _ := ticketExportRequest(t, f, f.engineer, query, http.StatusOK)
	for _, sheet := range ticketExportWorkbook(t, openData) {
		for _, row := range sheet.Rows {
			for _, cell := range row.Cells {
				if cell.Reference == "I7" && cell.text() != "" {
					t.Fatal("open-only report invented a zero-valued resolution average")
				}
			}
		}
	}
	query.Del("status")
	query.Set("format", "csv")
	query.Set("locale", "ru")
	russian, _ := ticketExportRequest(t, f, f.engineer, query, http.StatusOK)
	if !bytes.Contains(russian, []byte("'"+unsafeCategoryRU)) || bytes.Contains(russian, []byte(unsafeCategoryKK)) {
		t.Fatal("export did not honor Russian category localization")
	}
	ticketExportPreserveQA(t, xlsxData, csvData)

	if _, err := f.pool.Exec(f.ctx, `UPDATE users SET role_id=(SELECT id FROM roles WHERE code='EMPLOYEE') WHERE id=$1::uuid`, f.engineer.user.ID); err != nil {
		t.Fatal(err)
	}
	ticketExportRequest(t, f, f.engineer, query, http.StatusForbidden)
	// A caller's old User struct is also insufficient if bypassing HTTP guard.
	service := tickets.NewService(tickets.NewRepository(f.pool), nil)
	if _, err := service.Export(f.ctx, f.engineer.user, tickets.ExportInput{Format: "csv"}); !errors.Is(err, tickets.ErrForbidden) {
		t.Fatalf("export repository trusted an old staff role: %v", err)
	}
}

func TestTicketExportHTTPCivilDayBoundariesDefaultTimezoneAndValidation(t *testing.T) {
	f := newTicketHTTPFixture(t)
	marker := "export-days-" + f.employee.user.ID
	for _, row := range []struct{ label, at string }{
		{"before", "2025-01-14T18:59:59Z"},
		{"start", "2025-01-14T19:00:00Z"},
		{"last", "2025-01-15T18:59:59Z"},
		{"after", "2025-01-15T19:00:00Z"},
	} {
		ticketExportInsert(t, f, marker+" "+row.label, row.at)
	}
	query := url.Values{"format": {"csv"}, "locale": {"ru"}, "q": {marker}, "department_id": {f.employee.user.DepartmentID}, "from": {"2025-01-15"}, "to": {"2025-01-15"}, "timezone": {"Asia/Almaty"}}
	data, _ := ticketExportRequest(t, f, f.engineer, query, http.StatusOK)
	rows := ticketExportFindRows(ticketExportCSV(t, data), marker)
	if len(rows) != 2 || !bytes.Contains(data, []byte(marker+" start")) || !bytes.Contains(data, []byte(marker+" last")) {
		t.Fatal("inclusive local dates must include start and final second, excluding adjacent civil days")
	}
	var originalTimezone string
	if err := f.pool.QueryRow(f.ctx, `SELECT reporting_timezone FROM application_settings WHERE singleton`).Scan(&originalTimezone); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := f.pool.Exec(ctx, `UPDATE application_settings SET reporting_timezone=$1 WHERE singleton`, originalTimezone); err != nil {
			t.Error(err)
		}
	})
	if _, err := f.pool.Exec(f.ctx, `UPDATE application_settings SET reporting_timezone='UTC' WHERE singleton`); err != nil {
		t.Fatal(err)
	}
	query.Del("timezone")
	data, _ = ticketExportRequest(t, f, f.engineer, query, http.StatusOK)
	rows = ticketExportFindRows(ticketExportCSV(t, data), marker)
	if len(rows) != 2 || !bytes.Contains(data, []byte(marker+" last")) || !bytes.Contains(data, []byte(marker+" after")) || bytes.Contains(data, []byte(marker+" start")) {
		t.Fatal("export without timezone ignored application reporting timezone")
	}

	markerDST := "export-dst-" + f.employee.user.ID
	for _, row := range []struct{ label, at string }{
		{"before", "2025-03-09T04:59:59Z"},
		{"start", "2025-03-09T05:00:00Z"},
		{"last", "2025-03-10T03:59:59Z"},
		{"after", "2025-03-10T04:00:00Z"},
	} {
		ticketExportInsert(t, f, markerDST+" "+row.label, row.at)
	}
	query.Set("q", markerDST)
	query.Set("from", "2025-03-09")
	query.Set("to", "2025-03-09")
	query.Set("timezone", "America/New_York")
	data, _ = ticketExportRequest(t, f, f.engineer, query, http.StatusOK)
	if rows := ticketExportFindRows(ticketExportCSV(t, data), markerDST); len(rows) != 2 || !bytes.Contains(data, []byte(markerDST+" start")) || !bytes.Contains(data, []byte(markerDST+" last")) {
		t.Fatal("23-hour DST civil day was treated as a fixed 24 hours")
	}
	query.Del("from")
	query.Del("to")
	data, _ = ticketExportRequest(t, f, f.admin, query, http.StatusOK)
	if rows := ticketExportFindRows(ticketExportCSV(t, data), markerDST); len(rows) != 4 {
		t.Fatal("export with no dates should include all selected history")
	}
	defaults := ticketExportClone(query)
	defaults.Del("format")
	defaults.Del("locale")
	defaultData, headers := ticketExportRequest(t, f, f.admin, defaults, http.StatusOK)
	ticketExportAssertDownload(t, headers, "xlsx")
	for _, sheet := range ticketExportWorkbook(t, defaultData) {
		if rows := ticketExportFindRows(ticketExportRowsFromSheet(sheet), markerDST); len(rows) != 4 {
			t.Fatal("default XLSX export lost all-time requests")
		}
	}

	invalid := []url.Values{
		{"format": {"pdf"}}, {"locale": {"en"}}, {"format": {"csv", "xlsx"}}, {"format": {""}},
		{"format": {"csv"}, "offset": {"0"}}, {"format": {"csv"}, "limit": {"20"}},
		{"format": {"csv"}, "unknown": {"true"}}, {"format": {"csv"}, "mine": {"yes"}},
		{"format": {"csv"}, "status": {"INVALID"}}, {"format": {"csv"}, "priority": {"INVALID"}},
		{"format": {"csv"}, "assignee_id": {"invalid"}}, {"format": {"csv"}, "department_id": {"invalid"}},
		{"format": {"csv"}, "category_id": {"invalid"}}, {"format": {"csv"}, "q": {strings.Repeat("x", 101)}},
		{"format": {"csv"}, "from": {"2025-01-01"}}, {"format": {"csv"}, "to": {"2025-01-01"}},
		{"format": {"csv"}, "from": {"2025-02-30"}, "to": {"2025-03-01"}},
		{"format": {"csv"}, "from": {"2025-03-01"}, "to": {"2025-02-28"}},
		{"format": {"csv"}, "from": {"2023-01-01"}, "to": {"2024-01-02"}},
		{"format": {"csv"}, "timezone": {"Invalid/Timezone"}},
	}
	for index, values := range invalid {
		t.Run(fmt.Sprintf("invalid_%d", index), func(t *testing.T) {
			ticketExportRequest(t, f, f.engineer, values, http.StatusBadRequest)
		})
	}
	for _, malformed := range []string{"format=csv&status=%GG", "format=csv&q=ignored;status=NEW"} {
		f.call(t, f.engineer, "GET", "/tickets/export?"+malformed, nil, http.StatusBadRequest)
	}
}

func TestTicketExportHTTPLimitRejectsWholeFile(t *testing.T) {
	f := newTicketHTTPFixture(t)
	marker := "export-limit-" + f.employee.user.ID
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO tickets(title,description,requester_id,department_id) SELECT $1||' '||n,'Synthetic export limit',$2::uuid,$3::uuid FROM generate_series(1,5001) n`, marker, f.employee.user.ID, f.employee.user.DepartmentID); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"csv", "xlsx"} {
		t.Run(format, func(t *testing.T) {
			data, _ := ticketExportRequest(t, f, f.engineer, url.Values{"format": {format}, "q": {marker}, "department_id": {f.employee.user.DepartmentID}}, http.StatusConflict)
			var failure struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(data, &failure); err != nil || failure.Error.Code != "export_limit" {
				t.Fatalf("oversized export did not return actionable export_limit JSON: %s (%v)", data, err)
			}
		})
	}
}

// Excel serials represent local civil times. This helper is used when checking
// that timestamps and hours remain sortable numeric cells in the workbook.
func ticketExportNumberEqual(value string, want float64) bool {
	number, err := strconv.ParseFloat(value, 64)
	return err == nil && math.Abs(number-want) < 0.000001
}
