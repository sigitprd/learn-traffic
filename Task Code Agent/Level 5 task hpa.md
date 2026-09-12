# Task Brief — Level 5: Horizontal Pod Autoscaler (HPA)

## Konteks
Level 4 sudah menyiapkan semua prasyarat teknis buat level ini:
- `metrics-server` sudah jalan dan `kubectl top pods` sudah terverifikasi
  keluar angka nyata
- Deployment `go-api` sudah punya `resources.requests.cpu: 100m` dan
  `limits.cpu: 500m`
- Sudah dibuktikan CPU throttling terjadi kalau usage mendekati/melebihi
  `limits`
- Sudah ditulis eksplisit di README Level 4: HPA berbasis CPU menghitung
  persentase dari **requests**, bukan **limits**

Level 5 ini yang jadi tujuan awal kamu nanya soal Kubernetes: **autoscaling
otomatis berdasarkan beban, bukan jumlah pod statis**. Ini juga saat yang
tepat buat luruskan salah kaprah umum: autoscaling itu **bukan**
"kalau traffic > 1000 request, spawn pod" — itu berbasis **resource
utilization** (atau custom metrics), bukan threshold jumlah request.

---

## Objective
Pasang HPA ke Deployment `go-api` (target CPU 70%, min 2 max 10 replica),
generate traffic bertahap dari ringan ke berat, amati replica count naik
mengikuti beban — lalu turunkan beban dan amati scale-down-nya (yang jauh
lebih lambat, dan ini disengaja oleh desain Kubernetes).

---

## Deliverables

```
level-5/
├── manifests/
│   └── hpa.yaml
├── k6/
│   ├── load-ramp-light.js     (simulasi traffic naik bertahap, ringan)
│   └── load-ramp-heavy.js     (simulasi traffic naik bertahap, berat)
└── README.md
```

---

## Task List (kerjakan berurutan)

### 1. Pastikan prasyarat dari Level 4 masih hidup
- [ ] `kubectl top pods` harus keluar angka (metrics-server jalan)
- [ ] `kubectl describe deployment go-api` — konfirmasi `requests.cpu: 100m`
      masih terpasang (kalau cluster baru, re-apply manifest Level 4)

### 2. Buat HPA
- [ ] Buat `hpa.yaml`:
      ```yaml
      apiVersion: autoscaling/v2
      kind: HorizontalPodAutoscaler
      metadata:
        name: go-api-hpa
      spec:
        scaleTargetRef:
          apiVersion: apps/v1
          kind: Deployment
          name: go-api
        minReplicas: 2
        maxReplicas: 10
        metrics:
        - type: Resource
          resource:
            name: cpu
            target:
              type: Utilization
              averageUtilization: 70
      ```
- [ ] `kubectl apply -f manifests/hpa.yaml`
- [ ] `kubectl get hpa` — **tunggu sampai kolom `TARGETS` menunjukkan angka
      nyata** (misal `12%/70%`), bukan `<unknown>/70%`. Kalau tetap
      `<unknown>` setelah 1-2 menit, berarti metrics-server bermasalah —
      jangan lanjut sebelum ini beres
- [ ] Catat baseline: berapa replica saat traffic idle (harus turun ke
      `minReplicas: 2`, bukan 3 seperti Level 3/4 — HPA yang sekarang
      mengontrol jumlah replica, bukan spec `replicas` di Deployment lagi)

### 3. Hitung manual dulu sebelum generate load (biar tahu apa yang diharapkan)
- [ ] Dengan `requests.cpu: 100m` dan target `70%`, hitung: pada usage berapa
      m per Pod HPA mulai scale up? (jawaban: ~70m per Pod)
- [ ] Tulis prediksi di README **sebelum** menjalankan load test: pada rate
      berapa kira-kira replica akan bertambah, berdasarkan pengamatan
      CPU usage per-request dari Level 4

### 4. Load test ringan — replica 2 tetap cukup
- [ ] `load-ramp-light.js`: mulai dari rate rendah, naik bertahap, tapi tetap
      di bawah titik yang menurut prediksi kamu memicu scale up
- [ ] Jalankan sambil `kubectl get hpa go-api-hpa --watch` di terminal lain
- [ ] Laporkan: replica tetap 2, `TARGETS` naik tapi di bawah 70%

### 5. Load test berat — replica harus naik (eksperimen utama)
- [ ] `load-ramp-heavy.js`: target `/cpu-intensive` (bukan `/api/users` —
      endpoint ini didesain sengaja bikin CPU usage tinggi), rate naik
      bertahap selama beberapa menit (HPA butuh waktu buat bereaksi, jangan
      short burst)
- [ ] Selama load jalan, `kubectl get hpa --watch` DAN `kubectl get pods
      --watch` di dua terminal terpisah, catat timeline:
      - jam berapa `TARGETS` melewati 70%
      - jam berapa replica count mulai naik (2→3→5→dst, jangan expect
        lompat langsung ke max)
      - `kubectl describe hpa go-api-hpa` bagian `Events` — catat pesan
        `SuccessfulRescale` beserta alasannya (`New size: X; reason: cpu
        resource utilization above target`)
- [ ] Bandingkan hasil actual dengan prediksi manual di poin 3 — kalau beda
      jauh, coba jelaskan kenapa (ingat: HPA sync period default 15 detik,
      jadi reaksinya tidak instan)

### 6. Scale-down — amati bahwa ini JAUH lebih lambat dari scale-up
- [ ] Hentikan load test
- [ ] `kubectl get hpa --watch`, catat berapa lama replica count mulai turun
      lagi ke arah `minReplicas: 2`
- [ ] Cari tahu kenapa (`kubectl describe hpa` atau dokumentasi): Kubernetes
      punya `stabilization window` default 5 menit untuk scale-down (supaya
      tidak "flapping" naik-turun terus kalau traffic naik-turun sebentar-
      sebentar), sementara scale-up jauh lebih agresif/cepat
- [ ] Laporkan di README: berapa lama actual scale-down di lab kamu, apakah
      sesuai ekspektasi ~5 menit

### 7. Bandingkan dengan asumsi awal ("traffic > 1000 request = spawn pod")
- [ ] Tulis di README perbandingan eksplisit: kenapa model itu tidak dipakai
      Kubernetes secara default, dan kenapa **resource utilization** lebih
      masuk akal sebagai basis — kaitkan dengan hasil load test Level 2,
      dimana request rate yang sama bisa punya CPU cost yang beda jauh
      tergantung endpoint yang dipanggil (`/api/users` murah, `/cpu-intensive`
      mahal)

### 8. Dokumentasi
- [ ] README: prediksi vs actual, timeline scale-up, timeline scale-down,
      kesimpulan kenapa basis utilization lebih tepat dari basis request count

---

## Acceptance Criteria

- [ ] HPA terpasang, `kubectl get hpa` menunjukkan `TARGETS` dengan angka nyata
      (bukan `<unknown>`)
- [ ] Replica tetap di `minReplicas` (2) saat load ringan
- [ ] Replica terbukti naik mengikuti beban saat load berat, dengan timeline
      tercatat (bukan cuma "akhirnya naik", tapi kapan dan ke berapa)
- [ ] `kubectl describe hpa` Events menunjukkan alasan scaling
      (`SuccessfulRescale` dengan reason yang jelas)
- [ ] Scale-down terbukti lebih lambat dari scale-up, dengan penjelasan kenapa
      (stabilization window)
- [ ] Ada perbandingan eksplisit antara model "utilization-based" (yang
      dipakai HPA) vs "request-count-based" (asumsi awal yang keliru), dengan
      alasan konkret kenapa yang pertama lebih masuk akal

## Constraints
- Load test HARUS ke `/cpu-intensive`, bukan `/api/users` — endpoint yang
  disebut terakhir terlalu ringan buat memicu CPU utilization tinggi dalam
  waktu wajar
- Jangan generate burst traffic sesaat lalu berhenti — HPA butuh window waktu
  observasi, jadi load harus sustained (minimal beberapa menit) biar polanya
  kelihatan jelas, bukan noise
- Jangan ubah `minReplicas`/`maxReplicas` di tengah eksperimen supaya hasilnya
  konsisten dibandingkan dengan prediksi di poin 3

## Out of Scope (jangan dikerjakan di task ini)
- Custom metrics (selain CPU) — bisa jadi eksplorasi lanjutan opsional,
  di luar scope lab ini
- Prometheus/Grafana sebagai sumber metrics HPA — itu observability layer,
  beda topik dari HPA dasar berbasis metrics-server
- Redis/Postgres, simulasi production penuh — itu Level 6