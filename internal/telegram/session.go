package telegram

import(
 "context"
 "errors"
 "fmt"

 "github.com/gotd/td/session"
 "github.com/hctty27/telegram-backup/internal/security"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)

type SessionStore struct{Pool *pgxpool.Pool;Box *security.Box;AccountID int64}
func(s *SessionStore)purpose()string{return fmt.Sprintf("telegram-session:%d:v1",s.AccountID)}
func(s *SessionStore)LoadSession(ctx context.Context)([]byte,error){
 var data []byte
 err:=s.Pool.QueryRow(ctx,"SELECT ciphertext FROM telegram_sessions WHERE account_id=$1 AND key_version=1",s.AccountID).Scan(&data)
 if errors.Is(err,pgx.ErrNoRows){return nil,session.ErrNotFound}
 if err!=nil{return nil,err}
 return s.Box.Open(s.purpose(),data)
}
func(s *SessionStore)StoreSession(ctx context.Context,data []byte)error{
 encrypted,err:=s.Box.Seal(s.purpose(),data);if err!=nil{return err}
 _,err=s.Pool.Exec(ctx,`INSERT INTO telegram_sessions(account_id,ciphertext,key_version) VALUES($1,$2,1)
 ON CONFLICT(account_id) DO UPDATE SET ciphertext=EXCLUDED.ciphertext,key_version=1,updated_at=now()`,s.AccountID,encrypted)
 return err
}
var _ session.Storage=(*SessionStore)(nil)
