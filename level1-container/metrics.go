package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "go_api_http_requests_total",
			Help: "Total HTTP request diterima, per path dan status code.",
		},
		[]string{"path", "status"},
	)
	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "go_api_http_request_duration_seconds",
			Help:    "Distribusi durasi request HTTP, per path.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"path"},
	)
	cacheStatusTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "go_api_cache_status_total",
			Help: "Total hasil cache-aside /api/users, per status (hit/miss).",
		},
		[]string{"status"},
	)
)

// metricsMiddleware bungkus tiap handler, catat count + durasi ke
// Prometheus. Dipasang di semua route lewat instrumentHandler().
func instrumentHandler(path string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		h(rec, r)
		duration := time.Since(start).Seconds()
		httpRequestsTotal.WithLabelValues(path, strconv.Itoa(rec.status)).Inc()
		httpRequestDuration.WithLabelValues(path).Observe(duration)
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func metricsHandlerRegister() http.Handler {
	return promhttp.Handler()
}
