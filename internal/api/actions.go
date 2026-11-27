package api

import(
 "context"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "strconv"
 "time"

 "github.com/go-chi/chi/v5"
 "github.com/hctty27/telegram-backup/internal/backup"
 "github.com/hctty27/telegram-backup/internal/storage"
 "github.com/hctty27/telegram-backup/internal/telegram"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)

type Actions struct{
 Pool *pgxpool.Pool
 Telegram *telegram.Service
 Login *telegram.LoginManager
 Engine *backup.Engine
 Managed *storage.Managed
 OAuth *DriveOAuth
}
func decode(w http.ResponseWriter,r *http.Request,v any)error{
 r.Body=http.MaxBytesReader(w,r.Body,64<<10)
 d:=json.NewDecoder(r.Body);d.DisallowUnknownFields()
 if err:=d.Decode(v);err!=nil{return err}
 var extra any
 if err:=d.Decode(&extra);!errors.Is(err,io.EOF){return errors.New("invalid trailing JSON")}
 return nil
}
func reply(w http.ResponseWriter,v any){w.Header().Set("Content-Type","application/json");_=json.NewEncoder(w).Encode(v)}
func fail(w http.ResponseWriter){http.Error(w,"operation failed; check configuration and connection",400)}
func idParam(r *http.Request)(int64,error){id,err:=strconv.ParseInt(chi.URLParam(r,"id"),10,64);if err!=nil||id<=0{return 0,errors.New("invalid ID")};return id,nil}

func(a *Actions)Routes(r chi.Router){
 if a.OAuth!=nil{r.Post("/storage/oauth/start",a.OAuth.start)}
 r.Get("/monitor",func(w http.ResponseWriter,r *http.Request){reply(w,a.snapshot())})
 r.Get("/monitor/events",a.stream(make(chan struct{},8)))
 r.Post("/accounts",func(w http.ResponseWriter,r *http.Request){
  var req struct{APIID int `json:"api_id"`;APIHash string `json:"api_hash"`;Name string `json:"name"`}
  if decode(w,r,&req)!=nil{fail(w);return}
  id,err:=a.Telegram.CreateAccount(r.Context(),req.APIID,req.APIHash,req.Name);if err!=nil{fail(w);return}
  reply(w,map[string]string{"id":strconv.FormatInt(id,10)})
 })
 r.Post("/accounts/{id}/qr",func(w http.ResponseWriter,r *http.Request){
  id,err:=idParam(r);if err!=nil{fail(w);return}
  if !a.accountExists(r.Context(),id){fail(w);return}
  if a.Login.Start(id)!=nil{http.Error(w,"login unavailable or active",409);return}
  reply(w,a.Login.Status(id))
 })
 r.Post("/accounts/{id}/phone",func(w http.ResponseWriter,r *http.Request){
  id,err:=idParam(r);var req struct{Phone string `json:"phone"`}
  if err!=nil||decode(w,r,&req)!=nil||!a.accountExists(r.Context(),id){fail(w);return}
  if a.Login.StartPhone(id,req.Phone)!=nil{http.Error(w,"invalid phone or login unavailable",409);return}
  reply(w,a.Login.Status(id))
 })
 status:=func(w http.ResponseWriter,r *http.Request){
  id,err:=idParam(r);if err!=nil{fail(w);return};reply(w,a.Login.Status(id))
 }
 r.Get("/accounts/{id}/qr",status)
 r.Get("/accounts/{id}/login",status)
 r.Post("/accounts/{id}/code",func(w http.ResponseWriter,r *http.Request){
  id,err:=idParam(r);var req struct{AttemptID string `json:"attempt_id"`;Code string `json:"code"`}
  if err!=nil||decode(w,r,&req)!=nil||a.Login.Code(id,req.AttemptID,req.Code)!=nil{fail(w);return}
  reply(w,a.Login.Status(id))
 })
 r.Post("/accounts/{id}/password",func(w http.ResponseWriter,r *http.Request){
  id,err:=idParam(r);var req struct{AttemptID string `json:"attempt_id"`;Password string `json:"password"`}
  if err!=nil||decode(w,r,&req)!=nil||a.Login.Password(id,req.AttemptID,req.Password)!=nil{fail(w);return}
  reply(w,a.Login.Status(id))
 })
 r.Post("/accounts/{id}/resend",func(w http.ResponseWriter,r *http.Request){
  id,err:=idParam(r);var req struct{AttemptID string `json:"attempt_id"`}
  if err!=nil||decode(w,r,&req)!=nil||a.Login.Resend(id,req.AttemptID)!=nil{http.Error(w,"resend unavailable",409);return}
  reply(w,a.Login.Status(id))
 })
 r.Post("/accounts/{id}/cancel",func(w http.ResponseWriter,r *http.Request){
  id,err:=idParam(r);var req struct{AttemptID string `json:"attempt_id"`}
  if err!=nil||decode(w,r,&req)!=nil||a.Login.Cancel(id,req.AttemptID)!=nil{http.Error(w,"no active login",409);return}
  reply(w,a.Login.Status(id))
 })
 r.Post("/accounts/{id}/chats",func(w http.ResponseWriter,r *http.Request){
  id,err:=idParam(r);if err!=nil{fail(w);return}
  count,err:=a.Telegram.ImportChats(r.Context(),id);if err!=nil{fail(w);return}
  reply(w,map[string]int{"imported":count})
 })
 r.Post("/storage",func(w http.ResponseWriter,r *http.Request){
  var req storage.DriveConfig
  if a.Managed==nil||decode(w,r,&req)!=nil{fail(w);return}
  ctx,cancel:=context.WithTimeout(r.Context(),30*time.Second);defer cancel()
  if a.Managed.CreateDrive(ctx,req)!=nil{fail(w);return}
  reply(w,map[string]bool{"ok":true})
 })
 r.Post("/storage/{id}/test",func(w http.ResponseWriter,r *http.Request){
  id,err:=idParam(r);if err!=nil||a.Managed==nil{fail(w);return}
  ctx,cancel:=context.WithTimeout(r.Context(),30*time.Second);defer cancel()
  var name string
  if a.Pool.QueryRow(ctx,"SELECT name FROM storage_remotes WHERE id=$1",id).Scan(&name)!=nil||a.Managed.TestRemote(ctx,name)!=nil{fail(w);return}
  reply(w,map[string]bool{"ok":true})
 })
 r.Put("/chats/{id}/policy",func(w http.ResponseWriter,r *http.Request){
  id,err:=idParam(r)
  var req struct{RemoteID string `json:"remote_id"`;Enabled bool `json:"enabled"`;Interval int `json:"scan_interval_seconds"`}
  if err!=nil||decode(w,r,&req)!=nil||req.Interval<60||req.Interval>86400{fail(w);return}
  remote,err:=strconv.ParseInt(req.RemoteID,10,64);if err!=nil||remote<=0{fail(w);return}
  tx,err:=a.Pool.Begin(r.Context());if err!=nil{fail(w);return};defer tx.Rollback(r.Context())
  var chatLocked int64
  if err=tx.QueryRow(r.Context(),"SELECT id FROM telegram_chats WHERE id=$1 FOR UPDATE",id).Scan(&chatLocked);err!=nil{fail(w);return}
  var existing int64
  err=tx.QueryRow(r.Context(),"SELECT remote_id FROM backup_policies WHERE chat_id=$1 FOR UPDATE",id).Scan(&existing)
  if err!=nil&&!errors.Is(err,pgx.ErrNoRows){fail(w);return}
  if existing!=0&&existing!=remote{http.Error(w,"destination is immutable for an existing backup policy",409);return}
  tag,err:=tx.Exec(r.Context(),`INSERT INTO backup_policies(chat_id,remote_id,enabled,scan_interval_seconds)
  SELECT c.id,s.id,$3,$4 FROM telegram_chats c JOIN telegram_accounts a ON a.id=c.account_id
  CROSS JOIN storage_remotes s WHERE c.id=$1 AND s.id=$2 AND a.enabled AND s.enabled
  ON CONFLICT(chat_id) DO UPDATE SET enabled=EXCLUDED.enabled,scan_interval_seconds=EXCLUDED.scan_interval_seconds`,id,remote,req.Enabled,req.Interval)
  if err!=nil||tag.RowsAffected()!=1{fail(w);return}
  if _,err=tx.Exec(r.Context(),"UPDATE telegram_chats SET enabled=$2 WHERE id=$1",id,req.Enabled);err!=nil{fail(w);return}
  if _,err=tx.Exec(r.Context(),"INSERT INTO sync_checkpoints(chat_id) VALUES($1) ON CONFLICT(chat_id) DO UPDATE SET next_scan_at=now()",id);err!=nil{fail(w);return}
  if tx.Commit(r.Context())!=nil{fail(w);return};reply(w,map[string]bool{"ok":true})
 })
 r.Post("/chats/{id}/sync",func(w http.ResponseWriter,r *http.Request){
  id,err:=idParam(r);if err!=nil{fail(w);return}
  tag,err:=a.Pool.Exec(r.Context(),`UPDATE sync_checkpoints s SET next_scan_at=now() FROM backup_policies p
   WHERE s.chat_id=$1 AND p.chat_id=s.chat_id AND p.enabled`,id)
  if err!=nil||tag.RowsAffected()!=1{fail(w);return};reply(w,map[string]bool{"ok":true})
 })
 r.Post("/tasks/{id}/retry",func(w http.ResponseWriter,r *http.Request){
  id,err:=idParam(r);if err!=nil{fail(w);return}
  tx,err:=a.Pool.Begin(r.Context());if err!=nil{fail(w);return};defer tx.Rollback(r.Context())
  var state string
  if err=tx.QueryRow(r.Context(),"SELECT state FROM media_files WHERE id=$1 FOR UPDATE",id).Scan(&state);err!=nil||state=="completed"||state=="blocked"{fail(w);return}
  var jobID int64
  err=tx.QueryRow(r.Context(),`SELECT j.id FROM river_job j WHERE j.state IN ('discarded','cancelled')
  AND (($2 IN ('discovered','downloading','retry_wait') AND j.kind='media_download' AND j.args->>'media_id'=$1::text)
  OR ($2 IN ('downloaded','uploading','verifying') AND j.kind='media_upload' AND EXISTS(SELECT 1 FROM media_replicas p WHERE p.media_id=$1 AND p.state<>'verified' AND p.id::text=j.args->>'replica_id'))
  OR ($2='cleanup_pending' AND j.kind='media_cleanup' AND j.args->>'media_id'=$1::text))
  ORDER BY j.id DESC LIMIT 1 FOR UPDATE`,id,state).Scan(&jobID)
  if err!=nil{http.Error(w,"no discarded job to retry",409);return}
  if _,err=a.Engine.Jobs.JobRetryTx(r.Context(),tx,jobID);err!=nil{fail(w);return}
  if tx.Commit(r.Context())!=nil{fail(w);return};reply(w,map[string]bool{"ok":true})
 })
 r.Get("/capabilities",func(w http.ResponseWriter,r *http.Request){reply(w,map[string]any{"workers_enabled":true,"sync_mode":"realtime+polling","storage_config":"encrypted Drive OAuth token","google_oauth":a.OAuth!=nil})})
}

func(a *Actions)accountExists(ctx context.Context,id int64)bool{
 var exists bool
 return a.Pool.QueryRow(ctx,"SELECT EXISTS(SELECT 1 FROM telegram_accounts WHERE id=$1)",id).Scan(&exists)==nil&&exists
}
