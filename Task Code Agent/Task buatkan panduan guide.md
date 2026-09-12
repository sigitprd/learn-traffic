# Task Brief — Bikin Panduan Step-by-Step (Level 1-6)

## Konteks
Level 1-6 sudah selesai dikerjakan dan diverifikasi oleh AI agent (code,
manifest, k6 script, README hasil eksperimen semua sudah ada di masing-masing
folder `level-1/` sampai `level-6/`).

Sekarang butuh dokumen yang BEDA TUJUAN: bukan laporan hasil eksperimen (itu
sudah ada di tiap README), tapi **panduan yang bisa diikuti manual oleh
manusia** (saya sendiri) di terminal, dari nol, tanpa perlu AI agent lagi.
Anggap pembacanya belum tahu apa-apa selain sudah ikuti Level 1-5 sebelumnya
(kalau baca guide Level 3, boleh asumsikan Level 1-2 sudah pernah dikerjakan,
tapi command Level 1-2 tidak perlu diulang detail).

**Jangan bangun ulang apapun dari nol.** Semua code/manifest/script sudah ada.
Tugas di sini murni dokumentasi: susun ulang jadi panduan yang runnable
step-by-step, sertakan expected output SEBENARNYA (ambil dari README hasil
eksperimen yang sudah ada), dan tambahkan troubleshooting untuk masalah yang
sudah pernah ketemu selama proses (banyak sudah tercatat di README tiap level).

---

## Objective
Buat 1 dokumen guide per level (total 6), plus 1 dokumen index/menu, supaya
saya bisa pilih mau coba level mana, ikuti langkah-langkahnya persis, dan tahu
apakah hasilnya sesuai ekspektasi atau ada yang salah.

---

## Deliverables

```
docs/
├── 00-index.md                  ← menu utama, mulai baca dari sini
├── 01-guide-container-basics.md
├── 02-guide-load-balancer.md
├── 03-guide-kubernetes-dasar.md
├── 04-guide-resource-limits.md
├── 05-guide-hpa.md
└── 06-guide-production-simulation.md
```

---

## Format yang WAJIB dipakai di tiap guide (kecuali index)

### 1. Header info
- Judul level & topik singkat
- Perkiraan waktu pengerjaan (menit)
- Prasyarat: level sebelumnya yang harus sudah selesai, tools yang harus
  sudah terinstall (dengan command cek versi, misal `docker --version`,
  `k3d version`, `kubectl version --client`, `k6 version`)

### 2. Langkah-langkah, format:
```
### Langkah N — <judul singkat>

<1-2 kalimat kenapa langkah ini perlu, bukan cuma "jalankan command ini">

\`\`\`bash
<command persis, copy-paste-able>
\`\`\`

**Expected output:**
\`\`\`
<output SEBENARNYA yang sudah tercatat di README eksperimen level ini —
JANGAN dikarang ulang, ambil dari data yang sudah ada>
\`\`\`

**Kalau outputnya beda / error:** <troubleshooting singkat, lihat bagian
Troubleshooting di bawah kalau perlu penjelasan lebih panjang>
```

### 3. Bagian "Apa yang barusan terjadi"
Setelah semua langkah inti selesai, 1 paragraf pendek yang ngaitin ke konsep
besarnya (referensi ke insight yang sudah ditemukan di README eksperimen asli
level itu) — bukan pengulangan step, tapi "kenapa ini penting".

### 4. Troubleshooting (WAJIB, berdasarkan masalah nyata yang sudah ketemu)
Tabel: gejala → kemungkinan penyebab → cara cek/perbaiki. Ambil dari
kejadian nyata yang sudah tercatat sepanjang proses, minimal termasuk:

- Level 3: Pod `ImagePullBackOff` → image belum di-`k3d image import`
- Level 4: `kubectl top` gagal / metrics `<unknown>` → metrics-server belum
  di-patch `--kubelet-insecure-tls`, atau belum tunggu cukup lama
- Level 5: HPA `TARGETS` tetap `<unknown>` → cek metrics-server dulu (Level 4)
  sebelum curiga ke HPA-nya
- Level 5: replica tidak naik walau CPU tinggi → cek `requests.cpu` di
  Deployment (HPA butuh ini untuk hitung persentase)
- Level 6: `/api/users` masih data statis, bukan dari Postgres → cek env var
  `POSTGRES_HOST` sudah di-set di Deployment, migration sudah dijalankan
- (dan lain-lain sesuai apa yang benar-benar ditemui saat pengerjaan asli
  tiap level — cek balik README eksperimen kalau ada catatan error/kejadian
  tak terduga yang belum masuk daftar ini)

### 5. Cleanup
Command buat beresin resource level ini (biar nggak numpuk kalau mau lanjut
ke level lain atau berhenti dulu), diambil dari bagian Cleanup di README asli.

---

## Isi khusus tiap guide (poin-poin minimal yang harus ada)

**01 — Container Basics**: build image, run dengan port mapping, ganti env
`PORT`, mount volume + buktikan persist, custom network + DNS by name,
resource limit + `docker stats` di bawah beban.

**02 — Load Balancer**: `docker compose up`, verifikasi round robin lewat
`/whoami`, jalankan 4 level k6 (10/100/500/1000 rps), lihat distribusi lewat
`/stats`, eksperimen kill 1 instance di tengah traffic.

**03 — Kubernetes Dasar**: buat cluster k3d, import image, apply Deployment/
Service/Ingress, buktikan Pod≠Container lewat `crictl`, buktikan Service load
balance, buktikan self-healing lewat `kubectl delete pod`.

**04 — Resource Limits**: install metrics-server, tambah requests/limits,
buktikan Pod `Pending` kalau requests kebesaran, buktikan CPU throttle di
bawah beban, buktikan OOMKill lewat `/memory-hog`.

**05 — HPA**: pasang HPA, hitung threshold manual, load ringan (replica
tetap), load berat (replica naik, catat timeline), amati scale-down yang
lambat.

**06 — Production Simulation**: deploy Postgres+Redis, buktikan cache-aside,
pasang Prometheus+Grafana, jalankan 4 eksperimen (A traffic bertahap, B kill
Pod stateful, C crash aplikasi, D traffic brutal).

## Isi khusus index (00-index.md)
- Tabel semua level: nama, topik 1 baris, waktu perkiraan, prasyarat level
  sebelumnya
- Penjelasan singkat: bisa mulai dari mana kalau cuma mau coba 1 topik
  spesifik (misal "cuma mau coba HPA" → perlu selesaikan Level 3 & 4 dulu
  minimal sampai cluster+metrics-server jalan, tidak perlu ulang Level 1-2
  penuh kalau image `go-api:v1` sudah ada)
- Link ke tiap guide

---

## Acceptance Criteria
- [ ] 7 file dokumen dibuat (`00-index.md` + 6 guide)
- [ ] Semua command di tiap guide bisa langsung copy-paste dijalankan
      berurutan tanpa langkah tersembunyi yang tidak disebutkan
- [ ] Expected output yang ditulis adalah data ASLI dari eksperimen yang
      sudah dilakukan sebelumnya, bukan dikarang ulang
- [ ] Tiap guide punya section troubleshooting dengan minimal 3 skenario
      gagal yang relevan untuk level itu
- [ ] Tiap guide punya cleanup command di akhir
- [ ] Index menjelaskan dependency antar level dengan jelas

## Constraints
- Jangan tulis ulang penjelasan konsep panjang lebar (itu sudah ada di
  README eksperimen) — guide ini fokus ke EKSEKUSI, bukan teori
- Jangan asumsikan pembaca sudah pernah baca README eksperimen — guide harus
  bisa berdiri sendiri untuk urusan "cara jalanin", walau untuk pemahaman
  konsep mendalam tetap merujuk ke README asli
- Kalau ada command yang butuh menunggu (misal tunggu Pod Ready, tunggu
  metrics-server siap), sebutkan estimasi waktu tunggu dan command buat cek
  statusnya (`--watch`, dsb), jangan cuma bilang "tunggu sebentar"