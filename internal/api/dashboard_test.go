package api
import (
 "context"
 "net/http"
 "net/http/httptest"
 "testing"
 "testing/fstest"
 "github.com/jackc/pgx/v5/pgxpool"
)
func TestPreviewAuthentication(t *testing.T){
 // A pool is lazy; rejected requests must not attempt network I/O.
 pool,err:=pgxpool.New(context.Background(),"postgres://unused:unused@127.0.0.1:1/unused")
 if err!=nil{t.Fatal(err)};defer pool.Close()
 assets:=fstest.MapFS{"index.html":&fstest.MapFile{Data:[]byte("test preview")}}
 if _,err:=NewDashboardRouter(pool,"short",assets);err==nil{t.Fatal("accepted short token")}
 r,err:=NewDashboardRouter(pool,"test-token-at-least-thirty-two-characters",assets)
 if err!=nil{t.Fatal(err)}
 for _,url:=range []string{"/api/overview","/api/accounts","/api/chats","/api/tasks","/api/archive","/api/storage"}{
  req:=httptest.NewRequest(http.MethodGet,url,nil)
  w:=httptest.NewRecorder();r.ServeHTTP(w,req)
  if w.Code!=http.StatusUnauthorized{t.Fatalf("%s: %d",url,w.Code)}
 }
 w:=httptest.NewRecorder();r.ServeHTTP(w,httptest.NewRequest(http.MethodGet,"/api/health",nil))
 if w.Code!=http.StatusOK{t.Fatal(w.Code)}
}
