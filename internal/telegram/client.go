package telegram

import(
 "context"
 "errors"
 "fmt"
 "sync"
 "time"
 "github.com/gotd/td/tgerr"

 gotd "github.com/gotd/td/telegram"
 "github.com/gotd/td/tg"
 "github.com/hctty27/telegram-backup/internal/security"
 "github.com/jackc/pgx/v5/pgxpool"
)

type BackoffError struct{ Wait time.Duration }
func(e *BackoffError)Error()string{return "Telegram rate limit active"}

type Service struct{
 Pool *pgxpool.Pool
 Box *security.Box
 mu sync.Mutex
 gates map[int64]chan struct{}
}

func New(pool *pgxpool.Pool,box *security.Box)*Service{
 return &Service{Pool:pool,Box:box,gates:make(map[int64]chan struct{})}
}
func(s *Service)gate(id int64)chan struct{}{
 s.mu.Lock();defer s.mu.Unlock()
 if s.gates[id]==nil{s.gates[id]=make(chan struct{},1)}
 return s.gates[id]
}
func(s *Service)CreateAccount(ctx context.Context,appID int,appHash,label string)(int64,error){
 if appID<=0||len(appHash)!=32||len(label)>200{return 0,errors.New("invalid Telegram credentials")}
 tx,err:=s.Pool.Begin(ctx);if err!=nil{return 0,err};defer tx.Rollback(ctx)
 var id int64
 if err=tx.QueryRow(ctx,"INSERT INTO telegram_accounts(api_id,api_hash_ciphertext,display_name) VALUES($1,$2,$3) RETURNING id",appID,[]byte{},label).Scan(&id);err!=nil{return 0,err}
 data,err:=s.Box.Seal(fmt.Sprintf("telegram-api:%d:v1",id),[]byte(appHash));if err!=nil{return 0,err}
 if _,err=tx.Exec(ctx,"UPDATE telegram_accounts SET api_hash_ciphertext=$2 WHERE id=$1",id,data);err!=nil{return 0,err}
 return id,tx.Commit(ctx)
}

// Run owns the session for the duration of one operation. Initially serialize
// account operations, including login; never run two clients on one session.
func(s *Service)Run(ctx context.Context,id int64,updates gotd.UpdateHandler,fn func(context.Context,*gotd.Client)error)error{
 g:=s.gate(id)
 select{case g<-struct{}{}:defer func(){<-g}();case <-ctx.Done():return ctx.Err()}
 var appID int
 var enc []byte
 var waitSeconds float64
 if err:=s.Pool.QueryRow(ctx,"SELECT api_id,api_hash_ciphertext,CASE WHEN flood_wait_until>now() THEN extract(epoch FROM flood_wait_until-now())::float8 ELSE 0 END FROM telegram_accounts WHERE id=$1",id).Scan(&appID,&enc,&waitSeconds);err!=nil{return err}
 if waitSeconds>0{return &BackoffError{Wait:time.Duration(waitSeconds*float64(time.Second))+time.Second}}
 secret,err:=s.Box.Open(fmt.Sprintf("telegram-api:%d:v1",id),enc);if err!=nil{return err}
 defer clear(secret)
 client:=gotd.NewClient(appID,string(secret),gotd.Options{UpdateHandler:updates,NoUpdates:updates==nil,SessionStorage:&SessionStore{Pool:s.Pool,Box:s.Box,AccountID:id}})
 err=client.Run(ctx,func(ctx context.Context)error{return fn(ctx,client)})
 if wait,ok:=tgerr.AsFloodWait(err);ok{
  wait=max(wait,time.Second)+time.Second
  save,cancel:=context.WithTimeout(context.Background(),3*time.Second);defer cancel()
  if _,dbErr:=s.Pool.Exec(save,"UPDATE telegram_accounts SET flood_wait_until=greatest(flood_wait_until,now()+make_interval(secs=>$2)) WHERE id=$1",id,wait.Seconds());dbErr!=nil{return dbErr}
  return &BackoffError{Wait:wait}
 }
 return err
}
func(s *Service)Authorized(ctx context.Context,id int64,fn func(context.Context,*gotd.Client)error)error{
 return s.Run(ctx,id,nil,func(ctx context.Context,c *gotd.Client)error{
  status,err:=c.Auth().Status(ctx);if err!=nil{return err}
  if !status.Authorized{return errors.New("Telegram account not authorized")}
  return fn(ctx,c)
 })
}
func Peer(kind string,id,hash int64)(tg.InputPeerClass,error){
 switch kind{
 case "self":return &tg.InputPeerSelf{},nil
 case "chat":return &tg.InputPeerChat{ChatID:id},nil
 case "channel":return &tg.InputPeerChannel{ChannelID:id,AccessHash:hash},nil
 case "user":return &tg.InputPeerUser{UserID:id,AccessHash:hash},nil
 default:return nil,errors.New("unsupported peer")
 }
}
