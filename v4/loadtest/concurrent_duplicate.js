import http from 'k6/http';
import { check } from 'k6';

const TOTAL_REQUESTS = 1000;

export const options = {
    scenarios: {
        concurrent_duplicates: {
            executor: 'shared-iterations',
            vus: 100,
            iterations: TOTAL_REQUESTS,
            maxDuration: '30s',
        },
    },
};

export default function () {
    const response = http.post(
        'http://localhost:8080/posts/1/like',
        null,
        {
            headers: {
                'X-User-ID': 'concurrent-user-1',
            },
        }
    );

    check(response, {
        'status is 200': (r) => r.status === 200,
    });
}