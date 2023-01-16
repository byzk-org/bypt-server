package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/byzk-org/bypt-server/helper"
	"github.com/byzk-org/bypt-server/vos"
	"gopkg.in/yaml.v2"
	"os"
)

var startService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	GlobalOperationLock.Lock()
	defer GlobalOperationLock.Unlock()

	msg, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	startInfo := &vos.DbAppStartInfo{}

	if err = json.Unmarshal(msg, startInfo); err != nil {
		return errors.New("转换启动参数失败")
	}

	return helper.AppStatusMgr.StartApp(startInfo)
}

var startYamlConfigService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	GlobalOperationLock.Lock()
	defer GlobalOperationLock.Unlock()

	msg, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	file, err := os.OpenFile(msg.String(), os.O_RDONLY, 0666)
	if err != nil {
		return errors.New("打开启动配置文件失败")
	}
	defer file.Close()

	startInfoMap := make(map[string]*vos.DbAppStartInfo)
	decoder := yaml.NewDecoder(file)
	if err = decoder.Decode(&startInfoMap); err != nil {
		return errors.New("解析启动配置文件失败 => " + err.Error())
	}

	if len(startInfoMap) == 0 {
		return errors.New("未从配置文件中解析出启动信息")
	}

	for k, v := range startInfoMap {
		v.Name = k
		if err = helper.AppStatusMgr.StartApp(v); err != nil {
			socketOperation.SendMsg([]byte(fmt.Sprintf("error:[%s-%s]启动失败: %s", k, v.Version, err.Error())))
			continue
		}
		socketOperation.SendMsg([]byte(fmt.Sprintf("[%s-%s]启动成功", k, v.Version)))
	}
	socketOperation.SendMsg([]byte("!!!!!!"))
	return nil
}
