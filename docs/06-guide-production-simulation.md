# Guide 06 — Production Simulation (Capstone)

Gabungan semua level: Postgres + Redis (state), cache-aside pattern,
Prometheus + Grafana (observability), lalu 4 eksperimen yang "menyerang"
sistem dari sudut berbeda.

**Waktu**: ~50 menit
**Prasyarat**:
- Level 5 selesai (paham HPA dasar) — level ini pakai `requests/limits`
  dari Level 4 dan `hpa.yaml` dari Level 5, tapi manifest-nya sudah
  disalin lengkap ke `level-6/manifests/`, jadi gak perlu copy manual
- Image `go-api:v1` harus versi TERBARU (dengan endpoint `/crash`,
  `/memory-hog`, `/metrics`, `db.go` buat Postgres/Redis) — kalau kamu
  ngikutin guide ini berurutan dari Level 4, PERLU rebuild image dulu
  (lihat Langkah 0)

Konsep detail ada di [`level-6/README.md`](../level-6/README.md).

---

### Langkah 0 — Cluster + image

```bash
cd level1-container
docker build -t go-api:v1 .
cd ../level-3
k3d cluster create -c cluster/k3d-config.yaml
k3d image import go-api:v1 -c go-api-lab
cd ../level-6
```

### Langkah 1 — Deploy Postgres + Redis, jalankan migration

```bash
kubectl apply -f manifests/postgres.yaml
kubectl apply -f manifests/redis.yaml
kubectl rollout status deployment/postgres --timeout=120s
kubectl rollout status deployment/redis --timeout=60s
```

**Expected output** (Postgres bisa makan waktu ~2 menit di run pertama,
lagi pull image `postgres:16-alpine`):
```
deployment "postgres" successfully rolled out
deployment "redis" successfully rolled out
```

Jalankan migration:
```bash
PG_POD=$(kubectl get pods -l app=postgres -o jsonpath='{.items[0].metadata.name}')
kubectl cp migrations/001_init_users.sql $PG_POD:/tmp/001_init_users.sql
kubectl exec $PG_POD -- psql -U goapi -d goapi -f /tmp/001_init_users.sql
```

**Expected output:**
```
CREATE TABLE
INSERT 0 3
 setval
--------
      3
(1 row)
```

### Langkah 2 — Deploy go-api + Service/Ingress/HPA

```bash
kubectl apply -f manifests/deployment.yaml
kubectl apply -f manifests/service.yaml
kubectl apply -f manifests/ingress.yaml
kubectl apply -f manifests/hpa.yaml
kubectl rollout status deployment/go-api --timeout=90s
```

**Expected output:**
```
deployment "go-api" successfully rolled out
```

### Langkah 3 — Buktikan cache-aside

**WAJIB port-forward ke 1 Pod spesifik** — kalau lewat Service/Ingress,
request bisa nyasar ke Pod lain yang cache-nya beda state.

```bash
POD=$(kubectl get pods -l app=go-api -o jsonpath='{.items[0].metadata.name}')
kubectl port-forward pod/$POD 18081:8080 &
sleep 2
curl -s -w "\ntime_total: %{time_total}s\n" localhost:18081/api/users
curl -s localhost:18081/api/users/cache-status
echo
curl -s -w "\ntime_total: %{time_total}s\n" localhost:18081/api/users
curl -s localhost:18081/api/users/cache-status
kill %1
```

**Expected output** (hit 1 = miss dari Postgres, hit 2 = hit dari Redis,
LEBIH CEPAT):
```
[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"},{"id":3,"name":"Charlie"}]
time_total: 0.023s
{"hostname":"...","last_cache_status":"miss"}

[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"},{"id":3,"name":"Charlie"}]
time_total: 0.012s
{"hostname":"...","last_cache_status":"hit"}
```

### Langkah 4 — Deploy observability

```bash
kubectl apply -f manifests/kube-state-metrics.yaml
kubectl apply -f manifests/prometheus.yaml
kubectl apply -f manifests/grafana.yaml
kubectl rollout status deployment/prometheus --timeout=90s
kubectl rollout status deployment/grafana --timeout=90s
```

Verifikasi scrape target sehat (tunggu ~15 detik dulu setelah rollout
selesai):
```bash
kubectl port-forward svc/prometheus 19090:9090 &
sleep 2
curl -s localhost:19090/api/v1/targets | python3 -c "
import json,sys
d=json.load(sys.stdin)
for t in d['data']['activeTargets']:
    print(t['scrapePool'], t['health'])
"
```

**Expected output** (semua `up`):
```
cadvisor up
cadvisor up
cadvisor up
go-api-pods up
go-api-pods up
go-api-pods up
kube-state-metrics up
```

Buka Grafana buat lihat dashboard (opsional, browser):
```bash
kubectl port-forward svc/grafana 13000:3000 &
```
Buka `http://localhost:13000` di browser — dashboard "go-api - Level 6
lab" harusnya sudah ada otomatis (provisioning lewat ConfigMap, gak perlu
setup manual). Kalau gak mau buka browser, cek lewat API:
```bash
curl -s localhost:13000/api/search
```
**Expected output:**
```
[{"title":"go-api - Level 6 lab","url":"/d/.../go-api-level-6-lab",...}]
```

### Langkah 5 — Experiment A: traffic bertahap

Port-forward Prometheus/Grafana dari Langkah 4 gak perlu di-stop — beda
port, gak bentrok. Enaknya buka TERMINAL BARU biar log k6/HPA di bawah gak
campur sama spam `Handling connection for ...` dari port-forward. Terminal
baru mulai dari home directory, jadi `cd` dulu ke folder level ini sebelum
lanjut (command di bawah pakai path relatif seperti `k6/...`):

```bash
cd level-6
```

5 menit, campuran 80% `/api/users` + 20% `/cpu-intensive`.

```bash
k6 run k6/experiment-a-gradual.js &
K6PID=$!
PG_POD=$(kubectl get pods -l app=postgres -o jsonpath='{.items[0].metadata.name}')
for i in $(seq 1 16); do
  sleep 20
  kubectl get hpa go-api-hpa
  kubectl exec $PG_POD -- psql -U goapi -d goapi -tAc "SELECT count(*) FROM pg_stat_activity WHERE datname='goapi';"
done
wait $K6PID
```

**Expected timeline** (replica naik bertahap ngikutin beban, koneksi
Postgres ikut naik tapi TETAP JAUH di bawah limit default 100):
```
t=20s : TARGETS=2%    REPLICAS=3   postgres_conn=4
t=160s: TARGETS=134%  REPLICAS=4   postgres_conn=6
t=240s: TARGETS=86%   REPLICAS=10  postgres_conn=11   ← mentok maxReplicas
```

Poin penting buat dicatat: HPA cuma nge-scale Deployment `go-api`,
**Postgres TETAP `replicas: 1`** sepanjang eksperimen — kalau cache gak
efektif, ini yang bisa jadi bottleneck baru (lihat README asli buat
analisis lengkap kenapa di lab ini belum kejadian: cache hit ratio ~99%).

### Langkah 6 — Experiment B: kill Pod dengan state

Command ini multi-baris dengan subshell `(...)` — kalau di-paste langsung
ke terminal (terutama macOS Terminal.app/iTerm dengan bracketed-paste),
sering kepotong/rusak dan bikin nyangkut gak selesai-selesai. Cara aman:
tulis ke file dulu, baru dieksekusi sebagai script:

```bash
cat <<'SCRIPT' > /tmp/expB.sh
#!/bin/bash
> /tmp/expB.log
(for i in $(seq 1 60); do
  curl -s -o /dev/null -w "%{http_code}\n" --max-time 2 localhost:8080/api/users >> /tmp/expB.log
  sleep 0.3
done) &
TRAFFIC_PID=$!
sleep 3
TARGET=$(kubectl get pods -l app=go-api -o jsonpath='{.items[0].metadata.name}')
kubectl delete pod $TARGET --wait=false
wait $TRAFFIC_PID
sort /tmp/expB.log | uniq -c
SCRIPT
chmod +x /tmp/expB.sh
/tmp/expB.sh
```

**Expected output** (BEDA dari Level 3 — mungkin ada 1-2 request gagal,
bukan cuma 200 semua):
```
  59 200
   1 502
```

Cek connection leak di Postgres:
```bash
PG_POD=$(kubectl get pods -l app=postgres -o jsonpath='{.items[0].metadata.name}')
kubectl exec $PG_POD -- psql -U goapi -d goapi -c "SELECT pid, client_addr, state FROM pg_stat_activity WHERE datname='goapi';"
kubectl get pods -l app=go-api -o custom-columns=NAME:.metadata.name,IP:.status.podIP
```

**Expected**: semua `client_addr` di hasil query pertama harus COCOK
dengan salah satu IP Pod yang MASIH HIDUP di hasil kedua — kalau ada IP
yang gak cocok dengan Pod manapun yang hidup, itu tanda connection leak.

Beres, bersihin file sementara-nya:
```bash
rm -f /tmp/expB.sh /tmp/expB.log
```

### Langkah 7 — Experiment C: crash aplikasi

**WAJIB port-forward ke 1 Pod spesifik, sama seperti `/memory-hog`.**

```bash
POD=$(kubectl get pods -l app=go-api -o jsonpath='{.items[0].metadata.name}')
kubectl port-forward pod/$POD 18082:8080 &
sleep 2
curl --max-time 5 localhost:18082/crash
kill %1 2>/dev/null
sleep 3
kubectl get pod $POD
kubectl describe pod $POD | grep -A4 "Last State:"
```

**Expected output:**
```
NAME       READY   STATUS    RESTARTS
$POD       1/1     Running   1 (5s ago)

Last State:     Terminated
  Reason:       Error
  Exit Code:    1
```

Beda dari Level 4 (`OOMKilled`, exit code 137, kernel yang bunuh) — di
sini `Reason: Error` exit code 1, APLIKASI SENDIRI yang `os.Exit(1)`.

Crash berkali-kali buat lihat `CrashLoopBackOff` (opsional, makan waktu
~1 menit):
```bash
for i in $(seq 1 5); do
  kubectl port-forward pod/$POD 18082:8080 &
  PF_PID=$!
  sleep 2
  curl -s --max-time 3 localhost:18082/crash >/dev/null 2>&1
  kill $PF_PID 2>/dev/null
  wait $PF_PID 2>/dev/null
  sleep 5
done
kubectl get pod $POD
```

**Expected output:**
```
NAME    READY   STATUS             RESTARTS
$POD    0/1     CrashLoopBackOff   3 (...)
```

Bersihin Pod yang crash-loop ini biar gak ganggu eksperimen selanjutnya:
```bash
kubectl delete pod $POD --wait=false
```

### Langkah 8 — Experiment D: traffic brutal

```bash
k6 run k6/experiment-d-brutal.js &
K6PID=$!
for i in $(seq 1 11); do sleep 16; kubectl get hpa go-api-hpa; done
wait $K6PID
```

**Expected timeline** (mentok `maxReplicas` cepat, TARGETS tetap tinggi
terus — sistem TETAP gak reject total, cuma lambat):
```
t=64s : TARGETS=378%   REPLICAS=10   ← mentok max
t=176s: TARGETS=341%   REPLICAS=10   (masih kewalahan sampai akhir)
```

**Expected hasil k6** (0% error meski TARGETS jauh di atas 70% terus):
```
http_req_failed: 0.00% (0 dari ~12000+)
http_req_duration: p95 ~700-800ms
```

Titik jenuh yang harus kamu identifikasi dari data ini: BUKAN Postgres
(connection count masih jauh dari limit), BUKAN API gagal total (0%
error), tapi `maxReplicas: 10` HPA sendiri.

---

## Apa yang barusan terjadi

Kamu baru saja menjalankan sistem yang menggabungkan SEMUA mekanisme dari
Level 1-5 secara bersamaan, dan menemukan lapisan baru yang PALING GAK
otomatis ikut ter-cover: **state**. HPA di sini cuma tahu cara nambah Pod
`go-api` — dia gak pernah menyentuh Postgres (`replicas: 1` sepanjang
semua eksperimen). Cache-aside (Redis) jadi mitigasi PRAKTIS yang menunda
titik di mana database jadi bottleneck, bukan solusi permanen. Prometheus +
Grafana di sini gak mengubah behavior sistem sama sekali, tapi jadi cara
kamu MELIHAT semua mekanisme (self-healing, resource enforcement,
autoscaling) bekerja bersamaan secara real-time — tanpa itu, kamu balik ke
cara Level 1-5: polling `kubectl` manual berkali-kali, yang gak scalable
buat sistem production beneran. Baca bagian "Retrospektif" di
[README asli Level 6](../level-6/README.md#bagian-7--retrospektif-level-1-sampai-6)
buat narasi lengkap gimana tiap level saling terhubung.

## Troubleshooting

| Gejala | Kemungkinan penyebab | Cara cek / perbaiki |
|---|---|---|
| `/api/users` masih balikin data statis (bukan dari Postgres) | Env var `POSTGRES_HOST` belum di-set di Deployment, atau migration belum dijalankan | `kubectl describe pod <go-api-pod> \| grep POSTGRES_HOST` — pastikan ada; kalau kosong app fallback ke data in-memory (ini SENGAJA, bukan bug — cek `db.go` `setupDB()`) |
| `kubectl rollout status deployment/postgres` timeout | Image `postgres:16-alpine` lagi di-pull (bisa 1-2 menit di run pertama), atau PVC gagal provision | `kubectl describe pod -l app=postgres` cek Events — kalau soal image pull, tunggu lebih lama; kalau soal PVC, cek `kubectl get pvc` statusnya `Bound` |
| Cache-status Langkah 3 selalu `"never-hit"`, gak pernah `"hit"`/`"miss"` | Request lewat Service/Ingress (round robin ke Pod BEDA tiap kali), bukan port-forward ke 1 Pod spesifik — tiap Pod punya state cache-status sendiri-sendiri | Pastikan pakai `kubectl port-forward pod/<nama-spesifik>`, BUKAN `localhost:8080` (itu lewat Ingress, round robin) |
| Prometheus target `go-api-pods` statusnya `down` | Annotation `prometheus.io/scrape: "true"` gak ada di Pod, atau Pod belum ready | `kubectl get pod <pod> -o yaml \| grep prometheus.io` cek annotation ada; cek `manifests/deployment.yaml` bagian `template.metadata.annotations` |
| Target `cadvisor` statusnya `down` (bukan `up`) | ServiceAccount `prometheus` belum ke-bind RBAC dengan benar | `kubectl get clusterrolebinding prometheus` pastikan ada; `kubectl logs -l app=prometheus` cek error auth |
| Grafana dashboard kosong / panel "No data" | Prometheus datasource belum tersambung, atau belum ada traffic sama sekali buat metric muncul | Cek `http://localhost:13000/datasources` (via port-forward) datasource "Prometheus" statusnya OK; generate sedikit traffic dulu (`curl localhost:8080/api/users` beberapa kali) sebelum expect data muncul |
| Experiment B: gak ada request gagal sama sekali (beda dari expected) | Kebetulan gak kena race condition kecil antara Pod terminate vs Service endpoint update — INI JUGA VALID, bukan berarti eksperimen gagal | Catat aja hasilnya apa adanya; ulangi 2-3x kalau mau lihat race condition-nya kejadian |
| Experiment C: `CrashLoopBackOff` gak muncul walau sudah crash 5x | Jeda antar percobaan crash kurang cepat (kubelet perlu pola restart RAPAT buat masuk backoff) | Pastikan Pod udah `Running` lagi SEBELUM crash berikutnya (cek `kubectl get pod` tiap sebelum crash lagi), kurangi jeda `sleep` antar percobaan |
| `k6 run k6/experiment-d-brutal.js` request ada yang gagal (bukan 0%) | Wajar variasi tergantung spek host — HPA mungkin belum sempat provisioning cukup Pod di awal ramp yang curam | Bukan berarti config salah, catat % gagal apa adanya sebagai bagian dari observasi titik jenuh |
| `kubectl port-forward` keluar `error creating forwarding stream: Timeout occurred` berulang, lalu percobaan baru keluar `bind: address already in use` | Proses `port-forward` lama masih nyangkut pegang port itu (rusak tapi belum mati) — job background lama gak otomatis exit walau koneksinya sendiri udah timeout | `jobs -l` buat lihat PID job port-forward lama, `kill -9 <PID>`, tunggu sebentar, baru jalankan `kubectl port-forward` lagi |

## Cleanup

Balik dulu ke terminal yang jalanin `port-forward` Prometheus/Grafana
(Langkah 4). Kalau terminal itu lagi banjir spam `Handling connection for
...`, Ctrl+C BELUM TENTU nyampe ke proses-nya (dan hasil paste yang
kepotong terminal bisa muncul error aneh kayak `zsh: bad pattern:
[200~...` — itu cuma glitch bracketed-paste, BUKAN command asli, abaikan).
Cek dan matiin manual pakai PID:

```bash
jobs -l
```
Catat PID tiap job `kubectl port-forward` yang masih `running`, lalu:
```bash
kill <PID1> <PID2>
sleep 1
jobs -l
```
Harus kosong / semua `Done`/`Terminated` — kalau masih ada yang nyangkut,
`kill -9 <PID>`.

```bash
lsof -i :19090 -i :13000
```
Kosong = port udah lepas, aman lanjut cluster delete.

```bash
k3d cluster delete go-api-lab
```

Verifikasi manual gak ada sisa (semua output di bawah HARUS kosong):

```bash
k3d cluster list
docker ps -a | grep k3d
docker network ls | grep k3d
docker volume ls | grep k3d
kubectl config get-contexts | grep go-api-lab
```

Ini level TERAKHIR dari rencana awal home lab. Kalau mau lanjut ke topik
lanjutan (GitOps/ArgoCD, service mesh, multi-cluster), itu di luar scope
6 guide ini.
