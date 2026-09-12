# Task Brief — Level 4: Resource Requests & Limits

## Konteks
Level 3 sudah selesai: Deployment 3 replica jalan dengan readiness/liveness
probe, Service, dan Ingress semua terbukti bekerja. Tapi Deployment itu belum
punya `resources` sama sekali — artinya Pod bisa pakai CPU/memory node sebebas-
bebasnya, dan scheduler nggak punya info buat nentuin Pod itu "berat" atau
"ringan" saat milih node.

Level 4 nambahin `resources.requests` dan `resources.limits`, lalu buktikan
efeknya lewat eksperimen — termasuk beberapa hal yang **tidak intuitif** dan
sering disalahpahami:

- `requests` dipakai scheduler buat keputusan penempatan Pod, `limits` dipakai
  kubelet/cgroup buat enforcement saat runtime — dua hal yang beda fungsi
- Melebihi CPU limit → **throttled** (melambat, tidak mati)
- Melebihi memory limit → **OOMKilled** (mati, bukan melambat)
- **Level ini juga jadi prasyarat teknis buat Level 5 (HPA)** — HPA berbasis
  CPU menghitung persentase dari `requests`, bukan `limits`. Kalau ini nggak
  dipahami sekarang, HPA nanti kelihatan "salah hitung" padahal sebenarnya
  bekerja sesuai definisi.

---

## Objective
Tambahkan resource requests/limits ke Deployment `go-api`, buktikan efeknya ke
scheduling, buktikan CPU throttling di bawah beban, buktikan OOMKill saat
memory limit terlampaui, dan install `metrics-server` (dipakai `kubectl top`
sekarang, dan wajib buat HPA di Level 5).

---

## Deliverables

```
level-4/
├── manifests/
│   └── deployment.yaml         (update dari Level 3, tambah resources)
└── README.md
```

---

## Task List (kerjakan berurutan)

### 1. Install metrics-server
- [ ] Apply metrics-server ke cluster k3d:
      ```bash
      kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
      ```
- [ ] k3d/cluster lokal biasanya butuh tambahan flag karena sertifikat
      self-signed — edit Deployment `metrics-server` di namespace
      `kube-system`, tambahkan args `--kubelet-insecure-tls`
      (`kubectl -n kube-system edit deployment metrics-server` atau patch)
- [ ] Tunggu sampai `kubectl -n kube-system get pods` nunjukin metrics-server
      `Running` dan `1/1`
- [ ] Verifikasi: `kubectl top nodes` dan `kubectl top pods` harus keluar angka
      (bukan error `metrics not available yet` — mungkin perlu tunggu 1-2 menit
      setelah Pod Running)

### 2. Tambahkan resource requests & limits
- [ ] Update `deployment.yaml` dari Level 3, tambahkan ke container spec:
      ```yaml
      resources:
        requests:
          cpu: "100m"
          memory: "64Mi"
        limits:
          cpu: "500m"
          memory: "128Mi"
      ```
- [ ] `kubectl apply -f manifests/deployment.yaml`
- [ ] `kubectl get pods` — verifikasi 3 Pod tetap `Running` (rolling update,
      bukan downtime total — catat di README kalau ada Pod yang sempat
      `Terminating` barengan sama yang baru `ContainerCreating`)
- [ ] `kubectl describe pod <nama-pod>` — konfirmasi `Requests` dan `Limits`
      muncul di section `Containers:`

### 3. Efek requests ke scheduling (eksperimen penting, sering dilewatkan)
- [ ] Cek kapasitas node: `kubectl describe node <nama-node>` bagian
      `Allocatable` (catat CPU & memory yang tersedia per node)
- [ ] Buat **Deployment kedua sementara** (`test-scheduling.yaml`, bukan bagian
      dari `go-api`) dengan `requests.cpu` yang sengaja diset SANGAT besar
      (misal `4000m` — lebih besar dari CPU node manapun di cluster k3d lokal
      kamu)
- [ ] `kubectl apply` deployment test itu, lalu `kubectl get pods` — Pod itu
      harus nyangkut di status `Pending`
- [ ] `kubectl describe pod <pod-yang-pending>` — cari section `Events`, catat
      pesan schedulernya (biasanya `Insufficient cpu`)
- [ ] Hapus lagi deployment test ini setelah dibuktikan
      (`kubectl delete -f test-scheduling.yaml`) — **jangan biarkan nyangkut**
- [ ] Kesimpulan yang harus ditulis di README: `requests` bukan cuma metadata,
      dia beneran dipakai scheduler buat nolak penempatan kalau node nggak
      punya kapasitas cukup

### 4. CPU throttling di bawah beban (bandingkan dengan Level 1!)
- [ ] Generate load paralel ke `/cpu-intensive` lewat Service/Ingress (mirip
      cara Level 2, bisa pakai loop curl atau k6)
- [ ] Selama load jalan, `kubectl top pods` berkali-kali, catat CPU usage tiap
      Pod
- [ ] Bandingkan ke `limits.cpu: 500m` — apakah mentok di situ (mirip hasil
      Level 1 yang mentok di 50% waktu `--cpus=0.5`)?
- [ ] Cek `kubectl describe pod <nama-pod>` — apakah ada indikator throttling
      di metrics (kalau tersedia), atau jelaskan kalau tidak ada indikator
      langsung dari `kubectl` biasa (throttling CPU itu "silent" — Pod tetap
      `Running`, cuma lambat, beda dari memory yang keliatan jelas statusnya)

### 5. OOMKill (bandingkan dengan Level 1 — di situ nggak sempat kejadian)
- [ ] Tambahkan endpoint baru sementara di Go API: `GET /memory-hog` yang
      sengaja alokasi slice besar di memory (misal 200MB, lebih besar dari
      `limits.memory: 128Mi`) dan **tahan referensinya** (jangan langsung
      di-garbage-collect) sebelum return response
- [ ] Rebuild image, `k3d image import` ulang, `kubectl rollout restart
      deployment/go-api`
- [ ] Hit endpoint `/memory-hog` itu ke salah satu Pod langsung (bukan lewat
      Service, biar pasti kena Pod yang sama)
- [ ] `kubectl get pods` — tunggu beberapa detik, Pod itu harus berubah status
      (`OOMKilled` di reason, lalu restart otomatis karena livenessProbe/restart
      policy default)
- [ ] `kubectl describe pod <nama-pod>` — cari `Last State: Terminated,
      Reason: OOMKilled`, catat exit code-nya
- [ ] Bandingkan eksplisit di README dengan Level 1 (di situ OOM nggak pernah
      kejadian karena app-nya nggak sengaja makan memory besar) — sekarang
      baru kelihatan bedanya CPU limit (throttle, tetap hidup) vs memory limit
      (mati total, di-restart)

### 6. Dokumentasi
- [ ] README merangkum semua eksperimen + 1 tabel ringkas: apa yang terjadi
      kalau requests kurang (Pending), CPU limit terlampaui (throttle), memory
      limit terlampaui (OOMKilled)

---

## Acceptance Criteria

- [ ] `metrics-server` jalan, `kubectl top pods`/`kubectl top nodes` keluar
      angka nyata
- [ ] Deployment `go-api` punya `requests` dan `limits` eksplisit, diverifikasi
      lewat `kubectl describe pod`
- [ ] Terbukti (bukan diasumsikan) bahwa `requests` mempengaruhi keputusan
      scheduler — lewat eksperimen Pod yang sengaja `Pending`
- [ ] Terbukti CPU throttling di bawah beban lewat `kubectl top pods`,
      dibandingkan dengan hasil serupa di Level 1
- [ ] Terbukti OOMKill saat memory limit terlampaui, dengan `Reason: OOMKilled`
      kelihatan di `kubectl describe pod`
- [ ] README ada tabel ringkas: requests kurang vs CPU limit terlampaui vs
      memory limit terlampaui — 3 skenario beda, 3 akibat beda

## Constraints
- Deployment test scheduling (poin 3) **harus dihapus** setelah dibuktikan —
  jangan biarkan nyangkut `Pending` selamanya di cluster
- Endpoint `/memory-hog` boleh permanen di code (untuk eksperimen ulang nanti),
  tapi JANGAN pernah di-hit lewat Service/Ingress ke Pod acak — selalu target
  Pod tertentu langsung, biar Pod lain nggak ikut kena OOM tanpa sengaja
- Jangan install HPA di level ini — itu Level 5, biar konsep requests/limits
  dulu yang matang sebelum ditambah lapisan autoscaling di atasnya

## Out of Scope (jangan dikerjakan di task ini)
- HPA / autoscaling — itu Level 5
- Prometheus/Grafana — nanti
- Redis/Postgres — itu Level 6