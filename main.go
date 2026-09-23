package main

import (
	"embed"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed dist/* assets/* data/seed.json prompts/post-game.txt
var files embed.FS

func loadEnv() {
	b, e := os.ReadFile(".env")
	if e != nil {
		return
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '\'' && v[len(v)-1] == '\'' || v[0] == '"' && v[len(v)-1] == '"') {
			v = v[1 : len(v)-1]
		}
		if ok && os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
}
func seedScenario() Scenario {
	b, _ := files.ReadFile("data/seed.json")
	var s Scenario
	if e := json.Unmarshal(b, &s); e != nil {
		panic(e)
	}
	return s
}
func main() {
	loadEnv()
	dir := os.Getenv("DATA_DIR")
	if dir == "" {
		dir = ".local"
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		log.Fatal(e)
	}
	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		path := filepath.Join(dir, "admin-password.txt")
		b, e := os.ReadFile(path)
		if e == nil {
			password = strings.TrimSpace(string(b))
		} else if os.IsNotExist(e) {
			password = token()
			if e = os.WriteFile(path, []byte(password), 0600); e != nil {
				log.Fatal(e)
			}
		} else {
			log.Fatal(e)
		}
		log.Printf("Пароль администратора: файл %s", path)
	}
	if len(password) < 12 {
		log.Fatal("ADMIN_PASSWORD должен содержать минимум 12 символов")
	}
	seed := seedScenario()
	if e := seed.validate(); e != nil {
		log.Fatal(e)
	}
	store, e := openStore(os.Getenv("DATABASE_URL"), seed, password)
	if e != nil {
		log.Fatal(e)
	}
	defer store.DB.Close()
	a := newApp(store)
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	host := os.Getenv("HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	log.Printf("Аким на 5 часов: http://%s:%s", host, port)
	srv := &http.Server{Addr: host + ":" + port, Handler: a, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
