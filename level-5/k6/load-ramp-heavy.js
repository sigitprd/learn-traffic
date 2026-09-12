import http from 'k6/http';

// Berat: rate jauh melewati threshold scale-up (0.14 req/s/pod), naik
// bertahap, sustained beberapa menit biar HPA (sync period 15s) punya cukup
// window buat bereaksi dan replica count kelihatan naik bertahap
// (2 -> 3 -> 5 -> dst, bukan langsung lompat ke max).
export const options = {
  scenarios: {
    ramp_heavy: {
      executor: 'ramping-arrival-rate',
      startRate: 2,
      timeUnit: '1s', // mulai 2 req/s - sudah di atas threshold ringan
      preAllocatedVUs: 50,
      maxVUs: 300,
      stages: [
        { target: 5, duration: '60s' },
        { target: 10, duration: '90s' },
        { target: 20, duration: '90s' },
        { target: 20, duration: '90s' }, // sustain di puncak
      ],
    },
  },
};

export default function () {
  http.get('http://localhost:8080/cpu-intensive');
}
