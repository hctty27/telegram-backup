package api

import(
 "bufio"
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "testing/fstest"
 "time"

 "github.com/hctty27/telegram-backup/internal/backup"
 "github.com/hctty27/telegram-backup/internal/disk"
 "github.com/hctty27/telegram-backup/internal/monitor"
 "github.com/jackc/pgx/v5/pgxpool"
)
func TestMonitorAuthAndStream(t *testing.T){
 const token="test-token-at-least-thirty-two-characters"
 ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second);defer cancel()
 pool,err:=pgxpool.New(ctx,"postgres://unused:unused@127.0.0.1:1/unused")
 if err!=nil{t.Fatal(err)};defer pool.Close()
 quota,err:=disk.New(100,20,func()(disk.Usage,error){return disk.Usage{Staged:30,Free:70},nil})
 if err!=nil{t.Fatal(err)}
 tracker:=monitor.New()
 transfer,end:=tracker.Begin(ctx,"download:1",1,"download",3);defer end()
 monitor.Report(transfer,3)
 actions:=&Actions{Engine:&backup.Engine{Quota:quota,Progress:tracker}}
 router,err:=NewDashboardRouter(pool,token,fstest.MapFS{"index.html":&fstest.MapFile{Data:[]byte("test")}},actions.Routes)
 if err!=nil{t.Fatal(err)}
 for _,path:=range []string{"/api/monitor","/api/monitor/events"}{
  w:=httptest.NewRecorder();router.ServeHTTP(w,httptest.NewRequest("GET",path,nil))
  if w.Code!=http.StatusUnauthorized{t.Fatalf("%s: unauthorized status %d",path,w.Code)}
 }
 srv:=httptest.NewServer(router);defer srv.Close()
 req,err:=http.NewRequestWithContext(ctx,"GET",srv.URL+"/api/monitor/events",nil)
 if err!=nil{t.Fatal(err)}
 req.Header.Set("Authorization","Bearer "+token)
 res,err:=srv.Client().Do(req);if err!=nil{t.Fatal(err)};defer res.Body.Close()
 if res.StatusCode!=200||res.Header.Get("Content-Type")!="text/event-stream"{t.Fatal("invalid SSE response")}
 reader:=bufio.NewReader(res.Body)
 event,err:=reader.ReadString('\n');if err!=nil||event!="event: snapshot\n"{t.Fatalf("event=%q err=%v",event,err)}
 line,err:=reader.ReadString('\n');if err!=nil{t.Fatal(err)}
 var s monitorSnapshot
 if err=json.Unmarshal([]byte(strings.TrimPrefix(line,"data: ")),&s);err!=nil{t.Fatal(err)}
 if s.Disk==nil||s.Disk.Available!=50||len(s.Transfers)!=1||s.Transfers[0].Bytes!=3{t.Fatalf("unexpected snapshot %+v",s)}
}
