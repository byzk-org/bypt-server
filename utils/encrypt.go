package utils

import (
	cryptoRand "crypto/rand"
	"encoding/base64"
	"errors"
	"github.com/tjfoc/gmsm/sm2"
	"github.com/tjfoc/gmsm/sm4"
	"math/rand"
	"time"
)

// Sm4Encrypt sm4加密
func Sm4Encrypt(key, plainText []byte) ([]byte, error) {
	return sm4.Sm4Ecb(key, plainText, true)
}

// Sm4Decrypt sm4解密
func Sm4Decrypt(key, cipherText []byte) ([]byte, error) {
	ecb, err := sm4.Sm4Ecb(key, cipherText, false)
	if err != nil {
		return nil, err
	}
	if len(ecb) == 0 {
		return nil, errors.New("数据解密失败")
	}
	return ecb, nil
}

// Sm4RandomKey Sm4随机ke
func Sm4RandomKey() []byte {
	return []byte(GetRandomString(16))[:16]
}

// Sm2Encrypt Sm2加密
func Sm2Encrypt(pubKey *sm2.PublicKey, data []byte) ([]byte, error) {
	encrypt, err := sm2.Encrypt(pubKey, data, cryptoRand.Reader)
	if err != nil {
		return nil, errors.New("加密数据失败")
	}
	return encrypt, err
}

// Sm2Decrypt sn2解密
func Sm2Decrypt(pri *sm2.PrivateKey, data []byte) ([]byte, error) {
	decrypt, err := sm2.Decrypt(pri, data)
	if err != nil {
		return nil, errors.New("解密数据失败")
	}
	return decrypt, nil
}

// Sm2DecryptByBase64Data sm2解密，数据格式为Base64
func Sm2DecryptByBase64Data(pri *sm2.PrivateKey, data string) ([]byte, error) {
	d, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, errors.New("解析数据失败")
	}
	return Sm2Decrypt(pri, d)
}

// GetRandomString 获取指定长度的随机字符串
func GetRandomString(l int) string {
	str := "0123456789abcdefghijklmnopqrstuvwxyz~！@#￥%……&*（）——+」|「P:>?/*-+.+*_*+我爱中国^_^"
	//str := "0123456789abcdefghijklmnopqrstuvwxyz"
	bytes := []rune(str)
	result := make([]rune, l, l)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < l; i++ {
		result[i] = bytes[r.Intn(len(bytes))]
		//result = append(result, bytes[r.Intn(len(bytes))])
	}
	return string(result)
}
