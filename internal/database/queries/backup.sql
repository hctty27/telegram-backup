-- name: UpsertMessage :one
INSERT INTO messages(chat_id,telegram_message_id,sent_at)
VALUES($1,$2,$3)
ON CONFLICT(chat_id,telegram_message_id) DO UPDATE SET sent_at=EXCLUDED.sent_at
RETURNING id;

-- name: InsertMedia :one
INSERT INTO media_files(message_id,attachment_key,filename,mime_type,expected_bytes)
VALUES($1,$2,$3,$4,$5)
ON CONFLICT(message_id,attachment_key) DO NOTHING
RETURNING id;

-- name: InsertReplica :one
INSERT INTO media_replicas(media_id,remote_id,remote_path)
VALUES($1,$2,$3)
ON CONFLICT(media_id,remote_id) DO NOTHING
RETURNING id;

-- name: DashboardCounts :one
SELECT
 (SELECT count(*) FROM telegram_accounts)::bigint AS accounts,
 (SELECT count(*) FROM telegram_chats WHERE enabled)::bigint AS enabled_chats,
 (SELECT count(*) FROM media_files WHERE state='completed')::bigint AS completed,
 (SELECT count(*) FROM media_files WHERE state IN ('downloading','uploading','verifying'))::bigint AS active,
 (SELECT count(*) FROM media_files WHERE state IN ('failed','blocked'))::bigint AS failed,
 (SELECT coalesce(sum(staged_bytes),0)::bigint FROM media_files) AS staged_bytes;

-- name: MarkReplicaVerified :execrows
UPDATE media_replicas SET state='verified',verified_at=now(),verified_sha256=$2
WHERE id=$1 AND state IN ('pending','uploading','failed');

-- name: MarkCleanupPending :execrows
UPDATE media_files m SET state='cleanup_pending',updated_at=now()
WHERE m.id=$1 AND m.local_path IS NOT NULL
AND EXISTS(SELECT 1 FROM media_replicas r WHERE r.media_id=m.id)
AND NOT EXISTS(SELECT 1 FROM media_replicas r WHERE r.media_id=m.id AND r.state<>'verified');

-- name: FinishCleanup :execrows
UPDATE media_files SET state='completed',local_path=NULL,staged_bytes=0,updated_at=now()
WHERE id=$1 AND state='cleanup_pending';
