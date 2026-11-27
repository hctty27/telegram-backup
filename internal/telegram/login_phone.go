package telegram

import (
 "context"
 "time"

 "github.com/gotd/td/telegram/auth"
 "github.com/gotd/td/tg"
 "github.com/gotd/td/tgerr"
)

type phoneAuth interface {
 SendCode(context.Context,string,auth.SendCodeOptions)(tg.AuthSentCodeClass,error)
 ResendCode(context.Context,string,string)(tg.AuthSentCodeClass,error)
 SignIn(context.Context,string,string,string)(*tg.AuthAuthorization,error)
}

// Only support channels that use a conventional numeric phone code. Do not
// fake SMS delivery, register accounts or bypass official-client challenges.
func phoneChallenge(sent tg.AuthSentCodeClass,resends int,now time.Time)(LoginStatus,string,bool,error){
 switch v:=sent.(type){
 case *tg.AuthSentCodeSuccess:return LoginStatus{},"",true,nil
 case *tg.AuthSentCode:
  status:=LoginStatus{State:"code",CanResend:v.NextType!=nil&&resends<2}
  switch t:=v.Type.(type){
  case *tg.AuthSentCodeTypeApp:status.Channel="app";status.CodeLength=t.Length
  case *tg.AuthSentCodeTypeSMS:status.Channel="sms";status.CodeLength=t.Length
  case *tg.AuthSentCodeTypeCall:status.Channel="call";status.CodeLength=t.Length
  case *tg.AuthSentCodeTypeMissedCall:status.Channel="missed_call";status.CodeLength=t.Length
  default:return LoginStatus{},"",false,errChannel
  }
  if v.PhoneCodeHash==""||status.CodeLength<1||status.CodeLength>20{return LoginStatus{},"",false,errChannel}
  if status.CanResend{status.ResendAt=now.Add(time.Duration(max(v.Timeout,30))*time.Second)}
  return status,v.PhoneCodeHash,false,nil
 default:return LoginStatus{},"",false,errChannel
 }
}
func(m *LoginManager)phoneLogin(ctx context.Context,a *attempt,phone string,c phoneAuth)error{
 sent,err:=c.SendCode(ctx,phone,auth.SendCodeOptions{})
 if err!=nil{return err}
 resends,invalid:=0,0
 status,hash,authorized,err:=phoneChallenge(sent,resends,time.Now())
 if err!=nil||authorized{return err}
 for{
  m.set(a,status)
  select{
  case <-ctx.Done():return ctx.Err()
  case input:=<-a.input:
   if ctx.Err()!=nil{return ctx.Err()}
   if input.kind=="resend"{
    if !status.CanResend||time.Now().Before(status.ResendAt){return errChannel}
    sent,err=c.ResendCode(ctx,phone,hash)
    if err!=nil{return err}
    resends++
    status,hash,authorized,err=phoneChallenge(sent,resends,time.Now())
    if err!=nil||authorized{return err}
    continue
   }
   _,err=c.SignIn(ctx,phone,input.value,hash)
   if err==nil{return nil}
   if tgerr.Is(err,"PHONE_CODE_INVALID")||tgerr.Is(err,"PHONE_CODE_EMPTY"){
    invalid++
    if invalid>=5{return errAttempts}
    status.Error="invalid_code"
    continue
   }
   return err
  }
 }
}
