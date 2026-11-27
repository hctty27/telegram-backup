package storage

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "os"
 "os/exec"
 "path/filepath"
 "testing"
 "time"
)

// An actual local rclone backend exercises the same CLI used for Drive.
// It validates the transfer adapter, not Google's authorization or API behavior.
func TestRcloneLocalRoundTrip(t *testing.T){
 if _,err:=exec.LookPath("rclone");err!=nil{
  if os.Getenv("TGB_REQUIRE_RCLONE_TEST")=="true"{t.Fatal(err)}
  t.Skip("rclone binary unavailable")
 }
 root:=t.TempDir()
 remote:=filepath.Join(root,"remote")
 if err:=os.Mkdir(remote,0700);err!=nil{t.Fatal(err)}
 cfg:=filepath.Join(root,"rclone.conf")
 if err:=os.WriteFile(cfg,[]byte("[testremote]\ntype = alias\nremote = "+remote+"\n"),0600);err!=nil{t.Fatal(err)}
 file:=filepath.Join(root,"source.bin")
 data:=[]byte("telegram-backup verification test")
 if err:=os.WriteFile(file,data,0600);err!=nil{t.Fatal(err)}
 r:=&Rclone{Config:cfg}
 ctx,cancel:=context.WithTimeout(context.Background(),45*time.Second);defer cancel()
 if err:=r.TestRemote(ctx,"testremote");err!=nil{t.Fatal(err)}
 key:="archive/test.bin"
 want:=sha256.Sum256(data)
 for i:=0;i<2;i++{
  v,err:=r.UploadVerified(ctx,file,"testremote",key)
  if err!=nil||v.Bytes!=int64(len(data))||v.SHA256!=hex.EncodeToString(want[:]){t.Fatalf("verified upload %d: %+v %v",i,v,err)}
 }
 changed:=append([]byte(nil),data...);changed[0]^=0xff
 for _,other:=range [][]byte{[]byte("different content"),changed}{
  if err:=os.WriteFile(file,other,0600);err!=nil{t.Fatal(err)}
  if _,err:=r.UploadVerified(ctx,file,"testremote",key);err==nil{t.Fatal("overwriting archived content was allowed")}
  got,err:=os.ReadFile(filepath.Join(remote,"archive","test.bin"))
  if err!=nil||string(got)!=string(data){t.Fatal("remote archive changed")}
 }
 if err:=os.Mkdir(filepath.Join(remote,"archive","directory"),0700);err!=nil{t.Fatal(err)}
 if _,err:=r.UploadVerified(ctx,file,"testremote","archive/directory");err==nil{t.Fatal("directory conflict allowed")}
 got,err:=os.ReadFile(filepath.Join(remote,"archive","test.bin"))
 if err!=nil||string(got)!=string(data){t.Fatal("remote archive changed")}
 if _,err=os.Stat(file);err!=nil{t.Fatal("storage adapter deleted local source")}
}
