package utils

import (
	"errors"
	"github.com/byzk-org/bypt-server/consts"
	"github.com/tjfoc/gmsm/sm2"
	"io"
	"os"
)

// Sm4Decrypt2File Sm4解密到文件
func Sm4Decrypt2File(key []byte, reader io.Reader, destFilePath string) error {
	var (
		err      error
		destFile *os.File
		readSize int
		decrypt  []byte
	)
	bufferData := make([]byte, consts.Sm4EncDataLen)
	_ = os.RemoveAll(destFilePath)
	destFile, err = os.Create(destFilePath)
	if err != nil {
		return errors.New("创建目标文件失败")
	}
	defer destFile.Close()

	for {
		readSize, err = reader.Read(bufferData)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errors.New("读取文件失败")
		}

		decrypt, err = Sm4Decrypt(key, bufferData[:readSize])
		if err != nil || len(decrypt) == 0 {
			return errors.New("解密数据失败")
		}

		_, err = destFile.Write(decrypt)
		if err != nil {
			return errors.New("解密数据块失败")
		}

		if readSize != consts.Sm4EncDataLen {
			return nil
		}

	}
}

// Sm4Encrypt2File 加密sm4到文件
func Sm4Encrypt2File(key []byte, srcFilePath, destFilePath string) error {
	var (
		err      error
		srcFile  *os.File
		destFile *os.File
		readSize = consts.Sm4EncDataLen - 16
		s        int
		encrypt  []byte
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

	tmpBuffer := make([]byte, readSize)
	for {
		s, err = srcFile.Read(tmpBuffer)
		if err != nil && err == io.EOF {
			return nil
		}

		if err != nil {
			return errors.New("读取文件内容失败")
		}

		encryptContent := tmpBuffer[:s]
		encrypt, err = Sm4Encrypt(key, encryptContent)
		if err != nil {
			return errors.New("加密原文件内容失败")
		}

		_, err = destFile.Write(encrypt)
		if err != nil {
			return errors.New("写出文件失败")
		}

	}

}

func Sm4EncryptReader2File(key []byte, srcFile io.Reader, destFile *os.File) error {
	var (
		err      error
		readSize = consts.Sm4EncDataLen - 16
		s        int
		encrypt  []byte
	)
	defer destFile.Close()

	tmpBuffer := make([]byte, readSize)
	for {
		s, err = srcFile.Read(tmpBuffer)
		if err != nil && err == io.EOF {
			return nil
		}

		if err != nil {
			return errors.New("读取文件内容失败")
		}

		encryptContent := tmpBuffer[:s]
		encrypt, err = Sm4Encrypt(key, encryptContent)
		if err != nil {
			return errors.New("加密原文件内容失败")
		}

		_, err = destFile.Write(encrypt)
		if err != nil {
			return errors.New("写出文件失败")
		}

	}
}

func Sm4DecryptContentPath(priKey *sm2.PrivateKey, content []byte) (string, []byte, error) {
	key := content[:113]
	decryptKey, err := Sm2Decrypt(priKey, key)
	if err != nil || len(decryptKey) == 0 {
		return "", nil, errors.New("解密保护密钥失败")
	}

	decrypt, err := Sm4Decrypt(decryptKey, content[113:])
	if err != nil {
		return "", nil, err
	}
	return string(decrypt), decryptKey, nil
}
