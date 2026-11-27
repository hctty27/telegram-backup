package telegram

import (
 "context"
 "crypto/rand"
 "encoding/base64"
 "errors"
 "regexp"
 "sync"
 "time"

 gotd "github.com/gotd/td/telegram"
 "github.com/gotd/td/telegram/auth"
 "github.com/gotd/td/telegram/auth/qrlogin"
 "github.com/gotd/td/tg"
 "github.com/gotd/td/tgerr"
 "rsc.io/qr"
)

type LoginStatus struct {
 State string `json:"state"`
 AttemptID string `json:"attempt_id,omitempty"`
 Method string `json:"method,omitempty"`
 QR string `json:"qr,omitempty"`
 Expires time.Time `json:"expires,omitempty"`
 Deadline time.Time `json:"deadline,omitempty"`
 Channel string `json:"channel,omitempty"`
 CodeLength int `json:"code_length,omitempty"`
 CanResend bool `json:"can_resend,omitempty"`
 ResendAt time.Time `json:"resend_at,omitempty"`
 Error string `json:"error,omitempty"`
 RetryAt time.Time `json:"retry_at,omitempty"`
}

type loginInput struct { kind, value string }
type attempt struct {
 status LoginStatus
 input chan loginInput
 cancel context.CancelFunc
 active bool
 cancelling bool
 deadline time.Time
 id, method string
}
type LoginManager struct {
 service *Service
 root context.Context
 mu sync.Mutex
 attempts map[int64]*attempt
 phoneStarts map[int64]time.Time
}
func NewLoginManager(ctx context.Context,s *Service)*LoginManager{
 return &LoginManager{service:s,root:ctx,attempts:make(map[int64]*attempt),phoneStarts:make(map[int64]time.Time)}
}
func(m *LoginManager)Status(id int64)LoginStatus{
 m.mu.Lock();defer m.mu.Unlock()
 if a:=m.attempts[id];a!=nil{return a.status}
 return LoginStatus{State:"idle"}
}
func(m *LoginManager)set(a *attempt,status LoginStatus){
 m.mu.Lock();defer m.mu.Unlock()
 if a.cancelling||!a.active{return}
 status.AttemptID=a.id;status.Method=a.method;status.Deadline=a.deadline
 a.status=status
}
var internationalPhone=regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
var verificationCode=regexp.MustCompile(`^[0-9]{1,20}$`)
func(m *LoginManager)Start(id int64)error{return m.start(id,"qr","")}
func(m *LoginManager)StartPhone(id int64,phone string)error{
 if !internationalPhone.MatchString(phone){return errors.New("invalid international phone")}
 return m.start(id,"phone",phone)
}
func(m *LoginManager)start(id int64,method,phone string)error{
 m.mu.Lock();defer m.mu.Unlock()
 if m.root.Err()!=nil||id<=0{return errors.New("login unavailable")}
 if a:=m.attempts[id];a!=nil&&a.active{return errors.New("login already active")}
 now:=time.Now()
 if a:=m.attempts[id];a!=nil&&a.status.RetryAt.After(now){return errors.New("login cooldown")}
 active:=0
 for _,a:=range m.attempts{if a.active{active++}}
 if active>=8{return errors.New("too many active logins")}
 if method=="phone"{
  if m.phoneStarts[id].After(now.Add(-30*time.Second)){return errors.New("login cooldown")}
  m.phoneStarts[id]=now
 }
 ctx,cancel:=context.WithTimeout(m.root,5*time.Minute)
 deadline,_:=ctx.Deadline()
 a:=&attempt{id:rand.Text(),method:method,deadline:deadline,input:make(chan loginInput,1),cancel:cancel,active:true}
 a.status=LoginStatus{State:"connecting",AttemptID:a.id,Method:method,Deadline:deadline}
 m.attempts[id]=a
 go m.run(ctx,id,a,phone)
 return nil
}
func(m *LoginManager)submit(id int64,attemptID,kind,value string)error{
 m.mu.Lock();defer m.mu.Unlock()
 a:=m.attempts[id]
 if a==nil||!a.active||a.cancelling||a.id!=attemptID||time.Now().After(a.deadline){return errors.New("no active login")}
 switch kind{
 case "password":
  if a.status.State!="password"||len(value)==0||len(value)>1024{return errors.New("invalid password submission")}
 case "code":
  if a.status.State!="code"||!verificationCode.MatchString(value)||(a.status.CodeLength>0&&len(value)!=a.status.CodeLength){return errors.New("invalid code submission")}
 case "resend":
  if a.status.State!="code"||!a.status.CanResend||time.Now().Before(a.status.ResendAt){return errors.New("resend unavailable")}
 default:return errors.New("invalid action")
 }
 select{
 case a.input<-loginInput{kind,value}:
  a.status=LoginStatus{State:"checking",AttemptID:a.id,Method:a.method,Deadline:a.deadline}
  return nil
 default:return errors.New("submission already pending")
 }
}
func(m *LoginManager)Password(id int64,attemptID,password string)error{return m.submit(id,attemptID,"password",password)}
func(m *LoginManager)Code(id int64,attemptID,code string)error{return m.submit(id,attemptID,"code",code)}
func(m *LoginManager)Resend(id int64,attemptID string)error{return m.submit(id,attemptID,"resend","")}
func(m *LoginManager)Cancel(id int64,attemptID string)error{
 m.mu.Lock();defer m.mu.Unlock()
 a:=m.attempts[id]
 if a==nil||!a.active||a.id!=attemptID{return errors.New("no active login")}
 if !a.cancelling{
  a.cancelling=true
  a.status=LoginStatus{State:"cancelling",AttemptID:a.id,Method:a.method,Deadline:a.deadline}
  a.cancel()
 }
 return nil
}
func(m *LoginManager)finish(a *attempt,err error,expired bool){
 m.mu.Lock();defer m.mu.Unlock()
 status:=LoginStatus{State:"authorized",AttemptID:a.id,Method:a.method,Deadline:a.deadline}
 if err!=nil{
  status.State="failed";status.Error=loginError(err)
  var backoff *BackoffError
  if errors.As(err,&backoff){status.Error="rate_limited";status.RetryAt=time.Now().Add(backoff.Wait)}
  if a.cancelling{status.State="cancelled";status.Error=""}else if expired{status.State="expired";status.Error="timeout"}
 }
 // Do not allow a new attempt until the previous client and resident suspension
 // have been released. Drop any unconsumed credential from the channel.
 select{case <-a.input:default:}
 a.active=false;a.status=status
}
func(m *LoginManager)run(ctx context.Context,id int64,a *attempt,phone string){
 var err error
 defer func(){m.finish(a,err,errors.Is(ctx.Err(),context.DeadlineExceeded));a.cancel()}()
 if m.service.Residents!=nil{resume:=m.service.Residents.Suspend(id);defer resume()}
 dispatcher:=tg.NewUpdateDispatcher()
 loggedIn:=qrlogin.OnLoginToken(&dispatcher)
 err=m.service.Run(ctx,id,dispatcher,func(ctx context.Context,c *gotd.Client)error{
  status,err:=c.Auth().Status(ctx);if err!=nil{return err}
  if !status.Authorized{
   if a.method=="phone"{
    err=m.phoneLogin(ctx,a,phone,c.Auth())
   }else{
    _,err=c.QR().Auth(ctx,loggedIn,func(_ context.Context,token qrlogin.Token)error{
     data,err:=loginQR(token);if err!=nil{return err}
     m.set(a,LoginStatus{State:"waiting",QR:data,Expires:token.Expires()})
     return nil
    })
   }
   if errors.Is(err,auth.ErrPasswordAuthNeeded)||tgerr.Is(err,"SESSION_PASSWORD_NEEDED"){
    err=m.passwordLogin(ctx,a,c.Auth())
   }
   if err!=nil{return err}
  }
  user,err:=c.Self(ctx);if err!=nil{return err}
  if user.Bot{return errors.New("user account required")}
  tag,err:=m.service.Pool.Exec(ctx,"UPDATE telegram_accounts SET telegram_user_id=$2,enabled=true WHERE id=$1 AND (telegram_user_id IS NULL OR telegram_user_id=$2)",id,user.ID)
  if err!=nil{return err};if tag.RowsAffected()!=1{return errIdentityChanged};return nil
 })
}

type passwordAuth interface {
 Password(context.Context,string)(*tg.AuthAuthorization,error)
}
func(m *LoginManager)passwordLogin(ctx context.Context,a *attempt,c passwordAuth)error{
 message:=""
 for i:=0;i<5;i++{
  m.set(a,LoginStatus{State:"password",Error:message})
  select{
  case <-ctx.Done():return ctx.Err()
  case input:=<-a.input:
   if ctx.Err()!=nil{return ctx.Err()}
   _,err:=c.Password(ctx,input.value)
   if err==nil{return nil}
   if !errors.Is(err,auth.ErrPasswordInvalid)&&!tgerr.Is(err,"PASSWORD_HASH_INVALID"){return err}
   message="invalid_password"
  }
 }
 return errAttempts
}

var errIdentityChanged=errors.New("account identity changed")
var errAttempts=errors.New("login attempts exhausted")
var errChannel=errors.New("unsupported login channel")
func loginError(err error)string{
 switch{
 case errors.Is(err,errIdentityChanged):return "identity_changed"
 case errors.Is(err,errAttempts):return "attempts_exhausted"
 case errors.Is(err,errChannel):return "unsupported_channel"
 case tgerr.Is(err,"PHONE_NUMBER_INVALID"):return "invalid_phone"
 case tgerr.Is(err,"PHONE_NUMBER_BANNED"):return "phone_banned"
 case tgerr.Is(err,"PHONE_CODE_EXPIRED"):return "code_expired"
 case tgerr.Is(err,"SEND_CODE_UNAVAILABLE"):return "resend_unavailable"
 case tgerr.Is(err,"PHONE_NUMBER_APP_SIGNUP_FORBIDDEN"):return "unsupported_channel"
 case tgerr.Is(err,"AUTH_RESTART"):return "restart_required"
 default:
  var signup *auth.SignUpRequired
  if errors.As(err,&signup){return "signup_required"}
  return "login_failed"
 }
}

// Dedicated PNG encoder scales all modules and preserves the quiet zone.
func loginQR(token qrlogin.Token)(string,error){
 code,err:=qr.Encode(token.URL(),qr.M)
 if err!=nil{return "",err}
 code.Scale=8
 return "data:image/png;base64,"+base64.StdEncoding.EncodeToString(code.PNG()),nil
}
