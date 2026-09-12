# Level 3 — Kubernetes (Deployment → Pod → Service → Ingress)

Deploy `go-api:v1` (image sama dari Level 1/2, sudah punya `/health`,
`/api/users`, `/whoami`, `/stats`, `/cpu-intensive`) ke cluster k3d lokal.

```
   curl/k6 traffic
          │
          ▼
      Ingress (Traefik, bawaan k3d)
          │
          ▼
       Service (ClusterIP)
          │
   ┌──────┼──────┐
   ▼      ▼      ▼
  Pod    Pod    Pod
  (go-api:v1, x3, dikelola 1 Deployment)
```

## Menjalankan dari nol

```bash
k3d cluster create -c cluster/k3d-config.yaml
k3d image import go-api:v1 -c go-api-lab
kubectl apply -f manifests/deployment.yaml
kubectl apply -f manifests/service.yaml
kubectl apply -f manifests/ingress.yaml
curl localhost:8080/health
```

---

## 1. Setup cluster

```
[Cluster create + node check]
Command: k3d cluster create -c cluster/k3d-config.yaml
         kubectl get nodes -o wide
Output:
NAME                      STATUS   ROLES           VERSION
k3d-go-api-lab-agent-0    Ready    <none>          v1.35.5+k3s1
k3d-go-api-lab-agent-1    Ready    <none>          v1.35.5+k3s1
k3d-go-api-lab-server-0   Ready    control-plane   v1.35.5+k3s1
Kesimpulan: 1 server + 2 agent, semua Ready. Server node di k3s TIDAK
di-taint NoSchedule secara default (beda dari kubeadm), jadi Pod bisa
kejadwal di server juga — kelihatan di eksperimen poin 2.
```

```
[Import image lokal]
Command: k3d image import go-api:v1 -c go-api-lab
Output: "Successfully imported 1 image(s) into 1 cluster(s)"
Kesimpulan: image go-api:v1 dari Docker daemon host di-pack jadi tarball
dan di-load ke containerd di tiap node k3d. TANPA langkah ini, Pod bakal
ImagePullBackOff karena k3d nyoba pull "go-api:v1" dari Docker Hub (yang
tidak ada di sana).
```

## 2. Deployment

`manifests/deployment.yaml` — 3 replica, `imagePullPolicy: IfNotPresent`,
`readinessProbe` + `livenessProbe` ke `/health`.

```
[Apply + verifikasi]
Command: kubectl apply -f manifests/deployment.yaml
         kubectl get pods -o wide
Output:
NAME                      READY   STATUS    NODE
go-api-586f49bd6b-24d5l   1/1     Running   k3d-go-api-lab-agent-1
go-api-586f49bd6b-hd679   1/1     Running   k3d-go-api-lab-server-0
go-api-586f49bd6b-ljbbv   1/1     Running   k3d-go-api-lab-agent-0
Kesimpulan: Scheduler nyebar 1 Pod per node (3 Pod, 3 node berbeda,
termasuk server) — default scheduling behavior nyoba spread demi
availability, bukan numpuk di 1 node.
```

**readinessProbe vs livenessProbe** (konsep baru, tidak ada di Level 1/2):
- **readinessProbe**: nentuin apakah Pod BOLEH nerima traffic dari Service.
  Kalau gagal, Pod tetap `Running` tapi ditarik dari daftar endpoint Service
  (gak dapet traffic lagi) sampai probe sukses lagi. Dipakai buat kasus
  "container hidup tapi belum siap" (misal masih loading data).
- **livenessProbe**: nentuin apakah container harus di-RESTART. Kalau gagal
  terus (`failureThreshold` kelampaui), kubelet bunuh & restart container itu
  di dalam Pod yang sama.
- Analoginya ke Level 1: ini kayak `docker run --health-cmd` tapi Kubernetes
  yang ambil aksi otomatis (readiness = keluar dari load balancing, liveness
  = restart), bukan cuma status yang keliatan di `docker ps`.

## 3. Pod ≠ Container (eksperimen konkret)

```
[hostname dalam Pod vs nama Pod]
Command: kubectl exec go-api-586f49bd6b-24d5l -- hostname
Output: go-api-586f49bd6b-24d5l
Kesimpulan: Kubernetes set hostname container = nama Pod. Kelihatan sama,
tapi ini konsekuensi desain Pod (unit jaringan/identitas), bukan berarti
Pod = container.
```

```
[describe pod - section Containers]
Command: kubectl describe pod go-api-586f49bd6b-24d5l
Output (potongan):
Containers:
  go-api:
    Container ID:  containerd://038e4a4239bb56d911b8fbe82779920585e3632a3ba1af13024642e57d3c4cb1
    Image:         go-api:v1
    Liveness:      http-get http://:8080/health delay=5s period=10s #failure=3
    Readiness:     http-get http://:8080/health delay=2s period=5s #failure=3
Kesimpulan: 1 Pod di lab ini = 1 container. TAPI secara desain, Pod BISA
isi lebih dari 1 container (sidecar pattern - misal container utama +
container logging agent + container proxy, semua share network namespace
& bisa saling komunikasi lewat localhost). Pod adalah unit terkecil yang
di-scheduling Kubernetes, bukan unit terkecil runtime container.
```

```
[docker ps di HOST vs crictl ps di DALAM node]
Command: docker ps --format 'table {{.Names}}\t{{.Image}}'   (di host)
Output:
k3d-go-api-lab-serverlb   ghcr.io/k3d-io/k3d-proxy:5.9.0
k3d-go-api-lab-agent-1    rancher/k3s:v1.35.5-k3s1
k3d-go-api-lab-agent-0    rancher/k3s:v1.35.5-k3s1
k3d-go-api-lab-server-0   rancher/k3s:v1.35.5-k3s1
Kesimpulan #1: `docker ps` di host CUMA nunjukin 4 container node k3d
(yang masing-masing adalah "VM" k3s), TIDAK nunjukin Pod go-api sama
sekali — karena k3d nodes itu sendiri docker container yang di dalamnya
jalan containerd terpisah, bukan berbagi docker daemon host.

Command: docker exec k3d-go-api-lab-agent-1 crictl ps
Output:
CONTAINER       IMAGE           STATE     NAME     POD ID          POD
038e4a4239bb5   b772ff86f531c   Running   go-api   26db39a43f404   go-api-586f49bd6b-24d5l
Kesimpulan #2: container ID 038e4a4239bb5 PERSIS sama dengan yang muncul
di `kubectl describe pod` — jadi ketemu, tapi HARUS lewat containerd
(crictl) di dalam node, bukan `docker ps` biasa. Ada juga "POD ID"
(26db39a43f404) terpisah dari container ID — itu sandbox/pause container
yang megang network namespace Pod. Ini bukti nyata: Pod = abstraksi
Kubernetes (sandbox + 1/lebih container di dalamnya), BUKAN unit yang
dikenal langsung oleh container engine sebagai satu kesatuan.
```

## 4. Service

```
[DNS resolution dari dalam Pod]
Command: kubectl exec go-api-586f49bd6b-24d5l -- wget -qO- http://go-api-service/health
Output: {"status":"ok"}
Kesimpulan: nama Service (`go-api-service`) resolve otomatis lewat CoreDNS
cluster-internal, mirip Docker DNS di custom network Level 2, tapi di
layer Kubernetes (Service, bukan container name langsung).
```

```
[Load balancing ke Pod berbeda - 15x hit /whoami]
Command: kubectl run tmp-curl --image=busybox --rm -i --command -- sh -c \
         'for i in $(seq 1 15); do wget -qO- http://go-api-service/whoami; echo; done'
Output (urutan hostname):
ljbbv, 24d5l, hd679, hd679, 24d5l, 24d5l, 24d5l, 24d5l, ljbbv, hd679, 24d5l, 24d5l, 24d5l, ljbbv, hd679
Distribusi: 24d5l=8x, hd679=4x, ljbbv=3x  (dari 15 request)
Kesimpulan: Service TERBUKTI load-balance ke 3 Pod berbeda — tapi
distribusinya TIDAK serapi NGINX round robin di Level 2. kube-proxy mode
default (iptables) pakai random probabilistic selection per-koneksi,
bukan round robin bergilir murni seperti upstream NGINX. Jumlah sample
kecil (15) juga bikin ketimpangan lebih kelihatan.
```

## 5. Ingress

k3d cluster ini pakai **Traefik** (bundled default k3d), BUKAN NGINX Ingress
Controller yang diminta task secara eksplisit — pakai opsi yang task izinkan
("pakai Traefik bawaan k3d dan catat perbedaannya"). Alasan: instalasi NGINX
Ingress Controller manual butuh manifest tambahan (Deployment+Service+RBAC)
di luar scope inti lab ini, dan Traefik sudah cukup buat buktikan konsep
routing Ingress → Service → Pod yang jadi tujuan Level 3.

Perbedaan yang relevan:
- Konsep **IngressClass** dan `ingressClassName: traefik` di manifest ini
  persis sama strukturnya kalau pakai NGINX (`ingressClassName: nginx`) —
  API Ingress-nya standar Kubernetes, cuma controller di baliknya beda.
- Annotation berbeda: Traefik pakai `traefik.ingress.kubernetes.io/*`, NGINX
  pakai `nginx.ingress.kubernetes.io/*`. Di lab ini cuma dipakai 1 annotation
  ringan buat entrypoint, gak ada fitur advanced yang dites.
- Traefik jalan sebagai Deployment + ada `svclb-traefik` (k3d ServiceLB) yang
  ekspos ke node port — inilah yang di-map k3d loadbalancer container ke
  `localhost:8080` host.

```
[Full chain test: host → Ingress → Service → Pod]
Command: curl -s -w "\nHTTP:%{http_code}\n" localhost:8080/health
         curl -s localhost:8080/api/users
Output:
{"status":"ok"}
HTTP:200
[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"},{"id":3,"name":"Charlie"}]
Kesimpulan: request dari HOST (localhost:8080, port yang di-mapping k3d
config ke loadbalancer) berhasil nyampe ke Pod lewat rantai penuh:
k3d loadbalancer container → Traefik Ingress Controller → Service
(ClusterIP) → salah satu Pod. 4 layer network berbeda, transparan.
```

## 6. Self-healing (dibandingkan dengan Level 2)

```
[Baseline]
Command: kubectl get pods -o wide
Output:
go-api-586f49bd6b-24d5l   Running   agent-1   age 100s
go-api-586f49bd6b-hd679   Running   server-0  age 100s
go-api-586f49bd6b-ljbbv   Running   agent-0   age 100s
```

```
[Delete pod di tengah traffic (setara "docker stop api-2" di Level 2)]
Command:
  # traffic loop di background: curl localhost:8080/health tiap 0.5s, 40x (20 detik)
  kubectl delete pod go-api-586f49bd6b-ljbbv   # dieksekusi di detik ke-2 dari traffic loop

Timeline dari kubectl get pods (sample tiap ~1 detik):
  t+0.6s : go-api-...-5mbpg muncul, STATUS=ContainerCreating, age=0s   ← Pod baru langsung dibuat
  t+1.7s : go-api-...-5mbpg STATUS=Running (belum Ready)
  t+3.0s : go-api-...-5mbpg READY=1/1                                  ← siap terima traffic lagi
  (pod ljbbv hilang total dari daftar sejak sample pertama - tidak ada
   status "Terminating" yang sempat ke-capture, kemungkinan graceful
   shutdown-nya cepat karena tidak ada active connection yang di-drain lama)

Output traffic log (40 request, interval 0.5s, span ~20 detik menutupi
seluruh proses delete → replacement → ready):
  40/40 request = HTTP 200
  0 request gagal

Kesimpulan:
- Pod pengganti (nama BEDA: ljbbv → 5mbpg, sesuai ekspektasi - Pod itu
  disposable, ReplicaSet controller bikin identitas baru, bukan restart
  Pod lama) muncul dalam <1 detik, dan full Ready (lolos readinessProbe)
  dalam ~3 detik sejak delete.
- SELAMA proses itu, traffic lewat Ingress 0% gagal - karena masih ada
  2 Pod lain yang tetap Ready dan Service otomatis exclude Pod yang lagi
  Terminating/belum Ready dari daftar endpoint-nya.
```

### Perbandingan eksplisit dengan Level 2 (NGINX manual + Docker Compose)

| Aspek | Level 2 (NGINX + Docker Compose) | Level 3 (Kubernetes) |
|---|---|---|
| Availability saat 1 instance mati | 0% error (NGINX retry ke backend hidup) | 0% error (Service exclude Pod yang belum Ready) |
| **Recovery instance yang mati** | **TIDAK otomatis** - `api-2` yang di-`docker stop` tetap mati sampai manual `docker start api-2` | **Otomatis** - ReplicaSet controller langsung bikin Pod baru (nama beda) buat kembalikan ke desired replica count (3) |
| Load balancing method | Round robin murni (rapi 1,2,3,1,2,3,...) | kube-proxy random per-koneksi (kurang rapi, tapi tetap ke-3 backend kena) |
| Yang jaga "jumlah instance ideal" | Tidak ada - Compose cuma start apa yang didefinisikan sekali di awal | Deployment controller terus-menerus reconcile actual state ke desired state (declarative) |
| Deteksi "instance siap nerima traffic" | Tidak ada healthcheck bawaan di config Level 2 | readinessProbe eksplisit, Pod ditarik dari Service kalau gagal |

**Insight utama**: Level 2 (NGINX) dan Level 3 (Kubernetes) SAMA-SAMA
menyerap kegagalan 1 instance tanpa request gagal (availability). Yang BEDA
secara fundamental: NGINX itu **pasif** — dia cuma re-route ke backend yang
masih hidup dari daftar yang SUDAH ADA, tidak pernah bikin backend baru.
Kubernetes itu **aktif** — Deployment terus mengawasi jumlah Pod actual vs
`replicas: 3` yang diinginkan, dan otomatis bikin Pod pengganti begitu ada
yang hilang. Ini persis definisi **self-healing**: sistem mengembalikan
dirinya sendiri ke desired state tanpa campur tangan manual.

---

## Ringkasan definisi (merujuk eksperimen di atas)

- **Container**: unit isolasi proses tunggal (dari Level 1). Di sini terlihat
  sebagai entry `038e4a4239bb5` di `crictl ps` — punya image, state, ID sendiri.
- **Pod**: abstraksi Kubernetes yang membungkus 1+ container yang share
  network namespace & lifecycle. Terbukti dari: `docker ps` di host TIDAK
  melihatnya, cuma `crictl ps` di dalam node yang melihat container-nya, dan
  ada "POD ID" (sandbox) terpisah dari container ID. Pod itu disposable
  (dibuktikan dari nama Pod baru yang beda pasca `kubectl delete pod`).
- **Deployment**: controller yang menjaga N replica Pod tetap hidup sesuai
  desired state, deklaratif. Terbukti dari eksperimen self-healing - begitu
  1 Pod dihapus, otomatis dibuatkan Pod baru tanpa perintah eksplisit lain.
- **Service**: alamat stabil + load balancer internal ke sekumpulan Pod yang
  cocok dengan `selector`. Terbukti dari DNS resolution (`go-api-service`)
  dan distribusi request ke 3 Pod berbeda lewat `/whoami`.
- **Ingress**: pintu masuk traffic dari LUAR cluster ke Service tertentu di
  DALAM cluster, lewat Ingress Controller (di sini Traefik). Terbukti dari
  `curl localhost:8080` di host berhasil menembus ke Pod lewat rantai
  Ingress → Service → Pod.

## Cleanup

```bash
k3d cluster delete go-api-lab
```
