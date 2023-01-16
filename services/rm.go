package services

import (
	"errors"
	"github.com/byzk-org/bypt-server/db"
	"github.com/byzk-org/bypt-server/helper"
	"github.com/byzk-org/bypt-server/vos"
	"github.com/jinzhu/gorm"
)

// rmAllService 删除所有的app
var rmAllService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {

	helper.AppStatusMgr.Lock()
	defer helper.AppStatusMgr.Unlock()

	num := helper.AppStatusMgr.NowStartNum()
	if num > 0 {
		return errors.New("请先停止所有应用然后在删除")
	}

	return db.GetDb().Transaction(func(tx *gorm.DB) error {

		if err := tx.Delete(&vos.DbAppStartInfo{}).Error; err != nil {
			return errors.New("删除启动参数列表失败")
		}
		if err := tx.Delete(&vos.DbAppInfo{}).Error; err != nil {
			return errors.New("删除应用信息失败")
		}

		if err := tx.Delete(&vos.DbAppVersionInfo{}).Error; err != nil {
			return errors.New("删除应用版本失败")
		}

		if err := tx.Delete(&vos.DbAppPlugin{}).Error; err != nil {
			return errors.New("删除应用插件失败")
		}

		return nil
	})
}

// rmAppByNameService 删除App根据名称
var rmAppByNameService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	msg, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	return db.GetDb().Transaction(func(tx *gorm.DB) error {
		count := 0
		if err = tx.Model(&vos.DbAppInfo{}).Where(&vos.DbAppInfo{
			Name: msg.String(),
		}).Count(&count).Error; err != nil || count == 0 {
			return errors.New("获取应用信息失败")
		}

		if err = tx.Where(vos.DbAppInfo{
			Name: msg.String(),
		}).Delete(&vos.DbAppInfo{}).Error; err != nil {
			panic("删除应用信息失败")
		}

		if err = tx.Where(vos.DbAppPlugin{
			AppName: msg.String(),
		}).Delete(&vos.DbAppPlugin{}).Error; err != nil {
			panic("删除应用插件信息失败")
		}

		if err = tx.Where(vos.DbAppVersionInfo{
			AppName: msg.String(),
		}).Delete(&vos.DbAppVersionInfo{}).Error; err != nil {
			panic("删除应用版本失败")
		}

		if err = tx.Where(vos.DbAppStartInfo{
			Name: msg.String(),
		}).Delete(&vos.DbAppStartInfo{}).Error; err != nil {
			panic("删除应用启动信息失败")
		}

		if helper.AppStatusMgr.IsStart(msg.String()) {
			return errors.New("请先停止应用")
		}

		return nil
	})
}

// rmAppByNameAndVersionService 删除App根据名称和版本
var rmAppByNameAndVersionService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	name, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	version, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	return db.GetDb().Transaction(func(tx *gorm.DB) error {
		count := 0
		if err = tx.Model(&vos.DbAppInfo{}).Where(&vos.DbAppInfo{
			Name: name.String(),
		}).Count(&count).Error; err != nil || count == 0 {
			return errors.New("获取应用信息失败")
		}

		if err = tx.Where(vos.DbAppPlugin{
			AppName:    name.String(),
			AppVersion: version.String(),
		}).Delete(&vos.DbAppPlugin{}).Error; err != nil {
			panic("删除应用插件信息失败")
		}

		if err = tx.Where(vos.DbAppVersionInfo{
			AppName: name.String(),
			Name:    version.String(),
		}).Delete(&vos.DbAppVersionInfo{}).Error; err != nil {
			panic("删除应用版本失败")
		}

		if err = tx.Where(vos.DbAppStartInfo{
			Name:    name.String(),
			Version: version.String(),
		}).Delete(&vos.DbAppStartInfo{}).Error; err != nil {
			panic("删除应用启动信息失败")
		}

		if helper.AppStatusMgr.IsStart(name.String()) {
			return errors.New("请先停止应用")
		}

		return nil
	})
}
