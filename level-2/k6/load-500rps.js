import http from 'k6/http';

export const options = {
  scenarios: {
    constant_rate: {
      executor: 'constant-arrival-rate',
      rate: 500,
      timeUnit: '1s',
      duration: '30s',
      preAllocatedVUs: 250,
      maxVUs: 500,
    },
  },
};

export default function () {
  http.get('http://localhost:8080/api/users');
}
