# Guide 03 — Kubernetes Dasar (Deployment → Pod → Service → Ingress)

Deploy `go-api:v1` ke cluster k3d lokal, buktikan Pod ≠ Container, buktikan
Service load balance, buktikan Ingress meneruskan traffic dari luar, dan
buktikan self-healing lewat `kubectl delete pod`.

**Waktu**: ~40 menit
**Prasyarat**:
- Level 1 selesai — image `go-api:v1` harus ada: `docker images go-api:v1`
- `k3d version` — kalau belum ada: `brew install k3d` (Mac)
- `kubectl version --client`
- Level 2 TIDAK wajib diulang — Level 3 gak depend ke NGINX/Compose

Konsep detail ada di [`level-3/README.md`](../level-3/README.md).

---

### Langkah 1 — Buat cluster + import image

k3d nyoba pull image dari registry publik secara default — image lokal
HARUS di-import manual, atau Pod bakal `ImagePullBackOff`.

```bash
cd level-3
k3d cluster create -c cluster/k3d-config.yaml
kubectl get nodes -o wide
```

**Expected output** (tunggu ~20-25 detik sampai cluster selesai dibuat):
```
NAME                      STATUS   ROLES           VERSION
k3d-go-api-lab-agent-0    Ready    <none>          v1.35.5+k3s1
k3d-go-api-lab-agent-1    Ready    <none>          v1.35.5+k3s1
k3d-go-api-lab-server-0   Ready    control-plane   v1.35.5+k3s1
```
(1 server + 2 agent, semua `Ready`. Versi k3s bisa beda tergantung rilis
terbaru saat kamu jalankan.)

```bash
k3d image import go-api:v1 -c go-api-lab
```

**Expected output:**
```
Successfully imported 1 image(s) into 1 cluster(s)
```

### Langkah 2 — Deploy dan verifikasi

```bash
kubectl apply -f manifests/deployment.yaml
kubectl get pods -o wide
```

Tunggu ~15-20 detik sampai semua `1/1 Running` (ada `readinessProbe` yang
nunggu 2 detik + periodic check). Kalau mau nunggu otomatis:
```bash
kubectl rollout status deployment/go-api --timeout=60s
```

**Expected output:**
```
NAME                      READY   STATUS    NODE
go-api-586f49bd6b-24d5l   1/1     Running   k3d-go-api-lab-agent-1
go-api-586f49bd6b-hd679   1/1     Running   k3d-go-api-lab-server-0
go-api-586f49bd6b-ljbbv   1/1     Running   k3d-go-api-lab-agent-0
```

3 Pod, biasanya tersebar 1 per node (termasuk `server-0` — k3s gak taint
control plane secara default, beda dari `kubeadm`).

### Langkah 3 — Buktikan Pod ≠ Container

Ambil salah satu nama Pod dari `kubectl get pods` (punyamu, ganti di
command di bawah).

```bash
POD=<nama-pod-kamu>
kubectl exec -it $POD -- hostname
```

**Expected output:** persis sama dengan nama Pod (Kubernetes set hostname
container = nama Pod).

```bash
kubectl describe pod $POD | sed -n '/^Containers:/,/^Conditions:/p'
```

**Expected output** (potongan penting — catat `Container ID`):
```
Containers:
  go-api:
    Container ID:  containerd://038e4a4239bb...
    Image:         go-api:v1
```

Sekarang buktikan `docker ps` di HOST TIDAK bisa lihat Pod ini langsung —
harus lewat `crictl` DI DALAM node:

```bash
docker ps --format 'table {{.Names}}\t{{.Image}}' | grep k3d
```

**Expected output** (cuma 4 node container k3d, BUKAN Pod go-api):
```
k3d-go-api-lab-serverlb   ghcr.io/k3d-io/k3d-proxy:5.9.0
k3d-go-api-lab-agent-1    rancher/k3s:v1.35.5-k3s1
k3d-go-api-lab-agent-0    rancher/k3s:v1.35.5-k3s1
k3d-go-api-lab-server-0   rancher/k3s:v1.35.5-k3s1
```

```bash
NODE=$(kubectl get pod $POD -o jsonpath='{.spec.nodeName}')
docker exec $NODE crictl ps | grep go-api
```

**Expected output** (Container ID di kolom pertama harus PERSIS sama
dengan `Container ID` dari `kubectl describe pod` di atas — cocokkan
sendiri):
```
038e4a4239bb5   b772ff86f531c   Running   go-api   26db39a43f404   go-api-586f49bd6b-24d5l
```

Kolom "POD ID" (`26db39a43f404`) BEDA dari container ID — itu sandbox
network Pod, bukti Pod adalah abstraksi di atas container, bukan sama
dengan container.

### Langkah 4 — Service: DNS resolution + load balancing

```bash
kubectl apply -f manifests/service.yaml
kubectl exec $POD -- wget -qO- http://go-api-service/health
```

**Expected output:**
```
{"status":"ok"}
```

Buktikan load balance ke Pod berbeda-beda. Pakai `--restart=Never` — tanpa
ini, `kubectl run` default `restartPolicy: Always`, jadi walau container
sukses selesai (`exit 0`), kubelet restart terus container-nya, pod gak
pernah dianggap "final", dan `--rm` yang nunggu pod beres jadi timeout:

```bash
kubectl run tmp-curl --image=busybox --restart=Never --rm -i --command -- sh -c '
for i in $(seq 1 15); do wget -qO- http://go-api-service/whoami; echo; done
'
```

**Expected output** (15 baris hostname — HARUS ada minimal 2-3 hostname
BEDA, distribusinya BOLEH gak serapi round robin NGINX Level 2):
```
{"hostname":"go-api-586f49bd6b-ljbbv"}
{"hostname":"go-api-586f49bd6b-24d5l"}
{"hostname":"go-api-586f49bd6b-hd679"}
... (campuran 3 hostname, gak harus urutan rapi)
```

### Langkah 5 — Ingress: akses dari luar cluster

k3d bundle Traefik sebagai Ingress Controller default — dipakai apa adanya
di guide ini (bukan install NGINX Ingress Controller terpisah).

```bash
kubectl apply -f manifests/ingress.yaml
sleep 3
curl -s -w "\nHTTP:%{http_code}\n" localhost:8080/health
curl -s localhost:8080/api/users
```

**Expected output:**
```
{"status":"ok"}
HTTP:200
[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"},{"id":3,"name":"Charlie"}]
```

Kalau ini berhasil, request kamu barusan lewat rantai PENUH: host →
k3d loadbalancer → Traefik → Service → Pod.

### Langkah 6 — Self-healing

Catat baseline dulu:
```bash
kubectl get pods -o wide
```

Pilih salah satu Pod, jalankan traffic loop di background, lalu hapus Pod
itu:
```bash
POD_TO_KILL=<salah-satu-nama-pod>
(for i in $(seq 1 40); do curl -s -o /dev/null -w "%{http_code}\n" --max-time 1 localhost:8080/health; sleep 0.5; done) > /tmp/traffic.log &
sleep 2
kubectl delete pod $POD_TO_KILL
wait
```

Sambil nunggu, pantau di terminal lain (opsional):
```bash
kubectl get pods --watch
```
(tekan Ctrl+C buat keluar dari watch mode)

**Expected output** `kubectl get pods` (Pod baru muncul dalam <1 detik,
NAMA BEDA dari yang dihapus, full Ready dalam ~3 detik):
```
NAME                      READY   STATUS              RESTARTS   AGE
go-api-586f49bd6b-24d5l   1/1     Running             0          2m
go-api-586f49bd6b-hd679   1/1     Running             0          2m
go-api-586f49bd6b-5mbpg   1/1     Running             0          8s   ← Pod BARU, nama beda
```

**Expected output traffic log** (semua HTTP 200, 0 gagal):
```bash
grep -vc "^200$" /tmp/traffic.log   # harus keluar 0
```

---

## Apa yang barusan terjadi

Kamu baru saja membuktikan **self-healing** — beda fundamental dari Level 2
(NGINX). Di Level 2, `api-2` yang di-`docker stop` TETAP MATI sampai kamu
`docker start` manual. Di sini, `kubectl delete pod` justru bikin
ReplicaSet controller LANGSUNG bikin Pod pengganti (nama beda, karena Pod
itu disposable) tanpa perintah tambahan apapun — Deployment terus-menerus
"mengawasi" jumlah Pod aktual vs `replicas: 3` yang diinginkan (declarative
reconciliation). Ini pola yang sama persis akan dipakai HPA di Level 5
buat naik-turunin jumlah Pod otomatis berdasarkan beban.

## Troubleshooting

| Gejala | Kemungkinan penyebab | Cara cek / perbaiki |
|---|---|---|
| `kubectl get pods` nunjukin `ImagePullBackOff` | Image `go-api:v1` belum di-`k3d image import` ke cluster ini, atau nama image di manifest typo | `kubectl describe pod <nama>` cek section Events; `k3d image import go-api:v1 -c go-api-lab` ulang |
| `kubectl` error `couldn't get resource list for metrics.k8s.io` | metrics-server belum diinstall (baru ada di Level 4) | HARMLESS di level ini, abaikan — bukan tanda kegagalan |
| `k3d cluster create` gagal, port 8080 sudah dipakai | Level 2 (Docker Compose) masih jalan dan pegang port 8080 | `docker compose down` di folder `level-2/` dulu sebelum buat cluster |
| Pod stuck `Pending` terus (bukan `Running`) | Node belum Ready, atau resource node abis (jarang di lab baru) | `kubectl get nodes` cek semua `Ready`; `kubectl describe pod <nama>` cek Events |
| `crictl ps` di Langkah 3 gak nemu container `go-api` | Salah nama node (Pod-nya jalan di node lain) | Cek `kubectl get pod $POD -o jsonpath='{.spec.nodeName}'` dulu buat tau node yang benar |
| Langkah 4: `kubectl run tmp-curl ...` keluar `pod "tmp-curl" deleted` lalu `error: timed out waiting for the condition`, tanpa output hostname (atau `kubectl get events` nunjukin `BackOff restarting failed container`) | `kubectl run` default `restartPolicy: Always` — container yang udah `exit 0` sukses tetap direstart kubelet, pod gak pernah "final", `--rm` nunggu selesai jadi timeout | Tambah flag `--restart=Never` di command `kubectl run` |
| `curl localhost:8080` di Langkah 5 gagal padahal Ingress sudah di-apply | Traefik belum selesai reload config, atau belum ready | Tunggu 3-5 detik lagi, cek `kubectl get pods -n kube-system | grep traefik` harus `Running` |
| Self-healing di Langkah 6: ada request yang GAGAL (bukan 0) | Kebetulan kena race condition kecil antara Pod terminate vs Service update endpoint (LEBIH SERING muncul di Level 6 yang ada state, jarang tapi mungkin di sini) | Ulangi eksperimen sekali lagi; kalau konsisten gagal banyak, cek `readinessProbe` di manifest masih ada |

## Cleanup

```bash
k3d cluster delete go-api-lab
```

Ini juga otomatis hapus semua Deployment/Service/Ingress di dalamnya —
gak perlu `kubectl delete` satu-satu.

Verifikasi manual gak ada sisa (semua output di bawah HARUS kosong):

```bash
k3d cluster list
docker ps -a | grep k3d
docker network ls | grep k3d
docker volume ls | grep k3d
kubectl config get-contexts | grep go-api-lab
```

Lanjut ke [Guide 04 — Resource Limits](04-guide-resource-limits.md).
