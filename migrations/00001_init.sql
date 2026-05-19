-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS _fathom_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    set_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO _fathom_meta (key, value) VALUES ('schema_initialized', 'true')
ON CONFLICT (key) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS _fathom_meta;
-- +goose StatementEnd
