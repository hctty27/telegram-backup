package main

import (
 "context"
 "errors"
 "log/slog"
 "net/http"
 "os"
 "os/signal"
 "syscall"
 "time"

 "github.com/hctty27/telegram-backup/internal/api"
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
  handler,err=api.NewDashboardRouter(pool,os.Getenv("V2_ADMIN_TOKEN"),assets)
  if err!=nil{return err}
  slog.Info("V2 read-only dashboard enabled; backup workers remain disabled")
 }
 srv:=&http.Server{Addr:addr,Handler:handler,ReadHeaderTimeout:5*time.Second,ReadTimeout:15*time.Second,IdleTimeout:60*time.Second,MaxHeaderBytes:32<<10}
 errs:=make(chan error,1)
 go func(){slog.Info("V2 listening","addr",addr);errs<-srv.ListenAndServe()}()
 select{
 case err:=<-errs:if errors.Is(err,http.ErrServerClosed){return nil};return err
 case <-ctx.Done():shutdown,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel();return srv.Shutdown(shutdown)
 }
}
