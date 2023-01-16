package socket

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/byzk-org/bypt-server/consts"
	"github.com/byzk-org/bypt-server/helper"
	"github.com/byzk-org/bypt-server/services"
	"github.com/sirupsen/logrus"
	"github.com/tjfoc/gmsm/gmtls"
	"github.com/tjfoc/gmsm/x509"
	"net"
	"os"
)

var (
	splitMsg     = []byte("&&")
	endMsg       = []byte("end!!")
	okMsgPrefix  = hex.EncodeToString([]byte("ok"))
	errMsgPrefix = hex.EncodeToString([]byte("error"))
)

func ServerRun() {
	signCert, err := gmtls.GMX509KeyPairsSingle(consts.SignCert, consts.SignKey)
	if err != nil {
		logrus.Error("创建签名证书失败 => " + err.Error())
		os.Exit(1)
	}

	encryptCert, err := gmtls.GMX509KeyPairsSingle(consts.EncryptCert, consts.EncryptKey)
	if err != nil {
		logrus.Error("创建加密证书失败 => " + err.Error())
		os.Exit(1)
	}

	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(consts.CaCert)

	listener, err := gmtls.Listen("tcp", fmt.Sprintf(":%d", consts.ServerPort), &gmtls.Config{
		GMSupport:    &gmtls.GMSupport{},
		ClientAuth:   gmtls.RequireAndVerifyClientCert,
		Certificates: []gmtls.Certificate{signCert, encryptCert},
		ClientCAs:    certPool,
	})
	if err != nil {
		logrus.Error("启动服务失败 => " + err.Error())
		os.Exit(2)
	}

	fmt.Printf("server start ok, listener:%d\n", consts.ServerPort)
	go func() {
		defer func() { recover() }()

		helper.AppStatusMgr.StartAppByPrevConfig()

	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}

		msgChannel := make(chan []byte, 10)
		outChannel := make(chan []byte, 10)

		go func(connection net.Conn, msgChan, ouChan chan []byte) {
			defer func() {
				//fmt.Println("关闭读取通道以及链接")
				closeChannel(msgChan)
				closeChannel(outChannel)
				_ = conn.Close()
			}()
			_ = readData(conn, msgChannel)
		}(conn, msgChannel, outChannel)

		go func(connection net.Conn, outChan chan []byte) {
			defer func() {
				//fmt.Println("关闭写出通道")
				closeChannel(outChan)
			}()
			_ = writeData(connection, outChan)
			//closeChannel(outChannel)

		}(conn, outChannel)

		readMsg := getReadMsg(msgChannel)
		sendMsg := getOutMsg(outChannel)

		cmdByte, err := readMsg()
		if err != nil {
			continue
		}

		fn, ok := services.ServiceMap[cmdByte.String()]
		if !ok {
			sendErrMsg(outChannel, "未知的命令")
			continue
		}

		sendMsg([]byte("ok"))

		if err = execFn(fn, readMsg, sendMsg); err != nil {
			sendErrMsg(outChannel, err.Error())
		} else {
			sendMsg([]byte("ok"))
		}
	}
}

func execFn(fn services.ServiceInterfaceFn, readMsg services.ReadMsg, sendMsg services.SendSuccessMsg) (returnErr error) {
	defer func() {
		err := recover()
		if err != nil {
			switch e := err.(type) {
			case error:
				returnErr = errors.New(e.Error())
			case string:
				returnErr = errors.New(e)
			default:
				returnErr = errors.New("未知的业务异常")
			}
		}
	}()
	return fn(&services.SocketOperation{
		ReadMsg: readMsg, SendMsg: sendMsg,
	})
}

func getReadMsg(msgChannel chan []byte) services.ReadMsg {
	return func() (services.SliceBytes, error) {
		defer func() { recover() }()
		msg := <-msgChannel
		if msg == nil {
			return nil, errors.New("读取消息失败")
		}
		decodeString, err := hex.DecodeString(string(msg))
		if err != nil {
			return nil, errors.New("解析消息错误")
		}
		return decodeString, nil
	}
}

func getOutMsg(outChannel chan []byte) services.SendSuccessMsg {
	return func(content []byte) {
		defer func() { recover() }()
		outChannel <- []byte(okMsgPrefix + "&&" + hex.EncodeToString(content) + "&&")
	}
}

func sendErrMsg(outChannel chan []byte, errMsg string) {
	defer func() { recover() }()
	outChannel <- []byte(errMsgPrefix + "&&" + hex.EncodeToString([]byte(errMsg)) + "&&")
}

func closeChannel(channel chan []byte) {
	defer func() { recover() }()
	close(channel)
}

func readData(conn net.Conn, msgChannel chan []byte) (returnErr error) {
	var tmpMsg = &bytes.Buffer{}
	defer func() {
		e := recover()
		if e != nil {
			returnErr = errors.New("读取数据出现异常")
		}
	}()
	buffer := make([]byte, 1024*1024)
	for {
		read, err := conn.Read(buffer)
		if err != nil {
			return err
		}

		tmpMsg.Write(buffer[:read])

		c := tmpMsg.Bytes()
		allMsg := make([]byte, len(c))
		copy(allMsg, c)
		if !bytes.Contains(allMsg, splitMsg) {
			continue
		}
		tmpMsg.Reset()

		splitByte := bytes.Split(allMsg, splitMsg)
		if bytes.Equal(splitByte[0], endMsg) {
			//fmt.Println("读取到结束消息")
			return errors.New("消息被关闭")
		}

		msgChannel <- splitByte[0]
		if len(splitByte) > 2 {
			for i := 1; i < len(splitByte)-1; i++ {
				if bytes.Equal(splitByte[i], endMsg) {
					return errors.New("消息被关闭")
				}

				msgChannel <- splitByte[i]
			}
		}

		tmpOtherMsg := splitByte[len(splitByte)-1]
		if len(tmpOtherMsg) > 0 {
			tmpMsg.Write(tmpOtherMsg)
		}
	}
}

func writeData(conn net.Conn, outChannel chan []byte) (returnErr error) {
	defer func() {
		e := recover()
		if e != nil {
			returnErr = errors.New("写出数据错误")
		}
	}()

	for {
		outMsg, isOpen := <-outChannel
		if !isOpen {
			return errors.New("写出数据通道已关闭")
		}

		_, err := conn.Write(outMsg)
		if err != nil {
			return err
		}
	}
}
