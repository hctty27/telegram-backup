package telegram

import (
 "context"
 "errors"
 "sync"
 "time"

 gotd "github.com/gotd/td/telegram"
 "github.com/gotd/td/telegram/query/messages"
 "github.com/gotd/td/tg"
)

type LiveEvent struct {
 AccountID int64
 PeerKind string
 PeerID int64
 Media Media
}
type LiveHandler func(context.Context,LiveEvent)error
type request struct{
 ctx context.Context
 fn func(context.Context,*gotd.Client)error
 result chan error
}
type resident struct{
 cancel context.CancelFunc
 done chan struct{}
 requests chan request
}
type Residents struct{
 service *Service
 root context.Context
 handler LiveHandler
 mu sync.Mutex
 clients map[int64]*resident
 suspended map[int64]bool
 wg sync.WaitGroup
}
func NewResidents(ctx context.Context,s *Service,h LiveHandler)*Residents{
 return &Residents{service:s,root:ctx,handler:h,clients:make(map[int64]*resident),suspended:make(map[int64]bool)}
}
func(r *Residents)get(id int64)(*resident,error){
 r.mu.Lock();defer r.mu.Unlock()
 if r.root.Err()!=nil{return nil,r.root.Err()}
 if r.suspended[id]{return nil,errors.New("account login in progress")}
 if c:=r.clients[id];c!=nil{return c,nil}
 ctx,cancel:=context.WithCancel(r.root)
 c:=&resident{cancel:cancel,done:make(chan struct{}),requests:make(chan request)}
 r.clients[id]=c
 r.wg.Add(1)
 go r.run(ctx,id,c)
 return c,nil
}
func(r *Residents)Do(ctx context.Context,id int64,fn func(context.Context,*gotd.Client)error)error{
 c,err:=r.get(id);if err!=nil{return err}
 req:=request{ctx:ctx,fn:fn,result:make(chan error,1)}
 select{case c.requests<-req:case <-c.done:return errors.New("Telegram connection unavailable");case <-ctx.Done():return ctx.Err()}
 // Wait for accepted operations to exit before releasing their writer or file.
 // The operation context is cancelled when the client or caller stops.
 return <-req.result
}
// Suspend prevents a resident from reconnecting while QR authorization owns
// the same account. Service.Run's gate is the final ownership barrier.
func(r *Residents)Suspend(id int64)func(){
 r.mu.Lock();r.suspended[id]=true
 c:=r.clients[id];if c!=nil{c.cancel()}
 r.mu.Unlock()
 return func(){r.mu.Lock();delete(r.suspended,id);r.mu.Unlock()}
}
func(r *Residents)run(ctx context.Context,id int64,c *resident){
 defer r.wg.Done()
 defer func(){r.mu.Lock();if r.clients[id]==c{delete(r.clients,id)};close(c.done);r.mu.Unlock()}()
 d:=tg.NewUpdateDispatcher()
 deliver:=func(ctx context.Context,msg tg.MessageClass)error{
  m,ok:=msg.(*tg.Message);if !ok{return nil}
  media,ok:=extract(messages.Elem{Msg:m});if !ok{return nil}
  e:=LiveEvent{AccountID:id,Media:media}
  switch p:=m.PeerID.(type){
  case *tg.PeerChat:e.PeerKind="chat";e.PeerID=p.ChatID
  case *tg.PeerChannel:e.PeerKind="channel";e.PeerID=p.ChannelID
  case *tg.PeerUser:e.PeerKind="user";e.PeerID=p.UserID
  default:return nil
  }
  if r.handler==nil{return nil}
  call,cancel:=context.WithTimeout(ctx,5*time.Second);defer cancel()
  return r.handler(call,e)
 }
 d.OnNewMessage(func(ctx context.Context,_ tg.Entities,u *tg.UpdateNewMessage)error{return deliver(ctx,u.Message)})
 d.OnNewChannelMessage(func(ctx context.Context,_ tg.Entities,u *tg.UpdateNewChannelMessage)error{return deliver(ctx,u.Message)})
 // Single client owner; updates are delivered independently of serialized RPC
 // operations. Periodic history scans remain the durable gap-recovery path.
 _=r.service.Run(ctx,id,d,func(life context.Context,client *gotd.Client)error{
  status,err:=client.Auth().Status(life);if err!=nil{return err}
  if !status.Authorized{return errors.New("Telegram account not authorized")}
  for{
   select{
   case <-life.Done():return life.Err()
   case req:=<-c.requests:
    op,cancel:=context.WithCancel(req.ctx)
    stop:=context.AfterFunc(life,cancel)
    err:=r.service.checkBackoff(op,id)
    if err==nil{err=req.fn(op,client)}
    stop();cancel()
    err=r.service.recordBackoff(id,err)
    req.result<-err
   }
  }
 })
}
// Reconcile periodically starts listeners for enabled accounts, even if no
// backup task is currently running. Failed clients retry on the next tick.
func(r *Residents)Reconcile(ctx context.Context)error{
 rows,err:=r.service.Pool.Query(ctx,"SELECT id FROM telegram_accounts WHERE enabled AND flood_wait_until<=now()");if err!=nil{return err}
 ids:=map[int64]bool{}
 for rows.Next(){var id int64;if err=rows.Scan(&id);err!=nil{rows.Close();return err};ids[id]=true}
 err=rows.Err();rows.Close();if err!=nil{return err}
 r.mu.Lock()
 for id,c:=range r.clients{if !ids[id]{c.cancel()}}
 r.mu.Unlock()
 for id:=range ids{_,_=r.get(id)}
 return nil
}
func(r *Residents)Wait(){r.wg.Wait()}
