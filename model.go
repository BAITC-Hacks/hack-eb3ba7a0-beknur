package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
)

var categories = []string{"Транспорт", "Экология", "Соцсфера", "Безопасность", "Сервисы"}

type District struct {
	Name       string    `json:"name"`
	Pop        float64   `json:"pop"`
	V          []float64 `json:"v"`
	Facilities []string  `json:"facilities"`
}
type Measure struct {
	ID       string             `json:"id"`
	Category string             `json:"category"`
	Name     string             `json:"name"`
	Type     string             `json:"type"`
	Cost     int64              `json:"cost"`
	Lag      int                `json:"lag"`
	Effects  map[string]float64 `json:"effects"`
	Source   string             `json:"source,omitempty"`
}
type Scenario struct {
	City        string     `json:"city"`
	Version     int        `json:"version"`
	Budget      int64      `json:"budget"`
	Methodology string     `json:"methodology"`
	Keys        []string   `json:"keys"`
	Weights     []float64  `json:"weights"`
	Districts   []District `json:"districts"`
	Measures    []Measure  `json:"measures"`
}
type Pick struct {
	ID       string `json:"id"`
	District string `json:"district"`
}
type DistrictResult struct {
	Name        string    `json:"name"`
	Before      []float64 `json:"before"`
	After       []float64 `json:"after"`
	Delta       []float64 `json:"delta"`
	BeforeScore float64   `json:"before_score"`
	Score       float64   `json:"score"`
	ScoreDelta  float64   `json:"score_delta"`
}
type Critical struct {
	District  string  `json:"district"`
	Indicator string  `json:"indicator"`
	Value     float64 `json:"value"`
}
type Synergy struct {
	Measures  []string `json:"measures"`
	District  string   `json:"district"`
	Indicator string   `json:"indicator"`
	Effect    float64  `json:"effect"`
}
type Calculation struct {
	Score     float64          `json:"score"`
	Average   float64          `json:"weighted_average"`
	Minimum   float64          `json:"minimum_district"`
	Districts []DistrictResult `json:"districts"`
	Critical  []Critical       `json:"critical_indicators"`
	Synergies []Synergy        `json:"synergies"`
}
type Decision struct {
	Measure       Measure          `json:"measure"`
	District      string           `json:"district"`
	MarginalScore float64          `json:"marginal_score"`
	Changes       []DistrictResult `json:"changes_without_vs_with"`
}
type Alternative struct {
	Picks       []Pick      `json:"picks"`
	Cost        int64       `json:"cost"`
	Score       float64     `json:"score"`
	Improvement float64     `json:"improvement"`
	Result      Calculation `json:"result"`
}
type Budget struct {
	Initial   int64 `json:"initial"`
	Spent     int64 `json:"spent"`
	Remaining int64 `json:"remaining"`
}
type Facts struct {
	Baseline     Calculation    `json:"baseline"`
	Final        Calculation    `json:"final"`
	Delta        float64        `json:"score_delta"`
	Budget       Budget         `json:"budget"`
	Decisions    []Decision     `json:"decisions"`
	Alternatives []Alternative  `json:"verified_alternatives"`
	Validation   map[string]any `json:"validation"`
}

func clone[T any](v T) T      { b, _ := json.Marshal(v); var out T; _ = json.Unmarshal(b, &out); return out }
func round(v float64) float64 { return math.Round(v*10000) / 10000 }
func index(ss []string, s string) int {
	for i, v := range ss {
		if v == s {
			return i
		}
	}
	return -1
}
func (s Scenario) measure(id string) (Measure, bool) {
	for _, m := range s.Measures {
		if m.ID == id {
			return m, true
		}
	}
	return Measure{}, false
}
func (s Scenario) validate() error {
	if s.City == "" || len(s.City) > 100 || s.Methodology == "" || s.Budget != 100 || len(s.Districts) < 1 || len(s.Districts) > 20 || len(s.Measures) < 5 || len(s.Measures) > 100 || len(s.Keys) != 10 || len(s.Weights) != 10 {
		return errors.New("Проверьте город, бюджет, 10 показателей, районы и мероприятия")
	}
	sum := 0.
	canonicalKeys := []string{"T1", "T2", "E1", "E2", "S1", "S2", "B1", "B2", "C1", "C2"}
	canonicalWeights := []float64{.10, .10, .09, .11, .11, .11, .09, .09, .10, .10}
	for i, k := range s.Keys {
		if k != canonicalKeys[i] || s.Weights[i] != canonicalWeights[i] {
			return errors.New("Ключи и веса модели фиксированы")
		}
	}
	seen := map[string]bool{}
	for i, k := range s.Keys {
		if k == "" || seen[k] || s.Weights[i] < 0 {
			return errors.New("Некорректные ключи или веса")
		}
		seen[k] = true
		sum += s.Weights[i]
	}
	if math.Abs(sum-1) > 1e-8 {
		return errors.New("Сумма весов должна быть 1")
	}
	sum = 0
	seen = map[string]bool{}
	for _, d := range s.Districts {
		if d.Name == "" || seen[d.Name] || d.Pop <= 0 || len(d.V) != 10 || len(d.Facilities) > 200 {
			return errors.New("Некорректный район")
		}
		seen[d.Name] = true
		sum += d.Pop
		for _, v := range d.V {
			if v < 0 || v > 100 {
				return errors.New("Показатели должны быть от 0 до 100")
			}
		}
	}
	if math.Abs(sum-1) > 1e-8 {
		return errors.New("Сумма долей населения должна быть 1")
	}
	seen = map[string]bool{}
	available := map[string]bool{}
	for _, m := range s.Measures {
		if m.ID == "" || seen[m.ID] || m.Name == "" || index(categories, m.Category) < 0 || m.Cost < 1 || m.Cost > s.Budget || m.Lag < 0 || m.Lag > 8 || (m.Type != "Город" && m.Type != "Район") {
			return errors.New("Некорректное мероприятие: " + m.ID)
		}
		seen[m.ID] = true
		available[m.Category] = true
		for k, v := range m.Effects {
			if index(s.Keys, k) < 0 || v < -100 || v > 100 {
				return errors.New("Некорректный эффект")
			}
		}
	}
	if len(available) != 5 {
		return errors.New("Нужны мероприятия во всех пяти направлениях")
	}
	// Find one feasible five-direction scenario before publishing (at most 100 measures).
	var find func(int, []Pick, int64) bool
	find = func(c int, p []Pick, cost int64) bool {
		if c == 5 {
			return s.validatePicks(p, true) == nil
		}
		for _, m := range s.Measures {
			if m.Category != categories[c] || cost+m.Cost > s.Budget {
				continue
			}
			targets := []string{""}
			if m.Type == "Район" {
				targets = nil
				for _, d := range s.Districts {
					targets = append(targets, d.Name)
				}
			}
			for _, d := range targets {
				next := append(append([]Pick{}, p...), Pick{m.ID, d})
				if s.validatePicks(next, false) == nil && find(c+1, next, cost+m.Cost) {
					return true
				}
			}
		}
		return false
	}
	if !find(0, nil, 0) {
		return errors.New("Нет допустимого набора пяти решений в бюджете")
	}
	return nil
}
func (s Scenario) validatePicks(p []Pick, complete bool) error {
	if len(p) > 5 || complete && len(p) != 5 {
		return errors.New("Нужно ровно пять решений")
	}
	ids := map[string]Pick{}
	cats := map[string]int{}
	var cost int64
	for _, x := range p {
		m, ok := s.measure(x.ID)
		if !ok {
			return errors.New("Неизвестное мероприятие")
		}
		if _, ok := ids[x.ID]; ok || cats[m.Category] >= 2 {
			return errors.New("Нельзя повторять меры или выбирать более двух мер одного направления")
		}
		ids[x.ID] = x
		cats[m.Category]++
		cost += m.Cost
		if m.Type == "Город" && x.District != "" {
			return errors.New("Общегородская мера не имеет отдельного района")
		}
		if m.Type == "Район" {
			found := false
			for _, d := range s.Districts {
				found = found || d.Name == x.District
			}
			if !found {
				return errors.New("Укажите существующий район")
			}
		}
	}
	if complete && len(cats) < 3 {
		return errors.New("Нужны минимум три направления")
	}
	if cost > s.Budget {
		return errors.New("Превышен общий бюджет")
	}
	if _, ok := ids["M1"]; ok {
		if _, ok = ids["M3"]; ok {
			return errors.New("M1 и M3 несовместимы")
		}
	}
	for _, pair := range [][2]string{{"M4", "M7"}, {"M5", "M13"}} {
		a, ok := ids[pair[0]]
		b, ok2 := ids[pair[1]]
		if ok && ok2 && a.District == b.District {
			return fmt.Errorf("%s и %s несовместимы в одном районе", pair[0], pair[1])
		}
	}
	return nil
}
func (s Scenario) compute(p []Pick) Calculation {
	p = append([]Pick{}, p...)
	sort.Slice(p, func(i, j int) bool { return p[i].ID < p[j].ID })
	r := Calculation{Districts: []DistrictResult{}, Critical: []Critical{}, Synergies: []Synergy{}, Minimum: math.Inf(1)}
	for _, d := range s.Districts {
		r.Districts = append(r.Districts, DistrictResult{Name: d.Name, Before: append([]float64{}, d.V...), After: append([]float64{}, d.V...)})
	}
	for _, x := range p {
		m, _ := s.measure(x.ID)
		for i := range r.Districts {
			d := &r.Districts[i]
			if m.Type == "Город" || d.Name == x.District {
				for k, e := range m.Effects {
					d.After[index(s.Keys, k)] += e * float64(8-m.Lag) / 8
				}
			}
		}
	}
	for _, pair := range [][3]string{{"M1", "M2", "T1"}, {"M10", "M12", "B1"}, {"M5", "M6", "E2"}} {
		a, b := -1, -1
		for i, x := range p {
			if x.ID == pair[0] {
				a = i
			}
			if x.ID == pair[1] {
				b = i
			}
		}
		k := index(s.Keys, pair[2])
		if a >= 0 && b >= 0 && k >= 0 {
			for i := range r.Districts {
				if r.Districts[i].Name == p[a].District {
					r.Districts[i].After[k] += 2
					r.Synergies = append(r.Synergies, Synergy{[]string{pair[0], pair[1]}, p[a].District, pair[2], 2})
				}
			}
		}
	}
	for i := range r.Districts {
		d := &r.Districts[i]
		for k, v := range d.After {
			v = math.Max(0, math.Min(100, v))
			d.After[k] = round(v)
			d.Delta = append(d.Delta, round(v-d.Before[k]))
			d.Score += v * s.Weights[k]
			d.BeforeScore += d.Before[k] * s.Weights[k]
			if v < 40 {
				r.Critical = append(r.Critical, Critical{d.Name, s.Keys[k], round(v)})
			}
		}
		d.ScoreDelta = round(d.Score - d.BeforeScore)
		r.Average += s.Districts[i].Pop * d.Score
		r.Minimum = math.Min(r.Minimum, d.Score)
		d.Score = round(d.Score)
		d.BeforeScore = round(d.BeforeScore)
	}
	r.Score = round(.7*r.Average + .3*r.Minimum - float64(len(r.Critical)))
	r.Average = round(r.Average)
	r.Minimum = round(r.Minimum)
	return r
}
func changes(a, b Calculation) []DistrictResult {
	out := clone(b.Districts)
	for i := range out {
		out[i].Before = a.Districts[i].After
		out[i].BeforeScore = a.Districts[i].Score
		out[i].ScoreDelta = round(out[i].Score - out[i].BeforeScore)
		for k := range out[i].Delta {
			out[i].Delta[k] = round(out[i].After[k] - out[i].Before[k])
		}
	}
	return out
}
func (s Scenario) facts(p []Pick) Facts {
	base, final := s.compute(nil), s.compute(p)
	f := Facts{Baseline: base, Final: final, Delta: round(final.Score - base.Score), Budget: Budget{Initial: s.Budget, Remaining: s.Budget}, Decisions: []Decision{}, Alternatives: []Alternative{}, Validation: map[string]any{"valid": true, "violations": []string{}}}
	for i, x := range p {
		m, _ := s.measure(x.ID)
		f.Budget.Spent += m.Cost
		without := append([]Pick{}, p[:i]...)
		without = append(without, p[i+1:]...)
		a := s.compute(without)
		f.Decisions = append(f.Decisions, Decision{m, x.District, round(final.Score - a.Score), changes(a, final)})
	}
	f.Budget.Remaining -= f.Budget.Spent
	// Evaluate legal one-decision replacements. No global-optimum claim.
	for i, x := range p {
		old, _ := s.measure(x.ID)
		for _, m := range s.Measures {

			targets := []string{""}
			if m.Type == "Район" {
				targets = nil
				for _, d := range s.Districts {
					targets = append(targets, d.Name)
				}
			}
			for _, d := range targets {
				q := append([]Pick{}, p...)
				q[i] = Pick{m.ID, d}
				if s.validatePicks(q, true) != nil {
					continue
				}
				r := s.compute(q)
				if r.Score > final.Score+1e-6 {
					f.Alternatives = append(f.Alternatives, Alternative{q, f.Budget.Spent - old.Cost + m.Cost, r.Score, round(r.Score - final.Score), r})
				}
			}
		}
	}
	sort.SliceStable(f.Alternatives, func(i, j int) bool { return f.Alternatives[i].Score > f.Alternatives[j].Score })
	if len(f.Alternatives) > 3 {
		f.Alternatives = f.Alternatives[:3]
	}
	return f
}
