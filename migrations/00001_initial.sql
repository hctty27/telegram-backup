-- +goose Up
CREATE TABLE telegram_accounts (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 telegram_user_id bigint UNIQUE,
 display_name text NOT NULL DEFAULT '',
 api_id integer NOT NULL CHECK (api_id > 0),
 api_hash_ciphertext bytea NOT NULL,
 enabled boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE telegram_sessions (
 account_id bigint PRIMARY KEY REFERENCES telegram_accounts(id),
 ciphertext bytea NOT NULL, key_version integer NOT NULL DEFAULT 1,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE telegram_chats (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 account_id bigint NOT NULL REFERENCES telegram_accounts(id),
 peer_kind text NOT NULL CHECK(peer_kind IN ('user','chat','channel','self')),
 peer_id bigint NOT NULL, access_hash bigint,
 title text NOT NULL DEFAULT '', enabled boolean NOT NULL DEFAULT false,
 UNIQUE(account_id,peer_kind,peer_id)
);
CREATE TABLE sync_checkpoints (
 chat_id bigint PRIMARY KEY REFERENCES telegram_chats(id),
 history_before_id bigint, history_complete boolean NOT NULL DEFAULT false,
 incremental_high_water bigint NOT NULL DEFAULT 0 CHECK(incremental_high_water >= 0),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE messages (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 chat_id bigint NOT NULL REFERENCES telegram_chats(id),
 telegram_message_id bigint NOT NULL CHECK(telegram_message_id > 0),
 sent_at timestamptz NOT NULL,
 UNIQUE(chat_id,telegram_message_id)
);
CREATE TABLE media_files (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 message_id bigint NOT NULL REFERENCES messages(id),
 attachment_key text NOT NULL, filename text NOT NULL, mime_type text NOT NULL DEFAULT '',
 expected_bytes bigint CHECK(expected_bytes > 0),
 staged_bytes bigint NOT NULL DEFAULT 0 CHECK(staged_bytes >= 0),
 local_path text, sha256 text,
 state text NOT NULL DEFAULT 'discovered' CHECK(state IN
 ('discovered','downloading','downloaded','uploading','verifying','cleanup_pending','completed','retry_wait','blocked','failed')),
 last_error text, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(message_id,attachment_key)
);
CREATE TABLE storage_remotes (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 name text NOT NULL UNIQUE, kind text NOT NULL,
 config_ciphertext bytea NOT NULL, key_version integer NOT NULL DEFAULT 1,
 enabled boolean NOT NULL DEFAULT false
);
CREATE TABLE media_replicas (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 media_id bigint NOT NULL REFERENCES media_files(id),
 remote_id bigint NOT NULL REFERENCES storage_remotes(id),
 remote_path text NOT NULL, remote_object_id text,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','uploading','verified','failed')),
 verified_at timestamptz, verified_sha256 text,
 UNIQUE(media_id,remote_id), UNIQUE(remote_id,remote_path)
);
CREATE TABLE backup_policies (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 chat_id bigint NOT NULL UNIQUE REFERENCES telegram_chats(id),
 remote_id bigint NOT NULL REFERENCES storage_remotes(id),
 enabled boolean NOT NULL DEFAULT false,
 scan_interval_seconds integer NOT NULL DEFAULT 300 CHECK(scan_interval_seconds >= 60)
);
CREATE TABLE sync_runs (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 chat_id bigint NOT NULL REFERENCES telegram_chats(id),
 state text NOT NULL DEFAULT 'queued',
 started_at timestamptz, finished_at timestamptz, last_error text
);
CREATE TABLE disk_reservations (
 media_id bigint PRIMARY KEY REFERENCES media_files(id),
 reserved_bytes bigint NOT NULL CHECK(reserved_bytes > 0),
 owner_token uuid NOT NULL, updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE task_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 media_id bigint REFERENCES media_files(id), event_type text NOT NULL,
 detail jsonb NOT NULL DEFAULT '{}'::jsonb, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE system_settings (
 key text PRIMARY KEY, value jsonb NOT NULL, updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX media_state_idx ON media_files(state,id);
CREATE INDEX replicas_media_state_idx ON media_replicas(media_id,state);
CREATE INDEX events_created_idx ON task_events(created_at DESC);
CREATE INDEX runs_chat_idx ON sync_runs(chat_id,id DESC);
-- +goose Down
DROP TABLE system_settings, task_events, disk_reservations, sync_runs, backup_policies,
 media_replicas, storage_remotes, media_files, messages, sync_checkpoints, telegram_chats,
 telegram_sessions, telegram_accounts;
