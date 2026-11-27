package api

import(
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "net/url"
 "strings"
 "testing"
 "time"
)
func TestDriveOAuthState(t *testing.T){
 o,err:=NewDriveOAuth(nil,"https://backup.example.com/api/oauth/google/callback");if err!=nil{t.Fatal(err)}
 req:=httptest.NewRequest("POST","/api/v2/storage/oauth/start",strings.NewReader(`{"name":"drive","client_id":"example.apps.googleusercontent.com","client_secret":"test-secret","token":"","root_folder_id":""}`))
 w:=httptest.NewRecorder();o.start(w,req)
 if w.Code!=200{t.Fatal(w.Code,w.Body.String())}
 var body map[string]string
 if err=json.Unmarshal(w.Body.Bytes(),&body);err!=nil{t.Fatal(err)}
 u,err:=url.Parse(body["url"]);if err!=nil{t.Fatal(err)}
 state:=u.Query().Get("state")
 if u.Host!="accounts.google.com"||u.Query().Get("code_challenge_method")!="S256"||u.Query().Get("access_type")!="offline"||strings.Contains(body["url"],"test-secret"){t.Fatal("invalid authorization request")}
 cookies:=w.Result().Cookies()
 if len(cookies)!=1||!cookies[0].HttpOnly||!cookies[0].Secure||cookies[0].SameSite!=http.SameSiteLaxMode||cookies[0].Value!=state{t.Fatal("invalid state cookie")}
 if _,ok:=o.take(state,"other");ok{t.Fatal("accepted wrong browser")}
 if _,ok:=o.take(state,state);!ok{t.Fatal("valid state rejected")}
 if _,ok:=o.take(state,state);ok{t.Fatal("state replay accepted")}
 expired:=strings.Repeat("a",43);o.pending[expired]=oauthAttempt{expires:time.Now().Add(-time.Second)}
 if _,ok:=o.take(expired,expired);ok{t.Fatal("expired state accepted")}
}
func TestDriveOAuthRedirect(t *testing.T){
 for _,v:=range []string{"http://example.com/api/oauth/google/callback","https://example.com/wrong","https://user@example.com/api/oauth/google/callback","https://example.com/api/oauth/google/callback?x=1"}{
  if _,err:=NewDriveOAuth(nil,v);err==nil{t.Errorf("accepted %s",v)}
 }
 if _,err:=NewDriveOAuth(nil,"http://127.0.0.1:8081/api/oauth/google/callback");err!=nil{t.Fatal(err)}
}
