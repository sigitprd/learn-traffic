# Task Brief — Level 2: Multiple Container + Load Balancer + Traffic Gen

## Konteks
Lanjutan dari Level 1 (Go API sudah ada, image `go-api:v1` sudah teruji).
Level 2 fokus ke **load balancing di depan banyak instance**, sebelum masuk ke
Kubernetes di Level 3. Tujuannya paham bagaimana traffic didistribusikan ke
beberapa container identik, dan apa yang terjadi saat salah satu instance mati
— ini fondasi buat paham konsep Service di Kubernetes nanti.

Reuse image `go-api:v1` dari Level 1. Jangan tulis ulang API-nya.

---

## Objective
Jalankan 3 instance `go-api:v1` di belakang satu load balancer (NGINX), generate
traffic dengan berbagai rate pakai k6, lalu amati pola distribusinya — termasuk
saat salah satu instance mati di tengah jalan.

---

## Deliverables

```
level-2/
├── nginx.conf
├── docker-compose.yml
├── k6/
│   ├── load-10rps.js
│   ├── load-100rps.js
│   ├── load-500rps.js
│   └── load-1000rps.js
└── README.md
```

Catatan: mulai Level 2 baru boleh pakai `docker-compose` (di Level 1 sengaja
manual pakai `docker run` biar tiap flag kelihatan jelas).

---

## Arsitektur target

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

---

## Task List (kerjakan berurutan)

### 1. Setup NGINX sebagai load balancer
- [ ] Buat `nginx.conf` dengan `upstream` block berisi 3 backend (`api-1`,
      `api-2`, `api-3`), default load balancing method (round robin)
- [ ] Buat `docker-compose.yml` yang menjalankan: 1 service NGINX + 3 service
      `go-api:v1` (nama container `api-1`, `api-2`, `api-3`), semua di network
      yang sama
- [ ] `docker compose up -d`, verifikasi semua container `Up` (`docker compose ps`)
- [ ] `curl localhost:8080/health` beberapa kali manual dulu, pastikan NGINX
      berhasil forward request ke backend

### 2. Buktikan round robin (bukan cuma asumsi)
- [ ] Tambahkan cara untuk membedakan instance mana yang menjawab tiap request.
      Cara termudah: tambahkan endpoint baru di Go API, `GET /whoami`, yang
      return hostname container (`os.Hostname()`) — **ini perubahan kecil di
      Level 1 code, rebuild image kalau perlu**
- [ ] Hit `/whoami` 30x berturut-turut lewat NGINX, catat urutan hostname yang
      muncul
- [ ] Laporkan di README: apakah polanya round robin rapi (1,2,3,1,2,3,...)
      atau tidak, dan kenapa (tergantung `keepalive` setting NGINX)

### 3. Generate traffic dengan k6
- [ ] Install k6 (`brew install k6` kalau di Mac) — dijalankan dari HOST, bukan
      dari dalam container
- [ ] Buat 4 script k6 terpisah, masing-masing target rate berbeda:
      - `load-10rps.js` → 10 req/s, durasi 30 detik
      - `load-100rps.js` → 100 req/s, durasi 30 detik
      - `load-500rps.js` → 500 req/s, durasi 30 detik
      - `load-1000rps.js` → 1000 req/s, durasi 30 detik
- [ ] Semua target endpoint `/api/users` (bukan `/cpu-intensive` — itu buat
      Level 4/5 nanti)
- [ ] Jalankan satu-satu (jangan barengan), catat output ringkasan k6 tiap run:
      request rate aktual tercapai, response time (p95), error rate

### 4. Amati distribusi traffic per instance
- [ ] Selama tiap load test jalan, hitung berapa request yang masuk ke
      masing-masing `api-1`/`api-2`/`api-3`. Cara termudah: tambahkan counter
      sederhana di Go API (in-memory, per-instance) yang di-expose lewat
      `GET /stats` → `{"hostname": "...", "request_count": N}`
- [ ] Sebelum tiap load test, hit `/stats` ke ketiga instance buat catat baseline
      (harus mulai dari 0 atau direset)
- [ ] Sesudah tiap load test, hit `/stats` lagi ke ketiga instance, laporkan
      pembagian request-nya (apakah merata ~33% masing-masing, atau timpang?)

### 5. Eksperimen: matikan salah satu instance di tengah traffic
- [ ] Jalankan `load-500rps.js`, di tengah durasinya (misal detik ke-10),
      `docker stop api-2`
- [ ] Amati output k6: apakah ada lonjakan error rate saat instance mati?
      Berapa lama sampai stabil lagi?
- [ ] Setelah load test selesai, `docker start api-2` lagi
- [ ] Laporkan di README: apa yang NGINX lakukan saat backend down (apakah ada
      retry ke backend lain otomatis, atau request itu gagal total?)

### 6. Dokumentasi
- [ ] README berisi hasil tiap eksperimen: distribusi per rate, hasil eksperimen
      kill instance, dan kesimpulan singkat tiap poin (bukan cuma command +
      output mentah, tapi juga insight-nya)

---

## Acceptance Criteria

- [ ] 3 instance `go-api:v1` + 1 NGINX jalan lewat `docker compose`
- [ ] Terbukti (bukan diasumsikan) bagaimana NGINX mendistribusikan request ke
      3 backend, lewat endpoint `/whoami` atau `/stats`
- [ ] 4 load test k6 (10/100/500/1000 req/s) dijalankan dan hasilnya (rate
      aktual, p95 latency, error rate) dicatat
- [ ] Distribusi request per instance dilaporkan untuk minimal 1 rate (idealnya
      semua 4)
- [ ] Eksperimen kill instance di tengah traffic dilakukan dan hasilnya
      dilaporkan (bukan cuma "container mati", tapi dampaknya ke traffic)
- [ ] README lengkap dan reproducible

## Constraints
- Jangan pakai Kubernetes/k3d di level ini — murni Docker Compose + NGINX
- k6 dijalankan dari host, bukan dari dalam container (biar hasil rate lebih
  akurat, tidak terpengaruh overhead container tambahan)
- Kalau perlu ubah source code Go API (endpoint `/whoami`, `/stats`), jangan buat
  API baru — extend yang sudah ada dari Level 1, lalu rebuild image sebagai
  `go-api:v1` (overwrite tag yang sama boleh, karena masih level eksperimen)

## Out of Scope (jangan dikerjakan di task ini)
- Kubernetes, Service, Ingress — itu Level 3
- Resource limit / HPA — itu Level 4 & 5
- Prometheus/Grafana — nanti