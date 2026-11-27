-- +goose Up
CREATE TABLE telegram_update_states (
 account_id bigint PRIMARY KEY REFERENCES telegram_accounts(id) ON DELETE CASCADE,
 user_id bigint NOT NULL,
 pts integer NOT NULL, qts integer NOT NULL, date integer NOT NULL, seq integer NOT NULL
);
CREATE TABLE telegram_update_channels (
 account_id bigint NOT NULL REFERENCES telegram_accounts(id) ON DELETE CASCADE,
 user_id bigint NOT NULL, channel_id bigint NOT NULL,
 pts integer, access_hash bigint,
 PRIMARY KEY(account_id,user_id,channel_id)
);
CREATE TABLE telegram_update_users (
 account_id bigint NOT NULL REFERENCES telegram_accounts(id) ON DELETE CASCADE,
 user_id bigint NOT NULL, target_user_id bigint NOT NULL, access_hash bigint NOT NULL,
 PRIMARY KEY(account_id,user_id,target_user_id)
);
-- +goose Down
DROP TABLE telegram_update_users,telegram_update_channels,telegram_update_states;
