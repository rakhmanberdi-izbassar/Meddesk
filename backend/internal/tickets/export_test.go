package tickets

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"
)

type reportCell struct {
	Ref   string `xml:"r,attr"`
	Type  string `xml:"t,attr"`
	Style string `xml:"s,attr"`
	Value string `xml:"v"`
	Text  string `xml:"is>t"`
}

func reportParts(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string][]byte{}
	for _, file := range z.File {
		r, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		part, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Fatal(err)
		}
		decoder := xml.NewDecoder(bytes.NewReader(part))
		for {
			_, err = decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("invalid %s: %v", file.Name, err)
			}
		}
		parts[file.Name] = part
	}
	return parts
}
func reportCells(t *testing.T, part []byte) map[string]reportCell {
	t.Helper()
	var sheet struct {
		Rows []struct {
			Cells []reportCell `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(part, &sheet); err != nil {
		t.Fatal(err)
	}
	cells := map[string]reportCell{}
	for _, row := range sheet.Rows {
		for _, cell := range row.Cells {
			cells[cell.Ref] = cell
		}
	}
	return cells
}
func exportFixtureRows() []ExportRow {
	created := time.Date(2026, 10, 11, 19, 0, 0, 0, time.UTC)
	resolved := created.Add(3 * time.Hour)
	closed := created.Add(5 * time.Hour)
	return []ExportRow{
		{Number: 42, Title: `=HYPERLINK("https://example.invalid","click")`, RequesterName: " +SUM(1,2)", DepartmentName: "\t=1", CategoryKK: "-1+2", CategoryRU: "@SUM(1,2)", Priority: "HIGH", Status: Resolved, AssigneeName: "\r+cmd", CreatedAt: created, UpdatedAt: resolved, ResolvedAt: &resolved},
		{Number: 41, Title: "Қазақша тақырып & <XML>", RequesterName: "Дәрігер", DepartmentName: "Бөлім", CategoryKK: "Желі", CategoryRU: "Сеть", Priority: "MEDIUM", Status: Closed, AssigneeName: "Инженер", CreatedAt: created, UpdatedAt: closed, ResolvedAt: &closed},
		{Number: 40, Title: "Ашық өтінім", Priority: "LOW", Status: New, CreatedAt: created, UpdatedAt: created},
	}
}
func TestExportXLSXRealWorkbookLiteralTextNumericDatesAndSummary(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Almaty")
	snapshot := ExportSnapshot{ClinicName: "Қалалық емхана", Timezone: "Asia/Almaty", Rows: exportFixtureRows()}
	data, err := exportXLSX(snapshot, Filter{Search: "=literal"}, exportLabelsFor("kk"), loc, time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	parts := reportParts(t, data)
	for _, required := range []string{"[Content_Types].xml", "_rels/.rels", "xl/workbook.xml", "xl/_rels/workbook.xml.rels", "xl/styles.xml", "xl/worksheets/sheet1.xml"} {
		if len(parts[required]) == 0 {
			t.Fatalf("missing workbook part %s", required)
		}
	}
	for name, part := range parts {
		if strings.Contains(string(part), "<f>") || strings.Contains(string(part), "<hyperlinks") || strings.Contains(string(part), `TargetMode="External"`) {
			t.Fatalf("executable text in %s", name)
		}
	}
	cells := reportCells(t, parts["xl/worksheets/sheet1.xml"])
	if cells["B10"].Type != "inlineStr" || cells["B10"].Text != snapshot.Rows[0].Title || cells["B11"].Text != snapshot.Rows[1].Title {
		t.Fatal("user text was modified or not stored literally")
	}
	if cells["A10"].Type != "" || cells["A10"].Value != "42" {
		t.Fatal("ticket number is not numeric")
	}
	// 12 October 2026 is serial 46307; 19:00 UTC is midnight in Almaty.
	if cells["I10"].Type != "" || cells["I10"].Style != "8" || cells["I10"].Value != "46307" {
		t.Fatalf("wrong local numeric creation date: %+v", cells["I10"])
	}
	for ref, want := range map[string]string{"A7": "3", "C7": "1", "E7": "2", "G7": "1", "I7": "4", "L10": "3", "L11": "5"} {
		if cells[ref].Value != want || cells[ref].Type != "" {
			t.Errorf("%s: want numeric %s, got %+v", ref, want, cells[ref])
		}
	}
	if cells["L12"].Value != "" || cells["K12"].Value != "" {
		t.Fatal("unresolved ticket has resolution values")
	}
	if !strings.Contains(string(parts["xl/worksheets/sheet1.xml"]), `<autoFilter ref="A9:L12"`) || !strings.Contains(string(parts["xl/worksheets/sheet1.xml"]), `ySplit="9"`) {
		t.Fatal("report lacks frozen/filterable headers")
	}
	if !strings.Contains(string(parts["xl/styles.xml"]), `formatCode="dd.mm.yyyy hh:mm"`) {
		t.Fatal("report lacks readable date formatting")
	}
	if !strings.Contains(string(parts["xl/worksheets/sheet1.xml"]), `<pageSetUpPr fitToPage="1"`) {
		t.Fatal("report's fit-to-width print setting is not enabled")
	}
}
func TestExportCSVUnicodeLocalizedDatesAndFormulaSafety(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Almaty")
	snapshot := ExportSnapshot{Timezone: "Asia/Almaty", Rows: exportFixtureRows()}
	data, err := exportCSV(snapshot, exportLabelsFor("ru"), loc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		t.Fatal("CSV has no UTF-8 BOM")
	}
	rows, err := csv.NewReader(bytes.NewReader(data[3:])).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 || len(rows[0]) != 12 || rows[0][0] != "№ заявки" || rows[0][8] != "Создана (Asia/Almaty)" {
		t.Fatalf("wrong CSV layout: %v", rows)
	}
	for _, col := range []int{1, 2, 3, 4, 7} {
		if !strings.HasPrefix(rows[1][col], "'") {
			t.Errorf("column %d formula not escaped: %q", col, rows[1][col])
		}
	}
	if rows[1][5] != "Высокий" || rows[1][6] != "Решена" || rows[1][8] != "12.10.2026 00:00" || rows[1][10] != "12.10.2026 03:00" || rows[1][11] != "3.00" {
		t.Fatalf("wrong locale/date/duration: %v", rows[1])
	}
	if rows[2][1] != snapshot.Rows[1].Title || rows[2][4] != "Сеть" || rows[3][10] != "" || rows[3][11] != "" {
		t.Fatal("Unicode or missing resolution values changed")
	}
	for _, text := range []string{"=1", "+1", "-1", "@SUM(1)", "\ttext", "\rtext", "\ntext", "  =1", " \t@cmd", "\u2003=1"} {
		if csvLiteral(text) != "'"+text {
			t.Errorf("dangerous prefix left literal: %q", text)
		}
	}
	if csvLiteral("Тақырып") != "Тақырып" {
		t.Fatal("plain Unicode text modified")
	}
}
func TestExportDatesRemainOptionalAndCompleteCalendarIntervals(t *testing.T) {
	for _, f := range []Filter{{}, {Timezone: "Asia/Almaty"}, {From: "2024-01-01", To: "2024-12-31"}, {From: "2026-03-08", To: "2026-03-08", Timezone: "America/New_York"}} {
		if _, err := normalizeFilter(f); err != nil {
			t.Errorf("valid filter rejected: %+v: %v", f, err)
		}
	}
	for _, f := range []Filter{{From: "2026-10-12"}, {To: "2026-10-12"}, {From: "2026-10-13", To: "2026-10-12"}, {From: "2024-01-01", To: "2025-01-01"}, {From: "2026-02-30", To: "2026-03-01"}, {Timezone: "Local"}, {Timezone: "Invalid/Zone"}} {
		if _, err := normalizeFilter(f); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("invalid filter accepted: %+v: %v", f, err)
		}
	}
	period, err := resolveFilterPeriod(Filter{}, "Asia/Almaty", time.Now())
	if err != nil || period != nil {
		t.Fatal("default all-time report gained an implicit date range")
	}
	period, err = resolveFilterPeriod(Filter{From: "2026-03-08", To: "2026-03-08"}, "America/New_York", time.Now())
	if err != nil || period.EndAt.Sub(period.StartAt) != 23*time.Hour {
		t.Fatalf("DST calendar date must include complete day: %+v %v", period, err)
	}
}
func TestExportServiceAuthorizationFormatsFiltersAndRowCap(t *testing.T) {
	s, store := fixture()
	if _, err := s.Export(context.Background(), employee(), ExportInput{}); !errors.Is(err, ErrForbidden) || store.exports != 0 {
		t.Fatal("employee export reached snapshot")
	}
	for _, in := range []ExportInput{{Format: "pdf"}, {Locale: "en"}, {Filter: Filter{Limit: 20}}, {Filter: Filter{Offset: 20}}, {Filter: Filter{Search: strings.Repeat("x", 101)}}, {Filter: Filter{Mine: true, From: "2026-10-12"}}} {
		if _, err := s.Export(context.Background(), engineer(), in); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("invalid export accepted: %+v %v", in, err)
		}
	}
	file, err := s.Export(context.Background(), engineer(), ExportInput{Filter: Filter{Search: "  тақырып  ", Status: New, Mine: true}})
	if err != nil || file.ContentType != xlsxContentType || !strings.HasSuffix(file.Filename, ".xlsx") || store.exportFilter.Search != "тақырып" || !store.exportFilter.Mine {
		t.Fatalf("default export/filter contract: %+v %v", file, err)
	}
	store.tx.users[engineerID] = employee()
	if _, err := s.Export(context.Background(), engineer(), ExportInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatal("stale session role can export")
	}
	store.tx.users[engineerID] = engineer()
	store.exportSnapshot = &ExportSnapshot{Timezone: "Asia/Almaty", Rows: make([]ExportRow, MaxExportRows+1)}
	if file, err := s.Export(context.Background(), engineer(), ExportInput{}); !errors.Is(err, ErrExportLimit) || len(file.Data) != 0 {
		t.Fatal("over-limit export leaked partial/truncated file")
	}
	store.exportSnapshot.Rows = nil
	file, err = s.Export(context.Background(), engineer(), ExportInput{})
	if err != nil {
		t.Fatal(err)
	}
	cells := reportCells(t, reportParts(t, file.Data)["xl/worksheets/sheet1.xml"])
	if cells["A7"].Value != "0" || cells["I7"].Value != "" {
		t.Fatal("empty report must have zero total and null average")
	}
}
func TestExportSerializedSizeCapAndNumericTimezoneDate(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	date := time.Date(2026, 3, 8, 7, 30, 0, 0, time.UTC) // 03:30 after the DST jump.
	serial := excelDate(date, loc)
	clock := (serial - math.Floor(serial)) * 24
	if math.Abs(clock-3.5) > 1e-7 {
		t.Fatalf("wrong wall-clock serial %s", strconv.FormatFloat(serial, 'f', -1, 64))
	}
	var out exportBuffer
	if _, err := out.Write(make([]byte, MaxExportBytes)); err != nil {
		t.Fatal(err)
	}
	if _, err := out.Write([]byte{1}); !errors.Is(err, ErrExportLimit) {
		t.Fatal("serialized response limit not enforced")
	}
}
func TestExportFilterContextUsesLocalizedHistoricalDisplayNames(t *testing.T) {
	f := Filter{DepartmentID: actorID, CategoryID: categoryID, AssigneeID: engineerID}
	names := ExportFilterLabels{DepartmentName: "Реанимация", CategoryKK: "Желі", CategoryRU: "Сеть", AssigneeName: "Инженер"}
	context := filterContext(f, exportLabelsFor("ru"), names)
	if !strings.Contains(context, "Отдел: Реанимация") || !strings.Contains(context, "Категория: Сеть") || !strings.Contains(context, "Исполнитель: Инженер") || strings.Contains(context, categoryID) {
		t.Fatalf("report has unreadable filter context: %s", context)
	}
}

func TestExportXLSXWrappedTitleAndNamesReceiveReadableBoundedRowHeights(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Almaty")
	rows := exportFixtureRows()
	rows[0].Title = strings.Repeat("Қ", 250)
	rows[1].Title = "Қысқа тақырып"
	rows[1].RequesterName = strings.Repeat("Д", 150)
	rows[2].Title = "Қысқа"
	data, err := exportXLSX(ExportSnapshot{Timezone: "Asia/Almaty", Rows: rows}, Filter{}, exportLabelsFor("kk"), loc, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	parts := reportParts(t, data)
	var sheet struct {
		Rows []struct {
			Index  int `xml:"r,attr"`
			Height int `xml:"ht,attr"`
		} `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(parts["xl/worksheets/sheet1.xml"], &sheet); err != nil {
		t.Fatal(err)
	}
	heights := map[int]int{}
	for _, row := range sheet.Rows {
		heights[row.Index] = row.Height
	}
	if heights[3] < 36 || heights[4] < 36 {
		t.Fatal("wrapped metadata labels can be clipped")
	}
	if heights[10] < 6*15 || heights[11] < 6*15 || heights[12] != 42 {
		t.Fatalf("long content was not accommodated while short rows stayed compact: %+v", heights)
	}
	if cells := reportCells(t, parts["xl/worksheets/sheet1.xml"]); cells["B10"].Text != rows[0].Title || cells["C11"].Text != rows[1].RequesterName {
		t.Fatal("height adjustment truncated wrapped text")
	}
	if height := exportRowHeight([]string{strings.Repeat("line\n", 100)}); height != 409 {
		t.Fatalf("Excel row height limit exceeded: %d", height)
	}
	if height := exportRowHeight([]string{strings.Repeat("字", 250)}); height <= heights[10] || height > 409 {
		t.Fatalf("wide glyphs did not receive sufficient bounded space: %d", height)
	}
}
