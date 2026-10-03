import http from 'k6/http';
import { check } from 'k6';

export const options = {
    scenarios: {
        likes: {
            executor: 'constant-arrival-rate',

            rate: 7500,

            timeUnit: '1s',

            duration: '30s',

            preAllocatedVUs: 100,

            maxVUs: 1000,
        },
    },
};

export default function () {
    // Deterministic synthetic user identity.
    //
    // __VU identifies the virtual user.
    // __ITER identifies the iteration performed by that VU.
    //
    // Together they give us a deterministic user identifier
    // for this benchmark.
    const userID = `user-${__VU}-${__ITER}`;

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
        'status is 200': (response) => response.status === 200,
    });
}