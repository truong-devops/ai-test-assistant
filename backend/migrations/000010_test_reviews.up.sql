CREATE TABLE test_reviews (
    id BIGSERIAL PRIMARY KEY,
    generated_test_id BIGINT NOT NULL UNIQUE REFERENCES generated_tests(id) ON DELETE CASCADE,
    reviewer_name TEXT NOT NULL CHECK (length(btrim(reviewer_name)) BETWEEN 1 AND 128),
    decision TEXT NOT NULL CHECK (decision IN ('ACCEPTED', 'REJECTED')),
    comment TEXT NOT NULL DEFAULT '' CHECK (octet_length(comment) <= 4000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX test_reviews_generated_test_id_idx ON test_reviews (generated_test_id);
