package services

import (
	"errors"
	"github.com/byzk-org/bypt-server/consts"
	"github.com/byzk-org/bypt-server/db"
	"github.com/byzk-org/bypt-server/vos"
	"path/filepath"
)

var logByNameService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	appName, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	appInfo := &vos.DbAppInfo{}
	if err = db.GetDb().Model(&vos.DbAppInfo{}).Where(&vos.DbAppInfo{
		Name: appName.String(),
	}).Find(&appInfo).Error; err != nil {
		return errors.New("未查询到app信息")
	}

	logSetting := &vos.DbSetting{}
	if err = db.GetDb().Model(&vos.DbSetting{}).Where(&vos.DbSetting{
		Name: consts.DbSettingLogDir,
	}).Find(&logSetting).Error; err != nil {
		return errors.New("查询日志目录失败")
	}

	socketOperation.SendMsg([]byte(filepath.Join(logSetting.Val, appInfo.Name, appInfo.CurrentVersion)))
	return nil
}

var logByNameAndVersionService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {

	appName, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	appVersion, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	appVersionO := &vos.DbAppVersionInfo{}
	appVersionInfoModel := db.GetDb().Model(&vos.DbAppVersionInfo{})
	if err = appVersionInfoModel.Where(&vos.DbAppVersionInfo{
		AppName: appName.String(),
		Name:    appVersion.String(),
	}).First(&appVersionO).Error; err != nil || appVersionO.Name == "" || appVersionO.AppName == "" {
		return errors.New("获取app版本信息失败")
	}

	logSetting := &vos.DbSetting{}
	if err = db.GetDb().Model(&vos.DbSetting{}).Where(&vos.DbSetting{
		Name: consts.DbSettingLogDir,
	}).Find(&logSetting).Error; err != nil {
		return errors.New("查询日志目录失败")
	}
	socketOperation.SendMsg([]byte(filepath.Join(logSetting.Val, appVersionO.AppName, appVersionO.Name)))
	return nil
}
