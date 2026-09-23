package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"github.com/phpdave11/gofpdf"
	"strings"
	"time"
)

func tenge(n int64) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + " " + s[i:]
	}
	return s + " усл. ед."
}

type section struct {
	Title string
	Lines []string
}

func reportSections(x Result) []section {
	f := x.Facts
	when := x.Created.In(time.FixedZone("UTC+5", 5*3600)).Format("02.01.2006 15:04 UTC+5")
	mode := "Одиночная игра"
	if x.Mode == "multiplayer" {
		mode = "Комната"
	}
	out := []section{{"Информация об игре", []string{"Город: " + x.City, "Игрок: " + x.Player, "Режим: " + mode, "Дата: " + when, "Комната: " + x.RoomID, fmt.Sprintf("Место: %d / %d", x.Rank, x.PlayersCount)}}, {"Astana Quality of Life Score", []string{fmt.Sprintf("Итог: %.2f • Baseline: %.2f • Изменение: %+.2f", f.Final.Score, f.Baseline.Score, f.Delta)}}, {"Бюджет", []string{"Общий: " + tenge(f.Budget.Initial), "Использовано: " + tenge(f.Budget.Spent), "Осталось: " + tenge(f.Budget.Remaining)}}}
	lines := []string{}
	for _, d := range f.Decisions {
		target := d.District
		if target == "" {
			target = "Весь город"
		}
		lines = append(lines, fmt.Sprintf("%s — %s | %s | %s | %s", d.Measure.ID, d.Measure.Name, d.Measure.Category, target, tenge(d.Measure.Cost)), fmt.Sprintf("Предельный вклад в Score: %+.4f (сравнение с исключением этой меры).", d.MarginalScore))
		for _, district := range d.Changes {
			changes := []string{}
			for i, v := range district.Delta {
				if v != 0 {
					changes = append(changes, fmt.Sprintf("%s %+.2f", x.Scenario.Keys[i], v))
				}
			}
			if len(changes) > 0 {
				lines = append(lines, district.Name+": "+strings.Join(changes, ", "))
			}
		}
		lines = append(lines, x.AI.Decisions[d.Measure.ID])
		if d.Measure.Source != "" {
			lines = append(lines, "Источник: "+d.Measure.Source)
		}
	}
	out = append(out, section{"Пять решений и их вклад", lines})
	lines = nil
	for _, d := range f.Final.Districts {
		lines = append(lines, fmt.Sprintf("%s — %.2f → %.2f (%+.2f)", d.Name, d.BeforeScore, d.Score, d.ScoreDelta))
		for i, b := range d.Before {
			lines = append(lines, fmt.Sprintf("%s: %.2f → %.2f (%+.2f)", x.Scenario.Keys[i], b, d.After[i], d.Delta[i]))
		}
		lines = append(lines, x.AI.Districts[d.Name])
	}
	out = append(out, section{"Районы: до / после / изменение", lines}, section{"Что вы сделали правильно", x.AI.Right}, section{"Что можно было сделать лучше", x.AI.Improvements}, section{"Компромиссы", x.AI.Tradeoffs}, section{"Основные риски", x.AI.Risks}, section{"AI-рекомендации", x.AI.Recommendations})
	lines = nil
	for _, alt := range f.Alternatives {
		p := []string{}
		for _, pick := range alt.Picks {
			p = append(p, pick.ID+" / "+pick.District)
		}
		lines = append(lines, fmt.Sprintf("%s. Стоимость %s. Score %.4f, улучшение %+.4f", strings.Join(p, "; "), tenge(alt.Cost), alt.Score, alt.Improvement))
	}
	if len(lines) == 0 {
		lines = []string{"Улучшений при допустимой замене одного решения не найдено. Это не доказательство глобального оптимума."}
	}
	out = append(out, section{"Проверенные альтернативы", lines}, section{"Final AI Summary", []string{x.AI.Summary}}, section{"Технический разбор Score", []string{"Score = 0.7 × среднее по населению + 0.3 × слабейший район − число индикаторов ниже 40.", fmt.Sprintf("Среднее: %.4f; минимум: %.4f; критических: %d.", f.Final.Average, f.Final.Minimum, len(f.Final.Critical)), "Горизонт: 8 кварталов. Эффект × (8 − lag) / 8, затем синергии, затем ограничение индикаторов 0–100. Вклады исключения мер не суммируются из-за синергий и нелинейной формулы.", "Стоимость хранится в условных единицах. Остаток бюджета не даёт бонуса. Модель: " + x.Model, x.Scenario.Methodology}})
	return out
}
func renderPDF(x Result) ([]byte, error) {
	p := gofpdf.New("P", "mm", "A4", "")
	font, e := files.ReadFile("assets/PT_Sans-Web-Regular.ttf")
	if e != nil {
		return nil, e
	}
	p.AddUTF8FontFromBytes("PT", "", font)
	p.SetMargins(18, 18, 18)
	p.SetAutoPageBreak(true, 20)
	p.SetTitle("Аким на 5 часов — AI Post-Game Report", true)
	p.SetAuthor("Аким на 5 часов · учебная модель", true)
	p.SetFooterFunc(func() {
		p.SetY(-14)
		p.SetFont("PT", "", 9)
		p.SetTextColor(90, 100, 110)
		p.CellFormat(0, 6, fmt.Sprintf("Аким на 5 часов • %s • %d", x.ID[:12], p.PageNo()), "", 0, "C", false, 0, "")
	})
	p.AddPage()
	p.SetFillColor(14, 45, 56)
	p.Rect(0, 0, 210, 105, "F")
	p.SetY(27)
	p.SetTextColor(255, 255, 255)
	p.SetFont("PT", "", 30)
	p.MultiCell(174, 14, "Аким на 5 часов", "", "L", false)
	p.SetFont("PT", "", 19)
	p.MultiCell(174, 11, "AI Post-Game Report", "", "L", false)
	p.SetY(118)
	p.SetTextColor(20, 45, 55)
	p.SetFont("PT", "", 15)
	p.MultiCell(174, 9, x.City+" • "+x.Player, "", "L", false)
	p.SetFont("PT", "", 44)
	p.MultiCell(174, 21, fmt.Sprintf("%.2f", x.Facts.Final.Score), "", "L", false)
	p.SetFont("PT", "", 12)
	p.MultiCell(174, 8, fmt.Sprintf("Baseline %.2f  /  Изменение %+.2f  /  Место %d из %d", x.Facts.Baseline.Score, x.Facts.Delta, x.Rank, x.PlayersCount), "", "L", false)
	p.MultiCell(174, 8, "Учебный аналитический отчёт. Не является официальным прогнозом развития города.", "", "L", false)
	p.AddPage()
	p.SetFont("PT", "", 19)
	p.CellFormat(174, 12, "Сравнение районов", "", 1, "L", false, 0, "")
	for _, d := range x.Facts.Final.Districts {
		p.SetFont("PT", "", 11)
		p.CellFormat(174, 8, fmt.Sprintf("%s: %.2f → %.2f (%+.2f)", d.Name, d.BeforeScore, d.Score, d.ScoreDelta), "", 1, "L", false, 0, "")
		y := p.GetY()
		p.SetFillColor(199, 208, 211)
		p.Rect(18, y, 140*clamp(d.BeforeScore)/100, 3, "F")
		p.SetFillColor(24, 125, 107)
		p.Rect(18, y+4, 140*clamp(d.Score)/100, 3, "F")
		p.SetY(y + 12)
	}
	p.MultiCell(174, 7, "Серый — до; зелёный — после. Шкала графика 0–100.", "", "L", false)
	for _, s := range reportSections(x) {
		if p.GetY() > 235 {
			p.AddPage()
		}
		p.Ln(5)
		p.SetFont("PT", "", 16)
		p.SetTextColor(20, 80, 76)
		p.MultiCell(174, 9, s.Title, "", "L", false)
		p.SetFont("PT", "", 11)
		p.SetTextColor(25, 38, 48)
		for _, line := range s.Lines {
			p.MultiCell(174, 6, line, "", "L", false)
			p.Ln(2)
		}
	}
	p.AddPage()
	p.SetFont("PT", "", 16)
	p.CellFormat(174, 12, "Таблица показателей районов", "", 1, "L", false, 0, "")
	p.SetFont("PT", "", 10)
	for _, d := range x.Facts.Final.Districts {
		if p.GetY() > 200 {
			p.AddPage()
		}
		p.SetFillColor(226, 238, 235)
		p.CellFormat(174, 8, d.Name, "1", 1, "L", true, 0, "")
		for i, b := range d.Before {
			for j, value := range []string{x.Scenario.Keys[i], fmt.Sprintf("До: %.2f", b), fmt.Sprintf("После: %.2f", d.After[i]), fmt.Sprintf("Δ %+.2f", d.Delta[i])} {
				ln := 0
				if j == 3 {
					ln = 1
				}
				p.CellFormat(43.5, 7, value, "1", ln, "L", false, 0, "")
			}
		}
		p.Ln(5)
	}
	var buf bytes.Buffer
	e = p.Output(&buf)
	return buf.Bytes(), e
}
func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
func escaped(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
func renderDOCX(x Result) ([]byte, error) {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	write := func(name, body string) error {
		w, e := z.Create(name)
		if e != nil {
			return e
		}
		_, e = w.Write([]byte(body))
		return e
	}
	paragraph := func(text, style string) string {
		return `<w:p><w:pPr><w:pStyle w:val="` + style + `"/></w:pPr><w:r><w:t xml:space="preserve">` + escaped(text) + `</w:t></w:r></w:p>`
	}
	body := paragraph("Аким на 5 часов — AI Post-Game Report", "Title")
	for _, s := range reportSections(x) {
		body += paragraph(s.Title, "Heading1")
		for _, line := range s.Lines {
			body += paragraph(line, "Normal")
		}
	}
	parts := map[string]string{
		"[Content_Types].xml":          `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/></Types>`,
		"_rels/.rels":                  `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/_rels/document.xml.rels": `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`,
		"word/styles.xml":              `<?xml version="1.0" encoding="UTF-8"?><w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri"/><w:sz w:val="22"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:basedOn w:val="Normal"/><w:rPr><w:b/><w:sz w:val="40"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="240" w:after="120"/></w:pPr><w:rPr><w:b/><w:color w:val="14665A"/><w:sz w:val="30"/></w:rPr></w:style></w:styles>`,
		"word/document.xml":            `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body + `<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1134" w:right="1134" w:bottom="1134" w:left="1134"/></w:sectPr></w:body></w:document>`,
	}
	for name, body := range parts {
		if e := write(name, body); e != nil {
			return nil, e
		}
	}
	if e := z.Close(); e != nil {
		return nil, e
	}
	return b.Bytes(), nil
}
