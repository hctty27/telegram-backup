package transfer

import(
 "bytes"
 "context"
 "errors"
 "io"
 "os"
 "path/filepath"
 "testing"

 "github.com/hctty27/telegram-backup/internal/disk"
)

func testQuota(t *testing.T,root string,limit int64)*disk.Manager{
 t.Helper()
 q,err:=disk.New(limit,0,func()(disk.Usage,error){
  entries,err:=os.ReadDir(root);if err!=nil{return disk.Usage{},err}
  var staged int64
  for _,e:=range entries{st,err:=e.Info();if err!=nil{return disk.Usage{},err};if st.Mode().IsRegular(){staged+=st.Size()}}
  return disk.Usage{Staged:staged,Free:1<<30},nil
 })
 if err!=nil{t.Fatal(err)}
 return q
}
func TestResumeAfterInterruptedDownload(t *testing.T){
 root:=t.TempDir();q:=testQuota(t,root,1<<20)
 original:=bytes.Repeat([]byte("abcd"),4096)
 interrupted:=errors.New("network interrupted")
 ctx:=context.Background()
 _,err:=StageResume(ctx,root,1,int64(len(original)),"source-a",q,func(_ context.Context,offset int64,w io.Writer)error{
  if offset!=0{t.Fatal(offset)}
  if _,err:=w.Write(original[:8192]);err!=nil{return err}
  return interrupted
 })
 if !errors.Is(err,interrupted){t.Fatal(err)}
 if _,err=os.Stat(filepath.Join(root,"1.media"));!os.IsNotExist(err){t.Fatal("partial was published")}
 // A new process would create a new quota manager; the checkpoint is on disk.
 q=testQuota(t,root,1<<20)
 p,err:=StageResume(ctx,root,1,int64(len(original)),"source-a",q,func(_ context.Context,offset int64,w io.Writer)error{
  if offset!=8192{t.Fatalf("resume offset %d",offset)}
  _,err:=w.Write(original[offset:]);return err
 })
 if err!=nil{t.Fatal(err)}
 data,err:=os.ReadFile(p);if err!=nil||!bytes.Equal(data,original){t.Fatal("resumed content mismatch")}
 for _,name:=range []string{"1.resume.part","1.resume.json"}{if _,err=os.Stat(filepath.Join(root,name));!os.IsNotExist(err){t.Fatal("resume artifacts remain")}}
}
func TestResumeRejectsChangedPrefixAndIdentity(t *testing.T){
 for _,change:=range []string{"prefix","identity"}{t.Run(change,func(t *testing.T){
  root:=t.TempDir();q:=testQuota(t,root,1<<20)
  ctx:=context.Background();data:=bytes.Repeat([]byte{7},8192)
  _,err:=StageResume(ctx,root,2,8192,"source-a",q,func(_ context.Context,_ int64,w io.Writer)error{_,err:=w.Write(data[:4096]);if err!=nil{return err};return io.ErrUnexpectedEOF})
  if !errors.Is(err,io.ErrUnexpectedEOF){t.Fatal(err)}
  identity:="source-a"
  if change=="prefix"{
   f,err:=os.OpenFile(filepath.Join(root,"2.resume.part"),os.O_WRONLY,0600);if err!=nil{t.Fatal(err)}
   if _,err=f.WriteAt([]byte{8},0);err!=nil{t.Fatal(err)};if err=f.Close();err!=nil{t.Fatal(err)}
  }else{identity="source-b"}
  p,err:=StageResume(ctx,root,2,8192,identity,q,func(_ context.Context,offset int64,w io.Writer)error{if offset!=0{t.Fatal("invalid prefix reused")};_,err:=w.Write(data);return err})
  if err!=nil{t.Fatal(err)}
  got,err:=os.ReadFile(p);if err!=nil||!bytes.Equal(got,data){t.Fatal("data mismatch")}
 })}
}
func TestResumePressureDoesNotRemovePublishedFile(t *testing.T){
 root:=t.TempDir();ctx:=context.Background();q:=testQuota(t,root,100000)
 data:=bytes.Repeat([]byte{3},32768)
 _,err:=StageResume(ctx,root,1,65536,"source-a",q,func(_ context.Context,_ int64,w io.Writer)error{_,err:=w.Write(data);if err!=nil{return err};return io.ErrUnexpectedEOF})
 if !errors.Is(err,io.ErrUnexpectedEOF){t.Fatal(err)}
 if err=os.WriteFile(filepath.Join(root,"9.media"),bytes.Repeat([]byte{9},4096),0600);err!=nil{t.Fatal(err)}
 q=testQuota(t,root,80000)
 if _,err=StageResume(ctx,root,2,65536,"source-b",q,func(_ context.Context,_ int64,w io.Writer)error{_,err:=w.Write(bytes.Repeat([]byte{4},65536));return err});err!=nil{t.Fatal(err)}
 if _,err=os.Stat(filepath.Join(root,"1.resume.part"));!os.IsNotExist(err){t.Fatal("inactive partial not reclaimed")}
 if _,err=os.Stat(filepath.Join(root,"9.media"));err!=nil{t.Fatal("published file removed")}
}
func TestRecoverPublishedHardlink(t *testing.T){
 root:=t.TempDir();part:=filepath.Join(root,"1.resume.part");dest:=filepath.Join(root,"1.media")
 if err:=os.WriteFile(part,[]byte("data"),0600);err!=nil{t.Fatal(err)}
 if err:=os.Link(part,dest);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(root,"1.resume.json"),[]byte("{}"),0600);err!=nil{t.Fatal(err)}
 if err:=RecoverPublished(root,1);err!=nil{t.Fatal(err)}
 if _,err:=os.Stat(part);!os.IsNotExist(err){t.Fatal("stale link still exists")}
 if err:=RecoverPublished(root,1);err!=nil{t.Fatal(err)}
 if got,err:=os.ReadFile(dest);err!=nil||string(got)!="data"{t.Fatal("published data lost")}
}
