package services

import (
	"errors"
	"fmt"
	"github.com/byzk-org/bypt-server/helper"
	"github.com/byzk-org/bypt-server/vos"
	"gopkg.in/yaml.v2"
	"os"
)

var restartService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	msg, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	return helper.AppStatusMgr.RestartApp(msg.String())
}

var restartWithConfigService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	GlobalOperationLock.Lock()
	defer GlobalOperationLock.Unlock()

	msg, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	file, err := os.OpenFile(msg.String(), os.O_RDONLY, 0666)
	if err != nil {
		return errors.New("打开重启配置文件失败")
	}
	defer file.Close()

	restartInfoMap := make(map[string]*vos.DbAppStartInfo)
	decoder := yaml.NewDecoder(file)
	if err = decoder.Decode(&restartInfoMap); err != nil {
		return errors.New("解析重启配置文件失败 => " + err.Error())
	}

	if len(restartInfoMap) == 0 {
		return errors.New("未从配置文件中解析出重启信息")
	}

	for k, v := range restartInfoMap {
		v.Name = k
		if err = helper.AppStatusMgr.RestartAppWithStartInfo(v); err != nil {
			socketOperation.SendMsg([]byte(fmt.Sprintf("error:[%s-%s]重启失败: %s", k, v.Version, err.Error())))
			continue
		}
		socketOperation.SendMsg([]byte(fmt.Sprintf("[%s-%s]重启成功", k, v.Version)))
	}
	socketOperation.SendMsg([]byte("!!!!!!"))
	return nil

}
