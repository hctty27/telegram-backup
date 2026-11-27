package storage
import ("testing";"strings";"context";"io")
func TestDestination(t *testing.T){
 if v,e:=destination("drive","42/2026/123.mp4");e!=nil||v!="drive:42/2026/123.mp4"{t.Fatalf("%s %v",v,e)}
 for _,key:=range []string{"../outside","/absolute","a/../b","a:b","a\\b","."}{
  if _,e:=destination("drive",key);e==nil{t.Errorf("accepted %q",key)}
 }
 if _,e:=destination(":local","x");e==nil{t.Fatal("accepted invalid remote")}
}

func TestStatsWriter(t *testing.T){
 var counts []int64
 w:=&statsWriter{report:func(n int64){counts=append(counts,n)}}
 _,_=w.Write([]byte("{\"stats\":{\"by"))
 _,_=w.Write([]byte("tes\":42}}\nnot json\n"))
 _,_=w.Write([]byte(strings.Repeat("x",70<<10)))
 _,_=w.Write([]byte("\n{\"stats\":{\"bytes\":84}}\n"))
 if len(counts)!=2||counts[0]!=42||counts[1]!=84{t.Fatalf("unexpected stats: %v",counts)}
}

func TestExistingObjectConflict(t *testing.T){
 r:=&Rclone{}
 for _,items:=range [][]object{
  nil,{{Name:"x",Size:3},{Name:"x",Size:3}},{{Name:"x",Size:3,IsDir:true}},{{Name:"x",Size:4}},{{Name:"x",Size:3,Hashes:map[string]string{"SHA-256":"bad"}}},{{Name:"x",Size:3,Hashes:map[string]string{"MD5":"bad"}}},
 }{
  if _,err:=r.verifyObject(context.Background(),"test:x",items,3,"good","good");err==nil{t.Fatalf("conflict accepted: %+v",items)}
 }
 if v,err:=r.verifyObject(context.Background(),"test:x",[]object{{ID:"ok",Size:3,Hashes:map[string]string{"SHA-256":"GOOD"}}},3,"good","md");err!=nil||v.ID!="ok"{t.Fatalf("identical object rejected: %+v %v",v,err)}
}
func TestReadbackBound(t *testing.T){
 w:=&exactHashWriter{w:io.Discard,left:3}
 if n,e:=w.Write([]byte("abc"));e!=nil||n!=3||w.left!=0{t.Fatal(n,e,w.left)}
 if n,e:=w.Write([]byte("x"));e==nil||n!=0{t.Fatal("oversized readback accepted")}
}
