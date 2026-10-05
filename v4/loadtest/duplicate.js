import http from 'k6/http';
import { check } from 'k6';

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
    // Every VU represents one user.
    //
    // The same user repeatedly sends the request.
    // V4 must count only the first request.

    const userID = `user-${__VU}`;

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