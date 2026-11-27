package telegram

import(
 "bytes"
 "context"
 "encoding/base64"
 "errors"
 "image/png"
 "sync"
 "time"

 gotd "github.com/gotd/td/telegram"
 "github.com/gotd/td/telegram/auth/qrlogin"
 "github.com/gotd/td/tg"
 "github.com/gotd/td/tgerr"
)

type LoginStatus struct{
 State string `json:"state"`
 QR string `json:"qr,omitempty"`
 Expires time.Time `json:"expires,omitempty"`
}
type attempt struct{
 status LoginStatus
 password chan string
 cancel context.CancelFunc
}
type LoginManager struct{
 service *Service
 root context.Context
 mu sync.Mutex
 attempts map[int64]*attempt
}
func NewLoginManager(ctx context.Context,s *Service)*LoginManager{
 return &LoginManager{service:s,root:ctx,attempts:make(map[int64]*attempt)}
}
func(m *LoginManager)Status(id int64)LoginStatus{
 m.mu.Lock();defer m.mu.Unlock()
 if a:=m.attempts[id];a!=nil{return a.status}
 return LoginStatus{State:"idle"}
}
func(m *LoginManager)set(a *attempt,status LoginStatus){m.mu.Lock();a.status=status;m.mu.Unlock()}
func(m *LoginManager)Start(id int64)error{
 m.mu.Lock();defer m.mu.Unlock()
 if a:=m.attempts[id];a!=nil&&(a.status.State=="connecting"||a.status.State=="waiting"||a.status.State=="password"||a.status.State=="checking"){return errors.New("login already active")}
 ctx,cancel:=context.WithTimeout(m.root,5*time.Minute)
 a:=&attempt{status:LoginStatus{State:"connecting"},password:make(chan string,1),cancel:cancel}
 m.attempts[id]=a
 go m.run(ctx,id,a)
 return nil
}
func(m *LoginManager)Password(id int64,password string)error{
 m.mu.Lock();defer m.mu.Unlock()
 a:=m.attempts[id]
 if a==nil||a.status.State!="password"||len(password)==0||len(password)>1024{return errors.New("no pending password request")}
 select{case a.password<-password:a.status=LoginStatus{State:"checking"};return nil;default:return errors.New("password already submitted")}
}
func(m *LoginManager)run(ctx context.Context,id int64,a *attempt){
 defer a.cancel()
 dispatcher:=tg.NewUpdateDispatcher()
 loggedIn:=qrlogin.OnLoginToken(&dispatcher)
 err:=m.service.Run(ctx,id,dispatcher,func(ctx context.Context,c *gotd.Client)error{
  status,err:=c.Auth().Status(ctx);if err!=nil{return err}
  if !status.Authorized{
   _,err=c.QR().Auth(ctx,loggedIn,func(_ context.Context,token qrlogin.Token)error{
    img,err:=token.Image(1);if err!=nil{return err}
    var b bytes.Buffer
    if err=png.Encode(&b,img);err!=nil{return err}
    m.set(a,LoginStatus{State:"waiting",QR:"data:image/png;base64,"+base64.StdEncoding.EncodeToString(b.Bytes()),Expires:token.Expires()})
    return nil
   })
   if tgerr.Is(err,"SESSION_PASSWORD_NEEDED"){
    m.set(a,LoginStatus{State:"password"})
    select{case password:=<-a.password:_,err=c.Auth().Password(ctx,password);case <-ctx.Done():return ctx.Err()}
   }
   if err!=nil{return err}
  }
  user,err:=c.Self(ctx);if err!=nil{return err}
  if user.Bot{return errors.New("user account required")}
  _,err=m.service.Pool.Exec(ctx,"UPDATE telegram_accounts SET telegram_user_id=$2,enabled=true WHERE id=$1 AND (telegram_user_id IS NULL OR telegram_user_id=$2)",id,user.ID)
  return err
 })
 if err!=nil{m.set(a,LoginStatus{State:"failed"});return}
 m.set(a,LoginStatus{State:"authorized"})
}
