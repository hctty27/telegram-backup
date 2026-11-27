//go:build linux

package disk

import (
 "errors"
 "io/fs"
 "math"
 "path/filepath"
 "syscall"
)

// MeasureDirectory scans only the private staging filesystem; symlinks are not followed.
func MeasureDirectory(root string)(Usage,error){
 var u Usage
 var stat syscall.Statfs_t
 if err:=syscall.Statfs(root,&stat);err!=nil{return u,err}
 if stat.Bsize<=0||stat.Bavail>uint64(math.MaxInt64)/uint64(stat.Bsize){return u,errors.New("filesystem size overflow")}
 u.Free=int64(stat.Bavail)*stat.Bsize
 err:=filepath.WalkDir(root,func(_ string,d fs.DirEntry,err error)error{
  if err!=nil{return err}
  if !d.Type().IsRegular(){return nil}
  info,err:=d.Info();if err!=nil{return err}
  if info.Size()<0||info.Size()>math.MaxInt64-u.Staged{return errors.New("staging size overflow")}
  u.Staged+=info.Size()
  return nil
 })
 return u,err
}
