package socket

import (
	"bytes"
	"errors"
	"net"
)

var (
	tmpMsg   []byte
	splitMsg = []byte("&&")
)

func readData(conn net.Conn, msgChannel chan<- []byte) (returnErr error) {
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

		tmpByte := make([]byte, read)
		copy(tmpByte, buffer[:read])
		if tmpMsg != nil {
			tmpByte = bytes.Join([][]byte{
				tmpMsg,
				tmpByte,
			}, nil)
			tmpMsg = nil
		}

		if !bytes.Contains(tmpByte, splitMsg) {
			if tmpMsg != nil {
				tmpMsg = make([]byte, 0)
			}
			tmpMsg = append(tmpMsg, tmpByte...)
			continue
		}
		splitByte := bytes.Split(tmpByte, splitMsg)
		msgChannel <- splitByte[0]
		if len(splitByte) > 2 {
			for i := 1; i < len(splitByte)-1; i++ {
				msgChannel <- splitByte[i]
			}
		}

		tmpOtherMsg := splitByte[len(splitByte)-1]
		if len(tmpOtherMsg) == 0 {
			tmpMsg = nil
		} else {
			if tmpMsg == nil {
				tmpMsg = make([]byte, 0)
			}
			tmpMsg = append(tmpMsg, tmpOtherMsg...)
		}

	}
}
