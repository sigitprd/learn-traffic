# Tujuan Belajar

Nama folder-nya `learn-traffic`, tapi traffic (load balancing, k6, HPA)
cuma salah satu sumbu. Yang sebenernya dipelajari di 6 guide ini adalah
**operasional sistem produksi secara utuh** — gimana app beneran jalan dan
bertahan di production, bukan cuma `docker run` doang.

## 5 sumbu yang dipelajari

- **Traffic & scaling** — load balancing, distribusi request, HPA
  autoscaling, cari titik jenuh sistem di bawah beban brutal.
  (Guide 2, 5, 6-Experiment D)
- **Resource management** — requests/limits, efeknya ke scheduling, CPU
  throttling (silent), OOMKill (proses beneran dibunuh kernel).
  (Guide 4)
- **Resilience** — self-healing otomatis, crash recovery, CrashLoopBackOff,
  buktikan gak ada connection leak pas Pod mati.
  (Guide 3, 6-Experiment B, 6-Experiment C)
- **Observability** — metrics scraping (Prometheus), dashboard (Grafana),
  cara baca sinyal kesehatan sistem tanpa nebak-nebak.
  (Guide 6)
- **State management** — database (Postgres), cache-aside pattern (Redis),
  koneksi database yang ikut scale bareng replica.
  (Guide 6)

## Progresi 6 guide

1. **Container Basics** — fondasi: image, container, port, env, volume,
   network, resource limit level Docker.
2. **Load Balancer** — NGINX + Compose, load balancing manual, recovery
   manual kalau instance mati (gak otomatis).
3. **Kubernetes Dasar** — Deployment/Pod/Service/Ingress, self-healing
   OTOMATIS lewat declarative reconciliation.
4. **Resource Limits** — requests/limits, throttle vs OOMKill.
5. **HPA** — autoscaling berbasis resource utilization (bukan "traffic > N
   request = spawn pod").
6. **Production Simulation** — gabungin semuanya + state (Postgres/Redis)
   + observability (Prometheus/Grafana) + 4 eksperimen chaos.

## Kenapa urutannya begini

Tiap guide sengaja dibangun di atas guide sebelumnya, biar kelihatan
KONTRASnya — bukan cuma nambah fitur baru:

- Level 2 (manual recovery) vs Level 3 (self-healing otomatis) — beda
  fundamental soal siapa yang "mengawasi" kesehatan sistem.
- Level 4 (CPU throttle silent, gak ada Warning) vs OOMKill (proses mati,
  ada Event jelas) — dua mekanisme kegagalan yang keliatannya mirip tapi
  behaviour beda total.
- Level 5 (HPA pakai requests, bukan limits) — insight yang cuma kebentur
  kalau udah paham Level 4 duluan.
- Level 6 (app crash `exit 1` vs OOMKill `exit 137`) — practical
  differentiator debugging incident di production beneran.

Tujuan akhirnya: pas ketemu sistem production yang lebih kompleks di kerjaan
nyata (bukan lab), kamu udah punya mental model yang benar soal KENAPA
sistem itu berperilaku begitu — bukan cuma hafal command `kubectl`.
