package telegram

import (
 "bytes"
 "encoding/base64"
 "image/png"
 "strings"
 "testing"

 "github.com/gotd/td/telegram/auth/qrlogin"
 "rsc.io/qr"
)

func TestLoginQRScaledPNG(t *testing.T){
 // Fake token; never use a real login credential in fixtures or failure output.
 token:=qrlogin.NewToken(bytes.Repeat([]byte{0x42},32),0)
 data,err:=loginQR(token)
 if err!=nil{t.Fatal(err)}
 const prefix="data:image/png;base64,"
 if !strings.HasPrefix(data,prefix){t.Fatal("expected PNG data URL")}
 raw,err:=base64.StdEncoding.DecodeString(strings.TrimPrefix(data,prefix))
 if err!=nil{t.Fatal(err)}
 img,err:=png.Decode(bytes.NewReader(raw))
 if err!=nil{t.Fatal(err)}
 code,err:=qr.Encode(token.URL(),qr.M)
 if err!=nil{t.Fatal(err)}
 const scale=8
 side:=(code.Size+8)*scale
 if img.Bounds().Dx()!=side||img.Bounds().Dy()!=side{t.Fatal("incorrect QR dimensions")}
 // Validate every pixel against the unscaled matrix, including four white modules on all sides.
 for y:=0;y<side;y++{for x:=0;x<side;x++{
  r,g,b,a:=img.At(x,y).RGBA()
  want:=uint32(65535)
  if code.Black(x/scale-4,y/scale-4){want=0}
  if r!=want||g!=want||b!=want||a!=65535{t.Fatalf("incorrect QR pixel at %d,%d",x,y)}
 }}
}
