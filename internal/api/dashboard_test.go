package api
import (
 "context"
 "encoding/json"
 "github.com/hctty27/telegram-backup/internal/buildinfo"
 "net/http"
 "net/http/httptest"
 "testing"
 "testing/fstest"
 "github.com/jackc/pgx/v5/pgxpool"
)
func TestDashboardAuthentication(t *testing.T){
 // A pool is lazy; rejected requests must not attempt network I/O.
 pool,err:=pgxpool.New(context.Background(),"postgres://unused:unused@127.0.0.1:1/unused")
 if err!=nil{t.Fatal(err)};defer pool.Close()
 assets:=fstest.MapFS{"index.html":&fstest.MapFile{Data:[]byte("test dashboard")}}
 if _,err:=NewDashboardRouter(pool,"short",assets);err==nil{t.Fatal("accepted short token")}
 r,err:=NewDashboardRouter(pool,"test-token-at-least-thirty-two-characters",assets)
 if err!=nil{t.Fatal(err)}
 for _,url:=range []string{"/api/version","/api/overview","/api/accounts","/api/chats","/api/tasks","/api/archive","/api/storage"}{
  req:=httptest.NewRequest(http.MethodGet,url,nil)
  w:=httptest.NewRecorder();r.ServeHTTP(w,req)
  if w.Code!=http.StatusUnauthorized{t.Fatalf("%s: %d",url,w.Code)}
 }
 w:=httptest.NewRecorder();r.ServeHTTP(w,httptest.NewRequest(http.MethodGet,"/api/health",nil))
 if w.Code!=http.StatusOK{t.Fatal(w.Code)}
}

func TestVersionAuthentication(t *testing.T){
 pool,err:=pgxpool.New(context.Background(),"postgres://unused:unused@127.0.0.1:1/unused")
 if err!=nil{t.Fatal(err)};defer pool.Close()
 const token="test-token-at-least-thirty-two-characters"
 r,err:=NewDashboardRouter(pool,token,fstest.MapFS{"index.html":&fstest.MapFile{Data:[]byte("dashboard")}})
 if err!=nil{t.Fatal(err)}
 req:=httptest.NewRequest(http.MethodGet,"/api/version",nil);req.Header.Set("Authorization","Bearer "+token)
 w:=httptest.NewRecorder();r.ServeHTTP(w,req)
 var info buildinfo.Info
 if w.Code!=http.StatusOK||json.Unmarshal(w.Body.Bytes(),&info)!=nil||info!=buildinfo.Current(){t.Fatalf("unexpected version: %d %s",w.Code,w.Body.String())}
 if w.Header().Get("Cache-Control")!="no-store"{t.Fatal("version must not be cached")}
}
