package storage

import(
 "bytes"
 "context"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func testDrive()DriveConfig{return DriveConfig{
 Name:"drive-test",ClientID:"example.apps.googleusercontent.com",ClientSecret:"example-secret",
 Token:`{"access_token":"test-access","refresh_token":"test-refresh","token_type":"Bearer","expiry":"2026-12-01T00:00:00Z"}`,
 RootFolderID:"folder-id_1",
}}
func TestDriveINI(t *testing.T){
 cfg:=testDrive()
 raw,err:=driveINI(cfg);if err!=nil{t.Fatal(err)}
 for _,want:=range []string{"[drive-test]\n","type = drive\n","scope = drive\n","root_folder_id = folder-id_1\n"}{
  if !bytes.Contains(raw,[]byte(want)){t.Errorf("missing %q",want)}
 }
 for _,change:=range []func(*DriveConfig){
  func(c *DriveConfig){c.Name="evil]\n[local"},
  func(c *DriveConfig){c.ClientID="invalid"},
  func(c *DriveConfig){c.ClientSecret="a\ntype = local"},
  func(c *DriveConfig){c.RootFolderID="a\ntype = local"},
  func(c *DriveConfig){c.Token=`{"access_token":"only"}`},
  func(c *DriveConfig){c.Token="not json"},
  func(c *DriveConfig){c.Token=strings.Repeat("x",32769)},
 }{
  c:=testDrive();change(&c)
  if _,e:=driveINI(c);e==nil{t.Error("invalid credential accepted")}
 }
}
func TestTemporaryConfig(t *testing.T){
 m:=&Managed{TempRoot:"/dev/shm"}
 if _,e:=os.Stat(m.TempRoot);e!=nil{t.Skip("tmpfs not available")}
 p,cleanup,e:=m.tempConfig([]byte("[test]\ntype = drive\n"))
 if e!=nil{t.Fatal(e)}
 defer cleanup()
 st,e:=os.Stat(p)
 if e!=nil||st.Mode().Perm()!=0600{t.Fatalf("invalid file mode: %v",e)}
 dir,e:=os.Stat(filepath.Dir(p));if e!=nil||dir.Mode().Perm()!=0700{t.Fatal("private directory required")}
 cleanup()
 if _,e=os.Stat(p);!os.IsNotExist(e){t.Fatal("temporary config not removed")}
}
func TestManagedGateCancellation(t *testing.T){
 m:=&Managed{}
 release,e:=m.lock(context.Background(),"drive");if e!=nil{t.Fatal(e)};defer release()
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if _,e=m.lock(ctx,"drive");e==nil{t.Fatal("second remote operation acquired busy gate")}
 other,e:=m.lock(context.Background(),"other");if e!=nil{t.Fatal(e)};other()
}
