// Package monitor tracks ephemeral progress. PostgreSQL/River remain the
// authoritative source of durable task status.
package monitor

import (
 "context"
 "io"
 "sort"
 "sync"
 "time"
)

type Progress struct {
 ID string `json:"id"`
 MediaID int64 `json:"media_id,string"`
 Stage string `json:"stage"`
 Bytes int64 `json:"bytes"`
 Total int64 `json:"total"`
 Speed float64 `json:"speed"`
 Updated time.Time `json:"updated_at"`
}
type Tracker struct{
 mu sync.Mutex
 active map[string]*Progress
}
func New()*Tracker{return &Tracker{active:make(map[string]*Progress)}}
type key struct{}

// Begin associates a report callback with this operation only. End removes
// only its own record, even if the same job is restarted.
func(t *Tracker)Begin(ctx context.Context,id string,mediaID int64,stage string,total int64)(context.Context,func()){
 if t==nil{return ctx,func(){}}
 p:=&Progress{ID:id,MediaID:mediaID,Stage:stage,Total:total,Updated:time.Now()}
 t.mu.Lock();t.active[id]=p;t.mu.Unlock()
 report:=func(n int64){
  t.mu.Lock();defer t.mu.Unlock()
  if t.active[id]!=p||n<0||n>p.Total{return}
  now:=time.Now()
  elapsed:=now.Sub(p.Updated).Seconds()
  if n==p.Bytes&&elapsed<1{return}
  if elapsed<0.5&&n!=p.Total&&n>=p.Bytes&&p.Bytes!=0{return}
  p.Speed=0
  if elapsed>0&&n>=p.Bytes{p.Speed=float64(n-p.Bytes)/elapsed}
  p.Bytes=n;p.Updated=now
 }
 return context.WithValue(ctx,key{},report),func(){
  t.mu.Lock();defer t.mu.Unlock()
  if t.active[id]==p{delete(t.active,id)}
 }
}
func Report(ctx context.Context,n int64){
 if f,ok:=ctx.Value(key{}).(func(int64));ok{f(n)}
}
func(t *Tracker)Snapshot()[]Progress{
 out:=[]Progress{}
 if t==nil{return out}
 t.mu.Lock();defer t.mu.Unlock()
 for _,p:=range t.active{
  copy:=*p
  if time.Since(copy.Updated)>3*time.Second{copy.Speed=0}
  out=append(out,copy)
 }
 sort.Slice(out,func(i,j int)bool{return out[i].ID<out[j].ID})
 return out
}
type countWriter struct{ctx context.Context;w io.Writer;total int64}
func(w *countWriter)Write(p []byte)(int,error){
 n,err:=w.w.Write(p);w.total+=int64(n);Report(w.ctx,w.total);return n,err
}
// The Telegram Stream method writes sequentially; this wrapper does not
// support concurrent writes or random access.
func Writer(ctx context.Context,w io.Writer)io.Writer{return WriterFrom(ctx,w,0)}
func WriterFrom(ctx context.Context,w io.Writer,offset int64)io.Writer{return &countWriter{ctx:ctx,w:w,total:offset}}
