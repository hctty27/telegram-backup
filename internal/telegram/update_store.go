package telegram

import (
 "context"
 "errors"

 "github.com/gotd/td/telegram/updates"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)

// UpdateStore isolates state by application account and Telegram user.
type UpdateStore struct{ Pool *pgxpool.Pool; AccountID int64 }
var _ updates.StateStorage=(*UpdateStore)(nil)
var _ updates.ChannelAccessHasher=(*UpdateStore)(nil)
var _ updates.UserAccessHasher=(*UpdateStore)(nil)

func(s *UpdateStore)GetState(ctx context.Context,u int64)(v updates.State,found bool,err error){
 err=s.Pool.QueryRow(ctx,"SELECT pts,qts,date,seq FROM telegram_update_states WHERE account_id=$1 AND user_id=$2",s.AccountID,u).Scan(&v.Pts,&v.Qts,&v.Date,&v.Seq)
 if errors.Is(err,pgx.ErrNoRows){return v,false,nil};return v,err==nil,err
}
func(s *UpdateStore)SetState(ctx context.Context,u int64,v updates.State)error{
 tag,err:=s.Pool.Exec(ctx,`INSERT INTO telegram_update_states(account_id,user_id,pts,qts,date,seq) VALUES($1,$2,$3,$4,$5,$6)
 ON CONFLICT(account_id) DO UPDATE SET pts=EXCLUDED.pts,qts=EXCLUDED.qts,date=EXCLUDED.date,seq=EXCLUDED.seq WHERE telegram_update_states.user_id=EXCLUDED.user_id`,s.AccountID,u,v.Pts,v.Qts,v.Date,v.Seq)
 if err==nil&&tag.RowsAffected()!=1{return errors.New("update state identity mismatch")};return err
}
func(s *UpdateStore)set(ctx context.Context,u int64,query string,values ...any)error{
 args:=append([]any{s.AccountID,u},values...)
 tag,err:=s.Pool.Exec(ctx,query,args...)
 if err==nil&&tag.RowsAffected()!=1{return errors.New("update state missing")};return err
}
func(s *UpdateStore)SetPts(ctx context.Context,u int64,v int)error{return s.set(ctx,u,"UPDATE telegram_update_states SET pts=$3 WHERE account_id=$1 AND user_id=$2",v)}
func(s *UpdateStore)SetQts(ctx context.Context,u int64,v int)error{return s.set(ctx,u,"UPDATE telegram_update_states SET qts=$3 WHERE account_id=$1 AND user_id=$2",v)}
func(s *UpdateStore)SetDate(ctx context.Context,u int64,v int)error{return s.set(ctx,u,"UPDATE telegram_update_states SET date=$3 WHERE account_id=$1 AND user_id=$2",v)}
func(s *UpdateStore)SetSeq(ctx context.Context,u int64,v int)error{return s.set(ctx,u,"UPDATE telegram_update_states SET seq=$3 WHERE account_id=$1 AND user_id=$2",v)}
func(s *UpdateStore)SetDateSeq(ctx context.Context,u int64,date,seq int)error{return s.set(ctx,u,"UPDATE telegram_update_states SET date=$3,seq=$4 WHERE account_id=$1 AND user_id=$2",date,seq)}
func(s *UpdateStore)GetChannelPts(ctx context.Context,u,c int64)(v int,found bool,err error){
 err=s.Pool.QueryRow(ctx,"SELECT pts FROM telegram_update_channels WHERE account_id=$1 AND user_id=$2 AND channel_id=$3 AND pts IS NOT NULL",s.AccountID,u,c).Scan(&v)
 if errors.Is(err,pgx.ErrNoRows){return 0,false,nil};return v,err==nil,err
}
func(s *UpdateStore)SetChannelPts(ctx context.Context,u,c int64,v int)error{
 _,err:=s.Pool.Exec(ctx,`INSERT INTO telegram_update_channels(account_id,user_id,channel_id,pts) VALUES($1,$2,$3,$4)
 ON CONFLICT(account_id,user_id,channel_id) DO UPDATE SET pts=EXCLUDED.pts`,s.AccountID,u,c,v);return err
}
func(s *UpdateStore)ForEachChannels(ctx context.Context,u int64,f func(context.Context,int64,int)error)error{
 rows,err:=s.Pool.Query(ctx,"SELECT channel_id,pts FROM telegram_update_channels WHERE account_id=$1 AND user_id=$2 AND pts IS NOT NULL",s.AccountID,u)
 if err!=nil{return err}
 type item struct{id int64;pts int}
 var items []item
 for rows.Next(){var v item;if err=rows.Scan(&v.id,&v.pts);err!=nil{rows.Close();return err};items=append(items,v)}
 err=rows.Err();rows.Close();if err!=nil{return err}
 for _,v:=range items{if err=f(ctx,v.id,v.pts);err!=nil{return err}}
 return nil
}
func(s *UpdateStore)SetChannelAccessHash(ctx context.Context,u,c,h int64)error{
 _,err:=s.Pool.Exec(ctx,`INSERT INTO telegram_update_channels(account_id,user_id,channel_id,access_hash) VALUES($1,$2,$3,$4)
 ON CONFLICT(account_id,user_id,channel_id) DO UPDATE SET access_hash=EXCLUDED.access_hash`,s.AccountID,u,c,h);return err
}
func(s *UpdateStore)GetChannelAccessHash(ctx context.Context,u,c int64)(v int64,found bool,err error){
 err=s.Pool.QueryRow(ctx,`SELECT coalesce(
 (SELECT access_hash FROM telegram_update_channels WHERE account_id=$1 AND user_id=$2 AND channel_id=$3),
 (SELECT access_hash FROM telegram_chats WHERE account_id=$1 AND peer_kind='channel' AND peer_id=$3))
 WHERE EXISTS(SELECT 1 FROM telegram_accounts WHERE id=$1 AND telegram_user_id=$2)`,s.AccountID,u,c).Scan(&v)
 if errors.Is(err,pgx.ErrNoRows){return 0,false,nil};return v,err==nil,err
}
func(s *UpdateStore)SetUserAccessHash(ctx context.Context,u,target,h int64)error{
 _,err:=s.Pool.Exec(ctx,`INSERT INTO telegram_update_users(account_id,user_id,target_user_id,access_hash) VALUES($1,$2,$3,$4)
 ON CONFLICT(account_id,user_id,target_user_id) DO UPDATE SET access_hash=EXCLUDED.access_hash`,s.AccountID,u,target,h);return err
}
func(s *UpdateStore)GetUserAccessHash(ctx context.Context,u,target int64)(v int64,found bool,err error){
 err=s.Pool.QueryRow(ctx,"SELECT access_hash FROM telegram_update_users WHERE account_id=$1 AND user_id=$2 AND target_user_id=$3",s.AccountID,u,target).Scan(&v)
 if errors.Is(err,pgx.ErrNoRows){return 0,false,nil};return v,err==nil,err
}
