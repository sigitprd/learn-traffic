# Level 6 — Simulasi Production Penuh (Capstone)

Gabungan semua Level 1-5 jadi satu stack: k6 → Ingress → Service → 3+ Pod
`go-api` → Redis (cache) → Postgres (data), + Prometheus/Grafana buat
observability, + HPA yang masih aktif dari Level 5.

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
        Pod 1      Pod 2      Pod 3 (... sampai 10, dikontrol HPA)
          │          │          │
          └──────────┼──────────┘
                     │
                   Redis (cache, TTL 10s)
                     │
                  Postgres (data, PVC)

Prometheus ── scrape go-api /metrics, cAdvisor, kube-state-metrics ──► Grafana
```

## Menjalankan dari nol

```bash
k3d cluster create -c ../level-3/cluster/k3d-config.yaml
k3d image import go-api:v1 -c go-api-lab

kubectl apply -f manifests/postgres.yaml
kubectl apply -f manifests/redis.yaml
kubectl rollout status deployment/postgres
kubectl cp migrations/001_init_users.sql <postgres-pod>:/tmp/001_init_users.sql
kubectl exec <postgres-pod> -- psql -U goapi -d goapi -f /tmp/001_init_users.sql

kubectl apply -f manifests/deployment.yaml
kubectl apply -f manifests/service.yaml
kubectl apply -f manifests/ingress.yaml
kubectl apply -f manifests/hpa.yaml

kubectl apply -f manifests/kube-state-metrics.yaml
kubectl apply -f manifests/prometheus.yaml
kubectl apply -f manifests/grafana.yaml
```

---

## Bagian 1 — State (Postgres + Redis)

Go API di-update (`db.go` baru): `GET /api/users` sekarang cache-aside ke
Redis (TTL 10 detik) dengan fallback ke Postgres kalau miss, dan fallback
lagi ke data statis in-memory kalau `POSTGRES_HOST` gak di-set sama sekali
(biar image ini tetap kompatibel jalan standalone seperti Level 1-5).
Endpoint baru `GET /api/users/cache-status` nunjukin status cache
TERAKHIR dari instance yang diakses.

```
[Migration]
Command: kubectl cp migrations/001_init_users.sql <pod>:/tmp/...
         kubectl exec <pod> -- psql -U goapi -d goapi -f /tmp/001_init_users.sql
Output: CREATE TABLE / INSERT 0 3 / setval 3
Kesimpulan: tabel users terisi Alice/Bob/Charlie, konsisten sejak Level 1.
```

```
[Cache-aside terbukti - port-forward ke 1 Pod spesifik biar konsisten]
Command: kubectl port-forward pod/<pod> 18081:8080
         curl -w "time_total: %{time_total}s" localhost:18081/api/users   (x2)
         curl localhost:18081/api/users/cache-status                      (tiap sesudahnya)
Output:
  hit 1: time_total=0.023s, cache-status: "miss"
  hit 2: time_total=0.012s, cache-status: "hit"
Kesimpulan: cache-aside JALAN - hit pertama miss (query ke Postgres),
hit kedua hit (dari Redis, ~2x lebih cepat, dan gak nyentuh Postgres sama
sekali). Diverifikasi lewat endpoint /cache-status DAN response time.
```

Catatan penting: karena Redis adalah proses EKSTERNAL (bukan in-memory per
Pod), cache-nya SURVIVE lintas restart/rollout Pod go-api - beda dari
`/stats` counter Level 2 yang reset tiap Pod baru. Ini konsekuensi nyata
dari "state di luar Pod" yang jadi tema Level 6.

## Bagian 2 — Observability

Endpoint `/metrics` (format Prometheus, lewat `client_golang`) expose:
- `go_api_http_requests_total{path,status}` - counter
- `go_api_http_request_duration_seconds{path}` - histogram
- `go_api_cache_status_total{status}` - counter hit/miss (ditambahkan
  setelah Experiment A, lihat catatan di bagian Experiment A)

Prometheus scrape 3 job: `go-api-pods` (lewat `kubernetes_sd_configs` role
`pod`, filter annotation `prometheus.io/scrape: "true"` - otomatis ikut
Pod baru yang di-spawn HPA, gak perlu daftar IP manual), `cadvisor`
(CPU usage per Pod, lewat kubelet proxy API), dan `kube-state-metrics`
(replica count Deployment).

```
[Verifikasi semua scrape target sehat]
Command: kubectl port-forward svc/prometheus 19090:9090
         curl localhost:19090/api/v1/targets
Output: cadvisor (3x, 1 per node) = up, go-api-pods (3-10x tergantung
        replica saat itu, + 1 Traefik yang ikut ke-scrape karena
        annotation default-nya) = up, kube-state-metrics = up
Kesimpulan: semua target discovery otomatis dan sehat.
```

```
[Metrics real, bukan cuma config kosong]
Command: curl 'localhost:19090/api/v1/query?query=kube_deployment_status_replicas{deployment="go-api"}'
Output: value = 3   (sesuai jumlah Pod saat itu)
Kesimpulan: pipeline lengkap Deployment -> kube-state-metrics -> Prometheus
-> (siap dibaca Grafana) beneran ngalir data real.
```

Grafana disambungkan ke Prometheus sebagai datasource (provisioning
otomatis lewat ConfigMap, gak perlu klik manual), 1 dashboard
("go-api - Level 6 lab") dengan 4 panel: **Request rate**, **Error rate**,
**CPU usage per Pod**, **Replica count**. Dashboard di-export sebagai JSON
ke [`grafana-dashboard-export.json`](grafana-dashboard-export.json) di
folder ini (dipakai sebagai bukti sesuai constraint - tidak ada GUI buat
screenshot di environment ini).

```
[Dashboard ke-provision otomatis]
Command: curl localhost:13000/api/search   (setelah kubectl port-forward svc/grafana)
Output: [{"title":"go-api - Level 6 lab", "url":"/d/afxqwz62awnb4f/..."}]
Kesimpulan: dashboard muncul otomatis tanpa setup manual, siap dipakai
buat observasi Experiment A-D.
```

## Bagian 3 — Experiment A: traffic bertahap

`experiment-a-gradual.js`: rate naik 4 fase (rendah -> sedang -> tinggi ->
sustain, total 5 menit), 80% `/api/users` (murah) + 20% `/cpu-intensive`
(mahal) - representasi traffic campuran realistis.

```
Timeline (kubectl get hpa + query pg_stat_activity, tiap 20 detik):
  t=20s : TARGETS=2%    REPLICAS=3   postgres_conn=4
  t=80s : TARGETS=71%   REPLICAS=3   postgres_conn=4    (mulai lewat 70%)
  t=140s: TARGETS=79%   REPLICAS=3   postgres_conn=3
  t=160s: TARGETS=134%  REPLICAS=4   postgres_conn=6    ← scale-up pertama
  t=180s: TARGETS=115%  REPLICAS=6   postgres_conn=6
  t=220s: TARGETS=106%  REPLICAS=7   postgres_conn=11
  t=240s: TARGETS=86%   REPLICAS=10  postgres_conn=11   ← mentok maxReplicas
  t=320s: TARGETS=67%   REPLICAS=10  postgres_conn=11

Kesimpulan HPA: replica naik bertahap 3->4->6->7->10 mengikuti kenaikan
rate k6 tiap fase - persis pola yang sudah dibuktikan di Level 5, sekarang
dengan traffic campuran (bukan cuma /cpu-intensive murni) dan tetap
konsisten.
```

```
[Cache hit ratio - apakah cache mengurangi beban ke Postgres?]
Command (setelah instrumentasi go_api_cache_status_total ditambahkan -
         lihat catatan di bawah): 40 request burst ke /api/users lewat
         Ingress, lalu query keseluruhan histori metrics:
         sum(go_api_cache_status_total) by (status)
Output (akumulasi sepanjang semua eksperimen A+B+D, bukan cuma burst):
  hit: 6226
  miss: 69
  -> cache hit ratio = 6226 / (6226+69) = 98.9%
Kesimpulan: cache-aside SANGAT efektif mengurangi beban baca ke Postgres -
dari ribuan request /api/users, cuma 69 yang beneran nyampe query SQL.
Ini kenapa Postgres connection count di eksperimen A cuma naik ke 11
(bukan puluhan/ratusan) walau replica API sudah di-scale ke 10 - TANPA
cache, request rate segitu bisa dengan mudah bikin connection pool
Postgres (default max_connections=100, dan tiap Pod di lab ini
SetMaxOpenConns(10) -> 10 Pod x 10 = 100 potensi koneksi) mepet limit.

CATATAN JUJUR: metric go_api_cache_status_total ditambahkan SETELAH
Experiment A pertama kali jalan (baru kepikiran perlu counter eksplisit
buat ngukur ratio secara numerik, bukan cuma observasi 2x curl manual di
Bagian 1) - jadi rebuild+reimport+rollout restart dilakukan DI TENGAH
rangkaian eksperimen. Cache-aside pattern-nya sendiri sudah terbukti
sejak Bagian 1 (sebelum ada counter ini); counter cuma nambahin cara
ngukur secara agregat, gak mengubah behavior yang sudah diverifikasi.
```

```
[Apakah Postgres jadi bottleneck baru?]
Observasi: postgres_active_connections naik dari 4 -> 11 seiring replica
API naik dari 3 -> 10 (HPA scale API, TIDAK otomatis scale Postgres -
Postgres tetap `replicas: 1` sepanjang eksperimen). TAPI 11 koneksi masih
JAUH di bawah default max_connections Postgres (100), dan response time
DB gak keliatan naik drastis (request tetap 0% gagal, latency stabil).

Kesimpulan: DI LAB INI, Postgres BELUM jadi bottleneck nyata - berkat
cache-aside yang secara efektif menyerap >98% traffic baca sebelum nyampe
DB. TAPI ini poin penting yang harus digarisbawahi: HPA cuma scale
Deployment go-api (`replicas: 1` Postgres TETAP SAMA sepanjang semua
eksperimen, gak ada mekanisme apapun yang otomatis nambah kapasitas
Postgres). Kalau cache TIDAK ada (atau TTL-nya jauh lebih panjang dari
request rate sehingga selalu stale/tidak efektif, atau endpoint yang
di-scale itu WRITE-heavy yang gak bisa di-cache), 10 Pod x 10 koneksi =
100 bisa PERSIS mentok limit default Postgres - momen itulah Postgres
akan jadi TITIK LEMAH BARU yang HPA sendiri gak bisa atasi, karena HPA
cuma "tau" cara nambah Pod API, bukan nambah kapasitas database.
```

## Bagian 4 — Experiment B: kill Pod dengan state

```
[Traffic sedang + kubectl delete pod di tengah jalan]
Command: loop curl /api/users tiap 0.3s (60x, ~18 detik total) lewat
         Ingress, kubectl delete pod <salah-satu-go-api> di detik ke-3
Output: 59x HTTP 200, 1x HTTP 502
Kesimpulan: BEDA DARI LEVEL 3 (stateless, 0% gagal sepenuhnya) - di sini
ADA 1 request yang gagal (502 dari Traefik, bukan error dari aplikasi
sendiri). Kemungkinan penyebab: request itu sedang "in-flight" ke Pod
yang baru saja di-delete tepat di window singkat antara Pod berhenti
menerima koneksi baru TAPI Service/Endpoint belum sempat update daftar
endpoint-nya (race condition kecil antara kubelet men-terminate container
vs endpoint controller meng-update Service) - beda dari Level 3 yang
kebetulan gak pernah kena race ini di percobaannya.
```

```
[Connection leak check - pg_stat_activity vs Pod yang hidup]
Command: kubectl exec <postgres-pod> -- psql -U goapi -d goapi -c \
         "SELECT pid, client_addr, state FROM pg_stat_activity WHERE datname='goapi';"
         kubectl get pods -l app=go-api -o custom-columns=NAME:.metadata.name,IP:.status.podIP
Output: 10 baris pg_stat_activity, SEMUA client_addr-nya PERSIS cocok
        dengan 10 IP Pod go-api yang MASIH HIDUP sekarang - TIDAK ADA
        baris dengan IP dari Pod yang sudah di-delete.
Kesimpulan: TIDAK ADA connection leak di eksperimen ini. Koneksi dari Pod
yang mati BERSIH otomatis - begitu proses Go di dalam Pod berhenti
(baik dari SIGTERM kubelet atau socket ditutup paksa), TCP socket ke
Postgres ikut tertutup di level OS, dan Postgres mendeteksi koneksi
terputus lalu membersihkan backend process-nya sendiri. Ini kerja bagus
dari `database/sql` Go (connection pool yang dikelola per-proses, bukan
connection yang "dibagi" ke proses lain) DAN dari default behavior
kubelet/Postgres - TAPI catatan penting: ini TIDAK berarti SIGTERM
handling di aplikasi ini eksplisit/graceful (kode saat ini TIDAK punya
signal handler khusus) - yang terjadi adalah OS-level cleanup yang
kebetulan cukup buat mencegah leak DI SKALA LAB INI. Di sistem production
dengan volume Pod churn jauh lebih tinggi, koneksi yang ditutup PAKSA
(bukan graceful close) tetap punya risiko menyisakan koneksi "zombie" di
sisi Postgres untuk sesaat sampai TCP keepalive/timeout server
mendeteksinya - practice yang lebih benar adalah aplikasi menangani
SIGTERM secara eksplisit (`signal.NotifyContext` + `db.Close()`) sebelum
proses benar-benar exit.
```

## Bagian 5 — Experiment C: crash aplikasi

Endpoint baru `GET /crash` (`os.Exit(1)`, bukan `panic()` yang bisa
ke-recover HTTP server-nya) - dipanggil LANGSUNG ke 1 Pod lewat
`port-forward`, pola yang sama seperti `/memory-hog` di Level 4.

```
[Crash sekali]
Command: kubectl port-forward pod/<pod> 18082:8080
         curl --max-time 5 localhost:18082/crash
Output: curl exit 52 (koneksi putus - proses exit sebelum sempat respond)
        kubectl get pod <pod>: RESTARTS 0 -> 1, STATUS balik Running
        kubectl describe pod: Last State Terminated, Reason: Error, Exit Code: 1
Kesimpulan: proses Go exit(1), kubelet restart CONTAINER-nya (bukan bikin
Pod baru - nama Pod SAMA, cuma Restart Count naik) karena
restartPolicy default Always.
```

```
[Crash berkali-kali beruntun -> CrashLoopBackOff]
Command: crash 5x berturut-turut (tiap kali tunggu Pod Running dulu,
         jeda ~5-8 detik antar percobaan)
Output: kubectl get pod -> STATUS: CrashLoopBackOff, RESTARTS: 3
Kesimpulan: kubelet mendeteksi pola restart berulang dalam window waktu
pendek, masuk exponential backoff (mulai 10 detik, dobel tiap gagal lagi
sampai maks 5 menit) sebelum coba start ulang - ini MENCEGAH restart-loop
tak terkendali yang bisa nge-hammer resource node/API server.
```

### Perbandingan 3 penyebab restart - akar masalah beda-beda

| Skenario | Siapa yang "membunuh" | Trigger | Nama Pod | Reason di describe |
|---|---|---|---|---|
| Level 3: `kubectl delete pod` | Kubernetes API (operator manual/eksternal) | Perintah eksplisit dari luar | BERUBAH (Pod lama dihapus total, ReplicaSet bikin Pod BARU) | (Pod lama hilang dari `get pods`, gak ada "Last State" buat dilihat - Pod-nya sendiri sudah tidak ada) |
| Level 4: OOMKilled | Kernel Linux (cgroup OOM-killer) | Memory usage > `limits.memory` | SAMA (container di dalam Pod yang sama di-restart) | `Reason: OOMKilled`, `Exit Code: 137` (SIGKILL) |
| Level 6: `/crash` | Aplikasi itu sendiri (`os.Exit(1)`) | Bug/kondisi internal yang sengaja/tidak sengaja bikin proses berhenti | SAMA (container di dalam Pod yang sama di-restart) | `Reason: Error`, `Exit Code: 1` (exit code yang di-set aplikasi) |

Insight: Level 4 dan Level 6 SAMA-SAMA "container di-restart di Pod yang
sama" (beda dari Level 3 yang bikin Pod baru sama sekali) - tapi
AKARNYA beda total: OOMKilled adalah kernel yang MEMAKSA proses berhenti
dari LUAR (proses gak sempat "milih"), sedangkan `/crash` adalah proses
yang MEMILIH berhenti sendiri (`os.Exit`) - exit code-nya jadi penanda
paling jelas: 137 (128+SIGKILL 9) vs exit code custom aplikasi. Restart
count naik di KEDUANYA, tapi cara debug-nya beda: OOMKilled -> cek
`resources.limits.memory` dan usage aktual; Exit Code 1 dari aplikasi ->
cek log aplikasi buat tau kenapa dia sengaja/gak sengaja exit.

## Bagian 6 — Experiment D: traffic brutal

`experiment-d-brutal.js`: rate naik curam (5 -> 100 req/s dalam 20 detik
pertama, jauh lebih agresif dari Experiment A), 50/50 `/api/users` vs
`/cpu-intensive`, sustain di puncak 100 req/s selama 2 menit.

```
Timeline:
  t=16s : TARGETS=2%     REPLICAS=8    postgres_conn=9
  t=48s : TARGETS=227%   REPLICAS=8    postgres_conn=8   (mulai jauh di atas target)
  t=64s : TARGETS=378%   REPLICAS=10   postgres_conn=8   ← mentok maxReplicas
  t=96s : TARGETS=363%   REPLICAS=10   postgres_conn=13
  t=144s: TARGETS=354%   REPLICAS=10   postgres_conn=18
  t=176s: TARGETS=341%   REPLICAS=10   postgres_conn=19  (masih 341% - 10 Pod TETAP kewalahan)

Hasil k6 (2m50s, 12499 request):
  http_req_failed: 0.00% (0 dari 12499)   ← ZERO error dari sisi HTTP client
  http_req_duration: avg=378ms, p95=747ms, max=2s

Query Prometheus (akumulasi total, semua endpoint, status 200 semua):
  /api/users: 6295 request, semua status 200
  /cpu-intensive: 6275 request, semua status 200
```

**Rantai reaksi penuh yang teramati** (CPU naik -> HPA trigger -> replica
nambah -> Service update endpoint pool -> distribusi ulang -> cache/DB
ikut kena beban):
1. Rate k6 naik curam -> CPU tiap Pod naik cepat (t=48s sudah 227%)
2. HPA mendeteksi lewat threshold, scale up AGRESIF (langsung ke 10/max
   dalam 1-2 siklus, bukan bertahap pelan seperti Experiment A yang
   ramp-nya lebih landai)
3. Pod baru otomatis masuk endpoint list Service (dibuktikan sejak
   Level 3 - Service auto-discover Pod baru lewat label selector,
   gak perlu konfigurasi ulang manual)
4. Traffic terdistribusi ulang ke Pod baru (mengurangi beban per-Pod,
   tapi TETAP gak cukup - TARGETS tetap >300% bahkan di 10 Pod)
5. Postgres connection count naik seiring cache makin sering "kalah cepat"
   vs request rate (miss ratio naik saat traffic makin brutal, walau
   overall ratio sepanjang eksperimen tetap didominasi hit karena TTL
   10 detik masih relevan buat sebagian besar window)

**Titik jenuh sistem**: TERIDENTIFIKASI JELAS - bukan Postgres (connection
count 19, jauh dari limit), bukan API gagal total (0% error HTTP), tapi
**`maxReplicas: 10` HPA sendiri**. Sistem menunjukkan **graceful
degradation**, bukan collapse: response time melambat (p95 747ms vs
baseline ~500ms endpoint /cpu-intensive sendirian), TAPI tidak ada
request yang di-drop/gagal - 10 Pod yang ada menyerap SEMUA request,
cuma lebih lambat karena masing-masing Pod sudah dekat/di limit CPU-nya
(500m). Ini persis desain yang diharapkan: HPA membatasi blast radius
resource (gak akan spawn Pod tak terbatas), dan sistem di baliknya (Go
HTTP server dengan goroutine per-request) tetap sanggup ANTRE request
daripada langsung nolak - trade-off latency naik vs availability tetap
terjaga.
```

---

## Bagian 7 — Retrospektif: Level 1 sampai 6

```
Level 1 (Container)     -> isolasi proses, image kecil, port mapping,
                            resource limit dasar (--cpus/--memory)
Level 2 (Load Balancer) -> distribusi traffic manual (NGINX round robin),
                            retry otomatis saat 1 backend mati (TAPI TIDAK
                            self-healing - backend mati ya tetap mati)
Level 3 (Kubernetes)    -> Pod sebagai abstraksi di atas container,
                            Service = load balancer + DNS internal,
                            Ingress = pintu masuk dari luar,
                            SELF-HEALING nyata (ReplicaSet bikin Pod baru
                            otomatis - beda fundamental dari Level 2)
Level 4 (Resource)      -> requests (buat scheduler) vs limits (buat
                            enforcement runtime) - CPU limit = throttle
                            (tetap hidup), memory limit = OOMKilled (mati)
Level 5 (HPA)           -> autoscaling berbasis resource utilization
                            (bukan request count), scale-up agresif vs
                            scale-down konservatif (stabilization window)
Level 6 (Production)    -> state (Postgres+Redis) yang TIDAK ikut
                            di-scale HPA (potential bottleneck baru),
                            observability (Prometheus+Grafana) buat lihat
                            semua mekanisme di atas TANPA polling manual,
                            dan 3 cara restart yang beda akar (delete Pod,
                            OOMKilled, app crash) yang semuanya HARUS
                            dibedakan pas debugging production beneran
```

**Mekanisme yang saling terhubung buat bikin sistem ini resilient**:

1. **Container isolation** (L1) adalah fondasi paling dasar - tanpa ini,
   gak ada unit yang bisa di-replikasi/di-scale sama sekali.
2. **Load balancing** (L2 manual, lalu L3 Service) mendistribusikan
   traffic ke banyak instance identik - prasyarat SEBELUM autoscaling
   ada gunanya (nambah Pod percuma kalau traffic gak didistribusikan).
3. **Self-healing** (L3 ReplicaSet) menjamin JUMLAH instance yang
   diinginkan selalu tercapai, terlepas dari kegagalan individual
   (Pod dihapus, crash, atau mati sendiri - L6 Experiment B & C
   membuktikan mekanisme yang SAMA ini menangani skenario kegagalan
   yang BEDA-BEDA akar masalahnya).
4. **Resource enforcement** (L4) memberi BATAS yang jelas per-instance -
   tanpa ini, 1 Pod yang "nakal" (CPU/memory besar) bisa mengganggu Pod
   lain di node yang sama (noisy neighbor).
5. **Autoscaling** (L5) menyesuaikan JUMLAH instance secara dinamis
   berdasarkan beban NYATA (bukan asumsi statis) - baru masuk akal
   setelah #2, #3, #4 ada semua (load balancer buat distribusi, self-
   healing buat Pod baru gak "hilang" gitu aja, resource limit buat HPA
   punya basis hitung yang jelas).
6. **State management** (L6) adalah lapisan yang PALING GAK otomatis
   ikut ter-cover oleh #1-5 - Postgres TETAP `replicas: 1` sepanjang
   semua eksperimen HPA di level ini, karena database stateful gak bisa
   di-scale horizontal semudah API stateless. Cache-aside (Redis) jadi
   mitigasi PRAKTIS (bukan solusi permanen) buat menunda titik di mana
   state jadi bottleneck.
7. **Observability** (L6) adalah lapisan yang GAK mengubah behavior
   sistem sama sekali, tapi WAJIB buat bisa MELIHAT #1-6 bekerja
   bersamaan secara real-time - tanpa Prometheus/Grafana, eksperimen di
   level ini akan balik ke cara Level 1-5 (polling `kubectl` manual
   berkali-kali), yang gak scalable buat sistem production beneran.

**Kesimpulan akhir**: resilience bukan dari 1 mekanisme tunggal, tapi dari
LAPISAN-LAPISAN yang saling menutupi kelemahan lapisan lain -
load balancer gak akan berguna tanpa lebih dari 1 instance sehat (self-
healing), self-healing gak akan efisien tanpa resource limit yang jelas,
autoscaling gak akan aman tanpa resource limit sebagai basis hitung, dan
SEMUA mekanisme compute ini punya batas: begitu state (database) masuk ke
gambar, resilience compute-layer TIDAK otomatis menular ke data-layer -
itu harus didesain terpisah (cache, connection pool, replication - di
luar scope lab ini).

## Cleanup

```bash
k3d cluster delete go-api-lab
```

## File tambahan

- [`grafana-dashboard-export.json`](grafana-dashboard-export.json) - export
  dashboard Grafana (4 panel: request rate, error rate, CPU per Pod,
  replica count), bukti dashboard ke-provision dan terhubung ke data real.
