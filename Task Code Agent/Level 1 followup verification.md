# Follow-up Task — Lengkapi Verifikasi Level 1

Progress sejauh ini sudah oke: `go build` OK, `docker build` OK, container jalan
dengan resource limit, kedua endpoint respon benar. Tapi beberapa item di
checklist Level 1 **belum diverifikasi dan dilaporkan**. Selesaikan dan laporkan
item-item berikut — jangan cleanup dulu sebelum tiap langkah dicatat outputnya.

Jangan lanjut ke Level 2 sebelum semua ini selesai.

---

## 1. Ukuran image
- [ ] Jalankan `docker images go-api:v1` dan catat ukurannya
- [ ] Kalau masih > 50MB, cek apakah Dockerfile sudah benar-benar multi-stage
      (stage builder terpisah dari stage runtime, base image runtime pakai
      `distroless` atau `alpine`, bukan `golang:1.22` penuh di stage akhir)
- [ ] Laporkan ukuran sebelum vs sesudah kalau ada perbaikan

## 2. Environment variable `PORT`
- [ ] Jalankan container dengan `-e PORT=9090` dan mapping port yang sesuai
      (`-p 9090:9090`)
- [ ] `curl localhost:9090/health` harus sukses
- [ ] Jalankan lagi TANPA set `PORT`, buktikan aplikasi fallback ke default 8080
- [ ] Laporkan kedua command + output curl-nya

## 3. Volume — log persistence
- [ ] Tambahkan logging ke file di dalam container (misal setiap request ke
      `/api/users` ditulis ke `/var/log/app.log`)
- [ ] Mount volume: `docker run -v $(pwd)/logs:/var/log ...`
- [ ] Hit endpoint beberapa kali, lalu cek isi `./logs/app.log` di host
- [ ] **Stop dan hapus container**, lalu run container baru dengan volume yang
      sama — buktikan log lama masih ada (ini poin utamanya: data persist di
      luar lifecycle container)
- [ ] Laporkan isi log sebelum & sesudah container di-recreate

## 4. Custom network
- [ ] `docker network create app-net`
- [ ] Jalankan 2 container di network itu dengan `--network app-net` dan
      `--name` yang berbeda (misal `api-a`, `api-b`)
- [ ] Dari dalam `api-a`, curl ke `api-b` pakai **nama container**, bukan IP:
      `docker exec api-a curl http://api-b:8080/health`
- [ ] Laporkan output-nya — ini membuktikan Docker DNS resolution jalan

## 5. Resource limit di bawah beban (paling penting)
- [ ] Jalankan container dengan `--cpus="0.5" --memory="128m"`
- [ ] Generate load ke `/cpu-intensive` (looping curl atau pakai `hey`/`ab`
      kalau tersedia, boleh manual beberapa request paralel kalau tools belum
      ada)
- [ ] Selagi load jalan, buka terminal lain dan jalankan `docker stats
      --no-stream` beberapa kali, catat CPU% dan MEM usage-nya
- [ ] Laporkan apakah CPU usage mentok di sekitar limit (0.5 = 50% dari 1 core)
- [ ] Kalau memory limit terlampaui, laporkan apa yang terjadi (container di-kill
      dengan OOM? restart? catat exit code / `docker inspect` status-nya)

## 6. README
- [ ] Update README dengan semua command persis yang dipakai di atas + output/
      observasi masing-masing (bukan cuma "berhasil", tapi hasil curl, isi log,
      output docker stats, dll)
- [ ] Setelah semua dicatat, baru boleh cleanup container/network

---

## Format laporan yang diharapkan

Untuk tiap poin 1–5, laporkan dalam format:

```
[Nama eksperimen]
Command: <command persis>
Output: <output relevan>
Kesimpulan: <apa yang ini buktikan>
```

Kalau ada yang gagal atau hasilnya tidak sesuai ekspektasi, laporkan apa
adanya — itu justru bagian penting dari belajar, bukan sesuatu yang perlu
disembunyikan.