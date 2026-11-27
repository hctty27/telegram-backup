// Package storage provides a fail-closed, immutable rclone transfer adapter.
package storage

import (
 "context"
 "crypto/md5"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "io"
 "os"
 "os/exec"
 "path"
 "path/filepath"
 "regexp"
 "strings"
 "sync"
)

var remoteName=regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$")
type Rclone struct{
 Config string // private persistent config; OAuth refresh is retained
 mu sync.Mutex // serialize this config's uploads and refreshes
}
type Verified struct{ ID string; Bytes int64; SHA256 string }

func destination(remote,key string)(string,error){
 if !remoteName.MatchString(remote)||key==""||strings.ContainsAny(key, "\\:\r\n\x00")||path.IsAbs(key)||path.Clean(key)!=key||key=="."||key==".."||strings.HasPrefix(key,"../"){
  return "",errors.New("invalid storage destination")
 }
 return remote+":"+key,nil
}
type object struct{
 Name string
 ID string
 Size int64
 Hashes map[string]string
}
type cappedBuffer struct{ data []byte }
func(b *cappedBuffer)Write(p []byte)(int,error){
 if len(p)>8<<20-len(b.data){return 0,errors.New("rclone output exceeded limit")}
 b.data=append(b.data,p...);return len(p),nil
}
func(r *Rclone)command(ctx context.Context,args ...string)*exec.Cmd{
 // Explicit argument vector; never invoke a shell or inherit a user-provided command.
 return exec.CommandContext(ctx,"rclone",append([]string{"--config",r.Config},args...)...)
}
func(r *Rclone)UploadVerified(ctx context.Context,source,remote,key string)(Verified,error){
 r.mu.Lock();defer r.mu.Unlock()
 var result Verified
 cfg,err:=os.Lstat(r.Config)
 if err!=nil||!cfg.Mode().IsRegular()||cfg.Mode().Perm()&0077!=0{return result,errors.New("private rclone config required")}
 dest,err:=destination(remote,key);if err!=nil{return result,err}
 f,err:=os.Open(source);if err!=nil{return result,err}
 defer f.Close()
 st,err:=f.Stat();if err!=nil||!st.Mode().IsRegular(){return result,errors.New("source is not a regular file")}
 sha,md:=sha256.New(),md5.New()
 n,err:=io.Copy(io.MultiWriter(sha,md),f);if err!=nil||n!=st.Size(){return result,errors.New("cannot hash staged file")}
 if !filepath.IsAbs(source){return result,errors.New("absolute local source required")}
 // Source is immutable by contract during this operation.
 cmd:=r.command(ctx,"copyto",source,dest,"--immutable","--checksum","--retries","3","--low-level-retries","10")
 cmd.Stdout=io.Discard;cmd.Stderr=io.Discard
 if err:=cmd.Run();err!=nil{return result,errors.New("cloud upload failed")}
 parent:=path.Dir(key)
 if parent=="."{parent=""}
 var out cappedBuffer
 cmd=r.command(ctx,"lsjson",remote+":"+parent,"--files-only","--hash")
 cmd.Stdout=&out;cmd.Stderr=io.Discard
 if err:=cmd.Run();err!=nil{return result,errors.New("remote inventory failed")}
 var entries []object
 if err:=json.Unmarshal(out.data,&entries);err!=nil{return result,errors.New("invalid remote inventory")}
 var matches []object
 for _,entry:=range entries{if entry.Name==path.Base(key){matches=append(matches,entry)}}
 if len(matches)!=1||matches[0].Size!=n{return result,errors.New("remote object missing, ambiguous, or wrong size")}
 item:=matches[0]
 localSHA:=hex.EncodeToString(sha.Sum(nil))
 if remoteSHA:=item.Hashes["SHA-256"];remoteSHA!="" {
  if !strings.EqualFold(remoteSHA,localSHA){return result,errors.New("remote SHA-256 mismatch")}
 }else if remoteMD:=item.Hashes["MD5"];remoteMD!=""{
  if !strings.EqualFold(remoteMD,hex.EncodeToString(md.Sum(nil))){return result,errors.New("remote MD5 mismatch")}
 }else{
  // No comparable provider hash: stream back into a hasher, without another local file.
  remoteHash:=sha256.New()
  cmd=r.command(ctx,"cat",dest)
  cmd.Stdout=remoteHash;cmd.Stderr=io.Discard
  if err:=cmd.Run();err!=nil||hex.EncodeToString(remoteHash.Sum(nil))!=localSHA{return result,errors.New("remote readback verification failed")}
 }
 return Verified{ID:item.ID,Bytes:n,SHA256:localSHA},nil
}
