# Guide 04 — Resource Requests & Limits

Install `metrics-server`, tambah `resources.requests`/`limits` ke
Deployment, buktikan efeknya ke scheduling (Pod `Pending`), CPU throttling,
dan OOMKill.

**Waktu**: ~35 menit
**Prasyarat**:
- Level 3 selesai (paham Deployment/Pod/Service dasar)
- Image `go-api:v1` ada, cluster k3d k3d bisa dipakai lagi (buat baru kalau
  sudah di-delete: `k3d cluster list`)

Konsep detail ada di [`level-4/README.md`](../level-4/README.md).

---

### Langkah 0 — Setup cluster (kalau belum ada dari Level 3)

```bash
cd level-3
k3d cluster create -c cluster/k3d-config.yaml
k3d image import go-api:v1 -c go-api-lab
kubectl apply -f manifests/service.yaml
kubectl apply -f manifests/ingress.yaml
cd ../level-4
```

### Langkah 1 — Install metrics-server

k3d butuh flag tambahan karena kubelet-nya pakai sertifikat self-signed.

```bash
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
kubectl -n kube-system patch deployment metrics-server --type='json' \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'
kubectl -n kube-system rollout status deployment/metrics-server --timeout=90s
```

**Expected output** (setelah `rollout status` selesai):
```
deployment "metrics-server" successfully rolled out
```

Tunggu ~15 detik lagi biar metrics sempat ke-populate, lalu cek:
```bash
kubectl top nodes
```

**Expected output** (angka NYATA, bukan error):
```
NAME                      CPU(cores)   CPU%   MEMORY(bytes)   MEMORY%
k3d-go-api-lab-agent-0    48m          1%     208Mi           5%
k3d-go-api-lab-agent-1    57m          1%     188Mi           4%
k3d-go-api-lab-server-0   187m         4%     673Mi           17%
```

Kalau masih keluar error `metrics not available yet`, tunggu 15 detik lagi
dan ulangi — JANGAN lanjut ke langkah berikutnya sebelum ini beres.

### Langkah 2 — Deploy dengan requests & limits

```bash
kubectl apply -f manifests/deployment.yaml
kubectl rollout status deployment/go-api --timeout=60s
```

**Expected output:** 3 Pod, semua `1/1 Running` dalam ~13 detik.

```bash
POD=$(kubectl get pods -l app=go-api -o jsonpath='{.items[0].metadata.name}')
kubectl describe pod $POD | grep -A5 "Limits:"
```

**Expected output:**
```
    Limits:
      cpu:     500m
      memory:  128Mi
    Requests:
      cpu:      100m
      memory:   64Mi
```

### Langkah 3 — Buktikan `requests` mempengaruhi scheduling

Cek dulu kapasitas node:
```bash
kubectl describe node k3d-go-api-lab-agent-0 | grep -A6 "Allocatable:"
```

**Expected output** (biasanya `cpu: 4` — kalau host kamu beda spek, angkanya
bisa beda, sesuaikan requests di langkah berikut supaya > allocatable):
```
Allocatable:
  cpu:                4
  memory:             4026732Ki
```

Deploy Pod dengan `requests.cpu` sengaja kebesaran (file `test-scheduling.yaml`
sudah ada di folder ini):
```bash
kubectl apply -f test-scheduling.yaml
sleep 5
kubectl get pods -l app=test-scheduling
```

**Expected output:**
```
NAME                               READY   STATUS    RESTARTS   AGE
test-scheduling-6997584b76-s5npp   0/1     Pending   0          4s
```

```bash
POD_PENDING=$(kubectl get pods -l app=test-scheduling -o jsonpath='{.items[0].metadata.name}')
kubectl describe pod $POD_PENDING | sed -n '/^Events:/,$p'
```

**Expected output:**
```
Events:
  Type     Reason            Age   From               Message
  ----     ------            ----  ----               -------
  Warning  FailedScheduling  8s    default-scheduler  0/3 nodes are available: 3 Insufficient cpu. ...
```

**WAJIB hapus lagi, jangan dibiarkan nyangkut:**
```bash
kubectl delete -f test-scheduling.yaml
```

### Langkah 4 — CPU throttling di bawah beban

```bash
for wave in $(seq 1 30); do
  for i in $(seq 1 15); do curl -s localhost:8080/cpu-intensive >/dev/null & done
  wait
done &
LOADPID=$!
sleep 20
kubectl top pods
sleep 15
kubectl top pods
wait $LOADPID
```

**Expected output** (2 sample `kubectl top pods` selama load jalan, CPU
mendekati `500m` limit TAPI TIDAK PERNAH melewatinya):
```
NAME                      CPU(cores)   MEMORY(bytes)
go-api-795c585cdf-9skrv   271m         3Mi
go-api-795c585cdf-f6t67   368m         3Mi
go-api-795c585cdf-xf4f9   108m         3Mi
```

```bash
kubectl get pods
```

**Expected output:** semua `RESTARTS: 0` — CPU limit TIDAK bikin Pod mati,
cuma dipagerin kecepatannya (silent throttling, gak ada Event/Warning yang
kelihatan).

### Langkah 5 — OOMKill

Endpoint `GET /memory-hog?mb=200` sengaja alokasi 200MB (> `limits.memory:
128Mi`). **WAJIB lewat `port-forward` ke 1 Pod spesifik — JANGAN lewat
Service**, biar cuma 1 Pod yang kena.

```bash
POD=$(kubectl get pods -l app=go-api -o jsonpath='{.items[0].metadata.name}')
kubectl port-forward pod/$POD 18080:8080 &
sleep 2
curl --max-time 10 "localhost:18080/memory-hog?mb=200"
```

**Expected output:** koneksi putus di tengah (curl exit code 52, empty
reply) — proses kena OOM-kill sebelum sempat kirim response.

```bash
kill %1 2>/dev/null   # matiin port-forward
sleep 3
kubectl get pod $POD
```

**Expected output:** `RESTARTS` naik dari 0 ke 1.
```
NAME                      READY   STATUS    RESTARTS      AGE
go-api-795c585cdf-xf4f9   1/1     Running   1 (10s ago)   ...
```

```bash
kubectl describe pod $POD | grep -A5 "Last State:"
```

**Expected output:**
```
    Last State:     Terminated
      Reason:       OOMKilled
      Exit Code:    137
```

---

## Apa yang barusan terjadi

Kamu baru saja membuktikan 3 hal yang sering disalahpahami tentang resource
management Kubernetes: (1) `requests` bukan cuma metadata — scheduler
BENERAN pakai angka ini buat cek kapasitas node sebelum nge-bind Pod
(kalau gak cukup, Pod nyangkut `Pending` selamanya, gak pernah crash karena
memang belum pernah dijalankan); (2) CPU limit yang terlampaui menghasilkan
THROTTLE (proses tetap hidup, cuma dipagerin kecepatannya, silent — gak ada
indikator langsung di `kubectl` biasa); (3) memory limit yang terlampaui
menghasilkan OOMKILL (proses beneran DIBUNUH kernel, exit code 137, lalu
auto-restart). Insight paling penting buat Level 5: HPA berbasis CPU
menghitung persentase dari `requests` (bukan `limits`) — jadi `requests.cpu:
100m` dengan target 70% berarti threshold scale-up di ~70m usage, BUKAN 70%
dari limit 500m.

## Troubleshooting

| Gejala | Kemungkinan penyebab | Cara cek / perbaiki |
|---|---|---|
| `kubectl top nodes`/`pods` tetap error setelah 1-2 menit | Patch `--kubelet-insecure-tls` gagal ke-apply, atau metrics-server pod belum Running | `kubectl -n kube-system get pods -l k8s-app=metrics-server` cek status; `kubectl -n kube-system get deployment metrics-server -o yaml \| grep insecure-tls` cek flag beneran ada |
| `kubectl apply -f https://.../components.yaml` gagal, no internet | Environment gak ada akses internet | Perlu koneksi internet buat langkah ini (metrics-server manifest di-fetch dari GitHub) |
| Langkah 3: Pod `test-scheduling` malah `Running` (bukan `Pending`) | `requests.cpu` di file kurang besar dibanding allocatable node kamu | Cek allocatable node dulu (`kubectl describe node ... \| grep -A2 Allocatable`), edit `test-scheduling.yaml` naikkan `requests.cpu` sampai jelas melebihi |
| Langkah 4: `kubectl top pods` CPU usage jauh di bawah limit terus | Load `for wave` sudah selesai sebelum sempat `kubectl top`, atau host kamu jauh lebih kencang dari environment testing asli | Jalankan `kubectl top pods` LEBIH CEPAT setelah loop mulai, atau tambah jumlah wave/paralel di command |
| Langkah 5: `curl` ke `/memory-hog` malah sukses dapat response normal | `limits.memory` di manifest gak ke-apply (cek ulang `describe pod`), atau `mb=200` gak cukup besar dibanding limit aktual | `kubectl describe pod $POD \| grep -A3 Limits`, pastikan `memory: 128Mi` ada; kalau limit lebih besar dari 128Mi, naikkan parameter `?mb=` |
| `kubectl port-forward` macet / gak connect | Port 18080 di host kamu udah dipakai proses lain | Ganti port lokal, misal `19080:8080`, sesuaikan command curl-nya |

## Cleanup

```bash
k3d cluster delete go-api-lab
```

Pastikan `test-scheduling.yaml` sudah di-`kubectl delete` SEBELUM cluster
dihapus (harusnya sudah dari Langkah 3) — kebiasaan baik biar gak ada
resource nyangkut kalau kamu lanjut ke level lain di cluster yang sama.

Verifikasi manual gak ada sisa (semua output di bawah HARUS kosong):

```bash
k3d cluster list
docker ps -a | grep k3d
docker network ls | grep k3d
docker volume ls | grep k3d
kubectl config get-contexts | grep go-api-lab
```

Lanjut ke [Guide 05 — HPA](05-guide-hpa.md).
