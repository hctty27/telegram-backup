// Package transfer implements bounded staging; callers persist state before cleanup.
package transfer

import (
 "context"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"

 "github.com/hctty27/telegram-backup/internal/disk"
)

type Fetch func(context.Context,io.Writer)error

type bounded struct{w io.Writer; left int64}
func(b *bounded)Write(p []byte)(int,error){
 if int64(len(p))>b.left{return 0,errors.New("download exceeds reservation")}
 n,e:=b.w.Write(p);b.left-=int64(n);return n,e
}

// Stage never publishes partial content and never overwrites a completed stage.
// expected is trusted Telegram file metadata and is required to enforce the quota.
func Stage(ctx context.Context,root string,id,expected int64,quota *disk.Manager,fetch Fetch)(string,error){
 if id<=0||expected<=0||quota==nil||fetch==nil{return "",errors.New("invalid staging arguments")}
 root,err:=filepath.Abs(root);if err!=nil{return "",err}
 info,err:=os.Lstat(root);if err!=nil||!info.IsDir()||info.Mode().Perm()&0077!=0{return "",errors.New("private staging directory required")}
 release,err:=quota.Reserve(expected);if err!=nil{return "",err};defer release()
 f,err:=os.CreateTemp(root,fmt.Sprintf("%d-*.part",id));if err!=nil{return "",err}
 defer os.Remove(f.Name());defer f.Close()
 w:=&bounded{w:f,left:expected}
 if err:=fetch(ctx,w);err!=nil{return "",err}
 if err:=ctx.Err();err!=nil{return "",err}
 if w.left!=0{return "",errors.New("download size mismatch")}
 if err:=f.Sync();err!=nil{return "",err}
 if err:=f.Close();err!=nil{return "",err}
 dest:=filepath.Join(root,fmt.Sprintf("%d.media",id))
 if err:=os.Link(f.Name(),dest);err!=nil{return "",err}
 return dest,nil
}

// CleanupVerified requires the caller to have committed cleanup_pending and all
// replica verification records. Only the exact generated staging path is removed.
func CleanupVerified(root string,id int64,storedPath string)error{
 if id<=0{return errors.New("invalid media id")}
 root,err:=filepath.Abs(root);if err!=nil{return err}
 actual,err:=filepath.Abs(storedPath);if err!=nil{return err}
 if actual!=filepath.Join(root,fmt.Sprintf("%d.media",id)){return errors.New("staging path mismatch")}
 info,err:=os.Lstat(actual)
 if errors.Is(err,os.ErrNotExist){return syncDirectory(root)}
 if err!=nil{return err}
 if !info.Mode().IsRegular(){return errors.New("refusing nonregular staging path")}
 err=os.Remove(actual);if err!=nil&&!errors.Is(err,os.ErrNotExist){return err};return syncDirectory(root)
}
