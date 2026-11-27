package storage

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "regexp"
 "strings"
 "sync"
 "syscall"
 "time"

 "github.com/hctty27/telegram-backup/internal/security"
 "github.com/jackc/pgx/v5/pgxpool"
)

type DriveConfig struct {
 Name string `json:"name"`
 ClientID string `json:"client_id"`
 ClientSecret string `json:"client_secret"`
 Token string `json:"token"`
 RootFolderID string `json:"root_folder_id"`
}
var safeCredential=regexp.MustCompile("^[A-Za-z0-9_.-]+$")
func ValidateDriveCredentials(c DriveConfig)error{
 if !remoteName.MatchString(c.Name)||len(c.ClientID)>256||len(c.ClientSecret)>256||
 !safeCredential.MatchString(c.ClientID)||!strings.HasSuffix(c.ClientID,".apps.googleusercontent.com")||
 !safeCredential.MatchString(c.ClientSecret)||
 (c.RootFolderID!=""&&(!safeCredential.MatchString(c.RootFolderID)||len(c.RootFolderID)>256)){
  return errors.New("invalid Drive configuration")
 }
 return nil
}
func driveINI(c DriveConfig)([]byte,error){
 if err:=ValidateDriveCredentials(c);err!=nil{return nil,err}
 if len(c.Token)>32<<10{return nil,errors.New("token too large")}
 var token struct{
  AccessToken string `json:"access_token"`
  RefreshToken string `json:"refresh_token"`
  TokenType string `json:"token_type"`
  Expiry time.Time `json:"expiry"`
 }
 if err:=json.Unmarshal([]byte(c.Token),&token);err!=nil||token.RefreshToken==""||token.AccessToken==""{
  return nil,errors.New("valid rclone OAuth token with refresh token required")
 }
 normalized,err:=json.Marshal(token);if err!=nil{return nil,err}
 return []byte(fmt.Sprintf("[%s]\ntype = drive\nclient_id = %s\nclient_secret = %s\nscope = drive\ntoken = %s\nroot_folder_id = %s\n",c.Name,c.ClientID,c.ClientSecret,normalized,c.RootFolderID)),nil
}

// Managed stores one encrypted rclone INI per remote. Each invocation uses a
// private directory on tmpfs and persists refreshed OAuth tokens even when
// an upload fails. Serializing by name avoids stale-token write races.
type Managed struct{
 Pool *pgxpool.Pool
 Box *security.Box
 TempRoot string
 mu sync.Mutex
 gates map[string]chan struct{}
}
func(m *Managed)lock(ctx context.Context,name string)(func(),error){
 m.mu.Lock()
 if m.gates==nil{m.gates=make(map[string]chan struct{})}
 g:=m.gates[name];if g==nil{g=make(chan struct{},1);m.gates[name]=g}
 m.mu.Unlock()
 select{case g<-struct{}{}:return func(){<-g},nil;case <-ctx.Done():return nil,ctx.Err()}
}
func purpose(name string)string{return "rclone-remote:"+name+":v1"}
func(m *Managed)tempConfig(plain []byte)(string,func(),error){
 root:=m.TempRoot;if root==""{root="/dev/shm"}
 var stat syscall.Statfs_t
 if err:=syscall.Statfs(root,&stat);err!=nil||stat.Type!=0x01021994{return "",nil,errors.New("rclone credential temp root must be tmpfs")}
 dir,err:=os.MkdirTemp(root,"tgb-rclone-");if err!=nil{return "",nil,err}
 cleanup:=func(){_ = os.RemoveAll(dir)}
 path:=filepath.Join(dir,"rclone.conf")
 if err=os.WriteFile(path,plain,0600);err!=nil{cleanup();return "",nil,err}
 return path,cleanup,nil
}
func readConfig(path string)([]byte,error){
 st,err:=os.Lstat(path)
 if err!=nil||!st.Mode().IsRegular()||st.Mode().Perm()&0077!=0||st.Size()>128<<10||st.Size()==0{return nil,errors.New("invalid refreshed config")}
 return os.ReadFile(path)
}
func(m *Managed)CreateDrive(ctx context.Context,c DriveConfig)error{
 plain,err:=driveINI(c);if err!=nil{return err};defer clear(plain)
 release,err:=m.lock(ctx,c.Name);if err!=nil{return err};defer release()
 var exists bool
 if err=m.Pool.QueryRow(ctx,"SELECT EXISTS(SELECT 1 FROM storage_remotes WHERE name=$1)",c.Name).Scan(&exists);err!=nil{return err}
 if exists{return errors.New("remote already exists; create a new unique name")}
 p,cleanup,err:=m.tempConfig(plain);if err!=nil{return err};defer cleanup()
 r:=&Rclone{Config:p}
 if err=r.TestRemote(ctx,c.Name);err!=nil{return err}
 refreshed,err:=readConfig(p);if err!=nil{return err};defer clear(refreshed)
 encrypted,err:=m.Box.Seal(purpose(c.Name),refreshed);if err!=nil{return err}
 _,err=m.Pool.Exec(ctx,"INSERT INTO storage_remotes(name,kind,config_ciphertext,key_version,enabled) VALUES($1,'drive',$2,1,true)",c.Name,encrypted)
 return err
}
func(m *Managed)withRemote(ctx context.Context,name string,fn func(*Rclone)error)error{
 if !remoteName.MatchString(name){return errors.New("invalid remote name")}
 release,err:=m.lock(ctx,name);if err!=nil{return err};defer release()
 var encrypted []byte
 if err=m.Pool.QueryRow(ctx,"SELECT config_ciphertext FROM storage_remotes WHERE name=$1 AND kind='drive' AND enabled AND key_version=1",name).Scan(&encrypted);err!=nil{return errors.New("managed remote unavailable")}
 plain,err:=m.Box.Open(purpose(name),encrypted);if err!=nil{return err};defer clear(plain)
 p,cleanup,err:=m.tempConfig(plain);if err!=nil{return err};defer cleanup()
 opErr:=fn(&Rclone{Config:p})
 updated,err:=readConfig(p);if err!=nil{return err};defer clear(updated)
 sealed,err:=m.Box.Seal(purpose(name),updated);if err!=nil{return err}
 // The operation may have been cancelled after OAuth refreshed the token.
 // Persist the refreshed configuration with a separate bounded context.
 save,cancel:=context.WithTimeout(context.Background(),5*time.Second);defer cancel()
 tag,err:=m.Pool.Exec(save,"UPDATE storage_remotes SET config_ciphertext=$2 WHERE name=$1 AND config_ciphertext=$3 AND key_version=1",name,sealed,encrypted)
 if err!=nil||tag.RowsAffected()!=1{return errors.New("could not persist remote credential refresh")}
 return opErr
}
func(m *Managed)TestRemote(ctx context.Context,name string)error{
 return m.withRemote(ctx,name,func(r *Rclone)error{return r.TestRemote(ctx,name)})
}
func(m *Managed)UploadVerified(ctx context.Context,source,name,key string)(v Verified,err error){
 err=m.withRemote(ctx,name,func(r *Rclone)error{var e error;v,e=r.UploadVerified(ctx,source,name,key);return e})
 return v,err
}
