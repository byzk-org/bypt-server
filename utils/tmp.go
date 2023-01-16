package utils

import (
	"errors"
	"io/ioutil"
)

func TmpDir() (string, error) {
	dir, err := ioutil.TempDir("", "bypt*")
	if err != nil {
		return "", errors.New("创建临时目录失败")
	}
	return dir, nil
}
