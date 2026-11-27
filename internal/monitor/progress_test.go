package monitor
import("context";"io";"strings";"testing")
func TestProgressLifecycle(t *testing.T){
 m:=New()
 ctx,end:=m.Begin(context.Background(),"download:1",1,"download",3)
 if _,err:=io.Copy(Writer(ctx,io.Discard),strings.NewReader("abc"));err!=nil{t.Fatal(err)}
 s:=m.Snapshot();if len(s)!=1||s[0].Bytes!=3||s[0].Total!=3{t.Fatalf("unexpected snapshot: %+v",s)}
 _,next:=m.Begin(context.Background(),"download:1",1,"download",3)
 end()
 if len(m.Snapshot())!=1{t.Fatal("old completion removed new transfer")}
 next()
 if len(m.Snapshot())!=0{t.Fatal("transfer leaked")}
}
