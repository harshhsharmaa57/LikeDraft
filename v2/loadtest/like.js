import http from 'k6/http';
import { check } from 'k6';

export const options = {
    scenarios: {
        likes: {
            executor: 'constant-arrival-rate',

            // Number of requests per second.
            rate: 3000,

            timeUnit: '1s',

            // Run for 30 seconds.
            duration: '30s',

            // Initial virtual users.
            preAllocatedVUs: 100,

            // Maximum VUs k6 can create.
            maxVUs: 1000,
        },
    },
};

export default function () {
    const response = http.post(
        'http://localhost:8080/posts/1/like'
    );

    check(response, {
        'status is 200': (response) => response.status === 200,
    });
}