package utils

import (
	"crypto/rand"
	"errors"
	"github.com/tjfoc/gmsm/sm2"
)

func PrivateKeySign(pri *sm2.PrivateKey, signSrc []byte) ([]byte, error) {
	//block, _ := pem.Decode([]byte(priKeyPem))
	//if block == nil {
	//	return nil, errors.New("解析密钥失败")
	//}
	//pri, err := x509.ParsePKCS8UnecryptedPrivateKey(block.Bytes)
	//if err != nil {
	//	return nil, errors.New("解析密钥失败")
	//}

	signData, err := pri.Sign(rand.Reader, signSrc, nil)
	if err != nil {
		return nil, errors.New("制作数据签名失败")
	}
	return signData, nil
}

func PubKeyVerifySign(pubKey *sm2.PublicKey, signSrc, signData []byte) bool {

	//block, _ := pem.Decode([]byte(pubKeyPem))
	//if block == nil {
	//	return false
	//}
	//
	//key, err := x509.ParseSm2PublicKey(block.Bytes)
	//if err != nil {
	//	return false
	//}

	return pubKey.Verify(signSrc, signData)
}

