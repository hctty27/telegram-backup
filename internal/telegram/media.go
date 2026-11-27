package telegram

import (
 "context"
 "errors"
 "fmt"
 "io"
 "strings"
 "time"

 gotd "github.com/gotd/td/telegram"
 "github.com/gotd/td/telegram/query"
 "github.com/gotd/td/telegram/query/dialogs"
 "github.com/gotd/td/telegram/query/messages"
 "github.com/gotd/td/tg"
)

type Media struct{
 MessageID int
 Date time.Time
 Key,Name,MIME string
 Size int64
}
type Chat struct{ID,Hash int64;Kind,Title string}
func(s *Service)ImportChats(ctx context.Context,account int64)(int,error){
 count:=0
 err:=s.Authorized(ctx,account,func(ctx context.Context,c *gotd.Client)error{
  return query.GetDialogs(c.API()).ForEach(ctx,func(ctx context.Context,e dialogs.Elem)error{
   if e.Deleted(){return nil}
   var ch Chat
   switch p:=e.Peer.(type){
   case *tg.InputPeerChat:ch=Chat{ID:p.ChatID,Kind:"chat"};if v:=e.Entities.Chats()[p.ChatID];v!=nil{ch.Title=v.Title}
   case *tg.InputPeerChannel:ch=Chat{ID:p.ChannelID,Hash:p.AccessHash,Kind:"channel"};if v:=e.Entities.Channels()[p.ChannelID];v!=nil{ch.Title=v.Title}
   case *tg.InputPeerUser:ch=Chat{ID:p.UserID,Hash:p.AccessHash,Kind:"user"};if v:=e.Entities.Users()[p.UserID];v!=nil{ch.Title=strings.TrimSpace(v.FirstName+" "+v.LastName)}
   default:return nil
   }
   if ch.Title==""{ch.Title=fmt.Sprintf("%s %d",ch.Kind,ch.ID)}
   _,err:=s.Pool.Exec(ctx,`INSERT INTO telegram_chats(account_id,peer_kind,peer_id,access_hash,title) VALUES($1,$2,$3,$4,$5)
   ON CONFLICT(account_id,peer_kind,peer_id) DO UPDATE SET title=EXCLUDED.title,access_hash=EXCLUDED.access_hash`,account,ch.Kind,ch.ID,ch.Hash,ch.Title)
   if err==nil{count++};return err
  })
 })
 return count,err
}

func extract(e messages.Elem)(Media,bool){
 msg,ok:=e.Msg.(*tg.Message);if !ok{return Media{},false}
 file,ok:=e.File();if !ok{return Media{},false}
 m:=Media{MessageID:msg.ID,Date:time.Unix(int64(msg.Date),0).UTC(),Name:file.Name,MIME:file.MIMEType}
 switch loc:=file.Location.(type){
 case *tg.InputDocumentFileLocation:
  doc,ok:=e.Document();if !ok{return Media{},false}
  m.Key=fmt.Sprintf("document:%d",loc.ID);m.Size=doc.Size
 case *tg.InputPhotoFileLocation:
  photo,ok:=e.Photo();if !ok{return Media{},false}
  m.Key=fmt.Sprintf("photo:%d:%s",loc.ID,loc.ThumbSize)
  for _,raw:=range photo.Sizes{
   switch size:=raw.(type){
   case *tg.PhotoSize:if size.Type==loc.ThumbSize{m.Size=int64(size.Size)}
   case *tg.PhotoSizeProgressive:if size.Type==loc.ThumbSize{for _,n:=range size.Sizes{if int64(n)>m.Size{m.Size=int64(n)}}}
   case *tg.PhotoCachedSize:if size.Type==loc.ThumbSize{m.Size=int64(len(size.Bytes))}
   }
  }
 default:return Media{},false
 }
 if m.Name==""{m.Name=fmt.Sprintf("message-%d.bin",msg.ID)}
 return m,true
}

type Page struct{Media []Media;First,Last int;Count int;Complete bool}
var errPage=errors.New("history page limit")

// ReadPage limits each scan to 100 messages; cursor advances only in the
// caller's durable transaction after every media record has been inserted.
func(s *Service)ReadPage(ctx context.Context,account int64,peer tg.InputPeerClass,minID,before int)(Page,error){
 var p Page
 err:=s.Authorized(ctx,account,func(ctx context.Context,c *gotd.Client)error{
  iter:=query.Messages(c.API()).GetHistory(peer).MinID(minID).OffsetID(before).BatchSize(100).Iter()
  for iter.Next(ctx){
   e:=iter.Value()
   id:=e.Msg.GetID()
   if id<=minID|| (before>0&&id>=before){return errors.New("unexpected history ordering")}
   if p.Count==0{p.First=id}else if id>=p.Last{return errors.New("non-descending history")}
   p.Last=id;p.Count++
   if m,ok:=extract(e);ok{p.Media=append(p.Media,m)}
   if p.Count>=100{return errPage}
  }
  if err:=iter.Err();err!=nil{return err}
  p.Complete=true;return nil
 })
 if errors.Is(err,errPage){err=nil}
 return p,err
}

// Fetch resolves a fresh Telegram file reference for every attempt, rather
// than persisting an expiring reference. Verify the media identity and size.
func(s *Service)Fetch(ctx context.Context,account int64,peer tg.InputPeerClass,msgID int,key string,expected int64,w io.Writer)error{
 return s.Authorized(ctx,account,func(ctx context.Context,c *gotd.Client)error{
  list,err:=query.Messages(c.API()).GetMessagesWithEntities(ctx,peer,msgID);if err!=nil{return err}
  for _,e:=range list{
   m,ok:=extract(e);if !ok||m.MessageID!=msgID||m.Key!=key||m.Size!=expected{continue}
   file,_:=e.File()
   _,err=c.Download(file.Location).WithSize(expected).Stream(ctx,w);return err
  }
  return errors.New("media missing or changed since discovery")
 })
}
