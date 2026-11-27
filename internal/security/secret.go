package security

import (
 "crypto/aes"
 "crypto/cipher"
 "crypto/rand"
 "encoding/hex"
 "errors"
 "io"
)

type Box struct{ aead cipher.AEAD }

func NewBox(keyHex string)(*Box,error){
 key,err:=hex.DecodeString(keyHex)
 if err!=nil||len(key)!=32{return nil,errors.New("V2 encryption key must be 32 random bytes encoded as hex")}
 defer clear(key)
 block,err:=aes.NewCipher(key);if err!=nil{return nil,err}
 aead,err:=cipher.NewGCM(block);if err!=nil{return nil,err}
 return &Box{aead:aead},nil
}

// Binding the ciphertext to its purpose and record prevents cross-record substitution.
func(b *Box) Seal(purpose string,plain []byte)([]byte,error){
 nonce:=make([]byte,b.aead.NonceSize())
 if _,err:=io.ReadFull(rand.Reader,nonce);err!=nil{return nil,err}
 return b.aead.Seal(nonce,nonce,plain,[]byte(purpose)),nil
}
func(b *Box) Open(purpose string,data []byte)([]byte,error){
 n:=b.aead.NonceSize()
 if len(data)<n+b.aead.Overhead(){return nil,errors.New("invalid encrypted record")}
 return b.aead.Open(nil,data[:n],data[n:],[]byte(purpose))
}
