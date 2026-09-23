package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFullGame(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration")
	}
	db, e := openDatabase(dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	schema := "test_" + token()[:16]
	if _, e = db.conn.Exec("CREATE SCHEMA " + schema); e != nil {
		t.Fatal(e)
	}
	defer db.conn.Exec("DROP SCHEMA " + schema + " CASCADE")
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	store, e := openStore(u.String(), seedScenario(), "integration-admin-password")
	if e != nil {
		t.Fatal(e)
	}
	defer store.DB.Close()
	a := newApp(store)
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_MODEL", "test-model")
	var calls atomic.Int32
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var in struct {
			Input string `json:"input"`
		}
		if e := json.NewDecoder(r.Body).Decode(&in); e != nil {
			t.Error(e)
		}
		var x Result
		_ = json.Unmarshal([]byte(in.Input), &x)
		v := fixtureResult().AI
		b, _ := json.Marshal(v)
		reply(w, 200, map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": string(b)}}}}})
	}))
	defer mock.Close()
	a.aiURL = mock.URL
	request := func(method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "http://localhost/api"+path, bytes.NewReader(b))
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w
	}
	parseRoom := func(w *httptest.ResponseRecorder) Room {
		t.Helper()
		if w.Code >= 300 {
			t.Fatal(w.Code, w.Body.String())
		}
		var v struct {
			Room Room `json:"room"`
		}
		if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
			t.Fatal(e)
		}
		return v.Room
	}
	register := func(name string) *http.Cookie {
		w := request("POST", "/register", map[string]string{"name": name, "password": "secure-test-password"}, nil)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		return w.Result().Cookies()[0]
	}
	alice, bob := register("alice"), register("bob")
	if w := request("GET", "/admin/overview", nil, alice); w.Code != 403 {
		t.Fatal("RBAC")
	}
	room := parseRoom(request("POST", "/rooms", map[string]any{"mode": "multiplayer", "source": "template", "city_id": "default"}, alice))
	room = parseRoom(request("POST", "/rooms/"+room.ID+"/join", nil, bob))
	for _, c := range []*http.Cookie{alice, bob} {
		parseRoom(request("POST", "/rooms/"+room.ID+"/ready", nil, c))
	}
	room = parseRoom(request("POST", "/rooms/"+room.ID+"/start", nil, alice))
	for _, c := range []*http.Cookie{alice, bob} {
		bad := request("POST", "/rooms/"+room.ID+"/submit", nil, c)
		if bad.Code < 400 {
			t.Fatal("accepted empty submission")
		}
		parseRoom(request("POST", "/rooms/"+room.ID+"/decisions", map[string]any{"picks": example()}, c))
		room = parseRoom(request("POST", "/rooms/"+room.ID+"/submit", nil, c))
		if c == alice {
			if w := request("GET", "/results/"+room.Submitted["alice"], nil, bob); w.Code != 403 {
				t.Fatal("leaked opponent decisions")
			}
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		v, _ := read[Result](store, "results", room.Submitted["alice"])
		if v.Status == "complete" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("AI incomplete: %s %s", v.Status, v.Error)
		}
		time.Sleep(20 * time.Millisecond)
	}
	id := room.Submitted["alice"]
	for _, format := range []string{"pdf", "docx", "json"} {
		w := request("GET", "/results/"+id+"/"+format, nil, alice)
		if w.Code != 200 || w.Body.Len() < 100 {
			t.Fatal("download", format, w.Code, w.Body.String())
		}
	}
	count := calls.Load()
	request("GET", "/results/"+id+"/pdf", nil, alice)
	if calls.Load() != count {
		t.Fatal("download regenerated AI")
	}
	w := request("GET", "/leaderboard", nil, alice)
	if !strings.Contains(w.Body.String(), "alice") {
		t.Fatal("leaderboard missing player")
	}
	if w = request("GET", "/history", nil, bob); strings.Contains(w.Body.String(), `"player":"alice"`) {
		t.Fatal("history leaked")
	}
	a2 := newApp(store)
	r := httptest.NewRequest("GET", "http://localhost/api/results/"+id, nil)
	r.AddCookie(alice)
	out := httptest.NewRecorder()
	a2.ServeHTTP(out, r)
	if out.Code != 200 {
		t.Fatal("history not persistent")
	}
}
