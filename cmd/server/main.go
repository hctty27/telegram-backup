package main

import (
 "context"
 "errors"
 "log/slog"
 "net/http"
 "os"
 "os/signal"
 "path/filepath"
 "regexp"
 "strconv"
 "syscall"
 "time"

 "github.com/hctty27/telegram-backup/internal/api"
 "github.com/hctty27/telegram-backup/internal/backup"
 "github.com/hctty27/telegram-backup/internal/disk"
 "github.com/hctty27/telegram-backup/internal/monitor"
 "github.com/hctty27/telegram-backup/internal/queue"
 "github.com/hctty27/telegram-backup/internal/security"
 "github.com/hctty27/telegram-backup/internal/storage"
 "github.com/hctty27/telegram-backup/internal/telegram"
 "github.com/hctty27/telegram-backup/web"
 "github.com/jackc/pgx/v5/pgxpool"
)

func main(){
 if err:=run();err!=nil{slog.Error("server stopped","type","configuration or runtime failure");os.Exit(1)}
}
func run()error{
 ctx,stop:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM);defer stop()
 addr:=os.Getenv("V2_HTTP_ADDR");if addr==""{addr="127.0.0.1:8081"}
 var handler http.Handler=api.NewRouter()
 if url:=os.Getenv("V2_DATABASE_URL");url!=""{
  pool,err:=pgxpool.New(ctx,url);if err!=nil{return err};defer pool.Close()
  check,cancel:=context.WithTimeout(ctx,10*time.Second)
  err=pool.Ping(check);cancel();if err!=nil{return err}
  assets,err:=web.Assets();if err!=nil{return err}
  if os.Getenv("V2_ENABLE_WORKERS")=="true"{
   box,err:=security.NewBox(os.Getenv("V2_ENCRYPTION_KEY"));if err!=nil{return err}
   root:=os.Getenv("V2_DATA_DIR");if root==""{root="/var/lib/telegram-backup-v2"}
   root,err=filepath.Abs(root);if err!=nil{return err}
   stage:=filepath.Join(root,"staging")
   if err=privateDir(root);err!=nil{return err}
   if err=privateDir(stage);err!=nil{return err}
   // A second process must not share this staging directory or account sessions.
   // Keep the OS lock for the complete server lifetime. Deploy exactly one app.
   lock,err:=os.OpenFile(filepath.Join(root,"server.lock"),os.O_CREATE|os.O_RDWR,0600)
   if err!=nil{return err};defer lock.Close()
   if err=syscall.Flock(int(lock.Fd()),syscall.LOCK_EX|syscall.LOCK_NB);err!=nil{return errors.New("another worker owns this data directory")}
   defer syscall.Flock(int(lock.Fd()),syscall.LOCK_UN)
   owner,err:=pool.Acquire(ctx);if err!=nil{return err};defer owner.Release()
   var acquired bool
   if err=owner.QueryRow(ctx,"SELECT pg_try_advisory_lock(7427611001)").Scan(&acquired);err!=nil||!acquired{return errors.New("another database worker owner exists")}
   defer func(){c,cancel:=context.WithTimeout(context.Background(),3*time.Second);defer cancel();_,_=owner.Exec(c,"SELECT pg_advisory_unlock(7427611001)") }()
   if err=clearPartials(stage);err!=nil{return err}
   quotaBytes,err:=gibSetting("V2_CACHE_GIB",10);if err!=nil{return err}
   reserveBytes,err:=gibSetting("V2_MIN_FREE_GIB",5);if err!=nil{return err}
   q,err:=disk.New(quotaBytes,reserveBytes,func()(disk.Usage,error){return disk.MeasureDirectory(stage)});if err!=nil{return err}
   service:=telegram.New(pool,box)
   login:=telegram.NewLoginManager(ctx,service)
   cloud:=&storage.Managed{Pool:pool,Box:box}
   engine:=&backup.Engine{Pool:pool,Telegram:service,Cloud:cloud,Quota:q,Progress:monitor.New(),Root:stage,MaxFile:quotaBytes}
   jobs,err:=queue.New(pool,engine.Handlers());if err!=nil{return err};engine.Jobs=jobs
   residentCtx,cancelResidents:=context.WithCancel(ctx)
   residents:=telegram.NewResidents(residentCtx,service,engine.IngestLive)
   service.Residents=residents
   defer func(){cancelResidents();residents.Wait()}()
   actions:=&api.Actions{Pool:pool,Telegram:service,Login:login,Engine:engine,Managed:cloud}
   handler,err=api.NewDashboardRouter(pool,os.Getenv("V2_ADMIN_TOKEN"),assets,actions.Routes)
   if err!=nil{return err}
   if err=jobs.Start(ctx);err!=nil{return err}
   defer func(){c,cancel:=context.WithTimeout(context.Background(),20*time.Second);defer cancel();_=jobs.Stop(c)}()
   go func(){
    ticker:=time.NewTicker(15*time.Second);defer ticker.Stop()
    for{
     if err:=residents.Reconcile(residentCtx);err!=nil&&residentCtx.Err()==nil{slog.Warn("Telegram connection reconciliation failed")}
     if err:=engine.Schedule(ctx);err!=nil&&ctx.Err()==nil{slog.Warn("backup scheduling failed","type","database or queue error")}
     select{case <-ctx.Done():return;case <-ticker.C:}
    }
   }()
   slog.Info("V2 realtime and polling backup engine enabled")
  }else{
   handler,err=api.NewDashboardRouter(pool,os.Getenv("V2_ADMIN_TOKEN"),assets)
   if err!=nil{return err}
   slog.Info("V2 read-only dashboard; workers disabled")
  }
 }
 srv:=&http.Server{Addr:addr,Handler:handler,ReadHeaderTimeout:5*time.Second,ReadTimeout:15*time.Second,WriteTimeout:60*time.Second,IdleTimeout:60*time.Second,MaxHeaderBytes:32<<10}
 errs:=make(chan error,1)
 go func(){slog.Info("V2 listening","addr",addr);errs<-srv.ListenAndServe()}()
 select{
 case err:=<-errs:if errors.Is(err,http.ErrServerClosed){return nil};return err
 case <-ctx.Done():shutdown,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel();return srv.Shutdown(shutdown)
 }
}

func privateDir(p string)error{
 if err:=os.MkdirAll(p,0700);err!=nil{return err}
 info,err:=os.Lstat(p)
 if err!=nil{return err}
 if !info.IsDir()||info.Mode().Perm()&0077!=0{return errors.New("data directory must be private")}
 return nil
}
// Only obsolete random staging files and incomplete checkpoint writes are removed.
// Stable .resume.part/.resume.json files survive restart.
var partialName=regexp.MustCompile("^([1-9][0-9]*-[0-9]+[.]part|resume-checkpoint-[0-9]+[.]tmp)$")
func clearPartials(root string)error{
 entries,err:=os.ReadDir(root);if err!=nil{return err}
 for _,entry:=range entries{
  if !partialName.MatchString(entry.Name())||!entry.Type().IsRegular(){continue}
  if err=os.Remove(filepath.Join(root,entry.Name()));err!=nil{return err}
 }
 return nil
}

func gibSetting(name string,fallback int64)(int64,error){
 raw:=os.Getenv(name)
 if raw==""{return fallback<<30,nil}
 value,err:=strconv.ParseInt(raw,10,64)
 if err!=nil||value<1||value>1048576{return 0,errors.New("invalid positive GiB setting: "+name)}
 return value<<30,nil
}
