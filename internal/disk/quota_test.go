package disk

import (
 "errors"
 "sync"
 "sync/atomic"
 "testing"
)

func TestConcurrentReservations(t *testing.T){
 m,err:=New(100,20,func()(Usage,error){return Usage{Staged:0,Free:100},nil})
 if err!=nil {t.Fatal(err)}
 var wg sync.WaitGroup
 var accepted atomic.Int64
 releases:=make(chan func(),20)
 for i:=0;i<20;i++{
  wg.Add(1)
  go func(){defer wg.Done();r,e:=m.Reserve(10);if e==nil{accepted.Add(1);releases<-r}else if !errors.Is(e,ErrCapacity){t.Error(e)}}()
 }
 wg.Wait();close(releases)
 if accepted.Load()!=8{t.Fatalf("accepted %d, want 8",accepted.Load())}
 for r:=range releases{r();r()}
 n,e:=m.Available();if e!=nil||n!=80{t.Fatalf("available=%d err=%v",n,e)}
}
func TestBacklogAndUnknownSize(t *testing.T){
 m,_:=New(100,20,func()(Usage,error){return Usage{Staged:90,Free:1000},nil})
 if _,err:=m.Reserve(11);!errors.Is(err,ErrCapacity){t.Fatal(err)}
 if _,err:=m.Reserve(0);!errors.Is(err,ErrSize){t.Fatal(err)}
}
