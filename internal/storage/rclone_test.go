package storage
import "testing"
func TestDestination(t *testing.T){
 if v,e:=destination("drive","42/2026/123.mp4");e!=nil||v!="drive:42/2026/123.mp4"{t.Fatalf("%s %v",v,e)}
 for _,key:=range []string{"../outside","/absolute","a/../b","a:b","a\\b","."}{
  if _,e:=destination("drive",key);e==nil{t.Errorf("accepted %q",key)}
 }
 if _,e:=destination(":local","x");e==nil{t.Fatal("accepted invalid remote")}
}
