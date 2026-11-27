package api

import (
 "context"
 "crypto/rand"
 "crypto/sha256"
 "crypto/subtle"
 "encoding/base64"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "net/url"
 "strings"
 "sync"
 "time"

 "github.com/hctty27/telegram-backup/internal/storage"
)

const driveCallback="/api/oauth/google/callback"
const driveScope="https://www.googleapis.com/auth/drive"
type oauthAttempt struct{
 config storage.DriveConfig
 verifier string
 expires time.Time
}
type DriveOAuth struct{
 managed *storage.Managed
 redirect string
 secure bool
 mu sync.Mutex
 pending map[string]oauthAttempt
}
func NewDriveOAuth(m *storage.Managed,redirect string)(*DriveOAuth,error){
 u,err:=url.Parse(redirect)
 if err!=nil||u.Host==""||u.User!=nil||u.Path!=driveCallback||u.RawQuery!=""||u.Fragment!=""||
 (u.Scheme!="https"&&!(u.Scheme=="http"&&(u.Hostname()=="localhost"||u.Hostname()=="127.0.0.1"||u.Hostname()=="::1"))){
  return nil,errors.New("invalid Google OAuth callback URL")
 }
 return &DriveOAuth{managed:m,redirect:redirect,secure:u.Scheme=="https",pending:make(map[string]oauthAttempt)},nil
}
func randomOAuth()(string,error){
 var b [32]byte
 if _,err:=rand.Read(b[:]);err!=nil{return "",err}
 return base64.RawURLEncoding.EncodeToString(b[:]),nil
}
func(o *DriveOAuth)cookie(w http.ResponseWriter,state string,maxAge int){
 http.SetCookie(w,&http.Cookie{Name:"tgb_google_state",Value:state,Path:driveCallback,MaxAge:maxAge,HttpOnly:true,Secure:o.secure,SameSite:http.SameSiteLaxMode})
}
func(o *DriveOAuth)start(w http.ResponseWriter,r *http.Request){
 var cfg storage.DriveConfig
 if decode(w,r,&cfg)!=nil||cfg.Token!=""||storage.ValidateDriveCredentials(cfg)!=nil{fail(w);return}
 state,err:=randomOAuth();if err!=nil{fail(w);return}
 verifier,err:=randomOAuth();if err!=nil{fail(w);return}
 o.mu.Lock()
 for k,v:=range o.pending{if time.Now().After(v.expires){delete(o.pending,k)}}
 if len(o.pending)>=16{o.mu.Unlock();http.Error(w,"too many pending authorizations",429);return}
 o.pending[state]=oauthAttempt{config:cfg,verifier:verifier,expires:time.Now().Add(10*time.Minute)}
 o.mu.Unlock()
 o.cookie(w,state,600)
 sum:=sha256.Sum256([]byte(verifier))
 q:=url.Values{"client_id":{cfg.ClientID},"redirect_uri":{o.redirect},"response_type":{"code"},"scope":{driveScope},"access_type":{"offline"},"prompt":{"consent"},"state":{state},"code_challenge":{base64.RawURLEncoding.EncodeToString(sum[:])},"code_challenge_method":{"S256"}}
 reply(w,map[string]string{"url":"https://accounts.google.com/o/oauth2/v2/auth?"+q.Encode()})
}
func(o *DriveOAuth)take(state,cookie string)(oauthAttempt,bool){
 if len(state)!=43||subtle.ConstantTimeCompare([]byte(state),[]byte(cookie))!=1{return oauthAttempt{},false}
 o.mu.Lock();defer o.mu.Unlock()
 a,ok:=o.pending[state]
 delete(o.pending,state)
 return a,ok&&time.Now().Before(a.expires)
}
func(o *DriveOAuth)callback(w http.ResponseWriter,r *http.Request){
 w.Header().Set("Cache-Control","no-store")
 w.Header().Set("Referrer-Policy","no-referrer")
 w.Header().Set("Content-Type","text/plain; charset=utf-8")
 w.Header().Set("X-Content-Type-Options","nosniff")
 w.Header().Set("Content-Security-Policy","default-src 'none'; frame-ancestors 'none'")
 c,err:=r.Cookie("tgb_google_state")
 if err!=nil{http.Error(w,"Authorization expired or invalid. Return to TelegramBackup and try again.",400);return}
 a,ok:=o.take(r.URL.Query().Get("state"),c.Value)
 if !ok{http.Error(w,"Authorization expired or invalid. Return to TelegramBackup and try again.",400);return}
 o.cookie(w,"",-1)
 code:=r.URL.Query().Get("code")
 if code==""||len(code)>4096||r.URL.Query().Get("error")!=""{http.Error(w,"Authorization declined. Return to TelegramBackup.",400);return}
 ctx,cancel:=context.WithTimeout(r.Context(),50*time.Second);defer cancel()
 token,err:=o.exchange(ctx,a,code)
 if err!=nil{http.Error(w,"Google token exchange failed. Check the OAuth client and callback settings.",400);return}
 a.config.Token=token
 if o.managed.CreateDrive(ctx,a.config)!=nil{http.Error(w,"Drive connection or storage registration failed. Check the remote name and permissions.",400);return}
 _,_=io.WriteString(w,"Google Drive connected successfully. Return to TelegramBackup and refresh the cloud storage list.")
}
func(o *DriveOAuth)exchange(ctx context.Context,a oauthAttempt,code string)(string,error){
 data:=url.Values{"client_id":{a.config.ClientID},"client_secret":{a.config.ClientSecret},"code":{code},"code_verifier":{a.verifier},"grant_type":{"authorization_code"},"redirect_uri":{o.redirect}}
 req,err:=http.NewRequestWithContext(ctx,"POST","https://oauth2.googleapis.com/token",strings.NewReader(data.Encode()))
 if err!=nil{return "",err}
 req.Header.Set("Content-Type","application/x-www-form-urlencoded")
 client:=&http.Client{Timeout:15*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}
 res,err:=client.Do(req);if err!=nil{return "",err};defer res.Body.Close()
 if res.StatusCode!=200{return "",errors.New("token exchange rejected")}
 b,err:=io.ReadAll(io.LimitReader(res.Body,65537));if err!=nil||len(b)>65536{return "",errors.New("invalid token response")}
 var v struct{
  Access string `json:"access_token"`
  Refresh string `json:"refresh_token"`
  Type string `json:"token_type"`
  Scope string `json:"scope"`
  Expires int64 `json:"expires_in"`
 }
 if json.Unmarshal(b,&v)!=nil||v.Access==""||v.Refresh==""||v.Expires<=0||v.Expires>86400||!strings.EqualFold(v.Type,"Bearer"){return "",errors.New("incomplete offline token")}
 allowed:=false;for _,s:=range strings.Fields(v.Scope){if s==driveScope{allowed=true}}
 if !allowed{return "",errors.New("Drive scope not granted")}
 raw,err:=json.Marshal(map[string]any{"access_token":v.Access,"refresh_token":v.Refresh,"token_type":v.Type,"expiry":time.Now().Add(time.Duration(v.Expires)*time.Second).UTC()})
 return string(raw),err
}
// Only the state/cookie-bound callback is public; start is mounted behind the
// existing admin bearer-token middleware.
func(o *DriveOAuth)Wrap(next http.Handler)http.Handler{
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path==driveCallback&&r.Method=="GET"{o.callback(w,r);return}
  next.ServeHTTP(w,r)
 })
}
