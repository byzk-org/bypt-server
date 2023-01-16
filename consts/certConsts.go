package consts

import (
	_ "embed"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/tjfoc/gmsm/sm2"
	"github.com/tjfoc/gmsm/x509"
	"os"
)

var (
	//go:embed certs/ca.cert
	CaCert []byte
	//go:embed certs/ca.key
	CaKey []byte
	//go:embed certs/sign.cert
	SignCert []byte
	//go:embed certs/sign.key
	SignKey []byte
	//go:embed certs/encrypt.cert
	EncryptCert []byte
	//go:embed certs/encrypt.key
	EncryptKey []byte
	//go:embed certs/sync.cert
	SyncCert []byte
	//go:embed certs/sync.key
	SyncKey []byte
)

var (
	CaPubKey     *sm2.PublicKey
	CaPrivateKey *sm2.PrivateKey
)

func parsePubKeyByCertPem(certPem string) (*sm2.PublicKey, error) {
	decode, _ := pem.Decode([]byte(certPem))
	if decode == nil {
		return nil, errors.New("解析证书失败")
	}

	certificate, err := x509.ParseCertificate(decode.Bytes)
	if err != nil {
		return nil, errors.New("转换证书失败")
	}

	key, err := x509.MarshalPKIXPublicKey(certificate.PublicKey)
	if err != nil {
		return nil, errors.New("转换公钥失败")
	}

	pubKey, err := x509.ParseSm2PublicKey(key)
	if err != nil {
		return nil, err
	}
	return pubKey, nil
}

func init() {
	var err error
	CaPubKey, err = parsePubKeyByCertPem(string(CaCert))
	if err != nil {
		fmt.Println("解析运行密钥对失败")
		os.Exit(1)
	}

	block, _ := pem.Decode(CaKey)
	if block == nil {
		fmt.Println("解析运行密钥对失败")
		os.Exit(1)
	}

	CaPrivateKey, err = x509.ParsePKCS8UnecryptedPrivateKey(block.Bytes)
	if err != nil {
		fmt.Println("解析运行密钥对失败")
		os.Exit(1)
	}


}
