package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/jackc/pgx/v5/stdlib"
	"time"
)

// Versioned game aggregates remain JSONB snapshots. PostgreSQL owns transactions,
// durable writes and indexes; archived reports never depend on a changed template.
type Database struct{ conn *sql.DB }
type Tx struct {
	tx  *sql.Tx
	ctx context.Context
	err error
}
type Bucket struct {
	tx   *Tx
	name string
}

var tables = map[string]bool{"users": true, "sessions": true, "rooms": true, "results": true, "config": true, "cities": true}

func openDatabase(url string) (*Database, error) {
	if url == "" {
		return nil, errors.New("Задайте DATABASE_URL для PostgreSQL (см. README)")
	}
	db, e := sql.Open("pgx", url)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(12)
	db.SetMaxIdleConns(4)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if e = db.PingContext(ctx); e != nil {
		db.Close()
		return nil, errors.New("PostgreSQL недоступен: проверьте DATABASE_URL и запуск БД")
	}
	return &Database{db}, nil
}
func (d *Database) Close() error { return d.conn.Close() }
func (d *Database) transaction(write bool, fn func(*Tx) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tx, e := d.conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: !write})
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if write {
		if _, e = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(525605)"); e != nil {
			return e
		}
	}
	t := &Tx{tx: tx, ctx: ctx}
	if e = fn(t); e != nil {
		return e
	}
	if t.err != nil {
		return t.err
	}
	return tx.Commit()
}
func (d *Database) View(fn func(*Tx) error) error   { return d.transaction(false, fn) }
func (d *Database) Update(fn func(*Tx) error) error { return d.transaction(true, fn) }
func (t *Tx) Bucket(name []byte) *Bucket {
	n := string(name)
	if !tables[n] {
		panic("unknown internal table")
	}
	return &Bucket{t, n}
}
func (t *Tx) CreateBucketIfNotExists(name []byte) (*Bucket, error) {
	b := t.Bucket(name)
	_, e := t.tx.ExecContext(t.ctx, `CREATE TABLE IF NOT EXISTS `+b.name+` (key TEXT PRIMARY KEY, data JSONB NOT NULL)`)
	if e != nil {
		return nil, e
	}
	if b.name == "results" {
		_, e = t.tx.ExecContext(t.ctx, `CREATE INDEX IF NOT EXISTS results_player_idx ON results ((data->>'player')); CREATE INDEX IF NOT EXISTS results_room_idx ON results ((data->>'room_id')); CREATE INDEX IF NOT EXISTS results_status_idx ON results ((data->>'status'))`)
	}
	return b, e
}
func (b *Bucket) Get(key []byte) []byte {
	var v []byte
	e := b.tx.tx.QueryRowContext(b.tx.ctx, "SELECT data FROM "+b.name+" WHERE key=$1", string(key)).Scan(&v)
	if e == sql.ErrNoRows {
		return nil
	}
	if e != nil {
		b.tx.err = e
		return nil
	}
	return v
}
func (b *Bucket) Put(key, value []byte) error {
	_, e := b.tx.tx.ExecContext(b.tx.ctx, "INSERT INTO "+b.name+" (key,data) VALUES ($1,$2::jsonb) ON CONFLICT (key) DO UPDATE SET data=EXCLUDED.data", string(key), string(value))
	return e
}
func (b *Bucket) Delete(key []byte) error {
	_, e := b.tx.tx.ExecContext(b.tx.ctx, "DELETE FROM "+b.name+" WHERE key=$1", string(key))
	return e
}
func (b *Bucket) ForEach(fn func([]byte, []byte) error) error {
	rows, e := b.tx.tx.QueryContext(b.tx.ctx, "SELECT key,data FROM "+b.name+" ORDER BY key")
	if e != nil {
		return e
	}
	type row struct{ k, v []byte }
	all := []row{}
	for rows.Next() {
		var r row
		if e = rows.Scan(&r.k, &r.v); e != nil {
			rows.Close()
			return e
		}
		all = append(all, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, r := range all {
		if e = fn(r.k, r.v); e != nil {
			return fmt.Errorf("%s: %w", b.name, e)
		}
	}
	return nil
}
