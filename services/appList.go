package services

import (
	"crypto/md5"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"github.com/byzk-org/bypt-server/db"
	"github.com/byzk-org/bypt-server/vos"
	"github.com/jinzhu/gorm"
)

var appList ServiceInterfaceFn = func(socketOperation *SocketOperation) error {

	appInfoModel := db.GetDb().Model(&vos.DbAppInfo{})
	appVersionModel := db.GetDb().Model(&vos.DbAppVersionInfo{})

	appInfoList := make([]vos.DbAppInfo, 0)
	if err := appInfoModel.Order("create_time asc").Find(&appInfoList).Error; err != nil {
		return errors.New("查询应用列表失败")
	}

	if len(appInfoList) == 0 {
		marshal, _ := json.Marshal(appInfoList)
		socketOperation.SendMsg(marshal)
		return nil
	}

	appVersionOrder := appVersionModel.Order("create_time asc")
	for i, appInfo := range appInfoList {
		appVersionList := make([]*vos.DbAppVersionInfo, 0)
		if err := appVersionOrder.Where(&vos.DbAppVersionInfo{
			AppName: appInfo.Name,
		}).Find(&appVersionList).Error; err != nil && err != gorm.ErrRecordNotFound {
			return errors.New("查询版本列表失败")
		}
		appInfoList[i].Versions = appVersionList
	}

	marshal, _ := json.Marshal(appInfoList)
	socketOperation.SendMsg(marshal)
	return nil
}

var appListByAppName ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	name, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	appInfo := &vos.DbAppInfo{}
	if err = db.GetDb().Model(&vos.DbAppInfo{}).Where(&vos.DbAppInfo{
		Name: name.String(),
	}).First(&appInfo).Error; err != nil || appInfo.Name == "" {
		return errors.New("未查询到相关的app信息")
	}

	currentVersion := &vos.DbAppVersionInfo{}
	if err = db.GetDb().Model(&vos.DbAppVersionInfo{}).Where(&vos.DbAppVersionInfo{
		AppName: appInfo.Name,
		Name:    appInfo.CurrentVersion,
	}).First(&currentVersion).Error; err != nil || currentVersion.Name == "" {
		return errors.New("查询应用当前版本失败")
	}
	appInfo.CurrentVersionInfo = currentVersion

	appVersions := make([]*vos.DbAppVersionInfo, 0)
	if err = db.GetDb().Model(&vos.DbAppVersionInfo{}).Where(&vos.DbAppVersionInfo{
		AppName: appInfo.Name,
	}).Find(&appVersions).Error; err != nil && err != gorm.ErrRecordNotFound {
		return errors.New("获取应用版本失败")
	}

	appInfo.Versions = appVersions
	marshal, _ := json.Marshal(appInfo)
	socketOperation.SendMsg(marshal)

	return nil
}

var appListByAppNameAndVersion ServiceInterfaceFn = func(socketOperation *SocketOperation) error {

	name, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	version, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	appInfo := &vos.DbAppInfo{}
	if err = db.GetDb().Model(&vos.DbAppInfo{}).Where(&vos.DbAppInfo{
		Name: name.String(),
	}).First(&appInfo).Error; err != nil || appInfo.Name == "" {
		return errors.New("查询App信息失败")
	}

	appVersion := &vos.DbAppVersionInfo{}
	if err = db.GetDb().Model(&vos.DbAppVersionInfo{}).Where(&vos.DbAppVersionInfo{
		AppName: appInfo.Name,
		Name:    version.String(),
	}).First(&appVersion).Error; err != nil || appVersion.Name == "" {
		return errors.New("查询App版本信息失败")
	}
	pluginByteLen := len(appVersion.Plugins)
	if pluginByteLen <= 0 {
		return errors.New("查询插件信息失败, 数据可能已被篡改")
	}

	pluginBlockSize := md5.Size + sha1.Size
	if pluginByteLen%pluginBlockSize != 0 {
		return errors.New("查询插件信息失败, 数据可能已被篡改")
	}

	pluginLen := pluginByteLen / pluginBlockSize

	appVersion.PluginInfo = make([]*vos.DbAppPlugin, 0)
	for j := 0; j < pluginLen; j++ {
		startBlock := j * pluginBlockSize
		endBlock := (j + 1) * pluginBlockSize
		pluginFlag := appVersion.Plugins[startBlock:endBlock]
		plugin := &vos.DbAppPlugin{}
		if err = db.GetDb().Where(&vos.DbAppPlugin{
			Md5:  pluginFlag[:md5.Size],
			Sha1: pluginFlag[md5.Size:],
		}).First(&plugin).Error; err != nil {
			continue
		}
		if len(plugin.EnvConfigBytes) > 0 {
			_ = json.Unmarshal(plugin.EnvConfigBytes, &plugin.EnvConfig)
		}
		appVersion.PluginInfo = append(appVersion.PluginInfo, plugin)
	}

	if len(appVersion.EnvConfig) > 0 {
		_ = json.Unmarshal(appVersion.EnvConfig, &appVersion.EnvConfigInfos)
	}
	appInfo.CurrentVersion = appVersion.Name
	appInfo.CurrentVersionInfo = appVersion
	marshal, _ := json.Marshal(appInfo)
	socketOperation.SendMsg(marshal)
	return nil
}
