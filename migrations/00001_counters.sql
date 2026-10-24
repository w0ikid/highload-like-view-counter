-- +goose Up
CREATE TABLE posts (
    id bigint PRIMARY KEY CHECK (id > 0),
    title text NOT NULL
);
CREATE TABLE post_counters (
    post_id bigint PRIMARY KEY REFERENCES posts(id) ON DELETE CASCADE,
    likes_count bigint NOT NULL DEFAULT 0 CHECK (likes_count >= 0),
    views_count bigint NOT NULL DEFAULT 0 CHECK (views_count >= 0)
);
CREATE TABLE likes (
    post_id bigint NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    user_id bigint NOT NULL CHECK (user_id > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (post_id, user_id)
);
CREATE INDEX likes_user_post_idx ON likes(user_id, post_id);
CREATE TABLE counter_flush_log (
    batch_id uuid PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE counter_flush_log;
DROP TABLE likes;
DROP TABLE post_counters;
DROP TABLE posts;
