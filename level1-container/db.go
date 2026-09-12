package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

var db *sql.DB
var rdb *redis.Client
var lastCacheStatus atomic.Value // "hit" | "miss" | "no-cache"

const usersCacheKey = "users:all"
const usersCacheTTL = 10 * time.Second

// setupDB buka koneksi Postgres dari env var. Dipanggil sekali di startup;
// kalau env var DB gak di-set (misal jalan standalone tanpa Postgres),
// db tetap nil dan usersHandler fallback ke data statis in-memory.
func setupDB() *sql.DB {
	host := os.Getenv("POSTGRES_HOST")
	if host == "" {
		log.Printf("POSTGRES_HOST kosong, /api/users fallback ke data statis in-memory")
		return nil
	}
	port := os.Getenv("POSTGRES_PORT")
	if port == "" {
		port = "5432"
	}
	user := os.Getenv("POSTGRES_USER")
	pass := os.Getenv("POSTGRES_PASSWORD")
	dbname := os.Getenv("POSTGRES_DB")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable connect_timeout=5",
		host, port, user, pass, dbname)

	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Printf("warning: gagal buka koneksi postgres: %v", err)
		return nil
	}
	conn.SetMaxOpenConns(10)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		log.Printf("warning: postgres belum siap: %v (akan retry tiap request)", err)
	}
	return conn
}

// setupRedis buka koneksi Redis dari env var REDIS_ADDR. nil kalau gak
// di-set - cache-aside otomatis dilewati (langsung ke DB tiap request).
func setupRedis() *redis.Client {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		log.Printf("REDIS_ADDR kosong, cache-aside dilewati")
		return nil
	}
	return redis.NewClient(&redis.Options{
		Addr: addr,
	})
}

// usersHandlerDB - versi Level 6 dari /api/users: cache-aside ke Redis,
// fallback ke Postgres kalau miss, fallback ke data statis in-memory kalau
// db/rdb belum dikonfigurasi (biar image ini tetap kompatibel jalan
// standalone seperti Level 1-5 tanpa Postgres/Redis).
func usersHandlerDB(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&requestCount, 1)
	if appLog != nil {
		appLog.Printf("GET /api/users from %s", r.RemoteAddr)
	}
	w.Header().Set("Content-Type", "application/json")

	if db == nil {
		lastCacheStatus.Store("no-cache")
		json.NewEncoder(w).Encode(users)
		return
	}

	ctx := r.Context()

	// 1. cek Redis dulu
	if rdb != nil {
		val, err := rdb.Get(ctx, usersCacheKey).Result()
		if err == nil {
			lastCacheStatus.Store("hit")
			cacheStatusTotal.WithLabelValues("hit").Inc()
			w.Header().Set("X-Cache", "HIT")
			w.Write([]byte(val))
			return
		}
	}

	// 2. cache miss -> query Postgres
	rows, err := db.QueryContext(ctx, "SELECT id, name FROM users ORDER BY id")
	if err != nil {
		lastCacheStatus.Store("error")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	dbUsers := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name); err == nil {
			dbUsers = append(dbUsers, u)
		}
	}

	body, _ := json.Marshal(dbUsers)

	// 3. simpan ke Redis dengan TTL pendek
	if rdb != nil {
		rdb.Set(ctx, usersCacheKey, body, usersCacheTTL)
	}

	lastCacheStatus.Store("miss")
	cacheStatusTotal.WithLabelValues("miss").Inc()
	w.Header().Set("X-Cache", "MISS")
	w.Write(body)
}

// cacheStatusHandler nunjukin apakah response /api/users TERAKHIR dari
// instance ini asalnya dari cache (hit) atau dari DB (miss).
func cacheStatusHandler(w http.ResponseWriter, r *http.Request) {
	status, _ := lastCacheStatus.Load().(string)
	if status == "" {
		status = "never-hit"
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"last_cache_status": status,
		"hostname":          hostname,
	})
}
