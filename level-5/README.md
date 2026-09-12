# Level 5 — Horizontal Pod Autoscaler (HPA)

Pasang HPA berbasis CPU utilization ke Deployment `go-api` (dari Level 3/4),
buktikan scale-up/scale-down mengikuti beban nyata — bukan asumsi
"traffic > N request = spawn pod".

## Menjalankan

```bash
k3d cluster create -c ../level-3/cluster/k3d-config.yaml
k3d image import go-api:v1 -c go-api-lab
kubectl apply -f ../level-3/manifests/service.yaml
kubectl apply -f ../level-3/manifests/ingress.yaml
kubectl apply -f ../level-4/manifests/deployment.yaml   # requests.cpu:100m, limits.cpu:500m
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
kubectl -n kube-system patch deployment metrics-server --type='json' \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'
kubectl apply -f manifests/hpa.yaml
```

---

## 1. Prasyarat Level 4 masih hidup

```
Command: kubectl top pods
Output: 3 Pod, semua keluar angka (1m CPU, 1Mi memory) - bukan error
Command: kubectl describe deployment go-api | grep -A2 Requests
Output: cpu: 100m, memory: 64Mi
Kesimpulan: metrics-server jalan, requests.cpu masih 100m sesuai Level 4.
```

## 2. Pasang HPA

```
Command: kubectl apply -f manifests/hpa.yaml
         kubectl get hpa go-api-hpa
Output (langsung di attempt pertama, ~15 detik setelah apply):
NAME         REFERENCE           TARGETS       MINPODS   MAXPODS   REPLICAS   AGE
go-api-hpa   Deployment/go-api   cpu: 1%/70%   2         10        3          15s
Kesimpulan: TARGETS langsung keluar angka nyata (1%/70%), bukan <unknown> -
metrics-server sudah proven jalan dari Level 4 jadi gak ada delay.
```

```
[Baseline: turun ke minReplicas]
Command: kubectl get hpa go-api-hpa   (dipantau tiap 20 detik)
Output: REPLICAS bertahan di 3 selama ~5m11s, baru turun ke 2 di ~5m36s
Command: kubectl describe hpa | grep SuccessfulRescale
Output: New size: 2; reason: All metrics below target
Kesimpulan: Deployment awalnya `replicas: 3` (dari Level 3/4 manifest),
begitu HPA "ambil alih" kontrol replica count, dia hitung usage aktual
sangat rendah (idle) dan scale down ke `minReplicas: 2` - TAPI perlu nunggu
~5 menit (stabilization window default) sebelum eksekusi scale-down
pertama, bukan langsung instan.
```

## 3. Prediksi manual sebelum load test

```
requests.cpu = 100m, target = 70%
-> threshold scale-up = 70% x 100m = 70m per Pod

/cpu-intensive busy-loop persis 500ms, makan ~1 core penuh (1000m) selama
busy (dari observasi Level 1: --cpus=0.5 bikin usage mentok di limit saat
endpoint ini dipanggil terus-menerus - berarti tanpa limit dia akan coba
pakai sebanyak mungkin, bukan cuma sebagian core).

duty_cycle per Pod = (rate_per_pod x 0.5s)
avg_millicores = duty_cycle x 1000m

Supaya TETAP DI BAWAH 70m per Pod:
  duty_cycle < 0.07
  rate_per_pod < 0.07 / 0.5 = 0.14 req/s per Pod  (~1 request tiap 7 detik/Pod)

Dengan baseline 2 Pod: total rate aman (light) < ~0.28 req/s.
Prediksi: load-ramp-light.js (naik dari 0.1 req/s ke 0.4 req/s selama 3
menit) SEHARUSNYA tetap di bawah 70% karena masih di kisaran/sedikit di
atas batas per-Pod, sementara load-ramp-heavy.js (mulai 2 req/s, naik ke
20 req/s) PASTI jauh melewati threshold dalam hitungan detik pertama.
```

## 4. Load ringan — replica tetap 2

```
Command: k6 run k6/load-ramp-light.js   (sambil polling `kubectl get hpa` tiap 15s)
Output (timeline TARGETS selama 180 detik):
  t=15s : cpu 1%/70%    REPLICAS=2
  t=30s : cpu 9%/70%    REPLICAS=2
  t=60s : cpu 15%/70%   REPLICAS=2
  t=90s : cpu 22%/70%   REPLICAS=2
  t=120s: cpu 38%/70%   REPLICAS=2
  t=150s: cpu 40%/70%   REPLICAS=2
  t=180s: cpu 54%/70%   REPLICAS=2

Kesimpulan: TEPAT SESUAI PREDIKSI - REPLICAS bertahan di 2 sepanjang test,
TARGETS naik seiring rate k6 bertambah (0.1->0.4 req/s) tapi mentok di 54%,
tidak pernah tembus 70%. Prediksi manual di poin 3 akurat.
```

## 5. Load berat — replica naik mengikuti beban (eksperimen utama)

```
Command: k6 run k6/load-ramp-heavy.js   (sambil polling `kubectl get hpa`
         DAN `kubectl get pods` tiap 15s di background)

Timeline:
  t=15s : cpu 80%/70%    REPLICAS=2    (baru lewat threshold, HPA belum aksi)
  t=30s : cpu 349%/70%   REPLICAS=3    ← scale-up PERTAMA (2->3)
  t=45s : cpu 282%/70%   REPLICAS=6    ← scale-up KEDUA (3->6)
  t=60s : cpu 232%/70%   REPLICAS=10   ← scale-up KETIGA, langsung mentok maxReplicas (6->10)
  t=75s..345s: REPLICAS tetap 10 (mentok max), TARGETS ber-fluktuasi
               139% - 379%/70% - beban tetap jauh di atas kapasitas
               bahkan dengan 10 Pod (rate k6 terus naik sampai 20 req/s)
  t=360s: cpu 83%/70%    REPLICAS=10   (load k6 baru saja selesai, TARGETS mulai turun)

Command: kubectl describe hpa go-api-hpa   (section Events)
Output:
  SuccessfulRescale  New size: 3;  reason: cpu resource utilization (percentage of request) above target
  SuccessfulRescale  New size: 6;  reason: cpu resource utilization (percentage of request) above target
  SuccessfulRescale  New size: 10; reason: cpu resource utilization (percentage of request) above target
(3 event scale-up terjadi dalam ~45 detik: 2->3->6->10)

Hasil k6 (5m30s, 4034 request total):
  http_req_failed: 0.00% (0/4034)
  http_req_duration: avg=519.85ms, p95=551ms  (mendekati baseline 500ms
  busy-loop endpoint itu sendiri - artinya request TIDAK antre lama, HPA
  keburu nyediain kapasitas)

Kesimpulan vs prediksi: actual jauh LEBIH CEPAT & LEBIH AGRESIF dari
sekadar "scale up pelan-pelan" - begitu threshold 70% dilewati, HPA
langsung menghitung target replica count PROPORSIONAL ke rasio overshoot
(bukan nambah 1 Pod per siklus). Rumus HPA kira-kira:
  desiredReplicas = ceil(currentReplicas x (currentUtilization / targetUtilization))
Di t=30s misalnya utilization 349%, dari 2 Pod -> ceil(2 x 349/70) = ceil(9.97)
≈ 10, tapi ada default scaling policy yang membatasi laju kenaikan per
periode (maks dobel replica atau +4 Pod per 15 detik, mana yang lebih
besar) - itu kenapa progresinya bertahap 2->3->6->10, bukan langsung
lompat ke 10 di step pertama, walau secara matematis utilization-nya
sudah cukup tinggi buat langsung ke max di awal.
```

## 6. Scale-down — jauh lebih lambat dari scale-up

```
Command: hentikan k6 (biarkan selesai natural), lalu poll `kubectl get hpa`
         tiap 15 detik sampai REPLICAS balik ke 2
Output:
  TARGETS langsung turun ke 1%/70% begitu load berhenti (metrics real-time,
  gak nunggu)
  TAPI REPLICAS BERTAHAN DI 10 selama ~260 detik lebih (dari transisi
  akhir load test sampai drop), baru turun LANGSUNG ke 2 (bukan bertahap
  10->6->3->2, beda dari scale-up yang bertahap)

Command: kubectl describe hpa go-api-hpa (Events)
Output: SuccessfulRescale  New size: 2; reason: All metrics below target

Total waktu dari load BENAR-BENAR berhenti sampai REPLICAS balik ke 2:
  ≈ 305 detik (~5 menit 5 detik)

Kesimpulan: SESUAI EKSPEKTASI ~5 menit. Ini karena Kubernetes HPA punya
`stabilization window` default 300 detik (5 menit) KHUSUS buat scale-down
(scale-up TIDAK punya window sepanjang ini - makanya keliatan jauh lebih
gesit). Algoritmanya: HPA nyimpen histori "recommended replica count"
selama window itu, dan buat scale-down dia pakai nilai MAKSIMUM dari
seluruh histori tersebut (bukan langsung pakai rekomendasi terbaru) - jadi
walau CPU usage sudah drop ke 1% detik itu juga, HPA masih "inget" bahwa
beberapa saat sebelumnya rekomendasinya masih 10 Pod, dan baru mau nurunin
setelah histori 5 menit itu semuanya udah "bersih" dari rekomendasi tinggi.
Desain ini SENGAJA - mencegah flapping (naik-turun-naik-turun) kalau
traffic-nya sendiri naik-turun sebentar-sebentar (misal spike singkat
2-3 kali dalam beberapa menit) - kalau HPA langsung reaktif ke scale-down,
sistem bisa kena "thrashing": baru aja nurunin Pod, eh traffic naik lagi,
harus provisioning ulang, buang-buang waktu startup Pod berkali-kali.
Scale-down yang konservatif justru lebih aman buat availability, walau
"boros" resource sesaat.
```

## 7. Kenapa bukan "traffic > 1000 request = spawn pod"

Perbandingan eksplisit dengan hasil Level 2 (round robin, distribusi
request per instance):

| Model | Basis keputusan | Masalah |
|---|---|---|
| Asumsi awal ("request count threshold") | Jumlah request per detik yang masuk | TIDAK MEMPERHITUNGKAN biaya komputasi per request - endpoint beda bisa punya cost CPU yang beda jauh |
| HPA (utilization-based, dipakai lab ini) | % CPU usage relatif ke `requests` | Langsung merefleksikan beban KOMPUTASI aktual, apapun endpoint-nya |

Bukti konkret dari lab sendiri:
- Level 2: `/api/users` di rate **1000 req/s** (test k6 tertinggi) CPU
  usage per Pod cuma **~2-3m** (super ringan, tinggal serialize JSON
  statis).
- Level 5 di sini: `/cpu-intensive` di rate **cuma ~2 req/s** SUDAH
  memicu HPA scale-up (utilization 80%/70% di t=15s) - dan di rate
  **20 req/s** butuh 10 Pod (maxReplicas) buat nampung, dengan CPU
  utilization tetap 139-379% (Pod-nya masih "kewalahan" walau udah di-max).

Kalau pakai model "request count > threshold", threshold yang cocok buat
`/api/users` (misal >1000 req/s baru scale) akan SALAH TOTAL buat
`/cpu-intensive` (2 req/s aja udah kewalahan) - butuh threshold beda-beda
per endpoint, dan itu pun cuma proxy kasar buat resource usage yang
sebenarnya. Utilization-based langsung ukur hal yang benar-benar dipedulikan
(apakah Pod kewalahan atau tidak), independen dari endpoint mana yang
dipanggil atau berapa banyak request-nya - itulah kenapa Kubernetes pakai
ini sebagai basis default, bukan angka request count.

---

## Ringkasan

| Fase | Trigger | Kecepatan reaksi | Progresi replica |
|---|---|---|---|
| Scale-up | Utilization > target (70%) | Cepat (~15-45 detik ke max di lab ini) | Bertahap tapi agresif, proporsional ke rasio overshoot (2->3->6->10) |
| Scale-down | Semua metrics di bawah target, DAN histori 5 menit terakhir juga bersih | Lambat (default 5 menit stabilization window) | Langsung ke target akhir begitu window "bersih" (bukan bertahap turun) |

**Prediksi vs actual**: prediksi manual titik scale-up (~70m/Pod, rate
>0.14 req/s per Pod) TERBUKTI AKURAT - load ringan (di bawah threshold itu)
tetap di 2 replica, load berat (jauh di atas) langsung memicu scale-up
dalam hitungan detik. Yang TIDAK diprediksi secara presisi di awal adalah
KECEPATAN scale-up (2->10 dalam 45 detik) dan progresi bertahapnya - itu
konsekuensi dari default scaling policy Kubernetes (bukan langsung lompat
ke target replicas hasil hitungan, dibatasi laju kenaikan per periode
15 detik), yang baru kelihatan jelas begitu dijalankan beneran.

## Cleanup

```bash
k3d cluster delete go-api-lab
```
