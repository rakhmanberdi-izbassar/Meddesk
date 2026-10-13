package tickets

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/meddesk/meddesk/backend/internal/analytics"
	"github.com/meddesk/meddesk/backend/internal/auth"
)

const (
	MaxExportRows   = 5000
	MaxExportBytes  = 20 << 20
	xlsxContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)

func (s *Service) Export(ctx context.Context, actor auth.User, in ExportInput) (ExportFile, error) {
	if !analytics.IsStaff(actor) {
		return ExportFile{}, ErrForbidden
	}
	if in.Format == "" {
		in.Format = "xlsx"
	}
	if in.Locale == "" {
		in.Locale = "kk"
	}
	if (in.Format != "xlsx" && in.Format != "csv") || (in.Locale != "kk" && in.Locale != "ru") || in.Filter.Limit != 0 || in.Filter.Offset != 0 {
		return ExportFile{}, ErrInvalidInput
	}
	f, err := normalizeFilter(in.Filter)
	if err != nil {
		return ExportFile{}, err
	}
	now := time.Now()
	snapshot, err := s.repo.ExportSnapshot(ctx, actor, f, now)
	if err != nil {
		return ExportFile{}, err
	}
	if len(snapshot.Rows) > MaxExportRows {
		return ExportFile{}, ErrExportLimit
	}
	loc, err := time.LoadLocation(snapshot.Timezone)
	if err != nil {
		return ExportFile{}, ErrInvalidInput
	}
	labels := exportLabelsFor(in.Locale)
	file := ExportFile{Filename: "meddesk-tickets-" + now.UTC().Format("20060102-150405") + "." + in.Format, ContentType: xlsxContentType}
	if in.Format == "csv" {
		file.ContentType = "text/csv; charset=utf-8"
		file.Data, err = exportCSV(snapshot, labels, loc)
	} else {
		file.Data, err = exportXLSX(snapshot, f, labels, loc, now)
	}
	if err != nil {
		return ExportFile{}, err
	}
	return file, nil
}

type exportLabels struct {
	sheet, title, generated, timezone, period, allTime, filters, noFilters string
	total, open, resolved, closed, mean                                    string
	columns                                                                []string
	priorities, statuses                                                   map[string]string
	search, priority, status, assignee, department, category, mine         string
}

func exportLabelsFor(locale string) exportLabels {
	if locale == "ru" {
		return exportLabels{
			sheet: "Заявки", title: "Отчёт по заявкам IT-службы", generated: "Сформирован", timezone: "Часовой пояс", period: "Дата создания", allTime: "За всё время", filters: "Фильтры", noFilters: "Все заявки",
			total: "Всего заявок", open: "Открытые", resolved: "Решённые (включая закрытые)", closed: "Закрытые", mean: "Среднее время решения, ч",
			columns:    []string{"№ заявки", "Тема", "Заявитель", "Отдел", "Категория", "Приоритет", "Статус", "Исполнитель", "Создана", "Обновлена", "Решена", "Время решения, ч"},
			priorities: map[string]string{"LOW": "Низкий", "MEDIUM": "Средний", "HIGH": "Высокий", "CRITICAL": "Критический"},
			statuses:   map[string]string{New: "Новая", InProgress: "В работе", Waiting: "Ожидает пользователя", Resolved: "Решена", Closed: "Закрыта"},
			search:     "Поиск", priority: "Приоритет", status: "Статус", assignee: "Исполнитель", department: "Отдел", category: "Категория", mine: "Только мои заявки",
		}
	}
	return exportLabels{
		sheet: "Өтінімдер", title: "IT қызметінің өтінімдер есебі", generated: "Қалыптастырылған", timezone: "Уақыт белдеуі", period: "Құрылған күні", allTime: "Барлық уақыт", filters: "Сүзгілер", noFilters: "Барлық өтінімдер",
		total: "Барлық өтінімдер", open: "Ашық", resolved: "Шешілген (жабылғандармен)", closed: "Жабылған", mean: "Орташа шешу уақыты, сағ",
		columns:    []string{"Өтінім №", "Тақырып", "Өтініш беруші", "Бөлім", "Санат", "Басымдық", "Мәртебе", "Орындаушы", "Құрылған", "Жаңартылған", "Шешілген", "Шешу уақыты, сағ"},
		priorities: map[string]string{"LOW": "Төмен", "MEDIUM": "Орташа", "HIGH": "Жоғары", "CRITICAL": "Шұғыл"},
		statuses:   map[string]string{New: "Жаңа", InProgress: "Орындалуда", Waiting: "Қызметкер жауабы күтілуде", Resolved: "Шешілген", Closed: "Жабылған"},
		search:     "Іздеу", priority: "Басымдық", status: "Мәртебе", assignee: "Орындаушы", department: "Бөлім", category: "Санат", mine: "Тек менің өтінімдерім",
	}
}
func localized(mapping map[string]string, value string) string {
	if name, ok := mapping[value]; ok {
		return name
	}
	return value
}
func rowTexts(row ExportRow, labels exportLabels) []string {
	category := row.CategoryKK
	if labels.sheet == "Заявки" {
		category = row.CategoryRU
	}
	return []string{row.Title, row.RequesterName, row.DepartmentName, category, localized(labels.priorities, row.Priority), localized(labels.statuses, row.Status), row.AssigneeName}
}
func resolutionHours(row ExportRow) *float64 {
	if (row.Status != Resolved && row.Status != Closed) || row.ResolvedAt == nil {
		return nil
	}
	hours := row.ResolvedAt.Sub(row.CreatedAt).Hours()
	return &hours
}

// A bounded writer prevents a failed export from allocating an unbounded
// document or writing partial file bytes to the HTTP response.
type exportBuffer struct{ bytes.Buffer }

func (b *exportBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > MaxExportBytes {
		return 0, ErrExportLimit
	}
	return b.Buffer.Write(p)
}
func (b *exportBuffer) WriteString(s string) (int, error) { return b.Write([]byte(s)) }

// CSV has no literal-string cell type. Escape potentially executable prefixes
// even when they are hidden behind whitespace; check every textual field.
func csvLiteral(s string) string {
	trimmed := strings.TrimLeftFunc(s, unicode.IsSpace)
	if trimmed != s || (trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0]))) {
		return "'" + s
	}
	return s
}
func exportCSV(snapshot ExportSnapshot, labels exportLabels, loc *time.Location) ([]byte, error) {
	var out exportBuffer
	if _, err := out.Write([]byte{0xef, 0xbb, 0xbf}); err != nil {
		return nil, err
	}
	w := csv.NewWriter(&out)
	columns := append([]string{}, labels.columns...)
	for i := 8; i <= 10; i++ {
		columns[i] += " (" + snapshot.Timezone + ")"
	}
	if err := w.Write(columns); err != nil {
		return nil, err
	}
	for _, row := range snapshot.Rows {
		values := []string{strconv.FormatInt(row.Number, 10)}
		for _, value := range rowTexts(row, labels) {
			values = append(values, csvLiteral(value))
		}
		values = append(values, row.CreatedAt.In(loc).Format("02.01.2006 15:04"), row.UpdatedAt.In(loc).Format("02.01.2006 15:04"))
		resolved := ""
		if row.ResolvedAt != nil {
			resolved = row.ResolvedAt.In(loc).Format("02.01.2006 15:04")
		}
		values = append(values, resolved)
		hours := ""
		if duration := resolutionHours(row); duration != nil {
			hours = strconv.FormatFloat(*duration, 'f', 2, 64)
		}
		values = append(values, hours)
		if err := w.Write(values); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

const spreadsheetNS = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
const xmlDeclaration = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`

func xmlText(w io.Writer, value string) error {
	// XML 1.0 forbids a few characters that could exist in legacy display names.
	value = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' || (r >= 0x20 && r <= 0xd7ff) || (r >= 0xe000 && r <= 0xfffd) || (r >= 0x10000 && r <= 0x10ffff) {
			return r
		}
		return '\ufffd'
	}, value)
	return xml.EscapeText(w, []byte(value))
}

type sheetWriter struct {
	out *exportBuffer
	err error
}

func (w *sheetWriter) raw(s string) {
	if w.err == nil {
		_, w.err = w.out.WriteString(s)
	}
}
func (w *sheetWriter) text(ref string, style int, value string) {
	w.raw(fmt.Sprintf(`<c r="%s" s="%d" t="inlineStr"><is><t xml:space="preserve">`, ref, style))
	if w.err == nil {
		w.err = xmlText(w.out, value)
	}
	w.raw(`</t></is></c>`)
}
func (w *sheetWriter) number(ref string, style int, value float64) {
	w.raw(fmt.Sprintf(`<c r="%s" s="%d"><v>%s</v></c>`, ref, style, strconv.FormatFloat(value, 'f', -1, 64)))
}
func (w *sheetWriter) blank(ref string, style int) {
	w.raw(fmt.Sprintf(`<c r="%s" s="%d"/>`, ref, style))
}
func (w *sheetWriter) rowStart(row int, height int) {
	w.raw(fmt.Sprintf(`<row r="%d" ht="%d" customHeight="1">`, row, height))
}

// SpreadsheetML numeric dates describe wall-clock values. Convert to the chosen
// reporting zone before creating a serial, rather than shifting an Excel value.
func excelDate(t time.Time, loc *time.Location) float64 {
	v := t.In(loc)
	wall := time.Date(v.Year(), v.Month(), v.Day(), v.Hour(), v.Minute(), v.Second(), v.Nanosecond(), time.UTC)
	// Using Unix days also avoids time.Duration overflow for distant dates.
	return float64(wall.Unix())/86400 + float64(wall.Nanosecond())/86400e9 + 25569
}
func filterContext(f Filter, labels exportLabels, names ExportFilterLabels) string {
	parts := []string{}
	category := names.CategoryKK
	if labels.sheet == "Заявки" {
		category = names.CategoryRU
	}
	// Empty results still describe selected historical filters. When an unknown
	// identifier has no name, show that identifier rather than omit the filter.
	display := func(name, id string) string {
		if name != "" {
			return name
		}
		return id
	}
	for _, item := range []struct{ label, value string }{
		{labels.search, f.Search}, {labels.status, localized(labels.statuses, f.Status)}, {labels.priority, localized(labels.priorities, f.Priority)},
		{labels.department, display(names.DepartmentName, f.DepartmentID)}, {labels.category, display(category, f.CategoryID)}, {labels.assignee, display(names.AssigneeName, f.AssigneeID)},
	} {
		if item.value != "" {
			parts = append(parts, item.label+": "+item.value)
		}
	}
	if f.Mine {
		parts = append(parts, labels.mine)
	}
	if len(parts) == 0 {
		return labels.noFilters
	}
	return strings.Join(parts, "; ")
}

// Excel does not reliably auto-fit wrapped cells when a file is generated
// without an application. Estimate line counts against the actual column
// widths, leaving room for padding and wider glyphs, then bound the row height
// to Excel's supported range.
func exportRowHeight(texts []string) int {
	widths := [...]int{48, 28, 28, 28, 18, 28, 28}
	lines := 1
	for column, text := range texts {
		if column >= len(widths) {
			break
		}
		if count := exportWrappedLines(text, widths[column]-3); count > lines {
			lines = count
		}
	}
	return min(409, max(42, lines*15+6))
}

func exportWrappedLines(text string, capacity int) int {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	total := 0
	for _, paragraph := range strings.Split(text, "\n") {
		lines, used := 1, 0
		for _, word := range strings.Fields(paragraph) {
			width := 0
			for _, r := range word {
				switch {
				case unicode.Is(unicode.Mn, r), r == '\u200d':
					// Combining marks and emoji joiners add no separate width.
				case r >= 0x2e80, strings.ContainsRune("MWЖШЩЫЮФ", r):
					width += 2
				default:
					width++
				}
			}
			if used > 0 {
				if used+1+width <= capacity {
					used += 1 + width
					continue
				}
				lines++
				used = 0
			}
			if width > capacity {
				lines += (width - 1) / capacity
				used = (width-1)%capacity + 1
			} else {
				used = width
			}
		}
		total += lines
	}
	return total
}

func exportXLSX(snapshot ExportSnapshot, f Filter, labels exportLabels, loc *time.Location, generated time.Time) ([]byte, error) {
	var sheet exportBuffer
	w := sheetWriter{out: &sheet}
	last := 9 + len(snapshot.Rows)
	w.raw(xmlDeclaration + `<worksheet xmlns="` + spreadsheetNS + `"><sheetPr><pageSetUpPr fitToPage="1"/></sheetPr><dimension ref="A1:L` + strconv.Itoa(last) + `"/><sheetViews><sheetView workbookViewId="0"><pane ySplit="9" topLeftCell="A10" activePane="bottomLeft" state="frozen"/><selection pane="bottomLeft" activeCell="A10" sqref="A10"/></sheetView></sheetViews><sheetFormatPr defaultRowHeight="20"/><cols>`)
	for i, width := range []int{14, 48, 28, 28, 28, 18, 28, 28, 22, 22, 22, 22} {
		w.raw(fmt.Sprintf(`<col min="%d" max="%d" width="%d" customWidth="1"/>`, i+1, i+1, width))
	}
	w.raw(`</cols><sheetData>`)
	w.rowStart(1, 30)
	w.text("A1", 1, snapshot.ClinicName)
	w.raw(`</row>`)
	w.rowStart(2, 28)
	w.text("A2", 1, labels.title)
	w.raw(`</row>`)
	w.rowStart(3, 36)
	w.text("A3", 4, labels.generated)
	w.number("B3", 5, excelDate(generated, loc))
	w.text("D3", 4, labels.timezone)
	w.text("E3", 0, snapshot.Timezone)
	w.raw(`</row>`)
	period := labels.allTime
	if snapshot.Period != nil {
		period = snapshot.Period.From + " — " + snapshot.Period.To
	}
	w.rowStart(4, 36)
	w.text("A4", 4, labels.period)
	w.text("B4", 0, period)
	w.raw(`</row>`)
	w.rowStart(5, 36)
	w.text("A5", 4, labels.filters)
	w.text("B5", 0, filterContext(f, labels, snapshot.FilterLabels))
	w.raw(`</row>`)
	w.rowStart(6, 38)
	for i, value := range []string{labels.total, labels.open, labels.resolved, labels.closed, labels.mean} {
		w.text(fmt.Sprintf("%c6", 'A'+2*i), 4, value)
	}
	w.raw(`</row>`)
	open, resolved, closed, durations := 0, 0, 0, 0
	var durationTotal float64
	for _, row := range snapshot.Rows {
		switch row.Status {
		case New, InProgress, Waiting:
			open++
		case Resolved:
			resolved++
		case Closed:
			resolved++
			closed++
		}
		if hours := resolutionHours(row); hours != nil {
			durationTotal += *hours
			durations++
		}
	}
	w.rowStart(7, 28)
	for i, value := range []int{len(snapshot.Rows), open, resolved, closed} {
		w.number(fmt.Sprintf("%c7", 'A'+2*i), 6, float64(value))
	}
	if durations > 0 {
		w.number("I7", 7, durationTotal/float64(durations))
	} else {
		w.blank("I7", 7)
	}
	w.raw(`</row>`)
	w.rowStart(9, 40)
	for i, value := range labels.columns {
		w.text(fmt.Sprintf("%c9", 'A'+i), 2, value)
	}
	w.raw(`</row>`)
	for i, row := range snapshot.Rows {
		n := i + 10
		texts := rowTexts(row, labels)
		w.rowStart(n, exportRowHeight(texts))
		w.number(fmt.Sprintf("A%d", n), 3, float64(row.Number))
		for col, value := range texts {
			w.text(fmt.Sprintf("%c%d", 'B'+col, n), 3, value)
		}
		w.number(fmt.Sprintf("I%d", n), 8, excelDate(row.CreatedAt, loc))
		w.number(fmt.Sprintf("J%d", n), 8, excelDate(row.UpdatedAt, loc))
		if row.ResolvedAt != nil {
			w.number(fmt.Sprintf("K%d", n), 8, excelDate(*row.ResolvedAt, loc))
		} else {
			w.blank(fmt.Sprintf("K%d", n), 8)
		}
		if hours := resolutionHours(row); hours != nil {
			w.number(fmt.Sprintf("L%d", n), 9, *hours)
		} else {
			w.blank(fmt.Sprintf("L%d", n), 9)
		}
		w.raw(`</row>`)
	}
	w.raw(`</sheetData><autoFilter ref="A9:L` + strconv.Itoa(last) + `"/><mergeCells count="6"><mergeCell ref="A1:L1"/><mergeCell ref="A2:L2"/><mergeCell ref="B4:L4"/><mergeCell ref="B5:L5"/><mergeCell ref="I6:L6"/><mergeCell ref="I7:L7"/></mergeCells><pageMargins left="0.3" right="0.3" top="0.5" bottom="0.5" header="0.2" footer="0.2"/><pageSetup orientation="landscape" paperSize="9" fitToWidth="1" fitToHeight="0"/></worksheet>`)
	if w.err != nil {
		return nil, w.err
	}
	var out exportBuffer
	z := zip.NewWriter(&out)
	parts := []struct {
		name string
		data []byte
	}{
		{"[Content_Types].xml", []byte(xmlDeclaration + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/></Types>`)},
		{"_rels/.rels", []byte(xmlDeclaration + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`)},
		{"xl/workbook.xml", []byte(xmlDeclaration + `<workbook xmlns="` + spreadsheetNS + `" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><bookViews><workbookView/></bookViews><sheets><sheet name="` + labels.sheet + `" sheetId="1" r:id="rId1"/></sheets></workbook>`)},
		{"xl/_rels/workbook.xml.rels", []byte(xmlDeclaration + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`)},
		{"xl/styles.xml", []byte(xlsxStyles)},
		{"xl/worksheets/sheet1.xml", sheet.Bytes()},
	}
	for _, part := range parts {
		entry, err := z.Create(part.name)
		if err != nil {
			return nil, err
		}
		if _, err = entry.Write(part.data); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Style indexes: body, title, header, bordered body, metadata label, metadata
// date, summary integer, summary decimal, bordered date, bordered decimal.
const xlsxStyles = xmlDeclaration + `<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><numFmts count="2"><numFmt numFmtId="164" formatCode="dd.mm.yyyy hh:mm"/><numFmt numFmtId="165" formatCode="0.00"/></numFmts><fonts count="3"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="18"/><color rgb="FF176B56"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts><fills count="3"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill><fill><patternFill patternType="solid"><fgColor rgb="FFDFF3EC"/><bgColor indexed="64"/></patternFill></fill></fills><borders count="2"><border><left/><right/><top/><bottom/><diagonal/></border><border><left style="thin"><color rgb="FFB9D8CC"/></left><right style="thin"><color rgb="FFB9D8CC"/></right><top style="thin"><color rgb="FFB9D8CC"/></top><bottom style="thin"><color rgb="FFB9D8CC"/></bottom><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="10"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0" applyAlignment="1"><alignment vertical="center" wrapText="1"/></xf><xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/><xf numFmtId="0" fontId="2" fillId="2" borderId="1" xfId="0" applyFont="1" applyFill="1" applyBorder="1" applyAlignment="1"><alignment vertical="center" wrapText="1"/></xf><xf numFmtId="0" fontId="0" fillId="0" borderId="1" xfId="0" applyBorder="1" applyAlignment="1"><alignment vertical="top" wrapText="1"/></xf><xf numFmtId="0" fontId="2" fillId="0" borderId="0" xfId="0" applyFont="1" applyAlignment="1"><alignment vertical="center" wrapText="1"/></xf><xf numFmtId="164" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/><xf numFmtId="0" fontId="2" fillId="2" borderId="1" xfId="0" applyFont="1" applyFill="1" applyBorder="1"/><xf numFmtId="165" fontId="2" fillId="2" borderId="1" xfId="0" applyNumberFormat="1" applyFont="1" applyFill="1" applyBorder="1"/><xf numFmtId="164" fontId="0" fillId="0" borderId="1" xfId="0" applyNumberFormat="1" applyBorder="1"/><xf numFmtId="165" fontId="0" fillId="0" borderId="1" xfId="0" applyNumberFormat="1" applyBorder="1"/></cellXfs><cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles></styleSheet>`
