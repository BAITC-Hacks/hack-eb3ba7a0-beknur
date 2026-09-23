package main

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"time"
)

type User struct {
	Name string `json:"name"`
	Role string `json:"role"`
	Salt string `json:"-"`
	Hash string `json:"-"`
}
type credential struct {
	Name, Role, Salt, Hash string
	Email                  string
	DisplayName            string
	Avatar                 string
}
type Session struct {
	User    string
	Expires time.Time
}
type Room struct {
	Name      string            `json:"name"`
	Ready     map[string]bool   `json:"ready"`
	Drafts    map[string][]Pick `json:"drafts"`
	Source    string            `json:"source"`
	ID        string            `json:"id"`
	Host      string            `json:"host"`
	Mode      string            `json:"mode"`
	Status    string            `json:"status"`
	Created   time.Time         `json:"created"`
	Players   []string          `json:"players"`
	Scenario  Scenario          `json:"scenario"`
	Submitted map[string]string `json:"submitted"`
}
type Analysis struct {
	Summary         string            `json:"summary"`
	Right           []string          `json:"what_you_did_right"`
	Improvements    []string          `json:"what_you_did_wrong"`
	Tradeoffs       []string          `json:"tradeoffs"`
	Risks           []string          `json:"risks"`
	Recommendations []string          `json:"recommendations"`
	Districts       map[string]string `json:"districts"`
	Decisions       map[string]string `json:"decisions"`
}
type Result struct {
	Owner        string    `json:"owner"`
	ID           string    `json:"id"`
	RoomID       string    `json:"room_id"`
	Player       string    `json:"player"`
	City         string    `json:"city"`
	Mode         string    `json:"mode"`
	Created      time.Time `json:"created"`
	PlayersCount int       `json:"players_count"`
	Rank         int       `json:"rank"`
	Status       string    `json:"status"`
	Error        string    `json:"error,omitempty"`
	Scenario     Scenario  `json:"scenario"`
	Picks        []Pick    `json:"picks"`
	Facts        Facts     `json:"facts"`
	AI           *Analysis `json:"ai_analysis"`
	Model        string    `json:"ai_model,omitempty"`
}
type Store struct{ DB *Database }

func token() string {
	b := make([]byte, 24)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func hash(password, salt string) string {
	b, e := pbkdf2.Key(sha256.New, password, []byte(salt), 120000, 32)
	if e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func put(tx *Tx, bucket, key string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	return tx.Bucket([]byte(bucket)).Put([]byte(key), b)
}
func get[T any](tx *Tx, bucket, key string) (T, error) {
	var v T
	b := tx.Bucket([]byte(bucket)).Get([]byte(key))
	if b == nil {
		return v, errors.New("Запись не найдена")
	}
	e := json.Unmarshal(b, &v)
	return v, e
}
func read[T any](s *Store, bucket, key string) (T, error) {
	var v T
	e := s.DB.View(func(tx *Tx) error { var e error; v, e = get[T](tx, bucket, key); return e })
	return v, e
}
func openStore(path string, seed Scenario, adminPassword string) (*Store, error) {
	db, e := openDatabase(path)
	if e != nil {
		return nil, e
	}
	s := &Store{db}
	e = db.Update(func(tx *Tx) error {
		for _, b := range []string{"users", "sessions", "rooms", "results", "config", "cities"} {
			if _, e := tx.CreateBucketIfNotExists([]byte(b)); e != nil {
				return e
			}
		}
		if tx.Bucket([]byte("config")).Get([]byte("scenario")) == nil {
			if e := put(tx, "config", "scenario", seed); e != nil {
				return e
			}
		}
		if tx.Bucket([]byte("users")).Get([]byte("admin")) == nil {
			salt := token()
			if e := put(tx, "users", "admin", credential{Name: "admin", Role: "admin", Salt: salt, Hash: hash(adminPassword, salt)}); e != nil {
				return e
			}
		}
		return tx.Bucket([]byte("results")).ForEach(func(k, v []byte) error {
			var r Result
			if e := json.Unmarshal(v, &r); e != nil {
				return e
			}
			if r.Status == "analyzing" {
				r.Status = "analysis_failed"
				r.Error = "Сервер перезапущен. Повторите AI-анализ."
				return put(tx, "results", string(k), r)
			}
			return nil
		})
	})
	if e != nil {
		db.Close()
		return nil, e
	}
	return s, nil
}
