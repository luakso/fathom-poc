-- _smoke_v1: confirms the views pipeline (init-db applies *.sql in views/).
-- Delete this file when the first real view ships.
CREATE OR REPLACE VIEW _smoke_v1 AS
SELECT 'ok'::text AS status, now() AS computed_at;
