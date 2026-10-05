import http from 'k6/http';
import { check } from 'k6';

const NUM_USERS = 10000;

export const options = {
    scenarios: {
        seed_users: {
            executor: 'shared-iterations',
            vus: 100,
            iterations: NUM_USERS,
            maxDuration: '2m',
        },
    },
};

export default function () {
    const userID = `user-${__ITER}`;

    const response = http.post(
        'http://localhost:8080/posts/1/like',
        null,
        {
            headers: {
                'X-User-ID': userID,
            },
        }
    );

    check(response, {
        'status is 200': (r) => r.status === 200,
    });
}