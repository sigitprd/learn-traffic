# Level 4 — Resource Requests & Limits

Nambahin `resources.requests`/`resources.limits` ke Deployment `go-api`
(lanjutan Level 3), install `metrics-server`, lalu buktikan efeknya lewat
eksperimen: scheduling, CPU throttling, dan OOMKill.

## Menjalankan

```bash
# reuse cluster + service/ingress dari Level 3
k3d cluster create -c ../level-3/cluster/k3d-config.yaml
k3d image import go-api:v1 -c go-api-lab
kubectl apply -f ../level-3/manifests/service.yaml
kubectl apply -f ../level-3/manifests/ingress.yaml

kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
kubectl -n kube-system patch deployment metrics-server --type='json' \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'

kubectl apply -f manifests/deployment.yaml
```

Catatan: `manifests/deployment.yaml` di level ini adalah versi cluster BARU
(cluster Level 3 sudah di-`k3d cluster delete` setelah dokumentasi selesai),
jadi apply pertama = `Deployment` fresh create, bukan rolling update dari
Deployment lama yang masih hidup. Perilaku rolling update (Pod lama
`Terminating` barengan Pod baru `ContainerCreating`) tetap berlaku persis
sama kalau kamu apply manifest ini ke Deployment `go-api` Level 3 yang masih
hidup — cuma di run ini tidak ada Deployment sebelumnya buat dibandingkan.

---

## 1. metrics-server

```
[Install + patch + verifikasi]
Command: kubectl apply -f https://.../components.yaml
         kubectl -n kube-system patch deployment metrics-server --type='json' \
           -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'
         kubectl -n kube-system get pods -l k8s-app=metrics-server
Output: metrics-server-75dc6f8d87-d44ns   1/1   Running
Kesimpulan: patch --kubelet-insecure-tls WAJIB di k3d karena kubelet pakai
sertifikat self-signed, metrics-server default nolak koneksi TLS yang gak
dia percaya tanpa flag ini.
```

```
[kubectl top nodes]
Command: kubectl top nodes
Output:
NAME                      CPU(cores)   CPU%   MEMORY(bytes)   MEMORY%
k3d-go-api-lab-agent-0    48m          1%     208Mi           5%
k3d-go-api-lab-agent-1    57m          1%     188Mi           4%
k3d-go-api-lab-server-0   187m         4%     673Mi           17%
Kesimpulan: metrics-server jalan, angka nyata keluar (butuh ~15 detik
setelah Pod Running buat metrics ke-populate pertama kali).
```

## 2. Requests & Limits di Deployment

```
[Apply + verifikasi Pod tetap Running]
Command: kubectl apply -f manifests/deployment.yaml
         kubectl get pods -o wide
Output: 3 Pod, semua 1/1 Running dalam ~13 detik
```

```
[describe pod - Requests & Limits muncul]
Command: kubectl describe pod go-api-795c585cdf-9skrv
Output (potongan):
    Limits:
      cpu:     500m
      memory:  128Mi
    Requests:
      cpu:      100m
      memory:   64Mi
Kesimpulan: config resources ke-apply persis sesuai manifest.
```

## 3. Efek `requests` ke scheduling (eksperimen penting)

```
[Kapasitas node]
Command: kubectl describe node k3d-go-api-lab-agent-0
Output (Allocatable): cpu: 4, memory: 4026732Ki
Kesimpulan: tiap node k3d (container Docker di host yang sama) melaporkan
allocatable 4 CPU core — jadi request CPU harus > 4000m buat pasti gagal
di SEMUA node (dipakai 8000m di eksperimen ini biar aman).
```

```
[Deploy Pod dengan requests.cpu sengaja kebesaran]
Command: kubectl apply -f test-scheduling.yaml   (requests.cpu: 8000m)
         kubectl get pods -l app=test-scheduling
Output: test-scheduling-6997584b76-s5npp   0/1   Pending

Command: kubectl describe pod test-scheduling-6997584b76-s5npp   (section Events)
Output:
  Warning  FailedScheduling  default-scheduler
  0/3 nodes are available: 3 Insufficient cpu. no new claims to deallocate,
  preemption: 0/3 nodes are available: 3 Preemption is not helpful for scheduling.

Command: kubectl delete -f test-scheduling.yaml   (cleanup, tidak dibiarkan nyangkut)
Output: deployment.apps "test-scheduling" deleted

Kesimpulan: `requests` BUKAN cuma metadata dokumentasi — scheduler BENERAN
pakai angka ini buat cek "apakah node punya slot cukup" sebelum nge-bind
Pod ke node manapun. Kalau gak ada node yang cukup, Pod nyangkut `Pending`
selamanya (gak pernah crash, karena memang belum pernah dijalankan sama
sekali) sampai ada node dengan kapasitas cukup atau requests-nya diturunkan.
```

## 4. CPU throttling di bawah beban

```
[Load ke /cpu-intensive lewat Ingress, sample kubectl top pods]
Command: (30 wave x 15 curl paralel ke localhost:8080/cpu-intensive, ~60 detik total)
         kubectl top pods   (diambil tiap ~15 detik)
Output:
  t=5s:  9skrv=85m   f6t67=69m   xf4f9=75m
  t=20s: 9skrv=271m  f6t67=368m  xf4f9=108m
  t=35s: 9skrv=271m  f6t67=302m  xf4f9=439m
  t=50s: 9skrv=71m   f6t67=2m    xf4f9=1m    (load mulai reda)

Kesimpulan: usage naik mendekati `limits.cpu: 500m` (368m, 439m) TAPI TIDAK
PERNAH melewatinya — persis pola Level 1 yang mentok di ~50% waktu
`--cpus="0.5"` (sama-sama cgroup CPU quota, cuma beda cara declare: Docker
flag vs Kubernetes resources.limits). Pod tetap `Running`, `RESTARTS: 0` -
CPU limit TIDAK bikin Pod mati, cuma "dipagerin" kecepatannya.
```

```
[Bukti throttling langsung — kubectl biasa TIDAK expose ini]
Command: `kubectl describe pod` / `kubectl top` TIDAK punya kolom throttling
         eksplisit — throttling itu "silent" dari sudut pandang kubectl biasa
         (Pod tetap Running, Restart Count tetap 0, gak ada Event/Warning).
         Buat lihat buktinya beneran, harus gali ke cgroup langsung:

         docker exec k3d-go-api-lab-server-0 crictl ps | grep go-api
         docker exec k3d-go-api-lab-server-0 find /sys/fs/cgroup -iname "*<container-id>*"
         docker exec k3d-go-api-lab-server-0 cat ".../cpu.stat"
Output:
usage_usec 10245643
nr_periods 292
nr_throttled 198
throttled_usec 38234561
Kesimpulan: dari 292 periode CPU (tiap periode defaultnya 100ms), 198 di
antaranya (~68%) kena THROTTLE oleh kernel cgroup - total 38.2 detik waktu
proses "dipaksa nunggu" karena udah pakai jatah CPU-nya di periode itu.
Ini bukti definitif CPU limit di-enforce di level kernel (cgroup v2 CFS
bandwidth control), bukan asumsi dari angka `kubectl top` doang.
```

## 5. OOMKilled (dibandingkan dengan Level 1)

Endpoint baru `GET /memory-hog?mb=200` (default 200MB) — alokasi slice besar,
tiap byte disentuh biar beneran ke-commit (bukan cuma virtual alloc), dan
referensinya ditahan di variable global biar gak langsung di-GC.

```
[Hit /memory-hog LANGSUNG ke 1 Pod (port-forward, BUKAN lewat Service)]
Command: kubectl port-forward pod/go-api-795c585cdf-xf4f9 18080:8080
         curl --max-time 10 "localhost:18080/memory-hog?mb=200"
Output: curl exit code 52 (empty reply) - koneksi putus di tengah, proses
        kena OOM-kill SEBELUM sempat kirim response balik.

Command: kubectl get pods
Output:
NAME                      READY   RESTARTS
go-api-795c585cdf-xf4f9   1/1     1 (10s ago)   ← restart count naik dari 0 ke 1

Command: kubectl describe pod go-api-795c585cdf-xf4f9
Output (potongan):
    State:          Running
      Started:      ...21:37:04...
    Last State:     Terminated
      Reason:       OOMKilled
      Exit Code:    137
      Finished:     ...21:37:03...
    Restart Count:  1

Kesimpulan: alokasi 200MB > `limits.memory: 128Mi` → kernel OOM-killer
BUNUH proses (exit code 137 = SIGKILL akibat OOM, bukan graceful exit).
Container di dalam Pod yang sama otomatis di-restart oleh kubelet
(`restartPolicy: Always` default), Pod tetap ada dengan nama sama, cuma
Restart Count naik.
```

**Kenapa endpoint ini gak pernah dihit lewat Service/Ingress**: kalau lewat
Service, load balancer bisa nyasarin request ke Pod mana aja secara acak —
bisa aja 3 Pod sekaligus kena OOM cuma dari 1 request yang niatnya cuma
mau nes tes 1 instance. Makanya WAJIB `port-forward` (atau `kubectl exec`
curl dari dalam Pod itu sendiri) biar target-nya pasti 1 Pod tertentu.

**Perbandingan dengan Level 1**: di Level 1, `--memory="128m"` juga dipasang
di eksperimen resource limit, tapi app-nya (waktu itu) gak punya cara buat
sengaja makan memory besar — jadi limit itu ADA tapi gak pernah kena
(`OOMKilled: false` di `docker inspect`). Baru di Level 4 ini, dengan
endpoint `/memory-hog` yang sengaja dibikin, OOMKill beneran kejadian
dan kelihatan detilnya (exit code, reason, restart count) — Level 1 cuma
membuktikan limit-nya ADA, Level 4 membuktikan limit-nya BENERAN DI-ENFORCE.

---

## Ringkasan: 3 skenario, 3 akibat beda

| Skenario | Yang terjadi | Status Pod | Bisa dilihat lewat |
|---|---|---|---|
| `requests` Pod > kapasitas node manapun | Scheduler nolak nempatkan Pod ke node manapun | **Pending** selamanya (gak pernah `Running`) | `kubectl get pods` (status Pending), `kubectl describe pod` → Events: `FailedScheduling`, `Insufficient cpu` |
| Pemakaian CPU nyata > `limits.cpu` | Kernel cgroup THROTTLE proses (dikasih jatah CPU lebih dikit per periode) | Tetap **Running**, restart count TIDAK naik | `kubectl top pods` (usage mentok di limit, gak pernah lewat), atau gali `cpu.stat` di cgroup node (`nr_throttled`, `throttled_usec`) — TIDAK ada indikator langsung di `kubectl describe` biasa |
| Pemakaian memory nyata > `limits.memory` | Kernel OOM-killer BUNUH proses (SIGKILL) | Sempat **Terminated**, lalu **auto-restart** (Restart Count naik) | `kubectl get pods` (RESTARTS bertambah), `kubectl describe pod` → `Last State: Terminated, Reason: OOMKilled, Exit Code: 137` |

**Insight kunci buat Level 5 (HPA)**: HPA berbasis CPU ngitung persentase
dari `requests.cpu` (bukan `limits.cpu`). Di lab ini, `requests.cpu: 100m`
— jadi kalau HPA nanti diset target 70%, itu artinya trigger scale-up di
sekitar 70m usage (70% dari 100m), BUKAN 70% dari limit 500m (yang berarti
350m). Sering disalahpahami karena namanya "CPU utilization %" tapi basis
hitungnya `requests`, bukan `limits`.

## Cleanup

```bash
k3d cluster delete go-api-lab
```

(`test-scheduling.yaml` sudah dihapus dari cluster segera setelah dibuktikan
Pending — tidak dibiarkan nyangkut. File manifest-nya sendiri disimpan di
folder ini sebagai referensi, bukan bagian dari deployment aktif.)
