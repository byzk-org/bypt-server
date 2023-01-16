package utils

import (
	"encoding/json"
	"errors"
	"os"
)

// JsonEncoder json编码
func JsonEncoder(srcInterface interface{}, desc string) error {
	_ = os.RemoveAll(desc)
	destFile, err := os.Create(desc)
	if err != nil {
		return errors.New("创建写出文件失败")
	}
	defer destFile.Close()

	encoder := json.NewEncoder(destFile)
	err = encoder.Encode(srcInterface)
	if err != nil {
		return errors.New("结构体转换json失败")
	}
	return nil
}

// JsonDecoder json解码
func JsonDecoder(srcPath string, dest interface{}) error {
	file, err := os.OpenFile(srcPath, os.O_RDONLY, 0666)
	if err != nil {
		return errors.New("打开要转换的文件失败")
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	if err = decoder.Decode(dest); err != nil {
		return errors.New("json转换结构体失败")
	}
	return err
}
