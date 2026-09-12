import http from 'k6/http';

// Experiment A - traffic bertahap, campuran endpoint murah (/api/users,
// DB+cache) dan mahal (/cpu-intensive), rate naik dalam beberapa fase.
// Tujuan: amati HPA scaling realistis (bukan cuma 1 endpoint monoton) dan
// cache hit ratio di Postgres/Redis saat traffic naik.
export const options = {
  scenarios: {
    gradual: {
      executor: 'ramping-arrival-rate',
      startRate: 1,
      timeUnit: '1s',
      preAllocatedVUs: 50,
      maxVUs: 200,
      stages: [
        { target: 3, duration: '60s' },   // fase rendah
        { target: 8, duration: '90s' },   // fase sedang
        { target: 15, duration: '90s' },  // fase tinggi
        { target: 15, duration: '60s' },  // sustain
      ],
    },
  },
};

export default function () {
  // 80% ke /api/users (murah, DB+cache), 20% ke /cpu-intensive (mahal)
  if (Math.random() < 0.8) {
    http.get('http://localhost:8080/api/users');
  } else {
    http.get('http://localhost:8080/cpu-intensive');
  }
}
