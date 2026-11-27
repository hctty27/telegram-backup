-- +goose Up
ALTER TABLE sync_checkpoints ADD COLUMN scan_high_water bigint NOT NULL DEFAULT 0 CHECK(scan_high_water>=0);
ALTER TABLE sync_checkpoints ADD COLUMN next_scan_at timestamptz NOT NULL DEFAULT now();
CREATE INDEX sync_due_idx ON sync_checkpoints(next_scan_at);
-- +goose Down
DROP INDEX sync_due_idx;
ALTER TABLE sync_checkpoints DROP COLUMN next_scan_at, DROP COLUMN scan_high_water;
