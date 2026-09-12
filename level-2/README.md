# Level 2 — Multiple Container + Load Balancer + Traffic Gen

3 instance `go-api:v1` (dari Level 1, di-extend dengan `/whoami` + `/stats`) di
belakang 1 NGINX load balancer, digenerate traffic pakai k6 dari host.

```
                 ┌──────────────┐
   k6 traffic ──►│    NGINX     │
                 │ (port 8080)  │
                 └──────┬───────┘
                        │
          ┌─────────────┼─────────────┐
          ▼             ▼             ▼
       api-1         api-2         api-3
     (go-api:v1)   (go-api:v1)   (go-api:v1)
```

## Perubahan di Go API (Level 1 → v1 di-update)

Ditambahkan 2 endpoint di `level1-container/main.go`, rebuild image dengan tag
sama (`go-api:v1`, overwrite — masih level eksperimen):

- `GET /whoami` → `{"hostname": "<container id>"}`, pakai `os.Hostname()`
- `GET /stats` → `{"hostname": "...", "request_count": N}`, counter in-memory
  (atomic), nambah tiap kali `/api/users` di-hit. Reset ke 0 kalau container
  di-restart (state di memory, bukan persisted).

## Menjalankan

```bash
docker compose up -d
docker compose ps      # semua harus "Up"
curl localhost:8080/health
```

---

## 1. Setup NGINX load balancer

```
[Verifikasi compose stack]
Command: docker compose up -d && docker compose ps
Output: 4 container Up — nginx-lb (0.0.0.0:8080->8080/tcp), api-1, api-2, api-3
Kesimpulan: NGINX + 3 backend jalan bareng lewat satu docker-compose.yml,
semua di network lb-net yang sama.
```

```
[Manual curl lewat NGINX]
Command: curl -s -w " [HTTP:%{http_code}]" localhost:8080/health   (x5)
Output: {"status":"ok"} [HTTP:200]   (konsisten 5x)
Kesimpulan: NGINX berhasil forward request ke salah satu backend go-api.
```

## 2. Pembuktian round robin

```
[Hit /whoami 30x berturut-turut]
Command: for i in $(seq 1 30); do curl -s localhost:8080/whoami | ...; done
Output (urutan hostname, di-map ke nama container):
api-3, api-1, api-2, api-3, api-1, api-2, ... (berulang rapi 30x, siklus 3)
Kesimpulan: pola ROUND ROBIN RAPI (bukan acak/timpang). Setiap request dari
curl terpisah = koneksi TCP baru ke NGINX, dan NGINX tidak keep-alive ke
upstream secara default (proxy_pass tanpa "keepalive" directive di upstream
block) → tiap request baru dievaluasi round robin murni dari urutan
`server` di block upstream. Kalau client pakai persistent connection
(HTTP keep-alive) dan NGINX mengaktifkan upstream keepalive, pola bisa
kurang rapi karena request bisa numpuk di 1 koneksi/backend yang sama
selagi koneksi itu masih hidup.
```

## 3 & 4. Load test k6 + distribusi per instance

Semua target `/api/users`, durasi 30 detik, `constant-arrival-rate` executor
(k6 memaksa rate konstan, bukan cuma jumlah VU). `/stats` di-hit langsung ke
tiap container (`docker exec <container> wget -qO- http://localhost:8080/stats`)
sebelum & sesudah tiap test buat itung delta permintaan per instance.

### 10 req/s

```
Command: k6 run k6/load-10rps.js
Output ringkasan k6:
  http_reqs: 301, rate aktual = 10.03/s
  http_req_duration: avg=9.05ms  p90=6.74ms  p95=9.61ms  max=351.64ms
  http_req_failed: 0.00% (0/301)

Distribusi /stats (baseline 0/0/0 → sesudah):
  api-1: 100   api-2: 100   api-3: 101   (total 301)
  ≈ 33.2% / 33.2% / 33.6%

Kesimpulan: rate tercapai persis target, latency kecil, error 0%,
distribusi nyaris sempurna rata ke 3 instance.
```

### 100 req/s

```
Command: k6 run k6/load-100rps.js
Output ringkasan k6:
  http_reqs: 3000, rate aktual = 99.99/s
  http_req_duration: avg=2.4ms  p90=3.18ms  p95=5.29ms  max=101.57ms
  http_req_failed: 0.00% (0/3000)

Distribusi /stats (delta dari baseline 100/100/101):
  api-1: +1000   api-2: +1000   api-3: +1000   (total 3000)
  = 33.3% / 33.3% / 33.3%

Kesimpulan: distribusi PERSIS rata 1000/1000/1000. Latency malah lebih
rendah dari 10rps run (kemungkinan warm-up effect / OS-level connection
reuse), tetap 0% error.
```

### 500 req/s

```
Command: k6 run k6/load-500rps.js
Output ringkasan k6:
  http_reqs: 15001, rate aktual = 499.96/s
  http_req_duration: avg=4.61ms  p90=3.64ms  p95=8.1ms  max=319.97ms
  http_req_failed: 0.00% (0/15001)

Distribusi /stats (delta dari baseline 1100/1100/1101):
  api-1: +5001   api-2: +5000   api-3: +5000   (total 15001)
  ≈ 33.3% masing-masing

Kesimpulan: masih rata sempurna, error tetap 0%, tapi p95 mulai naik
(8.1ms vs 5.29ms di 100rps) — tanda beban makin berat meski belum jadi
masalah.
```

### 1000 req/s

```
Command: k6 run k6/load-1000rps.js
Output ringkasan k6:
  http_reqs: 29359, rate aktual = 978.5/s  (BUKAN 1000/s — meleset dari target)
  dropped_iterations: 642 (21.4/s)
  http_req_duration: avg=39.86ms  p90=74.61ms  p95=158.41ms  max=1.31s
  http_req_failed: 0.00% (0/29359)

Distribusi /stats (delta dari baseline 6101/6100/6101):
  api-1: +9786   api-2: +9787   api-3: +9786   (total 29359)
  ≈ 33.3% masing-masing (tetap rata walau saturasi)

Kesimpulan: di 1000rps, target rate TIDAK tercapai — k6 kehabisan VU
(maxVUs=1000 preallocated tidak cukup nampung latency yang mulai
memanjang), 642 iterasi di-drop karena tidak sempat dieksekusi dalam
window waktu k6. Latency naik tajam (p95 158ms vs 8.1ms di 500rps,
max sampai 1.31s) — ini indikasi NGINX + 3 backend + resource laptop lokal
mulai jadi bottleneck di sekitar ~1000rps, BUKAN backend yang reject
request (error tetap 0%, cuma makin lambat). Distribusi ke 3 instance
tetap merata meski dalam kondisi saturasi.
```

## 5. Eksperimen: kill instance di tengah traffic (500 req/s)

```
[500rps, docker stop api-2 di detik ke-10]
Command:
  k6 run k6/load-500rps.js &          # background
  sleep 10
  docker stop api-2
  wait                                 # tunggu k6 selesai (30s total)
  docker start api-2

Output k6 (full 30s run):
  http_reqs: 15001, rate aktual = 500.00/s (TETAP tercapai penuh!)
  http_req_duration: avg=5.45ms  p90=7.54ms  p95=19.25ms  max=2s
  http_req_failed: 0.00% (0/15001)   ← TIDAK ADA request gagal ke client

Distribusi /stats sesudah:
  api-1: +6666
  api-3: +6667
  api-2: 0 (restart => counter reset, tapi dari perhitungan total:
            15001 - 6666 - 6667 = 1668 request masuk ke api-2 SEBELUM mati)

Kesimpulan:
- NGINX MELAKUKAN RETRY OTOMATIS ke backend lain saat salah satu upstream
  down — ini efek `proxy_next_upstream error timeout http_502 http_503
  http_504` di nginx.conf (nilai ini sebenarnya default NGINX untuk
  "error timeout", jadi retry-nya sudah otomatis walau tanpa directive
  eksplisit). Client (k6) TIDAK melihat error sama sekali (0% failed).
- Dampaknya BUKAN di error rate, tapi di LATENCY: p95 melonjak dari
  baseline 8.1ms (test 500rps normal) ke 19.25ms, dan ada outlier sampai
  2 DETIK — ini kemungkinan request yang lagi "nyangkut" nunggu koneksi
  ke api-2 timeout dulu (proxy_connect_timeout 2s di config) sebelum
  NGINX pindah ke backend lain.
- api-2 sempat nerima ~1668 request (≈1/3 dari 5000 request di 10 detik
  pertama, sesuai proporsi round robin) sebelum mati, lalu 0 sesudahnya
  — sisa ~13333 request otomatis kebagi rata ke api-1 (6666) & api-3
  (6667), bukan numpuk di salah satu.
- Kesimpulan besar: NGINX sebagai load balancer di depan banyak instance
  identik memberi HIGH AVAILABILITY built-in — matinya 1 backend tidak
  bikin service down, cuma nambah latency sesaat saat request yang lagi
  "apes" nunggu timeout. Ini persis alasan kenapa di Kubernetes nanti,
  Service (yang juga load balance ke banyak Pod) jadi fondasi self-healing.
```

## Kesimpulan umum Level 2

1. **Round robin NGINX default itu genuinely rata**, bukan cuma asumsi —
   dibuktikan lewat `/whoami` (pola 30x rapi) dan `/stats` (distribusi
   ~33.3% konsisten di semua rate, termasuk pas saturasi 1000rps).
2. **Error rate tetap 0% di semua rate**, termasuk saat 1 backend mati —
   tapi actual throughput & latency yang jadi indikator kesehatan real,
   bukan cuma error rate. Di 1000rps, error masih 0% padahal sistem
   sebenarnya sudah kewalahan (rate meleset, latency naik 15-20x).
3. **NGINX retry otomatis ke backend hidup saat 1 backend down** — inilah
   yang bikin load balancer beda dari sekadar "kirim ke salah satu server":
   dia aktif recover dari kegagalan upstream, bukan pasif forward.

## Cleanup

```bash
docker compose down
```

(Semua container & network `level-2_lb-net` dihapus setelah semua
eksperimen dicatat.)
