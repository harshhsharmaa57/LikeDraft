import http from 'k6/http';
import { check } from 'k6';

export const options = {
  scenarios: {
    likes: {
      executor: 'constant-arrival-rate',
      rate: 10000,
      timeUnit: '1s',
      duration: '30s',
      preAllocatedVUs: 100,
      maxVUs: 100000,
    },
  },
};

export default function () {
  const response = http.post('http://localhost:8080/posts/1/like');

  check(response, {
    'status is 200': (r) => r.status === 200,
  });
}
