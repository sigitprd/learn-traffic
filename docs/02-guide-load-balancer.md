# Guide 02 — Load Balancer (NGINX + Docker Compose)

3 instance `go-api:v1` di belakang 1 NGINX, generate traffic pakai k6, lihat
distribusi round robin, dan apa yang terjadi kalau 1 instance mati di
tengah traffic.

**Waktu**: ~30 menit
**Prasyarat**:
- Level 1 selesai — image `go-api:v1` harus ada: `docker images go-api:v1`
- `docker compose version` (bundled di Docker Desktop, gak perlu install
  terpisah)
- `k6 version` — kalau belum ada: `brew install k6` (Mac)

Konsep detail ada di [`level-2/README.md`](../level-2/README.md).

---

### Langkah 1 — Jalankan stack

```bash
cd level-2
docker compose up -d
docker compose ps
```

**Expected output:** 4 container, semua `Up`:
```
NAME       IMAGE               STATUS    PORTS
nginx-lb   nginx:1.25-alpine   Up        0.0.0.0:8080->8080/tcp
api-1      go-api:v1           Up        8080/tcp
api-2      go-api:v1           Up        8080/tcp
api-3      go-api:v1           Up        8080/tcp
```

```bash
curl -s -w " [HTTP:%{http_code}]\n" localhost:8080/health
```

**Expected output** (jalankan beberapa kali, harus konsisten):
```
{"status":"ok"} [HTTP:200]
```

### Langkah 2 — Buktikan round robin

```bash
for i in $(seq 1 30); do
  curl -s localhost:8080/whoami | grep -o '"hostname":"[^"]*"'
done
```

**Expected output:** pola berulang rapi tiap 3 (bukan acak) — nama
hostname beda-beda tapi siklusnya konsisten, contoh pola:
```
"hostname":"8cf5be96c49a"
"hostname":"0f73beea686c"
"hostname":"f0f137494524"
"hostname":"8cf5be96c49a"
"hostname":"0f73beea686c"
"hostname":"f0f137494524"
... (berulang sampai 30x)
```

Hostname aktual di laptop kamu PASTI beda (itu container ID), yang penting
POLA-nya: 3 nilai berbeda berulang bergiliran, bukan 1 nilai dominan atau
acak.

### Langkah 3 — Load test k6 4 level rate

Jalankan SATU-SATU (bukan barengan), tiap script 30 detik.

```bash
k6 run k6/load-10rps.js
```

**Expected output (ringkasan):**
```
http_reqs......................: 301   10.03097/s
http_req_duration..............: avg=9.05ms p95=9.61ms
http_req_failed................: 0.00% 0 out of 301
```

```bash
k6 run k6/load-100rps.js
```

**Expected output:**
```
http_reqs......................: 3000   99.985106/s
http_req_duration..............: avg=2.4ms p95=5.29ms
http_req_failed................: 0.00% 0 out of 3000
```

```bash
k6 run k6/load-500rps.js
```

**Expected output:**
```
http_reqs......................: 15001   499.962855/s
http_req_duration..............: avg=4.61ms p95=8.1ms
http_req_failed................: 0.00% 0 out of 15001
```

```bash
k6 run k6/load-1000rps.js
```

**Expected output** (di sini rate mulai MELESET dari target — ini
EXPECTED, bukan bug):
```
http_reqs......................: 29359   978.538187/s
dropped_iterations.............: 642   21.397919/s
http_req_duration..............: avg=39.86ms p95=158.41ms max=1.31s
http_req_failed................: 0.00% 0 out of 29359
```

Rate aktual cuma ~978/s (bukan 1000/s), p95 latency melonjak ke 158ms, tapi
error tetap 0% — sistem melambat, bukan gagal. Kalau di laptop kamu angkanya
beda jauh (rate aktual < 900/s misalnya), itu wajar — tergantung spek CPU
host kamu, bukan berarti ada yang salah.

### Langkah 4 — Cek distribusi per instance

Cek `/stats` di tiap container SEBELUM dan SESUDAH tiap load test buat lihat
pembagian request. Contoh setelah `load-10rps.js`:

```bash
for c in api-1 api-2 api-3; do
  echo -n "$c: "; docker exec $c wget -qO- http://localhost:8080/stats; echo
done
```

**Expected output** (total harus dekat 301, hampir rata 1/3-1/3-1/3):
```
api-1: {"hostname":"...","request_count":100}
api-2: {"hostname":"...","request_count":100}
api-3: {"hostname":"...","request_count":101}
```

### Langkah 5 — Kill instance di tengah traffic

```bash
k6 run k6/load-500rps.js &
K6PID=$!
sleep 10
docker stop api-2
wait $K6PID
docker start api-2
```

**Expected output k6** (perhatikan: rate TETAP 500/s penuh, error TETAP
0%, tapi p95 naik):
```
http_reqs......................: 15001   500.00005/s
http_req_duration..............: avg=5.45ms p95=19.25ms max=2s
http_req_failed................: 0.00%  0 out of 15001
```

p95 naik dari baseline ~8.1ms ke ~19.25ms, dan ada outlier sampai 2 detik —
itu request yang lagi nunggu koneksi ke `api-2` timeout dulu sebelum NGINX
pindah ke backend lain. TAPI tidak ada request yang benar-benar gagal.

---

## Apa yang barusan terjadi

NGINX menunjukkan sifat **load balancer aktif**: begitu 1 backend mati, dia
otomatis retry ke backend lain (`proxy_next_upstream`) TANPA campur tangan
manual — client (k6) sama sekali gak lihat error, cuma latency naik sesaat.
TAPI ini beda fundamental dari self-healing Kubernetes nanti (Level 3):
`api-2` yang di-`docker stop` di sini TETAP MATI sampai kamu jalankan
`docker start api-2` secara manual — Compose gak pernah bikin container
pengganti sendiri. Level 3 akan menunjukkan Kubernetes melakukan hal yang
mirip (retry ke instance sehat) TAPI ditambah kemampuan bikin instance BARU
otomatis — itu yang bikin bedanya "load balancer pasif" vs "self-healing
aktif".

## Troubleshooting

| Gejala | Kemungkinan penyebab | Cara cek / perbaiki |
|---|---|---|
| `docker compose up` gagal, `pull access denied` buat `go-api:v1` | Image belum di-build di Level 1, atau nama image di `docker-compose.yml` typo | `docker images go-api:v1` — kalau kosong, balik ke Guide 01 Langkah 1 |
| `docker compose ps` nunjukin `api-1`/`2`/`3` gak `Up` (Exited) | App crash saat start — biasanya port konflik internal atau env var salah | `docker compose logs api-1` lihat error persis |
| `curl localhost:8080` connection refused | NGINX belum selesai start, atau port 8080 dipakai proses lain di host | `docker compose ps` cek status nginx-lb, `lsof -i :8080` cek proses lain yang pegang port itu |
| Pola `/whoami` gak rapi 3-siklus (dominan 1 hostname atau random) | Kemungkinan client (browser/tool lain) pakai HTTP keep-alive, beda dari `curl` command-line biasa yang buka koneksi baru tiap request | Pastikan pakai `curl` persis seperti di guide, bukan browser atau tool lain |
| k6 `load-1000rps.js` error rate TIDAK 0% (ada failed request beneran) | Laptop host lebih lemah dari environment testing asli, NGINX/Docker keburu reject sebelum sempat antre | Ini bukan berarti config salah — cek `http_req_duration` dan `dropped_iterations`, itu indikator wajar; kalau mau confirm config OK, ulangi test rate lebih rendah (500rps) dan pastikan itu 0% error |
| Setelah Langkah 5, `docker exec api-2 ...` gagal `No such container` | `docker start api-2` di akhir langkah lupa dijalankan / gagal | `docker ps -a | grep api-2` cek statusnya, `docker start api-2` ulang |

## Cleanup

```bash
docker compose down
```

Image `go-api:v1` **JANGAN dihapus** — masih dipakai di Level 3-6 (walau
Level 3+ pindah ke Kubernetes, bukan Docker Compose lagi).

Lanjut ke [Guide 03 — Kubernetes Dasar](03-guide-kubernetes-dasar.md).
