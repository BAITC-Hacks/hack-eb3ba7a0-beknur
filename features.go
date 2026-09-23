package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"sort"
	"strings"
)

type CityTemplate struct {
	ID       string   `json:"id"`
	Archived bool     `json:"archived"`
	Scenario Scenario `json:"scenario"`
}
type Standing struct {
	Name    string  `json:"name"`
	Games   int     `json:"games"`
	Wins    int     `json:"wins"`
	WinRate float64 `json:"win_rate"`
	Average float64 `json:"average"`
	Best    float64 `json:"best"`
}

func (a *App) standings() ([]Standing, error) {
	out := map[string]Standing{}
	e := a.store.DB.View(func(tx *Tx) error {
		return tx.Bucket([]byte("results")).ForEach(func(_, v []byte) error {
			var r Result
			if e := json.Unmarshal(v, &r); e != nil {
				return e
			}
			if r.Status != "complete" || strings.Contains(r.Player, " / ") {
				return nil
			}
			s := out[r.Player]
			s.Name = r.Player
			s.Games++
			s.Average += r.Facts.Final.Score
			if s.Games == 1 || r.Facts.Final.Score > s.Best {
				s.Best = r.Facts.Final.Score
			}
			if r.PlayersCount > 1 && r.Rank == 1 {
				s.Wins++
			}
			out[r.Player] = s
			return nil
		})
	})
	list := []Standing{}
	for _, s := range out {
		s.Average = round(s.Average / float64(s.Games))
		s.WinRate = round(100 * float64(s.Wins) / float64(s.Games))
		list = append(list, s)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Wins != list[j].Wins {
			return list[i].Wins > list[j].Wins
		}
		if list[i].Average != list[j].Average {
			return list[i].Average > list[j].Average
		}
		return list[i].Name < list[j].Name
	})
	return list, e
}
func (a *App) features(w http.ResponseWriter, r *http.Request, u User) bool {
	path := r.URL.Path
	if strings.HasPrefix(path, "/api/room-ranking/") && r.Method == "GET" {
		room, e := read[Room](a.store, "rooms", strings.TrimPrefix(path, "/api/room-ranking/"))
		if e != nil {
			fail(w, 404, e)
			return true
		}
		if !roomAccess(room, u) {
			fail(w, 403, errors.New("Нет доступа"))
			return true
		}
		if room.Status != "scored" && room.Status != "complete" {
			fail(w, 409, errors.New("Рейтинг доступен после сдачи всех решений"))
			return true
		}
		out := []map[string]any{}
		for _, p := range room.Players {
			x, e := read[Result](a.store, "results", room.Submitted[p])
			if e != nil {
				fail(w, 500, e)
				return true
			}
			out = append(out, map[string]any{"player": p, "rank": x.Rank, "score": x.Facts.Final.Score, "status": x.Status})
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i]["rank"].(int) < out[j]["rank"].(int) })
		reply(w, 200, out)
		return true
	}
	if path == "/api/leaderboard" && r.Method == "GET" {
		v, e := a.standings()
		if e != nil {
			fail(w, 500, e)
		} else {
			reply(w, 200, v)
		}
		return true
	}
	if path == "/api/profile" {
		cred, e := read[credential](a.store, "users", u.Name)
		if e != nil {
			fail(w, 500, e)
			return true
		}
		if r.Method == "PATCH" {
			var in struct {
				Email       string `json:"email"`
				DisplayName string `json:"display_name"`
				Avatar      string `json:"avatar"`
			}
			if e = decode(w, r, &in); e != nil {
				fail(w, 400, e)
				return true
			}
			if len(in.DisplayName) > 100 || len(in.Avatar) > 8 || len(in.Email) > 254 {
				fail(w, 400, errors.New("Слишком длинные поля профиля"))
				return true
			}
			if in.Email != "" {
				if _, e = mail.ParseAddress(in.Email); e != nil {
					fail(w, 400, errors.New("Неверный email"))
					return true
				}
			}
			e = a.store.DB.Update(func(tx *Tx) error {
				v, e := get[credential](tx, "users", u.Name)
				if e != nil {
					return e
				}
				v.Email = in.Email
				v.DisplayName = in.DisplayName
				v.Avatar = in.Avatar
				cred = v
				return put(tx, "users", u.Name, v)
			})
			if e != nil {
				fail(w, 500, e)
				return true
			}
		} else if r.Method != "GET" {
			w.WriteHeader(405)
			return true
		}
		all, _ := a.standings()
		stats := Standing{Name: u.Name}
		for _, s := range all {
			if s.Name == u.Name {
				stats = s
			}
		}
		reply(w, 200, map[string]any{"name": cred.Name, "role": cred.Role, "email": cred.Email, "display_name": cred.DisplayName, "avatar": cred.Avatar, "stats": stats})
		return true
	}
	if path == "/api/profile/password" && r.Method == "POST" {
		var in struct {
			Old string `json:"old"`
			New string `json:"new"`
		}
		if e := decode(w, r, &in); e != nil {
			fail(w, 400, e)
			return true
		}
		if len(in.New) < 12 || len(in.New) > 256 {
			fail(w, 400, errors.New("Новый пароль: 12–256 символов"))
			return true
		}
		e := a.store.DB.Update(func(tx *Tx) error {
			v, e := get[credential](tx, "users", u.Name)
			if e != nil {
				return e
			}
			if subtle.ConstantTimeCompare([]byte(hash(in.Old, v.Salt)), []byte(v.Hash)) != 1 {
				return errors.New("Неверный текущий пароль")
			}
			v.Salt = token()
			v.Hash = hash(in.New, v.Salt)
			if e = put(tx, "users", u.Name, v); e != nil {
				return e
			}
			return tx.Bucket([]byte("sessions")).ForEach(func(k, b []byte) error {
				var s Session
				if e := json.Unmarshal(b, &s); e != nil {
					return e
				}
				if s.User == u.Name {
					return tx.Bucket([]byte("sessions")).Delete(k)
				}
				return nil
			})
		})
		if e != nil {
			fail(w, 400, e)
		} else {
			reply(w, 200, map[string]bool{"ok": true})
		}
		return true
	}
	if path == "/api/cities" && r.Method == "GET" {
		out := []CityTemplate{}
		s, e := read[Scenario](a.store, "config", "scenario")
		if e == nil {
			out = append(out, CityTemplate{ID: "default", Scenario: s})
		}
		e = a.store.DB.View(func(tx *Tx) error {
			return tx.Bucket([]byte("cities")).ForEach(func(_, b []byte) error {
				var v CityTemplate
				if e := json.Unmarshal(b, &v); e != nil {
					return e
				}
				if !v.Archived || u.Role == "admin" {
					out = append(out, v)
				}
				return nil
			})
		})
		if e != nil {
			fail(w, 500, e)
		} else {
			reply(w, 200, out)
		}
		return true
	}
	if path == "/api/cities/generate" && r.Method == "POST" {
		a.generateCity(w, r)
		return true
	}
	if !strings.HasPrefix(path, "/api/admin/") {
		return false
	}
	if u.Role != "admin" {
		fail(w, 403, errors.New("Только администратор"))
		return true
	}
	if path == "/api/admin/cities" && (r.Method == "POST" || r.Method == "PUT") {
		var in CityTemplate
		if e := decode(w, r, &in); e != nil {
			fail(w, 400, e)
			return true
		}
		if e := in.Scenario.validate(); e != nil {
			fail(w, 400, e)
			return true
		}
		if in.ID == "" {
			in.ID = token()[:12]
		}
		if in.ID == "default" {
			fail(w, 400, errors.New("Для базового шаблона используйте /api/scenario"))
			return true
		}
		e := a.store.DB.Update(func(tx *Tx) error {
			if r.Method == "PUT" {
				old, e := get[CityTemplate](tx, "cities", in.ID)
				if e != nil {
					return e
				}
				if old.Scenario.Version != in.Scenario.Version {
					return errors.New("Шаблон уже изменён")
				}
				in.Scenario.Version++
			}
			return put(tx, "cities", in.ID, in)
		})
		if e != nil {
			fail(w, 409, e)
		} else {
			reply(w, 200, in)
		}
		return true
	}
	if strings.HasPrefix(path, "/api/admin/cities/") && r.Method == "DELETE" {
		id := strings.TrimPrefix(path, "/api/admin/cities/")
		e := a.store.DB.Update(func(tx *Tx) error { return tx.Bucket([]byte("cities")).Delete([]byte(id)) })
		if e != nil {
			fail(w, 500, e)
		} else {
			reply(w, 200, map[string]bool{"ok": true})
		}
		return true
	}
	if path == "/api/admin/overview" && r.Method == "GET" {
		users := []map[string]string{}
		rooms := []Room{}
		results := []Result{}
		e := a.store.DB.View(func(tx *Tx) error {
			if e := tx.Bucket([]byte("users")).ForEach(func(_, b []byte) error {
				var v credential
				if e := json.Unmarshal(b, &v); e != nil {
					return e
				}
				users = append(users, map[string]string{"name": v.Name, "role": v.Role, "email": v.Email})
				return nil
			}); e != nil {
				return e
			}
			if e := tx.Bucket([]byte("rooms")).ForEach(func(_, b []byte) error {
				var v Room
				if e := json.Unmarshal(b, &v); e != nil {
					return e
				}
				v.Drafts = nil
				rooms = append(rooms, v)
				return nil
			}); e != nil {
				return e
			}
			return tx.Bucket([]byte("results")).ForEach(func(_, b []byte) error {
				var v Result
				if e := json.Unmarshal(b, &v); e != nil {
					return e
				}
				results = append(results, v)
				return nil
			})
		})
		if e != nil {
			fail(w, 500, e)
			return true
		}
		active := 0
		avg, players := 0., 0.
		count := 0
		for _, v := range rooms {
			if v.Status == "lobby" || v.Status == "playing" {
				active++
			}
			players += float64(len(v.Players))
		}
		for _, v := range results {
			if v.Rank > 0 {
				avg += v.Facts.Final.Score
				count++
			}
		}
		if count > 0 {
			avg /= float64(count)
		}
		if len(rooms) > 0 {
			players /= float64(len(rooms))
		}
		reply(w, 200, map[string]any{"users": users, "rooms": rooms, "results": results, "stats": map[string]any{"users": len(users), "games": len(rooms), "active_rooms": active, "average_score": round(avg), "average_players": round(players)}})
		return true
	}
	http.NotFound(w, r)
	return true
}
