-- +goose Up
-- +goose StatementBegin
-- Active distinct payer/payee counts per day, verified payments only.
-- Distinct counts are NOT mergeable from the cube (cube groups by
-- facilitator, so the same payer in two facilitator rows is counted twice
-- when summed). They are computed directly from the view at rollup time.
CREATE TABLE IF NOT EXISTS metrics_active_entities_daily_v2 (
    day                 DATE     NOT NULL,
    payer_count         BIGINT   NOT NULL,
    payee_count         BIGINT   NOT NULL,
    methodology_version SMALLINT NOT NULL,
    PRIMARY KEY (day)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS metrics_active_entities_daily_v2;
-- +goose StatementEnd
