package api

import(
 "encoding/json"
 "fmt"
 "net/http"
 "time"

 "github.com/hctty27/telegram-backup/internal/disk"
 "github.com/hctty27/telegram-backup/internal/monitor"
)

type monitorSnapshot struct {
 Disk *disk.Snapshot `json:"disk"`
 Transfers []monitor.Progress `json:"transfers"`
}
func(a *Actions)snapshot()monitorSnapshot{
 result:=monitorSnapshot{Transfers:a.Engine.Progress.Snapshot()}
 if d,err:=a.Engine.Quota.Snapshot();err==nil{result.Disk=&d}
 return result
}
// Each event is a full snapshot, so reconnects need no replay or persisted
// event ID. Bearer authentication is supplied by the parent router.
func(a *Actions)stream(slots chan struct{})http.HandlerFunc{
 return func(w http.ResponseWriter,r *http.Request){
  select{case slots<-struct{}{}:defer func(){<-slots}();default:http.Error(w,"too many monitor streams",429);return}
  flusher,ok:=w.(http.Flusher);if !ok{http.Error(w,"stream unavailable",500);return}
  w.Header().Set("Content-Type","text/event-stream")
  w.Header().Set("Cache-Control","no-store")
  w.Header().Set("X-Accel-Buffering","no")
  tick:=time.NewTicker(2*time.Second);defer tick.Stop()
  expiry:=time.NewTimer(2*time.Minute);defer expiry.Stop()
  controller:=http.NewResponseController(w)
  for{
   data,err:=json.Marshal(a.snapshot());if err!=nil{return}
   if err=controller.SetWriteDeadline(time.Now().Add(5*time.Second));err!=nil{return}
   if _,err=fmt.Fprintf(w,"event: snapshot\ndata: %s\n\n",data);err!=nil{return}
   flusher.Flush()
   select{case <-r.Context().Done():return;case <-expiry.C:return;case <-tick.C:}
  }
 }
}
