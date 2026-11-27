package telegram

import (
 "context"
 "errors"
 "testing"
 "time"

 "github.com/gotd/td/telegram/auth"
 "github.com/gotd/td/tg"
)

func TestPhoneChallenge(t *testing.T){
 now:=time.Now()
 code:=&tg.AuthSentCode{Type:&tg.AuthSentCodeTypeApp{Length:5},PhoneCodeHash:"fixture",NextType:&tg.AuthCodeTypeSMS{},Timeout:60}
 status,hash,done,err:=phoneChallenge(code,0,now)
 if err!=nil||done||hash!="fixture"||status.Channel!="app"||status.CodeLength!=5||!status.CanResend||!status.ResendAt.Equal(now.Add(time.Minute)){t.Fatal("invalid app challenge")}
 status,_,_,err=phoneChallenge(code,2,now)
 if err!=nil||status.CanResend{t.Fatal("resend cap ignored")}
 code.Type=&tg.AuthSentCodeTypeSMS{Length:6};code.NextType=nil
 status,_,_,err=phoneChallenge(code,0,now)
 if err!=nil||status.Channel!="sms"||status.CodeLength!=6||status.CanResend{t.Fatal("invalid SMS challenge")}
 code.Type=&tg.AuthSentCodeTypeSetUpEmailRequired{}
 if _,_,_,err=phoneChallenge(code,0,now);!errors.Is(err,errChannel){t.Fatal("unsupported channel accepted")}
 if _,_,done,err=phoneChallenge(&tg.AuthSentCodeSuccess{},0,now);err!=nil||!done{t.Fatal("sent code success not recognized")}
}
func TestLoginSubmissionGuards(t *testing.T){
 m:=NewLoginManager(context.Background(),nil)
 now:=time.Now()
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 a:=&attempt{id:"test",method:"phone",deadline:now.Add(time.Minute),active:true,input:make(chan loginInput,1),cancel:cancel}
 m.attempts[1]=a
 m.set(a,LoginStatus{State:"code",CodeLength:5,CanResend:true,ResendAt:now.Add(time.Minute)})
 if m.Code(1,"old","12345")==nil||m.Code(1,"test","123")==nil||m.Resend(1,"test")==nil||m.Password(1,"test","secret")==nil{t.Fatal("invalid action accepted")}
 if err:=m.Code(1,"test","01234");err!=nil{t.Fatal(err)}
 if m.Code(1,"test","12345")==nil{t.Fatal("duplicate code accepted")}
 if in:=<-a.input;in.kind!="code"||in.value!="01234"{t.Fatal("unexpected submission")}
 if err:=m.Cancel(1,"test");err!=nil{t.Fatal(err)}
 if ctx.Err()==nil||m.Status(1).State!="cancelling"{t.Fatal("cancel not propagated")}
 m.finish(a,ctx.Err(),false)
 if m.Status(1).State!="cancelled"||a.active{t.Fatal("attempt not released")}
}
type fakePhoneAuth struct{signed bool}
func(f *fakePhoneAuth)SendCode(context.Context,string,auth.SendCodeOptions)(tg.AuthSentCodeClass,error){return &tg.AuthSentCode{Type:&tg.AuthSentCodeTypeApp{Length:5},PhoneCodeHash:"fixture"},nil}
func(f *fakePhoneAuth)ResendCode(context.Context,string,string)(tg.AuthSentCodeClass,error){return nil,errors.New("unexpected resend")}
func(f *fakePhoneAuth)SignIn(_ context.Context,phone,code,hash string)(*tg.AuthAuthorization,error){
 if phone!="+12025550123"||code!="01234"||hash!="fixture"{return nil,errors.New("wrong test challenge")}
 f.signed=true;return nil,auth.ErrPasswordAuthNeeded
}
func TestPhoneLoginPassesCodeAndRequiresPassword(t *testing.T){
 ctx,cancel:=context.WithTimeout(context.Background(),time.Second);defer cancel()
 m:=NewLoginManager(ctx,nil)
 a:=&attempt{id:"test",method:"phone",deadline:time.Now().Add(time.Second),active:true,input:make(chan loginInput,1),cancel:cancel}
 m.attempts[1]=a
 f:=&fakePhoneAuth{}
 done:=make(chan error,1)
 go func(){done<-m.phoneLogin(ctx,a,"+12025550123",f)}()
 for m.Status(1).State!="code"{select{case <-ctx.Done():t.Fatal("missing code challenge");case <-time.After(time.Millisecond):}}
 if err:=m.Code(1,"test","01234");err!=nil{t.Fatal(err)}
 if err:=<-done;!errors.Is(err,auth.ErrPasswordAuthNeeded)||!f.signed{t.Fatal("2FA transition failed")}
}
