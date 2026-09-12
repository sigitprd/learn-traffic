# Task Brief — Level 6: Simulasi Production Penuh (Capstone)

## Konteks
Ini level terakhir dari rencana awal home lab. Level 1-5 sudah membangun
fondasi lengkap: container basics, load balancing manual, Kubernetes dasar
(Deployment/Pod/Service/Ingress), resource requests/limits, dan HPA — semua
sudah dibuktikan lewat eksperimen konkret, bukan cuma teori.

Level 6 menggabungkan semuanya jadi satu stack yang menyerupai production
beneran: API yang punya **state** (nyambung ke database sungguhan, bukan data
statis di memory), plus **observability** (Prometheus + Grafana) buat lihat
apa yang terjadi tanpa harus polling `kubectl` terus-menerus.

Lalu dijalankan 4 eksperimen yang secara sengaja "menyerang" sistem dari sudut
berbeda — traffic bertahap, kematian Pod, crash aplikasi, dan traffic
brutal — buat lihat semua mekanisme dari Level 1-5 bekerja bersamaan.

---

## Objective
Bangun stack: k6 → Ingress → Service → 3 Pod (`go-api`) → Redis (cache) →
Postgres (data), tambahkan Prometheus + Grafana untuk observability, lalu
jalankan 4 eksperimen yang menguji sistem secara menyeluruh.

---

## Arsitektur target

```
                    k6
                     │
                     ▼
                  Ingress
                     │
                     ▼
                  Service
                     │
          ┌──────────┼──────────┐
          ▼          ▼          ▼
        Pod 1      Pod 2      Pod 3
          │          │          │
          └──────────┼──────────┘
                     │
                   Redis (cache)
                     │
                  Postgres (data)

Prometheus ──────────┐
                     ▼
                  Grafana

                     ▲
                     │
                    HPA
                     │
                     ▼
               Spawn / Remove Pod
```

---

## Deliverables

```
level-6/
├── manifests/
│   ├── postgres.yaml        (Deployment + PVC + Service + Secret)
│   ├── redis.yaml           (Deployment + Service)
│   ├── deployment.yaml      (go-api, updated: env var DB/Redis connection)
│   ├── service.yaml
│   ├── ingress.yaml
│   ├── hpa.yaml
│   ├── prometheus.yaml
│   └── grafana.yaml
├── migrations/
│   └── 001_init_users.sql
├── k6/
│   ├── experiment-a-gradual.js
│   └── experiment-d-brutal.js
└── README.md
```

---

## Task List

### Bagian 1 — Tambahkan state (Postgres + Redis)

- [ ] Deploy Postgres: Deployment + `PersistentVolumeClaim` (biar data tidak
      hilang kalau Pod restart) + Service (ClusterIP) + Secret untuk
      credential (jangan hardcode password di manifest biasa)
- [ ] Jalankan migration awal (`001_init_users.sql`): buat tabel `users`, isi
      dengan beberapa baris data (Alice, Bob, Charlie — konsisten dengan data
      dummy yang sudah dipakai sejak Level 1)
- [ ] Deploy Redis: Deployment + Service sederhana (tanpa persistence, murni
      cache layer)
- [ ] **Update Go API**: `GET /api/users` sekarang baca dari Postgres (bukan
      data statis di memory lagi). Tambahkan cache-aside pattern sederhana:
      cek Redis dulu, kalau miss baru query Postgres lalu simpan ke Redis
      dengan TTL pendek (misal 10 detik)
- [ ] Tambahkan endpoint baru `GET /api/users/cache-status` yang menunjukkan
      apakah response terakhir dari cache atau dari DB (buat verifikasi
      pola cache-aside benar-benar jalan)
- [ ] Rebuild image, `k3d image import`, redeploy
- [ ] Verifikasi: `curl` ke `/api/users` dua kali berturut-turut, hit pertama
      harus dari DB (cache miss), hit kedua dari cache (cache hit) — buktikan
      lewat `/cache-status` atau response time yang beda jauh

### Bagian 2 — Observability

- [ ] Deploy Prometheus (bisa pakai manifest sederhana community, tidak perlu
      Helm/Operator penuh untuk lab ini)
- [ ] Tambahkan endpoint `/metrics` di Go API (format Prometheus — request
      count, request duration histogram, minimal)
- [ ] Konfigurasi Prometheus buat scrape endpoint itu dari semua Pod `go-api`
- [ ] Deploy Grafana, sambungkan ke Prometheus sebagai data source
- [ ] Buat 1 dashboard sederhana: request rate, error rate, CPU usage per Pod,
      replica count dari waktu ke waktu (cukup functional, tidak perlu cantik)
- [ ] Screenshot atau ekspor dashboard JSON sebagai bukti (simpan di README
      atau lampiran)

### Bagian 3 — Experiment A: traffic bertahap, verifikasi HPA scaling realistis

- [ ] `experiment-a-gradual.js`: rate naik bertahap dalam beberapa fase (rate
      rendah → sedang → tinggi), target campuran `/api/users` (murah, DB+cache)
      dan `/cpu-intensive` (mahal, CPU)
- [ ] Amati di Grafana dashboard: replica count naik seiring rate naik, sambil
      catat juga cache hit ratio (apakah cache berhasil mengurangi beban ke
      Postgres saat traffic tinggi)
- [ ] Laporkan: pada fase traffic tinggi, apakah Postgres jadi bottleneck baru
      (connection pool exhausted, response time DB naik) meski Pod API
      sendiri sudah di-scale HPA — ini poin penting: **HPA scale API, TIDAK
      otomatis scale Postgres**, jadi database bisa jadi titik lemah baru
      kalau tidak diantisipasi

### Bagian 4 — Experiment B: kill salah satu Pod di tengah traffic (dengan state)

- [ ] Jalankan traffic sedang, `kubectl delete pod` salah satu Pod `go-api`
      (bukan Postgres/Redis)
- [ ] Verifikasi: request yang sedang mengarah ke Pod itu — apakah ada yang
      gagal (beda dengan Level 3 yang statless, sekarang ada koneksi ke
      Postgres/Redis yang mungkin sedang open saat Pod mati)
- [ ] Laporkan apakah ada connection leak di Postgres (`SELECT * FROM
      pg_stat_activity` — apakah ada koneksi "menggantung" dari Pod yang
      sudah mati) — kalau ada, ini insight penting soal pentingnya
      `SIGTERM` handling & connection pool timeout yang benar di aplikasi

### Bagian 5 — Experiment C: crash aplikasi (bukan sekadar delete Pod)

- [ ] Tambahkan endpoint sementara `GET /crash` yang sengaja `panic()` atau
      `os.Exit(1)` di Go API
- [ ] Hit endpoint itu ke satu Pod tertentu (lewat `port-forward`, seperti pola
      `/memory-hog` di Level 4 — jangan lewat Service)
- [ ] Amati `kubectl get pods`: status harus masuk `CrashLoopBackOff` kalau
      di-crash berkali-kali beruntun, atau `Running` lagi dengan Restart Count
      naik kalau cuma sekali
- [ ] Bedakan dengan jelas di README: apa bedanya skenario ini (proses
      benar-benar crash dari dalam) dengan Level 3 (`kubectl delete pod`,
      Kubernetes yang menghapus) dan Level 4 (`OOMKilled`, kernel yang bunuh)
      — tiga penyebab restart yang berbeda akar masalah

### Bagian 6 — Experiment D: traffic brutal, amati seluruh rantai bereaksi

- [ ] `experiment-d-brutal.js`: naikkan rate secara agresif dan cepat (jauh
      lebih curam dari Experiment A), campuran endpoint yang berat
- [ ] Amati di Grafana + `kubectl get hpa/pods --watch` bersamaan: seluruh
      rantai reaksi — CPU naik → HPA trigger → replica nambah → Service
      update endpoint pool → distribusi ulang → cache/DB ikut kena beban
- [ ] Laporkan apakah sistem tetap tidak error total (graceful degradation)
      atau ada titik jenuh yang jelas (dan di komponen mana: API, Postgres,
      atau limit `maxReplicas` HPA sendiri)

### Bagian 7 — Dokumentasi akhir
- [ ] README merangkum semua 4 eksperimen, dengan 1 bagian retrospektif:
      dari Level 1 sampai 6, mekanisme apa saja yang saling terhubung untuk
      membuat sistem ini resilient (container isolation → load balancing →
      self-healing → resource enforcement → autoscaling → state management)

---

## Acceptance Criteria

- [ ] Postgres & Redis jalan, `/api/users` benar-benar baca dari DB dengan
      cache-aside pattern terbukti (cache hit vs miss teramati)
- [ ] Prometheus + Grafana jalan, dashboard menunjukkan metrics real dari
      eksperimen yang dijalankan
- [ ] Experiment A: HPA scaling teramati di traffic campuran realistis, dan
      ada observasi soal Postgres sebagai potential bottleneck baru
- [ ] Experiment B: kill Pod dengan state, dicek soal connection leak di DB
- [ ] Experiment C: crash aplikasi dari dalam, dibedakan dari delete Pod
      (Level 3) dan OOMKill (Level 4)
- [ ] Experiment D: traffic brutal, seluruh rantai (HPA + LB + cache/DB)
      diamati bereaksi bersamaan, titik jenuh (kalau ada) diidentifikasi
- [ ] README retrospektif menghubungkan semua level 1-6 jadi satu narasi utuh

## Constraints
- Postgres pakai PVC supaya data survive Pod restart — tapi ini lab lokal,
  tidak perlu replication/backup strategy sungguhan
- Endpoint `/crash` dan `/memory-hog` boleh permanen di code untuk eksperimen
  ulang, tapi selalu diakses lewat `port-forward` ke Pod spesifik, tidak
  pernah lewat Service
- Grafana/Prometheus di lab ini cukup functional — tidak perlu alerting,
  retention policy panjang, atau HA setup

## Ini level terakhir dari rencana awal
Setelah level ini, homelab-nya sudah mendemonstrasikan siklus penuh dari
container tunggal sampai sistem stateful yang self-healing dan auto-scaling
dengan observability. Kalau nanti mau lanjut, itu sudah masuk topik lanjutan
di luar rencana awal (misal: GitOps/ArgoCD, service mesh, multi-cluster) —
bukan bagian dari lab ini.