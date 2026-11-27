package storage
import ("testing";"strings")
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
