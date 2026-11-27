package transfer
import(
 "context"
 "io"
 "os"
 "path/filepath"
 "testing"
 "github.com/hctty27/telegram-backup/internal/disk"
)
func TestStageAndCleanup(t *testing.T){
 root:=t.TempDir()
 if e:=os.Chmod(root,0700);e!=nil{t.Fatal(e)}
 q,_:=disk.New(100,10,func()(disk.Usage,error){return disk.Usage{Free:100},nil})
 fetch:=func(_ context.Context,w io.Writer)error{_,e:=io.WriteString(w,"abc");return e}
 if _,e:=Stage(context.Background(),root,1,2,q,fetch);e==nil{t.Fatal("oversized fetch accepted")}
 if _,e:=os.Stat(filepath.Join(root,"1.media"));!os.IsNotExist(e){t.Fatal("partial published")}
 p,e:=Stage(context.Background(),root,1,3,q,fetch);if e!=nil{t.Fatal(e)}
 if _,e:=Stage(context.Background(),root,1,3,q,fetch);e==nil{t.Fatal("overwrote staged file")}
 if e:=CleanupVerified(root,2,p);e==nil{t.Fatal("deleted wrong file")}
 if e:=CleanupVerified(root,1,p);e!=nil{t.Fatal(e)}
 if e:=CleanupVerified(root,1,p);e!=nil{t.Fatal(e)}
}
