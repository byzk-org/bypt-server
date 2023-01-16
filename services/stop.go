package services

import (
	"errors"
	"fmt"
	"github.com/byzk-org/bypt-server/helper"
	"github.com/byzk-org/bypt-server/vos"
	"gopkg.in/yaml.v2"
	"os"
)

var stopAppService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	msg, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}
	return helper.AppStatusMgr.StopApp(msg.String())
}

var stopYamlConfigService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	GlobalOperationLock.Lock()
	defer GlobalOperationLock.Unlock()

	msg, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	file, err := os.OpenFile(msg.String(), os.O_RDONLY, 0666)
	if err != nil {
		return errors.New("打开配置文件失败")
	}
	defer file.Close()

	stopInfoMap := make(map[string]*vos.DbAppStartInfo)
	decoder := yaml.NewDecoder(file)
	if err = decoder.Decode(&stopInfoMap); err != nil {
		return errors.New("解析配置文件失败 => " + err.Error())
	}
	for k, v := range stopInfoMap {
		if err = helper.AppStatusMgr.StopApp(k); err != nil {
			socketOperation.SendMsg([]byte(fmt.Sprintf("error:[%s-%s]停止失败: %s", k, v.Version, err.Error())))
			continue
		}
		socketOperation.SendMsg([]byte(fmt.Sprintf("[%s-%s]停止成功", k, v.Version)))
	}
	socketOperation.SendMsg([]byte("!!!!!!"))
	return nil

}
