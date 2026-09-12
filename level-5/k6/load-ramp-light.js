import http from 'k6/http';

// Ringan: rate diusahakan tetap DI BAWAH ~0.14 req/s per Pod (dengan 2 Pod
// baseline = ~0.28 req/s total), biar CPU usage per Pod tetap di bawah
// threshold scale-up HPA (70m dari requests.cpu=100m, target 70%).
// Naik bertahap tapi tetap ringan selama 3 menit.
export const options = {
  scenarios: {
    ramp_light: {
      executor: 'ramping-arrival-rate',
      startRate: 1,
      timeUnit: '10s', // mulai dari 0.1 req/s
      preAllocatedVUs: 10,
      maxVUs: 20,
      stages: [
        { target: 2, duration: '60s' },  // 0.2 req/s
        { target: 3, duration: '60s' },  // 0.3 req/s
        { target: 4, duration: '60s' },  // 0.4 req/s (masih di bawah ~0.28*2=0.56 buffer aman)
      ],
    },
  },
};

export default function () {
  http.get('http://localhost:8080/cpu-intensive');
}
