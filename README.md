# Learn Traffic — Kubernetes Home Lab

Home lab step-by-step buat belajar operasional sistem produksi secara
utuh: traffic/scaling, resource management, resilience, observability, dan
state management — bukan cuma `docker run` doang. Semua lab jalan 100%
lokal, gak butuh akun cloud.

Kenapa disusun begini dan apa sebenarnya yang mau dipelajari? Baca
[docs/tujuan-belajar.md](docs/tujuan-belajar.md).

## Mulai dari sini

Panduan lengkap step-by-step ada di [docs/00-index.md](docs/00-index.md).

## Daftar level

| # | Topik | Folder |
|---|---|---|
| 1 | Docker dasar: image, container, port, env, volume, network, resource limit | [level1-container/](level1-container/) |
| 2 | Load balancer manual: NGINX + Docker Compose, k6 load test | [level-2/](level-2/) |
| 3 | Kubernetes dasar: Deployment/Pod/Service/Ingress, self-healing | [level-3/](level-3/) |
| 4 | Resource requests/limits, metrics-server, CPU throttle, OOMKill | [level-4/](level-4/) |
| 5 | Horizontal Pod Autoscaler (HPA) | [level-5/](level-5/) |
| 6 | Production simulation: Postgres+Redis, observability, chaos experiment | [level-6/](level-6/) |

Tiap folder `level-*/README.md` isinya laporan hasil eksperimen yang
sudah dijalankan — beda dari `docs/0X-guide-*.md` yang panduan buat kamu
ikuti sendiri dari nol.

## Prasyarat

- Docker terinstall dan jalan
- `k3d`, `kubectl`, `k6` (Mac: `brew install k3d kubectl k6`)

Total waktu kalau dikerjakan berurutan dari nol: ± 3.5 jam. Cara mulai
dari topik tertentu tanpa ngerjain semua dari awal ada di
[docs/00-index.md](docs/00-index.md#mulai-dari-mana-kalau-cuma-mau-coba-1-topik).
