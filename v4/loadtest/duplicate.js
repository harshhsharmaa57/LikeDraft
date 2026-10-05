import http from 'k6/http';
import { check } from 'k6';

const NUM_USERS = 10000;

export const options = {
    scenarios: {
        duplicate_likes: {
            executor: 'constant-arrival-rate',
            rate: 5000,
            timeUnit: '1s',
            duration: '30s',
            preAllocatedVUs: 100,
            maxVUs: 1000,
        },
    },
};

export default function () {
    // Pick a user from the fixed 10,000-user population.
    const userIndex = Math.floor(Math.random() * NUM_USERS);
    const userID = `user-${userIndex}`;

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