package main
import(
 "context"
 "database/sql"
 "errors"
 "log/slog"
 "os"
 "time"
 "github.com/jackc/pgx/v5/pgxpool"
 _ "github.com/jackc/pgx/v5/stdlib"
 "github.com/pressly/goose/v3"
 "github.com/riverqueue/river/riverdriver/riverpgxv5"
 "github.com/riverqueue/river/rivermigrate"
)
func main(){if err:=run();err!=nil{slog.Error("migration failed","type",fmtType(err));os.Exit(1)}}
func fmtType(err error)string{if err==nil{return "nil"};return "database/configuration error"}
func run()error{
 url:=os.Getenv("V2_DATABASE_URL");if url==""{return errors.New("V2_DATABASE_URL required")}
 ctx,cancel:=context.WithTimeout(context.Background(),2*time.Minute);defer cancel()
 db,err:=sql.Open("pgx",url);if err!=nil{return err};defer db.Close()
 if err:=goose.SetDialect("postgres");err!=nil{return err}
 dir:=os.Getenv("V2_MIGRATIONS_DIR");if dir==""{dir="migrations"}
 if err:=goose.UpContext(ctx,db,dir);err!=nil{return err}
 pool,err:=pgxpool.New(ctx,url);if err!=nil{return err};defer pool.Close()
 migrator,err:=rivermigrate.New(riverpgxv5.New(pool),nil);if err!=nil{return err}
 _,err=migrator.Migrate(ctx,rivermigrate.DirectionUp,nil);return err
}
