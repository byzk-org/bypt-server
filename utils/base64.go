package utils

import (
	"encoding/base64"
	"errors"
	"io"
	"os"
)

func Base64Decoder2File(srcFilePath, destFilePath string) error {
	var (
		err      error
		srcFile  *os.File
		destFile *os.File
		readSize int
	)
	srcFile, err = os.OpenFile(srcFilePath, os.O_RDONLY, 0666)
	if err != nil {
		return errors.New("打开原文件失败")
	}
	defer srcFile.Close()

	destFile, err = os.Create(destFilePath)
	if err != nil {
		return errors.New("创建目标文件失败")
	}
	defer destFile.Close()

	tmpBuf := make([]byte, 1024, 1024)
	decoder := base64.NewDecoder(base64.StdEncoding, srcFile)

	for {
		readSize, err = decoder.Read(tmpBuf)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errors.New("读取原文件内容失败")
		}
		_, err = destFile.Write(tmpBuf[:readSize])
		if err != nil {
			return errors.New("写出内容失败")
		}
	}

}
