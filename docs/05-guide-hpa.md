# Guide 05 — Horizontal Pod Autoscaler (HPA)

Pasang HPA berbasis CPU utilization, buktikan replica naik mengikuti beban
(bukan asumsi "traffic > N request = spawn pod"), dan buktikan scale-down
jauh lebih lambat dari scale-up (by design).

**Waktu**: ~25 menit aktif + **~10 menit nunggu** (5 menit buat replica
turun ke minimum di awal, 5 menit lagi buat scale-down di akhir) — jangan
skip langkah tunggu ini, itu bagian dari yang mau dibuktikan.

**Prasyarat**:
- Level 4 selesai — `metrics-server` jalan (`kubectl top pods` keluar
  angka), Deployment `go-api` punya `requests.cpu: 100m`
- **Paling sering salah**: kalau HPA `TARGETS` stuck `<unknown>`, itu HAMPIR
  SELALU karena metrics-server belum beres — balik cek Guide 04 dulu,
  jangan langsung curiga ke HPA-nya

Konsep detail ada di [`level-5/README.md`](../level-5/README.md).

---

### Langkah 0 — Setup cluster (kalau belum ada)

```bash
cd level-3
k3d cluster create -c cluster/k3d-config.yaml
k3d image import go-api:v1 -c go-api-lab
kubectl apply -f manifests/service.yaml
kubectl apply -f manifests/ingress.yaml
kubectl apply -f ../level-4/manifests/deployment.yaml
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
kubectl -n kube-system patch deployment metrics-server --type='json' \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'
kubectl -n kube-system rollout status deployment/metrics-server --timeout=90s
cd ../level-5
```

Tunggu ~15 detik, verifikasi `kubectl top pods` keluar angka sebelum
lanjut.

### Langkah 1 — Pasang HPA

```bash
kubectl apply -f manifests/hpa.yaml
kubectl get hpa go-api-hpa
```

**Expected output** (biasanya langsung keluar angka nyata di attempt
pertama, ~15 detik setelah apply):
```
NAME         REFERENCE           TARGETS       MINPODS   MAXPODS   REPLICAS   AGE
go-api-hpa   Deployment/go-api   cpu: 1%/70%   2         10        3          15s
```

Kalau `TARGETS` masih `<unknown>/70%`, tunggu 15-30 detik lagi dan cek
ulang — JANGAN lanjut kalau masih `<unknown>` setelah 1-2 menit (lihat
Troubleshooting).

### Langkah 2 — Tunggu baseline turun ke minReplicas

`REPLICAS` awal masih 3 (dari manifest Deployment) — HPA butuh waktu buat
"ambil alih" dan scale down ke `minReplicas: 2`.

```bash
watch -n 15 kubectl get hpa go-api-hpa
```
(Ctrl+C buat keluar setelah `REPLICAS` jadi `2`)

> **Tip**: kalau keluar `zsh: command not found: watch` — Mac gak punya
> `watch` bawaan (beda dari Linux). Install dulu: `brew install watch`, lalu
> ulangi command di atas. Kalau gak mau install apa-apa, pakai fallback
> tanpa dependency tambahan:
> ```bash
> while true; do clear; kubectl get hpa go-api-hpa; sleep 15; done
> ```

**Expected timeline**: `REPLICAS` bertahan di 3 selama ~5 menit, baru turun
ke 2. Ini WAJAR — bukan lambat/error, itu stabilization window default
Kubernetes (lihat Troubleshooting kalau ragu).

```bash
kubectl describe hpa go-api-hpa | grep SuccessfulRescale
```

**Expected output:**
```
Normal  SuccessfulRescale  ...  New size: 2; reason: All metrics below target
```

### Langkah 3 — Load ringan (replica harus TETAP 2)

```bash
k6 run k6/load-ramp-light.js &
K6PID=$!
for i in $(seq 1 12); do sleep 15; kubectl get hpa go-api-hpa; done
wait $K6PID
```

**Expected output** (REPLICAS harus TETAP 2 sepanjang 180 detik, TARGETS
naik tapi gak pernah tembus 70%):
```
t=15s : cpu 1%/70%    REPLICAS=2
t=90s : cpu 22%/70%   REPLICAS=2
t=180s: cpu 54%/70%   REPLICAS=2
```

### Langkah 4 — Load berat (replica HARUS naik — eksperimen utama)

```bash
k6 run k6/load-ramp-heavy.js &
K6PID=$!
for i in $(seq 1 24); do sleep 15; kubectl get hpa go-api-hpa; kubectl get pods --no-headers | wc -l; done
wait $K6PID
```

**Expected timeline** (replica naik BERTAHAP, bukan langsung lompat ke
max):
```
t=15s : cpu 80%/70%    REPLICAS=2
t=30s : cpu 349%/70%   REPLICAS=3    ← scale-up pertama
t=45s : cpu 282%/70%   REPLICAS=6    ← scale-up kedua
t=60s : cpu 232%/70%   REPLICAS=10   ← mentok maxReplicas
t=75s..345s: REPLICAS tetap 10, TARGETS berfluktuasi 139%-379%/70%
```

```bash
kubectl describe hpa go-api-hpa | grep SuccessfulRescale
```

**Expected output** (3 event scale-up dalam ~45 detik):
```
New size: 3;  reason: cpu resource utilization (percentage of request) above target
New size: 6;  reason: cpu resource utilization (percentage of request) above target
New size: 10; reason: cpu resource utilization (percentage of request) above target
```

### Langkah 5 — Amati scale-down (lambat, by design)

Load test dari Langkah 4 sudah selesai — sekarang tunggu replica turun.

```bash
for i in $(seq 1 30); do
  sleep 15
  REP=$(kubectl get hpa go-api-hpa -o jsonpath='{.status.currentReplicas}')
  echo "t=$((i*15))s replicas=$REP"
  if [ "$REP" = "2" ]; then break; fi
done
```

**Expected output**: `TARGETS` langsung turun ke rendah begitu load
berhenti, TAPI `REPLICAS` bertahan di 10 selama **~5 MENIT** sebelum
langsung drop ke 2 (bukan turun bertahap 10→6→3→2).

```bash
kubectl describe hpa go-api-hpa | grep SuccessfulRescale | tail -1
```

**Expected output:**
```
New size: 2; reason: All metrics below target
```

---

## Apa yang barusan terjadi

Kamu baru saja membuktikan bahwa scale-up dan scale-down Kubernetes HPA
punya kecepatan yang SENGAJA berbeda jauh: scale-up agresif (2→10 dalam
~45 detik, proporsional ke rasio overshoot), scale-down konservatif (nunggu
`stabilization window` 5 menit — HPA "inget" rekomendasi TERTINGGI selama
window itu, bukan langsung reaktif ke angka terbaru) — desain ini sengaja
mencegah "flapping" (naik-turun-naik-turun) kalau traffic sendiri
naik-turun sebentar-sebentar. Yang juga penting: model HPA ini BUKAN
"traffic > N request = spawn pod" — dia berbasis resource utilization,
karena endpoint yang beda (`/api/users` murah vs `/cpu-intensive` mahal)
punya cost CPU yang beda jauh di rate request yang SAMA (lihat perbandingan
lengkap di README asli Level 5).

## Troubleshooting

| Gejala | Kemungkinan penyebab | Cara cek / perbaiki |
|---|---|---|
| HPA `TARGETS` tetap `<unknown>/70%` setelah 1-2 menit | metrics-server belum jalan/patched dari Level 4 — **cek ini SEBELUM curiga ke HPA** | `kubectl top pods` — kalau ini juga error, balik ke Guide 04 Langkah 1 dulu |
| Replica TIDAK naik walau CPU tinggi di `kubectl top pods` | `requests.cpu` gak ke-set di Deployment (HPA butuh ini buat hitung persentase — tanpa `requests`, HPA gak bisa hitung apa-apa) | `kubectl describe deployment go-api \| grep -A2 Requests` pastikan `cpu: 100m` ada |
| Replica gak pernah lewat `minReplicas` walau load ringan | Wajar — itu memang perilaku yang diharapkan di Langkah 3, bukan bug | Lanjut ke Langkah 4 buat lihat scale-up beneran |
| Scale-down kelamaan (> 6-7 menit) atau kecepetan (< 3 menit) | Variasi wajar tergantung timing pas load test berhenti vs mulai polling; JANGAN ubah `minReplicas`/`maxReplicas` di tengah eksperimen | Kalau beda jauh (>10 menit atau <1 menit), cek `kubectl describe hpa` bagian Events buat lihat histori rescale lengkap |
| `k6 run k6/load-ramp-heavy.js` request ada yang gagal (`http_req_failed` > 0%) | HPA belum sempat provisioning cukup Pod sebelum request numpuk — bisa terjadi kalau host lebih lambat dari environment testing asli | Bukan berarti config salah; cek berapa % gagal — kalau kecil (<1%) itu variasi wajar startup lag Pod baru |
| `kubectl get hpa` nunjukin `FailedGetResourceMetric` di Events | Sesaat metrics-server gak sempat scrape Pod yang lagi restart/pending (transient, biasanya self-recover) | Tunggu beberapa siklus lagi (~30-60 detik), cek lagi `kubectl get hpa` — kalau menetap terus, ulang cek metrics-server |
| `watch: command not found` (zsh) | Mac gak punya `watch` bawaan (beda dari Linux) | Pakai `while true; do clear; kubectl get hpa go-api-hpa; sleep 15; done`, atau `brew install watch` |

## Cleanup

```bash
k3d cluster delete go-api-lab
```

Ini juga otomatis hapus HPA, Deployment, Service, Ingress di dalamnya — gak
perlu `kubectl delete` satu-satu.

Verifikasi manual gak ada sisa (semua output di bawah HARUS kosong):

```bash
k3d cluster list
docker ps -a | grep k3d
docker network ls | grep k3d
docker volume ls | grep k3d
kubectl config get-contexts | grep go-api-lab
```

Lanjut ke [Guide 06 — Production Simulation](06-guide-production-simulation.md).
