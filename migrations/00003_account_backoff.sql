-- +goose Up
ALTER TABLE telegram_accounts ADD COLUMN flood_wait_until timestamptz NOT NULL DEFAULT '-infinity';
-- +goose Down
ALTER TABLE telegram_accounts DROP COLUMN flood_wait_until;
