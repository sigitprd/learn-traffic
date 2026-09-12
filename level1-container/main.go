package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

var users = []User{
	{ID: 1, Name: "Alice"},
	{ID: 2, Name: "Bob"},
	{ID: 3, Name: "Charlie"},
}

var appLog *log.Logger
var requestCount int64
var hostname string
var memoryHog [][]byte // nahan referensi alokasi /memory-hog biar nggak di-GC

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// whoamiHandler return hostname container - buat buktiin instance mana
// yang jawab tiap request (verifikasi load balancing).
func whoamiHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"hostname": hostname})
}

// statsHandler expose counter request per-instance (in-memory, reset tiap
// container restart).
func statsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"hostname":      hostname,
		"request_count": atomic.LoadInt64(&requestCount),
	})
}

// cpuIntensiveHandler sengaja membebani CPU (busy loop) buat simulasi load.
func cpuIntensiveHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	count := 0
	for time.Since(start) < 500*time.Millisecond {
		count++
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"result":      "done",
		"iterations":  count,
		"duration_ms": time.Since(start).Milliseconds(),
	})
}

// memoryHogHandler sengaja alokasi slice besar (default 200MB) dan nahan
// referensinya di variable global biar nggak langsung di-GC - buat
// eksperimen OOMKilled (Level 4). HANYA dipanggil manual ke Pod tertentu
// langsung, JANGAN lewat Service/Ingress (Pod lain nggak boleh ikut kena).
func memoryHogHandler(w http.ResponseWriter, r *http.Request) {
	mb := 200
	if v := r.URL.Query().Get("mb"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			mb = n
		}
	}
	buf := make([]byte, mb*1024*1024)
	for i := range buf {
		buf[i] = byte(i) // sentuh tiap byte biar beneran ke-commit, bukan cuma virtual alloc
	}
	memoryHog = append(memoryHog, buf)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"allocated_mb": mb,
		"total_chunks": len(memoryHog),
		"hostname":     hostname,
	})
}

func setupLogger() *log.Logger {
	logPath := os.Getenv("LOG_PATH")
	if logPath == "" {
		logPath = "/var/log/app.log"
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("warning: cannot open log file %s: %v (falling back to stdout only)", logPath, err)
		return nil
	}
	return log.New(f, "", log.LstdFlags)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	appLog = setupLogger()

	h, err := os.Hostname()
	if err != nil {
		h = "unknown"
	}
	hostname = h

	db = setupDB()
	rdb = setupRedis()
	lastCacheStatus.Store("never-hit")

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/api/users", instrumentHandler("/api/users", usersHandlerDB))
	http.HandleFunc("/api/users/cache-status", cacheStatusHandler)
	http.HandleFunc("/cpu-intensive", instrumentHandler("/cpu-intensive", cpuIntensiveHandler))
	http.HandleFunc("/whoami", whoamiHandler)
	http.HandleFunc("/stats", statsHandler)
	http.HandleFunc("/memory-hog", memoryHogHandler)
	http.HandleFunc("/crash", crashHandler)
	http.Handle("/metrics", metricsHandlerRegister())

	addr := ":" + port
	log.Printf("server listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}
