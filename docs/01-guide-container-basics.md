# Guide 01 — Container Basics

Docker dasar: build image, run container, port mapping, environment
variable, volume persistence, custom network, resource limit. Fondasi buat
semua level berikutnya.

**Waktu**: ~30 menit
**Prasyarat**: Docker terinstall dan jalan (`docker --version`, cek daemon
jalan dengan `docker ps`).

Konsep detail ada di [`level1-container/README.md`](../level1-container/README.md)
— guide ini fokus ke eksekusi.

---

### Langkah 1 — Build image

Compile Go API jadi image Docker. Dockerfile-nya multi-stage (build pakai
`golang:1.25-alpine`, runtime pakai `alpine` polos) biar image final kecil.

```bash
cd level1-container
docker build -t go-api:v1 .
docker images go-api:v1
```

**Expected output:**
```
REPOSITORY   TAG   IMAGE ID       CREATED         SIZE
go-api       v1    <id berbeda>   x seconds ago   ~15-25MB
```

**Kalau outputnya beda / error:** lihat tabel Troubleshooting di bawah.

### Langkah 2 — Run container dengan port mapping + resource limit

```bash
docker run -d --name go-api -p 8080:8080 -e PORT=8080 \
  --cpus="0.5" --memory="128m" go-api:v1
curl localhost:8080/health
curl localhost:8080/api/users
```

**Expected output:**
```
{"status":"ok"}
[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"},{"id":3,"name":"Charlie"}]
```

Bersihin dulu sebelum lanjut:
```bash
docker rm -f go-api
```

### Langkah 3 — Environment variable `PORT`

Buktikan app baca `PORT` dari env var, dan fallback ke 8080 kalau gak
di-set.

```bash
docker run -d --name go-api-port-test -p 9090:9090 -e PORT=9090 go-api:v1
curl -s -w "\nHTTP:%{http_code}\n" localhost:9090/health
docker rm -f go-api-port-test

docker run -d --name go-api-default-test -p 8080:8080 go-api:v1
curl -s -w "\nHTTP:%{http_code}\n" localhost:8080/health
docker rm -f go-api-default-test
```

**Expected output** (dua-duanya sama, cuma beda port):
```
{"status":"ok"}
HTTP:200
```

### Langkah 4 — Volume persistence

Buktikan log yang ditulis app ke `/var/log/app.log` di dalam container
tetap ada walau container-nya dihapus total, selama volume host-nya sama.

```bash
mkdir -p logs
docker run -d --name go-api-vol-test -p 8080:8080 -v $(pwd)/logs:/var/log go-api:v1
curl -s localhost:8080/api/users >/dev/null
curl -s localhost:8080/api/users >/dev/null
curl -s localhost:8080/api/users >/dev/null
sleep 1
cat logs/app.log
```

**Expected output** (jam beda, format sama):
```
2026/09/09 13:58:37 GET /api/users from 172.17.0.1:37330
2026/09/09 13:58:37 GET /api/users from 172.17.0.1:37346
2026/09/09 13:58:37 GET /api/users from 172.17.0.1:37360
```

Sekarang hapus container-nya TOTAL dan jalankan container BARU dengan
volume yang SAMA:

```bash
docker stop go-api-vol-test && docker rm go-api-vol-test
docker run -d --name go-api-vol-test2 -p 8080:8080 -v $(pwd)/logs:/var/log go-api:v1
curl -s localhost:8080/api/users >/dev/null
sleep 1
cat logs/app.log
```

**Expected output** (3 baris lama + 1 baris baru — buktikan data volume
survive):
```
2026/09/09 13:58:37 GET /api/users from 172.17.0.1:37330
2026/09/09 13:58:37 GET /api/users from 172.17.0.1:37346
2026/09/09 13:58:37 GET /api/users from 172.17.0.1:37360
2026/09/09 13:58:46 GET /api/users from 172.17.0.1:45382
```

```bash
docker rm -f go-api-vol-test2
```

### Langkah 5 — Custom network + DNS by name

Buktikan container bisa saling panggil pakai NAMA, bukan IP, kalau ada di
custom network yang sama.

```bash
docker network create app-net
docker run -d --name api-a --network app-net go-api:v1
docker run -d --name api-b --network app-net go-api:v1
docker exec api-a wget -qO- http://api-b:8080/health
```

**Expected output:**
```
{"status":"ok"}
```

Catatan: image ini base-nya alpine, `curl` gak ada di dalamnya — makanya
dipakai `wget` (bawaan busybox), bukan `curl -qO-` seperti yang mungkin
kamu kira.

### Langkah 6 — Resource limit di bawah beban

Buktikan `--cpus="0.5"` beneran di-enforce (CPU usage mentok ~50%, bukan
cuma angka di config), dan cek apakah `--memory="128m"` sampai bikin
container mati.

```bash
docker rm -f go-api-load-test 2>/dev/null
docker run -d --name go-api-load-test -p 8080:8080 --cpus="0.5" --memory="128m" go-api:v1
for i in $(seq 1 60); do curl -s localhost:8080/cpu-intensive >/dev/null & done
docker stats --no-stream go-api-load-test
```

**Expected output** (jalankan `docker stats --no-stream go-api-load-test`
beberapa kali selagi load masih jalan — endpoint `/cpu-intensive` busy-loop
500ms per request):
```
CONTAINER ID   NAME               CPU %     MEM USAGE / LIMIT   MEM %
<id>           go-api-load-test   49.96%    2.055MiB / 128MiB   1.61%
```

CPU % harus mentok di sekitar 50% (gak pernah jauh di atas), MEM jauh di
bawah 128MiB limit.

```bash
docker inspect -f 'Status={{.State.Status}} ExitCode={{.State.ExitCode}} OOMKilled={{.State.OOMKilled}}' go-api-load-test
```

**Expected output:**
```
Status=running ExitCode=0 OOMKilled=false
```

App ini stateless dan gak sengaja makan memory besar, jadi memory limit gak
pernah kena — OOMKill beneran baru dibuktikan di Level 4 (endpoint
`/memory-hog` belum ada gunanya sampai situ).

---

## Apa yang barusan terjadi

Kamu baru saja membuktikan enam konsep dasar container yang jadi fondasi
SEMUA level berikutnya: image adalah artifact hasil build (kecil karena
multi-stage), container adalah instance yang bisa naik-turun tanpa
mempengaruhi image-nya, port mapping menjembatani jaringan host↔container,
env var jadi cara utama konfigurasi container tanpa rebuild image, volume
memisahkan data dari lifecycle container (container mati ≠ data hilang), dan
resource limit di-enforce oleh kernel cgroup — bukan cuma dokumentasi.
Level 3 nanti akan menunjukkan bahwa Kubernetes Pod pada dasarnya
"membungkus" konsep-konsep container ini dengan lapisan orkestrasi di
atasnya.

## Troubleshooting

| Gejala | Kemungkinan penyebab | Cara cek / perbaiki |
|---|---|---|
| `docker build` gagal, `Cannot connect to the Docker daemon` | Docker Desktop belum jalan | Buka Docker Desktop, tunggu status "Running", ulangi |
| Image size jauh > 50MB (ratusan MB) | Dockerfile ke-edit jadi single-stage (base `golang:*-alpine` dipakai juga buat runtime) | Cek `Dockerfile` — harus ada 2x `FROM`, stage kedua pakai `alpine` polos, bukan image `golang:*` |
| `docker run` gagal, `port is already allocated` | Ada container lain yang masih pakai port 8080/9090 | `docker ps` cek container yang jalan, `docker rm -f <nama>` yang gak dipakai |
| `curl: (7) Failed to connect` ke `localhost:8080` | Container belum sempat start, atau container crash langsung setelah start | `docker ps` cek STATUS, `docker logs <nama>` lihat error startup |
| `wget` di Langkah 5 gagal resolve `api-b` | Container gak di-attach ke network yang sama, atau nama network typo | `docker network inspect app-net` — pastikan `api-a` dan `api-b` ada di `Containers` |
| `docker stats` CPU % jauh di bawah 50% terus | Load `for i in seq 1 60` sudah selesai duluan sebelum sempat di-cek | Jalankan `docker stats` LANGSUNG setelah loop `for`, atau ulangi loop-nya sambil `docker stats` di terminal lain |

## Cleanup

```bash
docker rm -f go-api-load-test api-a api-b 2>/dev/null
docker network rm app-net
```

Image `go-api:v1` **JANGAN dihapus** — dipakai lagi di Level 2, 3, 4, 5, 6.
Folder `logs/app.log` juga boleh dibiarkan sebagai bukti eksperimen volume.

Lanjut ke [Guide 02 — Load Balancer](02-guide-load-balancer.md).
