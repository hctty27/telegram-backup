package transfer

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "hash"
 "io"
 "os"
 "path/filepath"

 "github.com/hctty27/telegram-backup/internal/disk"
)

const ResumeAlignment int64=4096
const checkpointInterval int64=4<<20

type ResumeFetch func(context.Context,int64,io.Writer)error
type checkpoint struct{
 Identity string `json:"identity"`
 Expected int64 `json:"expected"`
 Offset int64 `json:"offset"`
 SHA256 string `json:"sha256"`
}
func regularPrivate(path string)(os.FileInfo,error){
 st,err:=os.Lstat(path)
 if err!=nil{return nil,err}
 if !st.Mode().IsRegular()||st.Mode().Perm()&0077!=0{return nil,errors.New("unsafe resume file")}
 return st,nil
}
func syncDirectory(root string)error{
 f,err:=os.Open(root);if err!=nil{return err};defer f.Close();return f.Sync()
}
func saveCheckpoint(root,path string,f *os.File,c checkpoint)error{
 if err:=f.Sync();err!=nil{return err}
 b,err:=json.Marshal(c);if err!=nil{return err}
 tmp,err:=os.CreateTemp(root,"resume-checkpoint-*.tmp");if err!=nil{return err}
 defer os.Remove(tmp.Name());defer tmp.Close()
 if _,err=tmp.Write(b);err!=nil{return err}
 if err=tmp.Sync();err!=nil{return err}
 if err=tmp.Close();err!=nil{return err}
 if err=os.Rename(tmp.Name(),path);err!=nil{return err}
 return syncDirectory(root)
}
type resumeWriter struct{
 f *os.File
 h hash.Hash
 root,path string
 c checkpoint
 done int64
}
func(w *resumeWriter)checkpoint(force bool)error{
 // Never checkpoint EOF until Fetch returns success. A retry must still
 // confirm file identity and retrieve at least the final bytes.
 if w.done<=w.c.Offset||w.done>=w.c.Expected||w.done%ResumeAlignment!=0{return nil}
 if !force&&w.done-w.c.Offset<checkpointInterval{return nil}
 next:=w.c;next.Offset=w.done;next.SHA256=hex.EncodeToString(w.h.Sum(nil))
 if err:=saveCheckpoint(w.root,w.path,w.f,next);err!=nil{return err}
 w.c=next
 return nil
}
func(w *resumeWriter)Write(p []byte)(int,error){
 if int64(len(p))>w.c.Expected-w.done{return 0,errors.New("download exceeds reservation")}
 n,err:=w.f.Write(p)
 if n>0{_,_=w.h.Write(p[:n]);w.done+=int64(n)}
 if err!=nil{return n,err}
 if n!=len(p){return n,io.ErrShortWrite}
 return n,w.checkpoint(false)
}

// StageResume retains only a checksummed, synced prefix across failures.
// One worker per media and an exclusively owned private root are required.
// Identity must include the immutable source identity, not a temporary URL.
func StageResume(ctx context.Context,root string,id,expected int64,identity string,quota *disk.Manager,fetch ResumeFetch)(string,error){
 if id<=0||expected<=0||identity==""||quota==nil||fetch==nil{return "",errors.New("invalid resume arguments")}
 root,err:=filepath.Abs(root);if err!=nil{return "",err}
 st,err:=os.Lstat(root);if err!=nil||!st.IsDir()||st.Mode().Perm()&0077!=0{return "",errors.New("private staging directory required")}
 dest:=filepath.Join(root,fmt.Sprintf("%d.media",id))
 if _,err=os.Lstat(dest);!errors.Is(err,os.ErrNotExist){return "",errors.New("completed stage already exists or cannot be inspected")}
 part:=filepath.Join(root,fmt.Sprintf("%d.resume.part",id))
 meta:=filepath.Join(root,fmt.Sprintf("%d.resume.json",id))
 for _,p:=range []string{part,meta}{
  if _,err=regularPrivate(p);err!=nil&&!errors.Is(err,os.ErrNotExist){return "",err}
 }
 f,err:=os.OpenFile(part,os.O_RDWR|os.O_CREATE,0600);if err!=nil{return "",err}
 defer f.Close()
 h:=sha256.New()
 c:=checkpoint{Identity:identity,Expected:expected}
 // Missing, corrupted, mismatched, or out-of-range checkpoint: start over.
 // The part remains private and is never published based on length alone.
 if info,e:=regularPrivate(meta);e==nil&&info.Size()<=4096{
  b,e:=os.ReadFile(meta)
  var old checkpoint
  if e==nil&&json.Unmarshal(b,&old)==nil&&old.Identity==identity&&old.Expected==expected&&old.Offset>0&&old.Offset<expected&&old.Offset%ResumeAlignment==0{
   if info,e:=f.Stat();e==nil&&info.Size()>=old.Offset{
    if n,e:=io.CopyN(h,f,old.Offset);e==nil&&n==old.Offset&&hex.EncodeToString(h.Sum(nil))==old.SHA256{c=old}
   }
  }
 }
 if c.Offset==0{h.Reset()}
 if err=f.Truncate(c.Offset);err!=nil{return "",err}
 if _,err=f.Seek(c.Offset,io.SeekStart);err!=nil{return "",err}
 // Reset invalid metadata before beginning. On a crash, old metadata cannot
 // authorize a prefix whose identity or digest no longer matches.
 if err=saveCheckpoint(root,meta,f,c);err!=nil{return "",err}
 release,err:=quota.Reserve(expected-c.Offset);if err!=nil{return "",err};defer release()
 w:=&resumeWriter{f:f,h:h,root:root,path:meta,c:c,done:c.Offset}
 if err=fetch(ctx,c.Offset,w);err!=nil{
  if checkpointErr:=w.checkpoint(true);checkpointErr!=nil{return "",errors.Join(err,checkpointErr)}
  return "",err
 }
 if err=ctx.Err();err!=nil{_ = w.checkpoint(true);return "",err}
 if w.done!=expected{return "",errors.New("download size mismatch")}
 if err=f.Sync();err!=nil{return "",err}
 if err=f.Close();err!=nil{return "",err}
 if err=os.Link(part,dest);err!=nil{return "",err}
 if err=syncDirectory(root);err!=nil{return "",err}
 if err=os.Remove(part);err!=nil{return "",err}
 if err=os.Remove(meta);err!=nil{return "",err}
 if err=syncDirectory(root);err!=nil{return "",err}
 return dest,nil
}

// RecoverPublished removes only a partial hard link proven to refer to the
// already published file, including a crash between link and unlink.
func RecoverPublished(root string,id int64)error{
 dest:=filepath.Join(root,fmt.Sprintf("%d.media",id))
 part:=filepath.Join(root,fmt.Sprintf("%d.resume.part",id))
 meta:=filepath.Join(root,fmt.Sprintf("%d.resume.json",id))
 d,err:=regularPrivate(dest);if err!=nil{return err}
 p,err:=regularPrivate(part)
 if err==nil{
  if !os.SameFile(d,p){return errors.New("conflicting partial and published file")}
  if err=os.Remove(part);err!=nil{return err}
 }else if !errors.Is(err,os.ErrNotExist){return err}
 if _,err=regularPrivate(meta);err==nil{
  if err=os.Remove(meta);err!=nil{return err}
 }else if !errors.Is(err,os.ErrNotExist){return err}
 return syncDirectory(root)
}
