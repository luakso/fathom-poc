-- +goose Up
-- +goose StatementBegin
-- Per-window new vs returning payer breakdown (item 6.5).
-- "New" payer: first-ever verified payment (facilitator_known) is within the window.
-- "Returning": at least one verified payment before the window start.
-- The "all" window is degenerate (every payer is "new" relative to time zero) and not stored here.
CREATE TABLE IF NOT EXISTS metrics_payer_cohorts_v1 (
    window_name                 TEXT          NOT NULL,
    new_payers                  BIGINT        NOT NULL,
    returning_payers            BIGINT        NOT NULL,
    new_payer_volume_usdc       NUMERIC(38,6) NOT NULL,
    returning_payer_volume_usdc NUMERIC(38,6) NOT NULL,
    methodology_version         SMALLINT      NOT NULL,
    PRIMARY KEY (window_name)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS metrics_payer_cohorts_v1;
-- +goose StatementEnd
