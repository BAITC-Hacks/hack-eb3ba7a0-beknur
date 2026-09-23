package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"math"
	"reflect"
	"testing"
	"time"
)

func example() []Pick {
	return []Pick{{"M7", "Нура"}, {"M8", "Нура"}, {"M10", "Нура"}, {"M12", ""}, {"M5", "Сарыарка"}}
}
func near(t *testing.T, a, b float64) {
	t.Helper()
	if math.Abs(a-b) > 1e-4 {
		t.Fatalf("got %.8f want %.8f", a, b)
	}
}
func TestScoring(t *testing.T) {
	s := seedScenario()
	if e := s.validate(); e != nil {
		t.Fatal(e)
	}
	near(t, s.compute(nil).Score, 52.5577)
	p := example()
	if e := s.validatePicks(p, true); e != nil {
		t.Fatal(e)
	}
	f := s.facts(p)
	near(t, f.Final.Score, 56.5431)
	if f.Budget.Spent != 95 || f.Budget.Remaining != 5 {
		t.Fatal(f.Budget)
	}
	for i, j := 0, len(p)-1; i < j; i, j = i+1, j-1 {
		p[i], p[j] = p[j], p[i]
	}
	if !reflect.DeepEqual(s.compute(p), f.Final) {
		t.Fatal("order changes result")
	}
	for _, a := range f.Alternatives {
		if e := s.validatePicks(a.Picks, true); e != nil {
			t.Fatal(e)
		}
		near(t, s.compute(a.Picks).Score, a.Score)
	}
}
func TestValidator(t *testing.T) {
	s := seedScenario()
	tests := map[string][]Pick{"four": example()[:4], "six": append(example(), Pick{"M2", ""}), "duplicate": {{"M7", "Нура"}, {"M7", "Алматы"}, {"M10", "Нура"}, {"M12", ""}, {"M5", "Сарыарка"}}, "three_category": {{"M7", "Нура"}, {"M8", "Нура"}, {"M9", "Есиль"}, {"M10", "Нура"}, {"M12", ""}}, "city_target": {{"M7", "Нура"}, {"M8", "Нура"}, {"M10", "Нура"}, {"M12", "Нура"}, {"M5", "Сарыарка"}}, "missing_district": {{"M7", ""}}, "unknown": {{"X", "Нура"}}, "transport_conflict": {{"M1", "Нура"}, {"M3", "Есиль"}}, "park_school": {{"M4", "Нура"}, {"M7", "Нура"}}, "fuel_network": {{"M5", "Нура"}, {"M13", "Нура"}}, "budget": {{"M3", "Нура"}, {"M5", "Нура"}, {"M7", "Нура"}, {"M10", "Нура"}, {"M12", ""}}}
	for name, p := range tests {
		t.Run(name, func(t *testing.T) {
			if s.validatePicks(p, len(p) >= 4) == nil {
				t.Fatal("accepted invalid picks")
			}
		})
	}
}
func TestEffects(t *testing.T) {
	s := seedScenario()
	r := s.compute([]Pick{{"M7", "Нура"}})
	near(t, r.Districts[4].After[4], 48)
	r = s.compute([]Pick{{"M10", "Нура"}, {"M12", ""}})
	near(t, r.Districts[4].After[6], 67.5)
	if len(r.Synergies) != 1 {
		t.Fatal("missing synergy")
	}
	for _, pair := range [][]Pick{{{"M1", "Есиль"}, {"M2", ""}}, {{"M5", "Есиль"}, {"M6", ""}}} {
		if len(s.compute(pair).Synergies) != 1 {
			t.Fatal("missing synergy")
		}
	}
	s.Districts[4].V[4] = 99
	r = s.compute([]Pick{{"M7", "Нура"}})
	near(t, r.Districts[4].After[4], 100)
	s.Districts[4].V[0] = 0
	r = s.compute([]Pick{{"M11", "Нура"}})
	near(t, r.Districts[4].After[0], 0)
	r = s.compute(nil)
	near(t, r.Score, round(.7*r.Average+.3*r.Minimum-float64(len(r.Critical))))
}
func fixtureResult() Result {
	s := seedScenario()
	p := example()
	x := Result{ID: token(), Player: "tester", Owner: "tester", City: s.City, Mode: "solo", Created: time.Now(), PlayersCount: 1, Rank: 1, Status: "complete", Scenario: s, Picks: p, Facts: s.facts(p)}
	a := Analysis{Summary: "Тестовый анализ", Right: []string{"Сильная сторона"}, Improvements: []string{"Улучшения"}, Tradeoffs: []string{"Компромиссы"}, Risks: []string{"Риски"}, Recommendations: []string{"Рекомендации"}, Districts: map[string]string{}, Decisions: map[string]string{}}
	for _, d := range s.Districts {
		a.Districts[d.Name] = "Район"
	}
	for _, p := range p {
		a.Decisions[p.ID] = "Решение"
	}
	x.AI = &a
	return x
}
func TestReports(t *testing.T) {
	x := fixtureResult()
	if e := validateAnalysis(*x.AI, x); e != nil {
		t.Fatal(e)
	}
	b, e := renderPDF(x)
	if e != nil || !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Fatalf("PDF %v", e)
	}
	b, e = renderDOCX(x)
	if e != nil {
		t.Fatal(e)
	}
	z, e := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if e != nil {
		t.Fatal(e)
	}
	if len(z.File) != 5 {
		t.Fatal("DOCX parts")
	}
	for _, f := range z.File {
		r, _ := f.Open()
		d := xml.NewDecoder(r)
		for {
			_, e = d.Token()
			if e == io.EOF {
				break
			}
			if e != nil {
				t.Fatal(e)
			}
		}
		r.Close()
	}
	delete(x.AI.Districts, "Нура")
	if validateAnalysis(*x.AI, x) == nil {
		t.Fatal("missing district accepted")
	}
}
func TestBot(t *testing.T) {
	s := seedScenario()
	p, e := (RuleBot{}).Choose(s)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.validatePicks(p, true); e != nil {
		t.Fatal(e)
	}
	q, e := (RuleBot{}).Choose(s)
	if e != nil || !reflect.DeepEqual(p, q) {
		t.Fatal("bot not deterministic")
	}
}
