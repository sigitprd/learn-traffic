import http from 'k6/http';

// Experiment D - traffic BRUTAL: rate naik agresif & cepat (jauh lebih
// curam dari Experiment A), campuran endpoint berat lebih dominan.
// Tujuan: amati seluruh rantai (CPU naik -> HPA trigger -> replica nambah
// -> distribusi ulang -> cache/DB ikut kena beban) dan cari titik jenuh.
export const options = {
  scenarios: {
    brutal: {
      executor: 'ramping-arrival-rate',
      startRate: 5,
      timeUnit: '1s',
      preAllocatedVUs: 100,
      maxVUs: 500,
      stages: [
        { target: 30, duration: '20s' },  // naik curam
        { target: 60, duration: '30s' },
        { target: 100, duration: '60s' }, // puncak brutal
        { target: 100, duration: '60s' }, // sustain di puncak
      ],
    },
  },
};

export default function () {
  // 50/50 - lebih berat dari Experiment A (20% cpu-intensive)
  if (Math.random() < 0.5) {
    http.get('http://localhost:8080/api/users');
  } else {
    http.get('http://localhost:8080/cpu-intensive');
  }
}
