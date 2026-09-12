# Task Brief — Level 3: Kubernetes (Deployment → Pod → Service → Ingress)

## Konteks
Level 1 & 2 sudah selesai: image `go-api:v1` sudah ada (dengan `/health`,
`/api/users`, `/cpu-intensive`, `/whoami`, `/stats`), dan sudah dibuktikan
bagaimana load balancing manual (NGINX + Docker Compose) bekerja — termasuk
insight soal round robin, failover, dan bedanya availability vs performance.

Level 3 pindah ke Kubernetes beneran. **Tujuan utama level ini bukan cuma bikin
sesuatu jalan**, tapi memahami dengan presisi:

```
Container ≠ Pod ≠ Deployment ≠ Service ≠ Ingress
```

Tiap eksperimen di bawah dirancang supaya bedanya kelihatan nyata, bukan
sekadar definisi di atas kertas. Beberapa hasil di Level 2 (retry otomatis
NGINX, distribusi round robin, dampak backend mati) akan dibandingkan langsung
dengan versi Kubernetes-nya di sini.

---

## Objective
Deploy `go-api:v1` ke cluster Kubernetes lokal (k3d), pakai Deployment dengan
3 replica, expose lewat Service, lalu expose ke luar cluster lewat Ingress.
Jalankan eksperimen self-healing dan bandingkan hasilnya dengan temuan Level 2.

---

## Deliverables

```
level-3/
├── cluster/
│   └── k3d-config.yaml
├── manifests/
│   ├── deployment.yaml
│   ├── service.yaml
│   └── ingress.yaml
└── README.md
```

---

## Arsitektur target

```
   curl/k6 traffic
          │
          ▼
      Ingress (NGINX Ingress Controller)
          │
          ▼
       Service (ClusterIP)
          │
   ┌──────┼──────┐
   ▼      ▼      ▼
  Pod    Pod    Pod
  (go-api:v1, x3, dikelola 1 Deployment)
```

---

## Task List (kerjakan berurutan)

### 1. Setup cluster lokal
- [ ] Install k3d (`brew install k3d`) kalau belum ada
- [ ] Buat cluster dengan port mapping yang expose Ingress ke host, misal:
      ```bash
      k3d cluster create go-api-lab \
        -p "8080:80@loadbalancer" \
        --agents 2
      ```
- [ ] Verifikasi: `kubectl get nodes` harus nunjukin 1 server + 2 agent, semua
      `Ready`
- [ ] **Import image lokal ke cluster** — ini langkah yang sering kelewat:
      `k3d image import go-api:v1 -c go-api-lab` (kalau tidak di-import, Pod
      akan `ImagePullBackOff` karena k3d nyoba pull dari registry, bukan pakai
      image lokal)

### 2. Deployment
- [ ] Buat `deployment.yaml`: 3 replica, image `go-api:v1`, `imagePullPolicy:
      IfNotPresent` (WAJIB — biar pakai image lokal yang di-import, bukan coba
      pull dari internet), container port 8080
- [ ] Tambahkan `readinessProbe` dan `livenessProbe` ke `/health` — ini konsep
      baru yang tidak ada di Level 1/2, jelaskan bedanya di README
- [ ] `kubectl apply -f manifests/deployment.yaml`
- [ ] `kubectl get pods -o wide` — verifikasi 3 Pod `Running`, catat nama Pod
      dan node tempat mereka jalan (apakah tersebar di 2 agent atau numpuk di 1?)

### 3. Bedakan Pod vs Container (eksperimen konkret)
- [ ] `kubectl exec -it <nama-pod> -- sh` masuk ke salah satu Pod
- [ ] Dari dalam situ, jalankan `hostname` — bandingkan dengan nama Pod dari
      `kubectl get pods`
- [ ] `kubectl describe pod <nama-pod>` — cari section `Containers:`, catat
      bahwa 1 Pod di sini isinya 1 container, tapi jelaskan di README: Pod bisa
      berisi LEBIH dari 1 container (sidecar pattern) — walau di lab ini cuma 1
- [ ] `docker ps` di host (bukan `kubectl`) — cari container yang sesuai Pod
      tadi, buktikan bahwa di baliknya, Pod itu sebenarnya containerd
      (bukan `docker ps` langsung — kalau di k3d, coba `docker exec` ke node
      container lalu `crictl ps` di dalamnya) — ini buat ngerti Pod adalah
      abstraksi Kubernetes, bukan unit runtime yang container engine kenal

### 4. Service
- [ ] Buat `service.yaml` tipe `ClusterIP`, selector cocok ke label Deployment,
      port 80 → targetPort 8080
- [ ] `kubectl apply -f manifests/service.yaml`
- [ ] Dari dalam salah satu Pod (`kubectl exec`), curl ke nama Service
      (`curl http://go-api-service/health`) — buktikan Service DNS resolution
      jalan (mirip custom network di Level 2, tapi ini di layer Kubernetes)
- [ ] Hit `/whoami` lewat Service berkali-kali dari dalam cluster (`kubectl run
      tmp-curl --image=busybox --rm -it -- wget -qO- ...` loop beberapa kali),
      buktikan Service load-balance ke Pod yang berbeda-beda

### 5. Ingress
- [ ] Cek Ingress Controller sudah ada (k3d biasanya bundle Traefik by default,
      tapi task ini pakai NGINX Ingress Controller secara eksplisit — install
      manual kalau belum ada, atau pakai Traefik bawaan k3d dan catat
      perbedaannya di README)
- [ ] Buat `ingress.yaml` yang route host/path tertentu ke Service
- [ ] `kubectl apply -f manifests/ingress.yaml`
- [ ] `curl localhost:8080/health` (lewat host, port yang di-mapping ke
      loadbalancer k3d) — ini harus sampai ke Pod lewat rantai penuh:
      Ingress → Service → Pod

### 6. Self-healing (bandingkan dengan Level 2!)
- [ ] Catat baseline: `kubectl get pods` (3 Pod, nama & umur masing-masing)
- [ ] `kubectl delete pod <salah-satu-nama-pod>` (setara `docker stop api-2` di
      Level 2)
- [ ] Langsung `kubectl get pods` berkali-kali (watch), catat:
      - berapa detik sampai Pod pengganti muncul
      - apakah nama Pod baru berbeda (harus beda — Pod itu disposable)
      - status Pod baru dari `Pending` → `ContainerCreating` → `Running`
- [ ] Sambil Pod mati & pengganti dibuat, jalankan traffic terus-menerus lewat
      Ingress (loop curl setiap 0.5 detik), catat: ada request gagal atau
      tidak, berapa banyak
- [ ] **Bandingkan eksplisit di README** dengan hasil Level 2 (NGINX manual):
      apa yang SAMA (retry/availability) dan apa yang BEDA (Kubernetes bikin
      Pod baru otomatis untuk kembalikan ke desired replica count — NGINX
      Level 2 TIDAK bikin container baru sendiri, `api-2` yang mati ya tetap
      mati sampai displore manual `docker start`)

### 7. Dokumentasi
- [ ] README menjelaskan definisi Container/Pod/Deployment/Service/Ingress
      **dengan merujuk ke eksperimen konkret di atas**, bukan definisi generik
      dari dokumentasi

---

## Acceptance Criteria

- [ ] Cluster k3d jalan, image lokal berhasil di-import (bukan pull dari
      registry publik)
- [ ] Deployment dengan 3 replica jalan, readiness/liveness probe terpasang
- [ ] Terbukti (via `kubectl exec` + `hostname` + `docker`/`crictl`) bahwa Pod
      adalah lapisan abstraksi di atas container, bukan sama dengan container
- [ ] Service terbukti melakukan DNS resolution & load balancing ke Pod yang
      berbeda-beda
- [ ] Ingress terbukti meneruskan traffic dari luar cluster sampai ke Pod
- [ ] Self-healing pod dibuktikan dengan `kubectl delete pod`, dengan traffic
      berjalan bersamaan (bukan cuma lihat Pod baru muncul, tapi juga efeknya
      ke traffic)
- [ ] Ada perbandingan eksplisit dengan hasil Level 2 di README

## Constraints
- Pakai k3d (bukan kind) — sesuai keputusan awal
- Image WAJIB di-import manual ke cluster (`k3d image import`), jangan push ke
  registry publik (di luar scope, dan tidak perlu untuk lab lokal)
- `imagePullPolicy: IfNotPresent` wajib ada di Deployment, kalau tidak Pod akan
  gagal pull image yang tidak exist di registry manapun

## Out of Scope (jangan dikerjakan di task ini)
- Resource requests/limits di manifest — itu Level 4
- HPA / autoscaling — itu Level 5
- Prometheus/Grafana — nanti
- Redis/Postgres — itu Level 6