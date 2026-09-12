# Level 1 — Container

Go API sederhana: `GET /health`, `GET /api/users`, `GET /cpu-intensive` (busy-loop
500ms, buat simulasi beban CPU).

## Build & run dasar

```bash
docker build -t go-api:v1 .
docker run -d --name go-api -p 8080:8080 -e PORT=8080 \
  --cpus="0.5" --memory="128m" go-api:v1
```

```bash
curl localhost:8080/health
curl localhost:8080/api/users
```

## Konsep yang dipraktikkan

- **image** — hasil `docker build`, dari Dockerfile (multi-stage: `golang:1.22-alpine`
  buat build, `alpine:3.19` buat runtime)
- **container** — instance jalan dari image (`docker run`)
- **port mapping** — `-p HOST:CONTAINER`
- **environment variable** — `PORT`, dibaca lewat `os.Getenv`, fallback ke 8080
- **volume** — mount `/var/log` ke host, log persist lepas dari lifecycle container
- **network** — custom bridge network, DNS resolution antar container by name
- **resource limit** — `--cpus`, `--memory`, diverifikasi di bawah beban

---

## Hasil verifikasi

### 1. Ukuran image

```
[Ukuran image go-api:v1]
Command: docker images go-api:v1
Output:
REPOSITORY   TAG   IMAGE ID       CREATED         SIZE
go-api       v1    b3fede707ed6   17 seconds ago  14.7MB
Kesimpulan: 14.7MB, jauh di bawah 50MB. Dockerfile sudah multi-stage
(builder pakai golang:1.22-alpine, runtime pakai alpine:3.19 polos,
binary Go static CGO_ENABLED=0). Tidak perlu perbaikan.
```

```
[Perbandingan sebelum vs sesudah — kalau TIDAK multi-stage]
Command: docker build -f /tmp/Dockerfile.naive -t go-api:naive .   (Dockerfile
         sementara, base golang:1.22-alpine dipakai juga sebagai runtime,
         tanpa stage kedua — tidak disimpan di repo, cuma buat pembanding)
Output:
REPOSITORY   TAG      SIZE
go-api       naive    303MB
go-api       v1       14.7MB
Kesimpulan: multi-stage build motong ukuran image dari 303MB ke 14.7MB
(~20x lebih kecil) — toolchain Go, cache module, dan source code tidak
ikut ke image final, cuma binary statis yang dibawa.
(Image go-api:naive dihapus lagi setelah perbandingan, tidak dipakai lanjut.)
```

### 2. Environment variable `PORT`

```
[PORT di-override ke 9090]
Command: docker run -d --name go-api-port-test -p 9090:9090 -e PORT=9090 go-api:v1
         curl -s -w "\nHTTP:%{http_code}\n" localhost:9090/health
Output:
{"status":"ok"}
HTTP:200
Kesimpulan: env var PORT berhasil override port default, app listen di 9090
sesuai mapping -p 9090:9090.
```

```
[PORT tidak di-set — fallback default 8080]
Command: docker run -d --name go-api-default-test -p 8080:8080 go-api:v1
         curl -s -w "\nHTTP:%{http_code}\n" localhost:8080/health
Output:
{"status":"ok"}
HTTP:200
Kesimpulan: tanpa env var PORT, aplikasi fallback ke default 8080 (dari
main.go: os.Getenv("PORT") kosong -> port = "8080"). Terbukti jalan.
```

### 3. Volume — log persistence

App log tiap request `/api/users` ke `/var/log/app.log` di dalam container.

```
[Log sebelum container di-recreate]
Command: docker run -d --name go-api-vol-test -p 8080:8080 -v $(pwd)/logs:/var/log go-api:v1
         curl -s localhost:8080/api/users  (x3)
         cat logs/app.log
Output:
2026/09/09 13:58:37 GET /api/users from 172.17.0.1:37330
2026/09/09 13:58:37 GET /api/users from 172.17.0.1:37346
2026/09/09 13:58:37 GET /api/users from 172.17.0.1:37360
Kesimpulan: 3 request tercatat ke file log yang di-mount ke host.
```

```
[Log sesudah container di-stop, di-hapus, lalu container baru dijalankan dengan volume yang sama]
Command: docker stop go-api-vol-test && docker rm go-api-vol-test
         docker run -d --name go-api-vol-test2 -p 8080:8080 -v $(pwd)/logs:/var/log go-api:v1
         curl -s localhost:8080/api/users  (x1)
         cat logs/app.log
Output:
2026/09/09 13:58:37 GET /api/users from 172.17.0.1:37330
2026/09/09 13:58:37 GET /api/users from 172.17.0.1:37346
2026/09/09 13:58:37 GET /api/users from 172.17.0.1:37360
2026/09/09 13:58:46 GET /api/users from 172.17.0.1:45382
Kesimpulan: 3 log lama dari container pertama TETAP ADA setelah container
itu dihapus total, plus 1 log baru dari container kedua. Membuktikan data
di volume hidup di luar lifecycle container — container cuma "compute",
volume yang nyimpen state.
```

### 4. Custom network — Docker DNS

```
[DNS resolution antar container by name]
Command: docker network create app-net
         docker run -d --name api-a --network app-net go-api:v1
         docker run -d --name api-b --network app-net go-api:v1
         docker exec api-a wget -qO- http://api-b:8080/health
         (curl tidak tersedia di base image alpine, dipakai wget/busybox sebagai gantinya)
Output:
{"status":"ok"}
Kesimpulan: api-a berhasil resolve nama container api-b jadi IP dan connect,
tanpa perlu tahu IP address-nya secara manual. Docker embedded DNS jalan
di custom bridge network (tidak jalan otomatis di default bridge network).
```

### 5. Resource limit di bawah beban

```
[CPU limit 0.5 core di bawah load paralel ke /cpu-intensive]
Command: docker run -d --name go-api-load-test -p 8080:8080 --cpus="0.5" --memory="128m" go-api:v1
         for i in $(seq 1 60); do curl -s localhost:8080/cpu-intensive >/dev/null & done
         docker stats --no-stream go-api-load-test   (diambil beberapa kali selama load)
Output:
sample A (load jalan):  CPU % = 49.96%   MEM = 2.5MiB / 128MiB
sample B (load jalan):  CPU % = 49.91%   MEM = 2.6MiB / 128MiB
sample C (load selesai): CPU % = 0.00%   MEM = 2.4MiB / 128MiB
Kesimpulan: CPU usage mentok persis di ~50% (sesuai --cpus="0.5" = setengah
dari 1 core), walau ada 60 request paralel ngebusy-loop bareng. Kernel
cgroup CPU throttle kerja sesuai limit — proses tidak bisa "curi" CPU
lebih dari yang dialokasikan.
```

```
[Memory limit 128MB — apakah kelampaui?]
Command: docker inspect -f 'Status={{.State.Status}} ExitCode={{.State.ExitCode}} OOMKilled={{.State.OOMKilled}}' go-api-load-test
Output:
Status=running ExitCode=0 OOMKilled=false
Kesimpulan: memory usage aktual cuma ~2.5MB dari limit 128MB (app ini
stateless, gak nyimpen data besar di memory) — limit memory TIDAK
kelampaui, container tidak pernah di-OOM-kill. Untuk lihat OOM-kill beneran
perlu app yang sengaja alokasi memory besar (di luar scope Level 1 ini).
```

---

## Cleanup

Semua container test, network, dan image test dihapus setelah observasi dicatat:

```bash
docker rm -f go-api-load-test api-a api-b go-api-vol-test go-api-vol-test2 \
  go-api-port-test go-api-default-test 2>/dev/null
docker network rm app-net
docker rmi go-api:v1 go-api:naive
```

Folder `./logs/app.log` sengaja **tidak** dihapus — dibiarkan sebagai bukti
hasil eksperimen poin 3 (volume persistence).
