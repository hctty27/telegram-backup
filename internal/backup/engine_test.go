package backup

import(
 "context"
 "crypto/sha256"
 "encoding/hex"
 "io"
 "os"
 "strings"
 "testing"
 "time"

 "github.com/gotd/td/tg"
 "github.com/gotd/td/telegram/updates"
 "github.com/hctty27/telegram-backup/internal/disk"
 "github.com/hctty27/telegram-backup/internal/queue"
 "github.com/hctty27/telegram-backup/internal/storage"
 tgservice "github.com/hctty27/telegram-backup/internal/telegram"
 "github.com/jackc/pgx/v5/pgxpool"
)

type sourceStub struct{}
func(sourceStub)ReadPage(_ context.Context,_ int64,_ tg.InputPeerClass,min,before int)(tgservice.Page,error){
 if min>=42{return tgservice.Page{Complete:true},nil}
 return tgservice.Page{Media:[]tgservice.Media{{MessageID:42,Date:time.Unix(1700000000,0),Key:"document:123",Name:"test.mp4",MIME:"video/mp4",Size:3}},First:42,Last:42,Count:1,Complete:true},nil
}
func(sourceStub)Fetch(_ context.Context,_ int64,_ tg.InputPeerClass,_ int,_ string,_ int64,_ int64,w io.Writer)error{_,err:=io.WriteString(w,"abc");return err}
type cloudStub struct{}
func(cloudStub)TestRemote(context.Context,string)error{return nil}
func(cloudStub)UploadVerified(_ context.Context,p,_,_ string)(storage.Verified,error){
 b,err:=os.ReadFile(p);if err!=nil{return storage.Verified{},err}
 h:=sha256.Sum256(b)
 return storage.Verified{ID:"test-object",Bytes:int64(len(b)),SHA256:hex.EncodeToString(h[:])},nil
}
func TestSafeName(t *testing.T){
 for _,input:=range []string{"../../abc.mp4","a\\b.mp4","..","a:b","a\x00b"}{
  name:=safeName(input)
  if name==""||name==".."||strings.ContainsAny(name,"/\\:\x00"){t.Fatalf("unsafe name %q",name)}
 }
}
// Run only with an explicitly provisioned disposable integration database.
func TestDurablePipeline(t *testing.T){
 url:=os.Getenv("V2_INTEGRATION_DATABASE_URL")
 if url==""{t.Skip("disposable integration database not configured")}
 ctx,cancel:=context.WithTimeout(context.Background(),time.Minute);defer cancel()
 pool,err:=pgxpool.New(ctx,url);if err!=nil{t.Fatal(err)};defer pool.Close()
 root:=t.TempDir();if err=os.Chmod(root,0700);err!=nil{t.Fatal(err)}
 quota,err:=disk.New(100,0,func()(disk.Usage,error){return disk.Usage{Free:100},nil});if err!=nil{t.Fatal(err)}
 e:=&Engine{Pool:pool,Telegram:sourceStub{},Cloud:cloudStub{},Quota:quota,Root:root,MaxFile:100}
 jobs,err:=queue.New(pool,e.Handlers());if err!=nil{t.Fatal(err)};e.Jobs=jobs
 var account,chat,remote,media,replica int64
 if err=pool.QueryRow(ctx,"INSERT INTO telegram_accounts(api_id,api_hash_ciphertext,enabled) VALUES(123,decode('','hex'),true) RETURNING id").Scan(&account);err!=nil{t.Fatal(err)}
 store:=&tgservice.UpdateStore{Pool:pool,AccountID:account}
 if _,found,err:=store.GetState(ctx,123);err!=nil||found{t.Fatalf("unexpected stored state %v %v",found,err)}
 if err=store.SetState(ctx,123,updates.State{Pts:1,Qts:2,Date:3,Seq:4});err!=nil{t.Fatal(err)}
 if err=store.SetPts(ctx,123,9);err!=nil{t.Fatal(err)}
 if err=store.SetDateSeq(ctx,123,5,6);err!=nil{t.Fatal(err)}
 if v,found,err:=store.GetState(ctx,123);err!=nil||!found||v.Pts!=9||v.Qts!=2||v.Date!=5||v.Seq!=6{t.Fatalf("state=%+v found=%v err=%v",v,found,err)}
 if err=store.SetChannelPts(ctx,123,100,42);err!=nil{t.Fatal(err)}
 if err=store.SetChannelAccessHash(ctx,123,100,456);err!=nil{t.Fatal(err)}
 if err=store.ForEachChannels(ctx,123,func(ctx context.Context,id int64,pts int)error{if id!=100||pts!=42{t.Fatal("wrong channel state")};return nil});err!=nil{t.Fatal(err)}
 if err=store.SetUserAccessHash(ctx,123,200,789);err!=nil{t.Fatal(err)}
 if h,found,err:=store.GetUserAccessHash(ctx,123,200);err!=nil||!found||h!=789{t.Fatalf("user hash=%d found=%v err=%v",h,found,err)}
 if err=pool.QueryRow(ctx,"INSERT INTO telegram_chats(account_id,peer_kind,peer_id,title,enabled) VALUES($1,'chat',100,'test',true) RETURNING id",account).Scan(&chat);err!=nil{t.Fatal(err)}
 name:="integration-"+time.Now().Format("20060102150405.000000000")
 if err=pool.QueryRow(ctx,"INSERT INTO storage_remotes(name,kind,config_ciphertext,enabled) VALUES($1,'test',decode('','hex'),true) RETURNING id",name).Scan(&remote);err!=nil{t.Fatal(err)}
 if _,err=pool.Exec(ctx,"INSERT INTO backup_policies(chat_id,remote_id,enabled) VALUES($1,$2,true)",chat,remote);err!=nil{t.Fatal(err)}
 if err=e.Schedule(ctx);err!=nil{t.Fatal(err)}
 event:=tgservice.LiveEvent{AccountID:account,PeerKind:"chat",PeerID:100,Media:tgservice.Media{MessageID:42,Date:time.Unix(1700000000,0),Key:"document:123",Name:"test.mp4",MIME:"video/mp4",Size:3}}
 if err=e.IngestLive(ctx,event);err!=nil{t.Fatal(err)}
 if err=e.IngestLive(ctx,event);err!=nil{t.Fatal(err)}
 var high int64
 if err=pool.QueryRow(ctx,"SELECT incremental_high_water FROM sync_checkpoints WHERE chat_id=$1",chat).Scan(&high);err!=nil||high!=0{t.Fatalf("live event moved history cursor: %d %v",high,err)}
 ignored:=event;ignored.PeerID=999
 if err=e.IngestLive(ctx,ignored);err!=nil{t.Fatal(err)}
 if err=e.Sync(ctx,queue.SyncArgs{ChatID:chat});err!=nil{t.Fatal(err)}
 if err=e.Sync(ctx,queue.SyncArgs{ChatID:chat});err!=nil{t.Fatal(err)}
 var count int
 if err=pool.QueryRow(ctx,"SELECT count(*) FROM media_files f JOIN messages m ON m.id=f.message_id WHERE m.chat_id=$1",chat).Scan(&count);err!=nil||count!=1{t.Fatalf("media dedup: count=%d err=%v",count,err)}
 if err=pool.QueryRow(ctx,"SELECT f.id,p.id FROM media_files f JOIN messages m ON m.id=f.message_id JOIN media_replicas p ON p.media_id=f.id WHERE m.chat_id=$1",chat).Scan(&media,&replica);err!=nil{t.Fatal(err)}
 if err=e.Download(ctx,queue.DownloadArgs{MediaID:media});err!=nil{t.Fatal(err)}
 var local,state string
 if err=pool.QueryRow(ctx,"SELECT local_path,state FROM media_files WHERE id=$1",media).Scan(&local,&state);err!=nil||state!="downloaded"{t.Fatalf("staging: %s %v",state,err)}
 if err=e.Cleanup(ctx,queue.CleanupArgs{MediaID:media});err!=nil{t.Fatal(err)}
 if _,err=os.Stat(local);err!=nil{t.Fatal("unverified local file deleted")}
 if err=e.Upload(ctx,queue.UploadArgs{ReplicaID:replica});err!=nil{t.Fatal(err)}
 if err=pool.QueryRow(ctx,"SELECT state FROM media_files WHERE id=$1",media).Scan(&state);err!=nil||state!="cleanup_pending"{t.Fatalf("verify: %s %v",state,err)}
 if err=e.Cleanup(ctx,queue.CleanupArgs{MediaID:media});err!=nil{t.Fatal(err)}
 if err=e.Cleanup(ctx,queue.CleanupArgs{MediaID:media});err!=nil{t.Fatal(err)}
 if _,err=os.Stat(local);!os.IsNotExist(err){t.Fatal("local file not removed after verification")}
 if err=pool.QueryRow(ctx,"SELECT state FROM media_files WHERE id=$1",media).Scan(&state);err!=nil||state!="completed"{t.Fatalf("cleanup: %s %v",state,err)}
}
