// Package backup coordinates the durable V2 transfer pipeline.
package backup

import(
 "context"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "strings"
 "time"

 "github.com/gotd/td/tg"
 "github.com/hctty27/telegram-backup/internal/disk"
 "github.com/hctty27/telegram-backup/internal/queue"
 "github.com/hctty27/telegram-backup/internal/storage"
 tgservice "github.com/hctty27/telegram-backup/internal/telegram"
 "github.com/hctty27/telegram-backup/internal/transfer"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
 "github.com/riverqueue/river"
)

type TelegramSource interface{
 ReadPage(context.Context,int64,tg.InputPeerClass,int,int)(tgservice.Page,error)
 Fetch(context.Context,int64,tg.InputPeerClass,int,string,int64,io.Writer)error
}
type CloudStore interface{
 UploadVerified(context.Context,string,string,string)(storage.Verified,error)
 TestRemote(context.Context,string)error
}

type Engine struct{
 Pool *pgxpool.Pool
 Telegram TelegramSource
 Cloud CloudStore
 Quota *disk.Manager
 Root string
 MaxFile int64
 Jobs *river.Client[pgx.Tx]
}

func(e *Engine)Handlers()queue.Handlers{
 return queue.Handlers{Sync:e.Sync,Download:e.Download,Upload:e.Upload,Cleanup:e.Cleanup}
}

// Schedule creates missing checkpoints and enqueues due scans transactionally.
// One sync worker and one server owner are required; the API does not enqueue
// scans directly. Active River jobs prevent a second scan for the same chat.
func(e *Engine)Schedule(ctx context.Context)error{
 tx,err:=e.Pool.Begin(ctx);if err!=nil{return err};defer tx.Rollback(ctx)
 if _,err=tx.Exec(ctx,`INSERT INTO sync_checkpoints(chat_id)
 SELECT p.chat_id FROM backup_policies p JOIN telegram_chats c ON c.id=p.chat_id
 JOIN telegram_accounts a ON a.id=c.account_id JOIN storage_remotes r ON r.id=p.remote_id
 WHERE p.enabled AND c.enabled AND a.enabled AND r.enabled ON CONFLICT DO NOTHING`);err!=nil{return err}
 rows,err:=tx.Query(ctx,`SELECT s.chat_id FROM sync_checkpoints s
 JOIN backup_policies p ON p.chat_id=s.chat_id JOIN telegram_chats c ON c.id=s.chat_id
 JOIN telegram_accounts a ON a.id=c.account_id JOIN storage_remotes r ON r.id=p.remote_id
 WHERE s.next_scan_at<=now() AND p.enabled AND c.enabled AND a.enabled AND r.enabled
 AND NOT EXISTS(SELECT 1 FROM river_job j WHERE j.kind='telegram_sync'
 AND j.args->>'chat_id'=s.chat_id::text AND j.state IN ('available','pending','running','retryable','scheduled'))
 ORDER BY s.next_scan_at LIMIT 100 FOR UPDATE OF s SKIP LOCKED`)
 if err!=nil{return err}
 var ids []int64
 for rows.Next(){var id int64;if err=rows.Scan(&id);err!=nil{rows.Close();return err};ids=append(ids,id)}
 err=rows.Err();rows.Close();if err!=nil{return err}
 for _,id:=range ids{
  if _,err=tx.Exec(ctx,"UPDATE sync_checkpoints SET next_scan_at=now()+interval '5 minutes' WHERE chat_id=$1",id);err!=nil{return err}
  // Repeated periodic scans must not be deduplicated against completed jobs.
  if _,err=e.Jobs.InsertTx(ctx,tx,queue.SyncArgs{ChatID:id},&river.InsertOpts{Queue:"telegram_sync",MaxAttempts:10});err!=nil{return err}
 }
 return tx.Commit(ctx)
}

func(e *Engine)Sync(ctx context.Context,a queue.SyncArgs)error{
 var account,peerID,hash,remote,high,before,scanHigh int64
 var kind string
 var interval int
 err:=e.Pool.QueryRow(ctx,`SELECT c.account_id,c.peer_kind,c.peer_id,coalesce(c.access_hash,0),p.remote_id,p.scan_interval_seconds,
 s.incremental_high_water,coalesce(s.history_before_id,0),s.scan_high_water
 FROM telegram_chats c JOIN telegram_accounts a ON a.id=c.account_id JOIN backup_policies p ON p.chat_id=c.id
 JOIN storage_remotes r ON r.id=p.remote_id JOIN sync_checkpoints s ON s.chat_id=c.id
 WHERE c.id=$1 AND c.enabled AND a.enabled AND p.enabled AND r.enabled`,a.ChatID).Scan(&account,&kind,&peerID,&hash,&remote,&interval,&high,&before,&scanHigh)
 if errors.Is(err,pgx.ErrNoRows){return nil};if err!=nil{return err}
 peer,err:=tgservice.Peer(kind,peerID,hash);if err!=nil{return err}
 page,err:=e.Telegram.ReadPage(ctx,account,peer,int(high),int(before))
 if err!=nil{var backoff *tgservice.BackoffError;if errors.As(err,&backoff){return river.JobSnooze(backoff.Wait)};return fmt.Errorf("telegram history failed: %T",err)}
 tx,err:=e.Pool.Begin(ctx);if err!=nil{return err};defer tx.Rollback(ctx)
 // Recheck policy and cursor before committing; changes made during network I/O
 // must not cause new discoveries to use a stale destination.
 var currentRemote,currentHigh,currentBefore,currentScan int64
 err=tx.QueryRow(ctx,`SELECT p.remote_id,s.incremental_high_water,coalesce(s.history_before_id,0),s.scan_high_water
 FROM backup_policies p JOIN sync_checkpoints s ON s.chat_id=p.chat_id
 JOIN telegram_chats c ON c.id=p.chat_id WHERE p.chat_id=$1 AND p.enabled AND c.enabled
 FOR UPDATE OF p,s,c`,a.ChatID).Scan(&currentRemote,&currentHigh,&currentBefore,&currentScan)
 if errors.Is(err,pgx.ErrNoRows){return nil};if err!=nil{return err}
 if currentRemote!=remote||currentHigh!=high||currentBefore!=before||currentScan!=scanHigh{return errors.New("scan configuration changed")}
 for _,m:=range page.Media{
  var message,media,replica int64
  err=tx.QueryRow(ctx,`INSERT INTO messages(chat_id,telegram_message_id,sent_at) VALUES($1,$2,$3)
  ON CONFLICT(chat_id,telegram_message_id) DO UPDATE SET sent_at=EXCLUDED.sent_at RETURNING id`,a.ChatID,m.MessageID,m.Date).Scan(&message)
  if err!=nil{return err}
  var size any=m.Size
  state:="discovered"
  if m.Size<=0{size=nil;state="blocked"}else if m.Size>e.MaxFile{state="blocked"}
  err=tx.QueryRow(ctx,`INSERT INTO media_files(message_id,attachment_key,filename,mime_type,expected_bytes,state,last_error)
  VALUES($1,$2,$3,$4,$5,$6,CASE WHEN $6='blocked' THEN 'unknown size or exceeds staging quota' END)
  ON CONFLICT(message_id,attachment_key) DO NOTHING RETURNING id`,message,m.Key,safeName(m.Name),m.MIME,size,state).Scan(&media)
  if errors.Is(err,pgx.ErrNoRows){continue};if err!=nil{return err}
  key:=fmt.Sprintf("TelegramBackup/%d/%s-%d/%s/%d-%d-%s",account,kind,peerID,m.Date.Format("2006/01"),m.MessageID,media,safeName(m.Name))
  err=tx.QueryRow(ctx,`INSERT INTO media_replicas(media_id,remote_id,remote_path) VALUES($1,$2,$3) RETURNING id`,media,remote,key).Scan(&replica)
  if err!=nil{return err}
  if state=="discovered"{if err=queue.EnqueueTx(ctx,e.Jobs,tx,queue.DownloadArgs{MediaID:media});err!=nil{return err}}
 }
 scanHigh=max(scanHigh,int64(page.First))
 if page.Complete{
  _,err=tx.Exec(ctx,`UPDATE sync_checkpoints SET incremental_high_water=greatest(incremental_high_water,$2),history_before_id=NULL,
  scan_high_water=0,history_complete=true,next_scan_at=now()+make_interval(secs=>$3),updated_at=now() WHERE chat_id=$1`,a.ChatID,scanHigh,interval)
 }else{
  _,err=tx.Exec(ctx,`UPDATE sync_checkpoints SET history_before_id=$2,scan_high_water=$3,next_scan_at=now(),updated_at=now() WHERE chat_id=$1`,a.ChatID,page.Last,scanHigh)
 }
 if err!=nil{return err};return tx.Commit(ctx)
}

func safeName(name string)string{
 name=filepath.Base(strings.ReplaceAll(name,"\\","/"))
 name=strings.Map(func(r rune)rune{if r<32||r==127||strings.ContainsRune(":/\\",r){return '_'};return r},name)
 name=strings.Trim(name," .")
 if name==""||name==".."{return "media.bin"}
 out:=[]rune(name);if len(out)>100{name=string(out[:100])}
 return name
}

func(e *Engine)Download(ctx context.Context,a queue.DownloadArgs)error{
 var account,peerID,hash,msgID,expected int64
 var kind,key,state string
 err:=e.Pool.QueryRow(ctx,`SELECT c.account_id,c.peer_kind,c.peer_id,coalesce(c.access_hash,0),m.telegram_message_id,
 f.attachment_key,coalesce(f.expected_bytes,0),f.state FROM media_files f JOIN messages m ON m.id=f.message_id
 JOIN telegram_chats c ON c.id=m.chat_id WHERE f.id=$1`,a.MediaID).Scan(&account,&kind,&peerID,&hash,&msgID,&key,&expected,&state)
 if errors.Is(err,pgx.ErrNoRows){return nil};if err!=nil{return err}
 if state!="discovered"&&state!="downloading"&&state!="retry_wait"{return nil}
 if expected<=0||expected>e.MaxFile{
  _,err=e.Pool.Exec(ctx,"UPDATE media_files SET state='blocked',last_error='file exceeds staging quota or size unknown',updated_at=now() WHERE id=$1",a.MediaID);return err
 }
 peer,err:=tgservice.Peer(kind,peerID,hash);if err!=nil{return err}
 if _,err=e.Pool.Exec(ctx,"UPDATE media_files SET state='downloading',last_error=NULL,updated_at=now() WHERE id=$1",a.MediaID);err!=nil{return err}
 target:=filepath.Join(e.Root,fmt.Sprintf("%d.media",a.MediaID))
 // Recover a fully published stage after a crash before DB commit. A private,
 // single-owner directory and the atomic link in Stage are required.
 info,statErr:=os.Lstat(target)
 if statErr==nil{
  if !info.Mode().IsRegular()||info.Size()!=expected{return errors.New("staged file conflict; operator inspection required")}
 }else if errors.Is(statErr,os.ErrNotExist){
  target,err=transfer.Stage(ctx,e.Root,a.MediaID,expected,e.Quota,func(ctx context.Context,w io.Writer)error{
   return e.Telegram.Fetch(ctx,account,peer,int(msgID),key,expected,w)
  })
  if errors.Is(err,disk.ErrCapacity){
   _,dbErr:=e.Pool.Exec(ctx,"UPDATE media_files SET state='retry_wait',last_error='waiting for disk space',updated_at=now() WHERE id=$1",a.MediaID)
   if dbErr!=nil{return dbErr};return river.JobSnooze(30*time.Second)
  }
  if err!=nil{var backoff *tgservice.BackoffError;if errors.As(err,&backoff){return river.JobSnooze(backoff.Wait)};return fmt.Errorf("media download failed: %T",err)}
 }else{return statErr}
 tx,err:=e.Pool.Begin(ctx);if err!=nil{return err};defer tx.Rollback(ctx)
 if _,err=tx.Exec(ctx,"UPDATE media_files SET state='downloaded',local_path=$2,staged_bytes=$3,last_error=NULL,updated_at=now() WHERE id=$1",a.MediaID,target,expected);err!=nil{return err}
 rows,err:=tx.Query(ctx,"SELECT id FROM media_replicas WHERE media_id=$1 AND state<>'verified'",a.MediaID);if err!=nil{return err}
 var ids []int64
 for rows.Next(){var id int64;if err=rows.Scan(&id);err!=nil{rows.Close();return err};ids=append(ids,id)}
 err=rows.Err();rows.Close();if err!=nil{return err}
 if len(ids)==0{return errors.New("no pending destination")}
 for _,id:=range ids{if err=queue.EnqueueTx(ctx,e.Jobs,tx,queue.UploadArgs{ReplicaID:id});err!=nil{return err}}
 return tx.Commit(ctx)
}

func(e *Engine)Upload(ctx context.Context,a queue.UploadArgs)error{
 var media,expected int64
 var local,remote,key,state string
 err:=e.Pool.QueryRow(ctx,`SELECT f.id,coalesce(f.expected_bytes,0),coalesce(f.local_path,''),r.name,p.remote_path,p.state
 FROM media_replicas p JOIN media_files f ON f.id=p.media_id JOIN storage_remotes r ON r.id=p.remote_id WHERE p.id=$1`,a.ReplicaID).Scan(&media,&expected,&local,&remote,&key,&state)
 if errors.Is(err,pgx.ErrNoRows){return nil};if err!=nil{return err};if state=="verified"{return nil}
 if local!=filepath.Join(e.Root,fmt.Sprintf("%d.media",media)){return errors.New("unexpected staging path")}
 info,err:=os.Lstat(local);if err!=nil||!info.Mode().IsRegular()||info.Size()!=expected{return errors.New("staged file missing or changed")}
 if _,err=e.Pool.Exec(ctx,"UPDATE media_files SET state='uploading',updated_at=now() WHERE id=$1 AND state IN ('downloaded','uploading','verifying')",media);err!=nil{return err}
 v,err:=e.Cloud.UploadVerified(ctx,local,remote,key);if err!=nil{return err};if v.Bytes!=expected{return errors.New("verified size mismatch")}
 tx,err:=e.Pool.Begin(ctx);if err!=nil{return err};defer tx.Rollback(ctx)
 // Serialize final verification and the decision to clean up across replicas.
 var locked int64
 if err=tx.QueryRow(ctx,"SELECT id FROM media_files WHERE id=$1 FOR UPDATE",media).Scan(&locked);err!=nil{return err}
 if _,err=tx.Exec(ctx,"UPDATE media_replicas SET state='verified',remote_object_id=$2,verified_sha256=$3,verified_at=now() WHERE id=$1",a.ReplicaID,v.ID,v.SHA256);err!=nil{return err}
 tag,err:=tx.Exec(ctx,`UPDATE media_files f SET state='cleanup_pending',sha256=$2,last_error=NULL,updated_at=now()
 WHERE f.id=$1 AND f.local_path IS NOT NULL AND NOT EXISTS(SELECT 1 FROM media_replicas p WHERE p.media_id=f.id AND p.state<>'verified')`,media,v.SHA256)
 if err!=nil{return err}
 if tag.RowsAffected()==1{if err=queue.EnqueueTx(ctx,e.Jobs,tx,queue.CleanupArgs{MediaID:media});err!=nil{return err}}
 return tx.Commit(ctx)
}
func(e *Engine)Cleanup(ctx context.Context,a queue.CleanupArgs)error{
 tx,err:=e.Pool.Begin(ctx);if err!=nil{return err};defer tx.Rollback(ctx)
 var local string
 err=tx.QueryRow(ctx,`SELECT f.local_path FROM media_files f WHERE f.id=$1 AND f.state='cleanup_pending'
 AND EXISTS(SELECT 1 FROM media_replicas p WHERE p.media_id=f.id)
 AND NOT EXISTS(SELECT 1 FROM media_replicas p WHERE p.media_id=f.id AND p.state<>'verified') FOR UPDATE`,a.MediaID).Scan(&local)
 if errors.Is(err,pgx.ErrNoRows){return nil};if err!=nil{return err}
 if err=transfer.CleanupVerified(e.Root,a.MediaID,local);err!=nil{return err}
 if _,err=tx.Exec(ctx,"UPDATE media_files SET state='completed',local_path=NULL,staged_bytes=0,last_error=NULL,updated_at=now() WHERE id=$1",a.MediaID);err!=nil{return err}
 return tx.Commit(ctx)
}
