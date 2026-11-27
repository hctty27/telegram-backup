package security

import("strings";"testing")
func TestBox(t *testing.T){
 b,e:=NewBox(strings.Repeat("ab",32));if e!=nil{t.Fatal(e)}
 c,e:=b.Seal("account:1",[]byte("test secret"));if e!=nil{t.Fatal(e)}
 p,e:=b.Open("account:1",c);if e!=nil||string(p)!="test secret"{t.Fatal("roundtrip failed")}
 if _,e=b.Open("account:2",c);e==nil{t.Fatal("cross-account ciphertext accepted")}
 c[len(c)-1]^=1
 if _,e=b.Open("account:1",c);e==nil{t.Fatal("tampered ciphertext accepted")}
}
