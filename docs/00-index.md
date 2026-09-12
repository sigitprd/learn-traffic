# Kubernetes Home Lab — Panduan Step-by-Step

Panduan ini beda dari README di tiap folder `level-*/` — README isinya
laporan hasil eksperimen yang sudah dijalankan. Dokumen di sini murni buat
**kamu ikuti sendiri di terminal**, dari nol, tanpa AI agent.

Belum jelas kenapa 6 guide ini disusun begini atau apa yang sebenernya mau
dipelajari? Baca [tujuan-belajar.md](tujuan-belajar.md) dulu.

Tools yang dipakai: Docker, k3d, kubectl, k6. Semua lab jalan 100% lokal —
gak butuh akun cloud.

## Daftar level

| # | Guide | Topik | Waktu | Prasyarat |
|---|---|---|---|---|
| 1 | [01-guide-container-basics.md](01-guide-container-basics.md) | Docker dasar: image, container, port, env, volume, network, resource limit | ~30 menit | Docker terinstall |
| 2 | [02-guide-load-balancer.md](02-guide-load-balancer.md) | NGINX + Docker Compose, round robin, k6 load test, kill instance | ~30 menit | Level 1 selesai (image `go-api:v1` ada) |
| 3 | [03-guide-kubernetes-dasar.md](03-guide-kubernetes-dasar.md) | k3d cluster, Deployment/Pod/Service/Ingress, self-healing | ~40 menit | Level 1 selesai, k3d+kubectl terinstall |
| 4 | [04-guide-resource-limits.md](04-guide-resource-limits.md) | requests/limits, metrics-server, scheduling, CPU throttle, OOMKill | ~35 menit | Level 3 selesai |
| 5 | [05-guide-hpa.md](05-guide-hpa.md) | HorizontalPodAutoscaler, scale-up/scale-down, k6 ramp test | ~25 menit (+ ~10 menit nunggu scale-down) | Level 4 selesai |
| 6 | [06-guide-production-simulation.md](06-guide-production-simulation.md) | Postgres+Redis, cache-aside, Prometheus+Grafana, 4 eksperimen | ~50 menit | Level 5 selesai |

Total kalau dikerjakan berurutan dari nol: ± 3.5 jam (belum termasuk waktu
baca/pahami tiap output).

## Mulai dari mana kalau cuma mau coba 1 topik

Kamu gak wajib ngerjain semua dari Level 1 tiap kali. Yang WAJIB diulang
tiap sesi baru cuma **image `go-api:v1` harus ada** (`docker images | grep
go-api`) dan **cluster k3d harus hidup** kalau levelnya butuh Kubernetes
(`k3d cluster list`) — keduanya kena "reset" kalau kamu matiin Docker Desktop
atau restart laptop.

- **Cuma mau coba Docker dasar** → langsung Level 1, gak butuh apa-apa lagi.
- **Cuma mau coba load balancer manual** → Level 1 dulu (build image), lalu
  langsung Level 2. Gak perlu Kubernetes sama sekali.
- **Cuma mau coba Kubernetes dasar (Pod/Service/Ingress)** → Level 1 dulu
  (build image), lalu langsung Level 3. Level 2 gak wajib diulang (Level 3
  gak depend ke NGINX/Compose Level 2).
- **Cuma mau coba resource limits** → Level 1 (image) → Level 3 (cluster +
  Deployment dasar) → Level 4. Jangan lompat ke Level 4 tanpa Level 3, karena
  Level 4 langsung update Deployment yang sudah ada dari Level 3.
- **Cuma mau coba HPA** → Level 1 → Level 3 → Level 4 (minimal sampai
  `metrics-server` jalan dan `kubectl top pods` keluar angka) → Level 5.
  Ini paling sering disepelekan: HPA butuh metrics-server (dari Level 4) dan
  `requests.cpu` di Deployment (juga dari Level 4) — tanpa itu HPA `TARGETS`
  bakal `<unknown>` selamanya.
- **Cuma mau coba stack production penuh (Level 6)** → semua level di
  bawahnya harus pernah beres minimal sekali (cluster jalan, image ada,
  metrics-server jalan, HPA konsepnya dipahami) sebelum Level 6, karena
  Level 6 gabungin SEMUA sekaligus (Deployment+resources dari L4, HPA dari
  L5) plus nambah Postgres/Redis/Prometheus/Grafana baru.

## Konvensi command di semua guide

- Command diasumsikan dijalankan dari root folder repo
  (`learn-traffic/`), kecuali disebutkan `cd` ke folder tertentu.
- `kubectl` di environment ini kadang nyetak warning
  `couldn't get resource list for metrics.k8s.io/v1beta1 ...` di stderr
  sebelum metrics-server jalan — ini HARMLESS, abaikan, bukan tanda error.
- Semua command Kubernetes butuh cluster k3d nyala. Kalau kamu baru buka
  terminal baru dan gak yakin cluster masih hidup: `k3d cluster list`.
- Port `localhost:8080` dipakai berkali-kali di guide berbeda (Docker
  Compose Level 2, k3d Ingress Level 3-6) — **jangan jalankan 2 level
  sekaligus** yang sama-sama mau pakai port 8080, matikan salah satu dulu
  (`docker compose down` atau `k3d cluster delete go-api-lab`).
