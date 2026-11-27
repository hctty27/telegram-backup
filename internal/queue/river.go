// Package queue wires River's durable queues to explicit stage handlers.
// It does not start workers until the application supplies real handlers.
package queue

import (
 "context"
 "errors"
 "time"

 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
 "github.com/riverqueue/river"
 "github.com/riverqueue/river/riverdriver/riverpgxv5"
)

type SyncArgs struct{ChatID int64 `json:"chat_id"`}
func(SyncArgs)Kind()string{return "telegram_sync"}
type DownloadArgs struct{MediaID int64 `json:"media_id"`}
func(DownloadArgs)Kind()string{return "media_download"}
type UploadArgs struct{ReplicaID int64 `json:"replica_id"`}
func(UploadArgs)Kind()string{return "media_upload"}
type CleanupArgs struct{MediaID int64 `json:"media_id"`}
func(CleanupArgs)Kind()string{return "media_cleanup"}

type handler[T river.JobArgs] struct{river.WorkerDefaults[T]; run func(context.Context,T)error}
func(h *handler[T])Work(ctx context.Context,job *river.Job[T])error{return h.run(ctx,job.Args)}
func(h *handler[T])Timeout(*river.Job[T])time.Duration{return 6*time.Hour}

type Handlers struct{
 Sync func(context.Context,SyncArgs)error
 Download func(context.Context,DownloadArgs)error
 Upload func(context.Context,UploadArgs)error
 Cleanup func(context.Context,CleanupArgs)error
}

func New(pool *pgxpool.Pool,h Handlers)(*river.Client[pgx.Tx],error){
 if pool==nil||h.Sync==nil||h.Download==nil||h.Upload==nil||h.Cleanup==nil{return nil,errors.New("all queue handlers are required")}
 workers:=river.NewWorkers()
 river.AddWorker(workers,&handler[SyncArgs]{run:h.Sync})
 river.AddWorker(workers,&handler[DownloadArgs]{run:h.Download})
 river.AddWorker(workers,&handler[UploadArgs]{run:h.Upload})
 river.AddWorker(workers,&handler[CleanupArgs]{run:h.Cleanup})
 return river.NewClient(riverpgxv5.New(pool),&river.Config{
  Workers:workers,Queues:map[string]river.QueueConfig{
   "telegram_sync":{MaxWorkers:1},"media_download":{MaxWorkers:2},
   "media_upload":{MaxWorkers:2},"media_cleanup":{MaxWorkers:1},
  },
 })
}

// EnqueueTx must share the transaction that commits the corresponding business state.
func EnqueueTx(ctx context.Context,client *river.Client[pgx.Tx],tx pgx.Tx,args river.JobArgs)error{
 switch args.(type){case SyncArgs,DownloadArgs,UploadArgs,CleanupArgs:default:return errors.New("unknown job type")}
 _,err:=client.InsertTx(ctx,tx,args,&river.InsertOpts{
  Queue:args.Kind(),MaxAttempts:10,UniqueOpts:river.UniqueOpts{ByArgs:true,ByQueue:true},
 })
 return err
}
