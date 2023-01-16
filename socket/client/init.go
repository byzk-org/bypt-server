package socket

import (
	"bytes"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"github.com/byzk-org/bypt-server/consts"
	"github.com/byzk-org/bypt-server/db"
	"github.com/byzk-org/bypt-server/vos"
	"github.com/tjfoc/gmsm/gmtls"
	"github.com/tjfoc/gmsm/x509"
	"time"
)

var endMsg = []byte("end!!&&")

type Conn struct {
	*gmtls.Conn
	msgChannel chan []byte
}

func (c *Conn) WriteDataStr(data string) error {
	return c.WriteData([]byte(data))
}

func (c *Conn) WriteData(data []byte) error {
	_, err := c.Write(bytes.Join([][]byte{
		[]byte(hex.EncodeToString(data)),
		splitMsg,
	}, nil))
	if err != nil {
		return errors.New("发送数据失败")
	}
	return nil
}

func (c *Conn) ReadMsg() ([]byte, error) {
	msgByte, isOpen := <-c.msgChannel
	if !isOpen || len(msgByte) == 0 {
		return nil, errors.New("未知的处理异常")
	}

	msgStr, err := hex.DecodeString(string(msgByte))
	if err != nil {
		return nil, errors.New("解析消息结构错误")
	}
	if string(msgStr) == "error" {
		msgByte, isOpen = <-c.msgChannel
		if !isOpen || len(msgByte) == 0 {
			return nil, errors.New("未知的处理异常")
		}
		msgStr, err = hex.DecodeString(string(msgByte))
		if err != nil {
			return nil, errors.New("解析消息结构错误")
		}
		return nil, errors.New(string(msgStr))
	}

	msgByte, isOpen = <-c.msgChannel
	if !isOpen || len(msgByte) == 0 {
		return nil, errors.New("未知的处理异常")
	}

	msgStr, err = hex.DecodeString(string(msgByte))
	if err != nil {
		return nil, errors.New("解析消息结构错误")
	}
	return msgStr, nil
}

func (c *Conn) ReadMsgStr() (string, error) {
	msg, err := c.ReadMsg()
	return string(msg), err
}

func (c *Conn) Wait() error {
	_, err := c.ReadMsgStr()
	return err
}

func (c *Conn) SendEndMsg() {
	_, _ = c.Write(endMsg)
}

func GetClientConn() (*Conn, error) {

	if !ValidCertExpire(consts.SyncCert) {
		return nil, errors.New("客户端已过期")
	}

	userCert, err := gmtls.GMX509KeyPairsSingle(consts.SyncCert, consts.SyncKey)
	if err != nil {
		return nil, errors.New("解析同步证书失败")
	}

	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(consts.CaCert)

	syncServerSetting := vos.DbSetting{}
	if err = db.GetDb().Where(&vos.DbSetting{
		Name: consts.DbSettingSyncServer,
	}).Find(&syncServerSetting).Error; err != nil || syncServerSetting.Val == "" {
		return nil, errors.New("请先使用 [config] 命令配置同步服务地址")
	}

	//localUrl := fmt.Sprintf("127.0.0.1:%d", consts.ServerPort)
	//if localUrl == syncServerSetting.Val {
	//	return nil, errors.New("远程服务地址不能和本机一致")
	//}

	conn, err := gmtls.Dial("tcp", syncServerSetting.Val, &gmtls.Config{
		GMSupport:    &gmtls.GMSupport{},
		ServerName:   "localhost",
		Certificates: []gmtls.Certificate{userCert},
		RootCAs:      certPool,
		ClientAuth:   gmtls.RequireAndVerifyClientCert,
	})
	if err != nil {
		return nil, errors.New("连接同步服务失败! ")
	}

	msgChannel := make(chan []byte)
	go func() {
		if err := readData(conn, msgChannel); err != nil {
			close(msgChannel)
		}
	}()

	return &Conn{
		Conn:       conn,
		msgChannel: msgChannel,
	}, nil
}

func ValidCertExpire(certPemBytes []byte) bool {
	block, _ := pem.Decode(certPemBytes)
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}

	return time.Now().Before(certificate.NotAfter)
}
