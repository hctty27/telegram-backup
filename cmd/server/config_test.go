package main

import "testing"
func TestGiBSetting(t *testing.T){
 const name="TEST_V2_GIB"
 t.Setenv(name,"")
 if n,e:=gibSetting(name,10);e!=nil||n!=10<<30{t.Fatalf("default: %d %v",n,e)}
 t.Setenv(name,"3")
 if n,e:=gibSetting(name,10);e!=nil||n!=3<<30{t.Fatalf("configured: %d %v",n,e)}
 for _,v:=range []string{"0","-1","x","999999999999999999"}{
  t.Setenv(name,v)
  if _,e:=gibSetting(name,10);e==nil{t.Errorf("accepted %q",v)}
 }
}
