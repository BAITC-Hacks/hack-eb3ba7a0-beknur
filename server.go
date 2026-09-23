package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"

	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type App struct {
	store    *Store
	client   *http.Client
	aiURL    string
	slots    chan struct{}
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func newApp(s *Store) *App {
	return &App{store: s, client: &http.Client{Timeout: 90 * time.Second}, aiURL: "https://api.openai.com/v1/responses", slots: make(chan struct{}, 2), attempts: map[string][]time.Time{}}
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, e error) {
	reply(w, status, map[string]string{"error": e.Error()})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return errors.New("Некорректный JSON")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("Лишние данные после JSON")
	}
	return nil
}
func (a *App) user(r *http.Request) (User, error) {
	c, e := r.Cookie("akim_session")
	if e != nil {
		return User{}, e
	}
	s, e := read[Session](a.store, "sessions", c.Value)
	if e != nil || time.Now().After(s.Expires) {
		return User{}, errors.New("Войдите в аккаунт")
	}
	u, e := read[credential](a.store, "users", s.User)
	return User{Name: u.Name, Role: u.Role}, e
}
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	if r.Method != "GET" && r.Method != "HEAD" {
		origin := r.Header.Get("Origin")
		expected := "http://" + r.Host
		if r.TLS != nil {
			expected = "https://" + r.Host
		}
		if public := os.Getenv("PUBLIC_ORIGIN"); public != "" {
			expected = public
		}
		if (origin != "" && origin != expected) || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			fail(w, 403, errors.New("Запрос с другого сайта отклонён"))
			return
		}
	}
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		if r.Method != "GET" && r.Method != "HEAD" {
			w.WriteHeader(405)
			return
		}
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}
		if path != "/index.html" && path != "/app.js" && path != "/style.css" {
			http.NotFound(w, r)
			return
		}
		b, e := files.ReadFile("dist" + path)
		if e != nil {
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(path, ".js") {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		}
		if strings.HasSuffix(path, ".css") {
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		}
		if strings.HasSuffix(path, ".html") {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		}
		_, _ = w.Write(b)
		return
	}
	if r.Method == "POST" && (r.URL.Path == "/api/login" || r.URL.Path == "/api/register") {
		a.login(w, r)
		return
	}
	u, e := a.user(r)
	if e != nil {
		fail(w, 401, errors.New("Войдите в аккаунт"))
		return
	}
	if a.features(w, r, u) {
		return
	}
	switch {
	case r.Method == "GET" && r.URL.Path == "/api/me":
		reply(w, 200, u)
	case r.Method == "POST" && r.URL.Path == "/api/logout":
		c, _ := r.Cookie("akim_session")
		e := a.store.DB.Update(func(tx *Tx) error { return tx.Bucket([]byte("sessions")).Delete([]byte(c.Value)) })
		if e != nil {
			fail(w, 500, errors.New("Не удалось завершить сессию"))
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "akim_session", Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
		reply(w, 200, map[string]bool{"ok": true})
	case r.URL.Path == "/api/scenario" && (r.Method == "GET" || r.Method == "PUT"):
		if r.Method == "GET" {
			s, e := read[Scenario](a.store, "config", "scenario")
			if e != nil {
				fail(w, 500, e)
				return
			}
			reply(w, 200, s)
			return
		}
		if u.Role != "admin" {
			fail(w, 403, errors.New("Только администратор меняет исходные данные"))
			return
		}
		var s Scenario
		if e = decode(w, r, &s); e == nil {
			e = s.validate()
		}
		if e != nil {
			fail(w, 400, e)
			return
		}
		e = a.store.DB.Update(func(tx *Tx) error {
			old, e := get[Scenario](tx, "config", "scenario")
			if e != nil {
				return e
			}
			if s.Version != old.Version {
				return errors.New("Данные уже изменены. Загрузите новую версию")
			}
			s.Version++
			return put(tx, "config", "scenario", s)
		})
		if e != nil {
			fail(w, 409, e)
			return
		}
		reply(w, 200, s)
	case strings.HasPrefix(r.URL.Path, "/api/rooms"):
		a.rooms(w, r, u)
	case r.Method == "GET" && r.URL.Path == "/api/history":
		results := []Result{}
		e = a.store.DB.View(func(tx *Tx) error {
			return tx.Bucket([]byte("results")).ForEach(func(_, v []byte) error {
				var x Result
				if e := json.Unmarshal(v, &x); e != nil {
					return e
				}
				if (x.Player == u.Name || x.Owner == u.Name || u.Role == "admin") && !((x.Mode == "local" || x.Player != u.Name) && x.Rank == 0 && u.Role != "admin") {
					results = append(results, x)
				}
				return nil
			})
		})
		if e != nil {
			fail(w, 500, e)
			return
		}
		sort.Slice(results, func(i, j int) bool { return results[i].Created.After(results[j].Created) })
		reply(w, 200, results)
	case strings.HasPrefix(r.URL.Path, "/api/results/"):
		a.result(w, r, u)
	default:
		http.NotFound(w, r)
	}
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	a.mu.Lock()
	now := time.Now()
	recent := []time.Time{}
	for _, t := range a.attempts[ip] {
		if now.Sub(t) < 15*time.Minute {
			recent = append(recent, t)
		}
	}
	if len(recent) >= 20 {
		a.mu.Unlock()
		fail(w, 429, errors.New("Слишком много попыток входа. Повторите через 15 минут"))
		return
	}
	if len(a.attempts) > 10000 {
		a.attempts = map[string][]time.Time{}
	}
	a.attempts[ip] = append(recent, now)
	a.mu.Unlock()
	var in struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if e := decode(w, r, &in); e != nil {
		fail(w, 400, e)
		return
	}
	in.Name = strings.ToLower(strings.TrimSpace(in.Name))
	if !regexp.MustCompile(`^[a-z0-9_-]{3,32}$`).MatchString(in.Name) || len(in.Password) < 12 || len(in.Password) > 256 {
		fail(w, 400, errors.New("Логин: 3–32 латинских символа, цифры, _ или -. Пароль: 12–256 символов"))
		return
	}
	var u credential
	var e error
	if r.URL.Path == "/api/register" {
		salt := token()
		u = credential{Name: in.Name, Role: "akim", Salt: salt, Hash: hash(in.Password, salt)}
		e = a.store.DB.Update(func(tx *Tx) error {
			if tx.Bucket([]byte("users")).Get([]byte(in.Name)) != nil {
				return errors.New("Логин уже занят")
			}
			return put(tx, "users", in.Name, u)
		})
		if e != nil {
			fail(w, 409, e)
			return
		}
	} else {
		u, e = read[credential](a.store, "users", in.Name)
		salt := u.Salt
		if salt == "" {
			salt = "unknown-user-constant-salt"
		}
		h := hash(in.Password, salt)
		if e != nil || subtle.ConstantTimeCompare([]byte(h), []byte(u.Hash)) != 1 {
			fail(w, 401, errors.New("Неверный логин или пароль"))
			return
		}
	}
	id := token()
	e = a.store.DB.Update(func(tx *Tx) error {
		if c, e := r.Cookie("akim_session"); e == nil {
			if e = tx.Bucket([]byte("sessions")).Delete([]byte(c.Value)); e != nil {
				return e
			}
		}
		return put(tx, "sessions", id, Session{u.Name, time.Now().Add(7 * 24 * time.Hour)})
	})
	if e != nil {
		fail(w, 500, e)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "akim_session", Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil || strings.HasPrefix(os.Getenv("PUBLIC_ORIGIN"), "https://"), MaxAge: 7 * 24 * 3600})
	reply(w, 200, User{Name: u.Name, Role: u.Role})
}
func (a *App) result(w http.ResponseWriter, r *http.Request, u User) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/results/"), "/")
	if len(parts) > 2 {
		http.NotFound(w, r)
		return
	}
	x, e := read[Result](a.store, "results", parts[0])
	if e != nil {
		fail(w, 404, e)
		return
	}
	if (x.Player != u.Name && x.Owner != u.Name && u.Role != "admin") || ((x.Mode == "local" || x.Player != u.Name) && x.Rank == 0 && u.Role != "admin") {
		fail(w, 403, errors.New("Этот отчёт принадлежит другому игроку"))
		return
	}
	if len(parts) == 1 && r.Method == "GET" {
		reply(w, 200, x)
		return
	}
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	format := parts[1]
	if format == "retry" && r.Method == "POST" {
		e = a.store.DB.Update(func(tx *Tx) error {
			v, e := get[Result](tx, "results", x.ID)
			if e != nil {
				return e
			}
			if v.Status != "analysis_failed" {
				return errors.New("Повтор доступен только после ошибки AI")
			}
			v.Status = "analyzing"
			v.Error = ""
			return put(tx, "results", v.ID, v)
		})
		if e != nil {
			fail(w, 409, e)
			return
		}
		go a.analyze(x.ID)
		reply(w, 202, map[string]string{"status": "analyzing"})
		return
	}
	if r.Method != "GET" {
		w.WriteHeader(405)
		return
	}
	if x.Status != "complete" || x.AI == nil {
		fail(w, 409, errors.New("Отчёт доступен после завершения AI-анализа"))
		return
	}
	var b []byte
	content := ""
	switch format {
	case "json":
		b, e = json.MarshalIndent(x, "", "  ")
		content = "application/json"
	case "pdf":
		b, e = renderPDF(x)
		content = "application/pdf"
	case "docx":
		b, e = renderDOCX(x)
		content = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		http.NotFound(w, r)
		return
	}
	if e != nil {
		fail(w, 500, errors.New("Не удалось создать документ"))
		return
	}
	w.Header().Set("Content-Type", content)
	w.Header().Set("Content-Disposition", "attachment; filename=akim-"+x.ID+"."+format)
	_, _ = w.Write(b)
}
