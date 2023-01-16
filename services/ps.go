package services

import (
	"github.com/byzk-org/bypt-server/helper"
)

var psListService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	list := helper.AppStatusMgr.StartAppList()
	socketOperation.SendMsg(list)
	return nil
}

var psAppNameService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	msg, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}
	info, err := helper.AppStatusMgr.QueryStartAppInfo(msg.String())
	if err != nil {
		return err
	}
	socketOperation.SendMsg(info)
	return nil
}

var psAppNamePluginService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	appName, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	pluginName, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	info, err := helper.AppStatusMgr.QueryStartAppPluginInfo(appName.String(), pluginName.String())
	if err != nil {
		return err
	}

	socketOperation.SendMsg(info)
	return nil
}
