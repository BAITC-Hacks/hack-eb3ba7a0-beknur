package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
	"sort"
	"strings"
	"time"
)

type Opponent interface {
	Choose(Scenario) ([]Pick, error)
}
type RuleBot struct{}

// Beam search is deterministic and knows only the frozen initial scenario.
func (RuleBot) Choose(s Scenario) ([]Pick, error) {
	type node struct {
		p     []Pick
		score float64
	}
	beam := []node{{nil, 0}}
	for depth := 0; depth < 5; depth++ {
		next := []node{}
		seen := map[string]bool{}
		for _, n := range beam {
			for _, m := range s.Measures {
				targets := []string{""}
				if m.Type == "Район" {
					targets = nil
					for _, d := range s.Districts {
						targets = append(targets, d.Name)
					}
				}
				for _, d := range targets {
					p := append(append([]Pick{}, n.p...), Pick{m.ID, d})
					if s.validatePicks(p, depth == 4) != nil {
						continue
					}
					ordered := append([]Pick{}, p...)
					sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
					b, _ := json.Marshal(ordered)
					if seen[string(b)] {
						continue
					}
					seen[string(b)] = true
					next = append(next, node{p, s.compute(p).Score})
				}
			}
		}
		sort.SliceStable(next, func(i, j int) bool { return next[i].score > next[j].score })
		if len(next) > 60 {
			next = next[:60]
		}
		beam = next
		if len(beam) == 0 {
			return nil, errors.New("Бот не нашёл допустимую стратегию для этого города")
		}
	}
	return beam[0].p, nil
}
func actor(room Room, user string) string {
	if room.Mode == "local" && room.Host == user {
		for _, p := range room.Players {
			if room.Submitted[p] == "" {
				return p
			}
		}
		return ""
	}
	return user
}
func roomAccess(room Room, u User) bool {
	return room.Host == u.Name || index(room.Players, u.Name) >= 0 || u.Role == "admin"
}
func roomView(room Room, u User) map[string]any {
	active := actor(room, u.Name)
	draft := room.Drafts[active]
	room.Drafts = nil
	var spent int64
	for _, p := range draft {
		m, _ := room.Scenario.measure(p.ID)
		spent += m.Cost
	}
	preview := room.Scenario.compute(draft).Districts
	return map[string]any{"room": room, "actor": active, "picks": draft, "budget": Budget{room.Scenario.Budget, spent, room.Scenario.Budget - spent}, "preview": preview}
}
func (a *App) rooms(w http.ResponseWriter, r *http.Request, u User) {
	if r.URL.Path == "/api/rooms" {
		if r.Method == "GET" {
			out := []Room{}
			e := a.store.DB.View(func(tx *Tx) error {
				return tx.Bucket([]byte("rooms")).ForEach(func(_, v []byte) error {
					var room Room
					if e := json.Unmarshal(v, &room); e != nil {
						return e
					}
					if roomAccess(room, u) || room.Mode == "multiplayer" && room.Status == "lobby" {
						room.Drafts = nil
						out = append(out, room)
					}
					return nil
				})
			})
			if e != nil {
				fail(w, 500, e)
				return
			}
			sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
			reply(w, 200, out)
			return
		}
		if r.Method == "POST" {
			a.createRoom(w, r, u)
			return
		}
		w.WriteHeader(405)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/rooms/"), "/")
	if len(parts) > 2 {
		http.NotFound(w, r)
		return
	}
	room, e := read[Room](a.store, "rooms", parts[0])
	if e != nil {
		fail(w, 404, e)
		return
	}
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}
	if r.Method == "GET" && action == "ws" {
		if !roomAccess(room, u) {
			fail(w, 403, errors.New("Нет доступа"))
			return
		}
		a.socket(w, r, u, room.ID)
		return
	}
	if r.Method == "GET" && action == "" {
		if !roomAccess(room, u) {
			fail(w, 403, errors.New("Нет доступа"))
			return
		}
		reply(w, 200, roomView(room, u))
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var in struct {
		Picks []Pick `json:"picks"`
	}
	if action == "decisions" {
		if e = decode(w, r, &in); e != nil {
			fail(w, 400, e)
			return
		}
	}
	var response any
	jobs := []string{}
	e = a.store.DB.Update(func(tx *Tx) error {
		var e error
		room, e = get[Room](tx, "rooms", parts[0])
		if e != nil {
			return e
		}
		if action == "join" {
			if index(room.Players, u.Name) >= 0 {
				response = roomView(room, u)
				return nil
			}
			if room.Mode != "multiplayer" || room.Status != "lobby" || len(room.Players) >= 8 {
				return errors.New("Комната запущена или заполнена")
			}
			room.Players = append(room.Players, u.Name)
		} else {
			if !roomAccess(room, u) {
				return errors.New("Нет доступа к комнате")
			}
			switch action {
			case "leave":
				if room.Status != "lobby" {
					return errors.New("Выйти можно до начала игры")
				}
				if room.Host == u.Name {
					room.Status = "cancelled"
				} else {
					i := index(room.Players, u.Name)
					if i < 0 {
						return errors.New("Вы не участник")
					}
					room.Players = append(room.Players[:i], room.Players[i+1:]...)
					delete(room.Ready, u.Name)
				}
			case "ready":
				if room.Status != "lobby" || index(room.Players, u.Name) < 0 {
					return errors.New("Готовность меняется только в лобби")
				}
				room.Ready[u.Name] = !room.Ready[u.Name]
			case "start":
				if room.Host != u.Name || room.Status != "lobby" {
					return errors.New("Только создатель запускает лобби")
				}
				if room.Mode == "multiplayer" && len(room.Players) < 2 {
					return errors.New("Нужно минимум два игрока")
				}
				for _, p := range room.Players {
					if !room.Ready[p] {
						return errors.New("Все игроки должны подтвердить готовность")
					}
				}
				room.Status = "playing"
			case "decisions", "submit":
				active := actor(room, u.Name)
				if room.Status != "playing" || active == "" || index(room.Players, active) < 0 || room.Submitted[active] != "" {
					return errors.New("Игра не началась или решения уже сданы")
				}
				if action == "decisions" {
					if e = room.Scenario.validatePicks(in.Picks, false); e != nil {
						return e
					}
					room.Drafts[active] = clone(in.Picks)
				} else {
					p := room.Drafts[active]
					if e = room.Scenario.validatePicks(p, true); e != nil {
						return e
					}
					x := makeResult(room, active, u.Name, p)
					if e = put(tx, "results", x.ID, x); e != nil {
						return e
					}
					room.Submitted[active] = x.ID
					delete(room.Drafts, active)
					if len(room.Submitted) == len(room.Players) {
						all := []Result{}
						for _, name := range room.Players {
							v, e := get[Result](tx, "results", room.Submitted[name])
							if e != nil {
								return e
							}
							all = append(all, v)
						}
						sort.SliceStable(all, func(i, j int) bool { return all[i].Facts.Final.Score > all[j].Facts.Final.Score })
						rank := 1
						for i, v := range all {
							if i > 0 && v.Facts.Final.Score != all[i-1].Facts.Final.Score {
								rank = i + 1
							}
							v.Rank = rank
							v.Status = "analyzing"
							if e = put(tx, "results", v.ID, v); e != nil {
								return e
							}
							jobs = append(jobs, v.ID)
						}
						room.Status = "scored"
					}
				}
			default:
				return errors.New("Неизвестное действие")
			}
		}
		if e = put(tx, "rooms", room.ID, room); e != nil {
			return e
		}
		response = roomView(room, u)
		return nil
	})
	if e != nil {
		fail(w, 409, e)
		return
	}
	for _, id := range jobs {
		go a.analyze(id)
	}
	reply(w, 200, response)
}
func makeResult(room Room, player, owner string, p []Pick) Result {
	return Result{ID: token(), RoomID: room.ID, Player: player, Owner: owner, City: room.Scenario.City, Mode: room.Mode, Created: time.Now().UTC(), PlayersCount: len(room.Players), Status: "waiting_players", Scenario: room.Scenario, Picks: clone(p), Facts: room.Scenario.facts(p)}
}
func (a *App) createRoom(w http.ResponseWriter, r *http.Request, u User) {
	var in struct {
		Mode     string    `json:"mode"`
		Name     string    `json:"name"`
		Source   string    `json:"source"`
		CityID   string    `json:"city_id"`
		Scenario *Scenario `json:"scenario"`
		Players  int       `json:"players"`
	}
	if e := decode(w, r, &in); e != nil {
		fail(w, 400, e)
		return
	}
	if index([]string{"solo", "multiplayer", "local", "ai"}, in.Mode) < 0 || len(in.Name) > 100 {
		fail(w, 400, errors.New("Проверьте режим и название"))
		return
	}
	var s Scenario
	var e error
	if in.Source == "manual" || in.Source == "generated" {
		if in.Scenario == nil {
			fail(w, 400, errors.New("Укажите город"))
			return
		}
		s = *in.Scenario
	} else {
		if in.CityID != "" && in.CityID != "default" {
			t, err := read[CityTemplate](a.store, "cities", in.CityID)
			if err != nil || t.Archived {
				fail(w, 400, errors.New("Шаблон недоступен"))
				return
			}
			s = t.Scenario
		} else {
			s, e = read[Scenario](a.store, "config", "scenario")
		}
	}
	if e == nil {
		e = s.validate()
	}
	if e != nil {
		fail(w, 400, e)
		return
	}
	if in.Name == "" {
		in.Name = s.City + " Challenge"
	}
	room := Room{ID: token()[:12], Name: in.Name, Host: u.Name, Mode: in.Mode, Status: "lobby", Created: time.Now().UTC(), Players: []string{u.Name}, Scenario: s, Submitted: map[string]string{}, Drafts: map[string][]Pick{}, Ready: map[string]bool{}, Source: in.Source}
	var botResult *Result
	if in.Mode == "solo" {
		room.Status = "playing"
	}
	if in.Mode == "local" {
		if in.Players < 2 || in.Players > 8 {
			fail(w, 400, errors.New("Локальная игра: 2–8 игроков"))
			return
		}
		room.Players = nil
		for i := 1; i <= in.Players; i++ {
			room.Players = append(room.Players, fmt.Sprintf("%s / Игрок %d", u.Name, i))
		}
		room.Status = "playing"
	}
	if in.Mode == "ai" {
		p, e := (RuleBot{}).Choose(s)
		if e != nil {
			fail(w, 400, e)
			return
		}
		bot := "Бот / " + room.ID
		room.Players = append(room.Players, bot)
		room.Status = "playing"
		v := makeResult(room, bot, u.Name, p)
		botResult = &v
		room.Submitted[bot] = v.ID
	}
	e = a.store.DB.Update(func(tx *Tx) error {
		if botResult != nil {
			if e := put(tx, "results", botResult.ID, *botResult); e != nil {
				return e
			}
		}
		return put(tx, "rooms", room.ID, room)
	})
	if e != nil {
		fail(w, 500, e)
		return
	}
	reply(w, 201, roomView(room, u))
}
func (a *App) socket(w http.ResponseWriter, r *http.Request, u User, id string) {
	upgrader := websocket.Upgrader{CheckOrigin: func(req *http.Request) bool {
		return req.Header.Get("Origin") == "http://"+req.Host || req.Header.Get("Origin") == "https://"+req.Host
	}}
	c, e := upgrader.Upgrade(w, r, nil)
	if e != nil {
		return
	}
	defer c.Close()
	c.SetReadLimit(1024)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, e := c.ReadMessage(); e != nil {
				return
			}
		}
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	last := ""
	for {
		room, e := read[Room](a.store, "rooms", id)
		if e != nil || !roomAccess(room, u) {
			return
		}
		view := roomView(room, u)
		raw, _ := json.Marshal(view)
		if string(raw) != last {
			event := "ROOM_UPDATED"
			if room.Status == "playing" {
				event = "GAME_STARTED"
			}
			if room.Status == "scored" {
				event = "GAME_FINISHED"
			}
			if room.Status == "complete" {
				event = "RESULTS_READY"
			}
			_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if e = c.WriteJSON(map[string]any{"event": event, "data": view}); e != nil {
				return
			}
			last = string(raw)
		}
		select {
		case <-done:
			return
		case <-ticker.C:
			if _, e := a.user(r); e != nil {
				return
			}
		}
	}
}
