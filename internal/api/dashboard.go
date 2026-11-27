package api

import(
 "context"
 "crypto/sha256"
 "crypto/subtle"
 "encoding/json"
 "errors"
 "io/fs"
 "net/http"
 "strings"
 "time"

 "github.com/go-chi/chi/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)

func NewDashboardRouter(pool *pgxpool.Pool,token string,assets fs.FS, additional ...func(chi.Router))(http.Handler,error){
 if pool==nil||len(token)<32||assets==nil{return nil,errors.New("database, assets, and a strong admin token required")}
 hash:=sha256.Sum256([]byte(token))
 r:=chi.NewRouter()
 r.Get("/api/health",NewRouter().ServeHTTP)
 r.Get("/api/ready",func(w http.ResponseWriter,r *http.Request){
  ctx,cancel:=context.WithTimeout(r.Context(),3*time.Second);defer cancel()
  if pool.Ping(ctx)!=nil{http.Error(w,"database unavailable",503);return}
  w.WriteHeader(204)
 })
 r.Route("/api/v2",func(r chi.Router){
  r.Use(func(next http.Handler)http.Handler{return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
   w.Header().Set("Cache-Control","no-store")
   h:=r.Header.Get("Authorization")
   got:=sha256.Sum256([]byte(strings.TrimPrefix(h,"Bearer ")))
   if !strings.HasPrefix(h,"Bearer ")||subtle.ConstantTimeCompare(got[:],hash[:])!=1{
    http.Error(w,"unauthorized",401);return
   }
   next.ServeHTTP(w,r)
  })})
  r.Get("/overview",jsonQuery(pool,`SELECT json_build_object(
   'accounts',(SELECT count(*) FROM telegram_accounts),
   'chats',(SELECT count(*) FROM telegram_chats WHERE enabled),
   'completed',(SELECT count(*) FROM media_files WHERE state='completed'),
   'active',(SELECT count(*) FROM media_files WHERE state IN ('downloading','uploading','verifying')),
   'failed',(SELECT count(*) FROM media_files WHERE state IN ('failed','blocked')),
   'staged_bytes',(SELECT coalesce(sum(staged_bytes),0) FROM media_files),
   'verified_bytes',(SELECT coalesce(sum(m.expected_bytes),0) FROM media_files m WHERE m.state='completed')
  )`))
  r.Get("/accounts",jsonQuery(pool,`SELECT coalesce(json_agg(x),'[]'::json) FROM
   (SELECT id::text,display_name,enabled,created_at FROM telegram_accounts ORDER BY id DESC LIMIT 100)x`))
  r.Get("/chats",jsonQuery(pool,`SELECT coalesce(json_agg(x),'[]'::json) FROM
   (SELECT c.id::text,c.title,c.peer_kind,c.enabled,p.remote_id::text,p.scan_interval_seconds FROM telegram_chats c LEFT JOIN backup_policies p ON p.chat_id=c.id ORDER BY c.id DESC LIMIT 100)x`))
  r.Get("/tasks",jsonQuery(pool,`SELECT coalesce(json_agg(x),'[]'::json) FROM
   (SELECT id::text,filename,state,expected_bytes,updated_at,last_error FROM media_files WHERE state<>'completed' ORDER BY id DESC LIMIT 100)x`))
  r.Get("/archive",jsonQuery(pool,`SELECT coalesce(json_agg(x),'[]'::json) FROM
   (SELECT id::text,filename,state,expected_bytes,updated_at FROM media_files WHERE state='completed' ORDER BY id DESC LIMIT 100)x`))
  for _,register:=range additional{register(r)}
  r.Get("/storage",jsonQuery(pool,`SELECT coalesce(json_agg(x),'[]'::json) FROM
   (SELECT id::text,name,kind,enabled FROM storage_remotes ORDER BY id DESC LIMIT 100)x`))
 })
 fileServer:=http.FileServerFS(assets)
 r.Get("/*",func(w http.ResponseWriter,r *http.Request){
  if strings.HasPrefix(r.URL.Path,"/api/")||r.URL.Path=="/api"{http.NotFound(w,r);return}
  w.Header().Set("X-Content-Type-Options","nosniff")
  w.Header().Set("Referrer-Policy","no-referrer")
  w.Header().Set("X-Frame-Options","DENY")
  if r.URL.Path=="/"||!strings.HasPrefix(r.URL.Path,"/assets/"){
   w.Header().Set("Cache-Control","no-store")
   http.ServeFileFS(w,r,assets,"index.html");return
  }
  fileServer.ServeHTTP(w,r)
 })
 return r,nil
}

func jsonQuery(pool *pgxpool.Pool,sql string)http.HandlerFunc{
 return func(w http.ResponseWriter,r *http.Request){
  ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second);defer cancel()
  var data []byte
  if err:=pool.QueryRow(ctx,sql).Scan(&data);err!=nil||!json.Valid(data){
   http.Error(w,"database query unavailable",503);return
  }
  w.Header().Set("Content-Type","application/json")
  _,_=w.Write(data)
 }
}
