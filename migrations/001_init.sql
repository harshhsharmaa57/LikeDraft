CREATE TABLE posts (
    id BIGSERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    like_count BIGINT NOT NULL DEFAULT 0
);

INSERT INTO posts (title)
VALUES ('My first post');