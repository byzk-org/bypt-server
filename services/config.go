package services

import (
	"errors"
	"github.com/byzk-org/bypt-server/consts"
	"github.com/byzk-org/bypt-server/db"
	"github.com/byzk-org/bypt-server/helper"
	"github.com/byzk-org/bypt-server/logs"
	"github.com/byzk-org/bypt-server/vos"
	"github.com/jinzhu/gorm"
	"os"
	"strconv"
	"time"
)

var configService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	key, err := socketOperation.ReadMsg()
	if err != nil {
		return errors.New("获取要修改的Val失败")
	}

	val, err := socketOperation.ReadMsg()
	if err != nil {
		return errors.New("获取要修改的key失败")
	}

	valStr := val.String()
	if key.String() == settingLogsClearSpaceKey {
		space, err := strconv.ParseInt(valStr, 10, 64)
		if err != nil {
			return errors.New("非法的时间间隔")
		}
		return logs.SetTimeSpace(space)
	}

	if key.String() == settingLogsClearSpaceUnitKey {

		unit := time.Hour
		switch valStr {
		case "MS":
			unit = time.Millisecond
		case "S":
			unit = time.Second
		case "M":
			unit = time.Minute
		case "H":
			unit = time.Hour
		default:
			return errors.New("非法的时间单位")
		}
		return logs.SetTimeSpaceUnit(unit)
	}

	return db.GetDb().Transaction(func(tx *gorm.DB) error {
		helper.AppStatusMgr.Lock()
		defer helper.AppStatusMgr.Unlock()

		srcSetting := &vos.DbSetting{}

		settingModel := tx.Model(&vos.DbSetting{})
		whereSetting := settingModel.Where(&vos.DbSetting{Name: key.String()})
		if err = whereSetting.First(&srcSetting).Error; err != nil || srcSetting.Name == "" {
			return errors.New("未识别要修改的配置")
		}

		if srcSetting.StopApp && helper.AppStatusMgr.NowStartNum() > 0 {
			return errors.New("请先关闭所有已经启动的应用然后尝试更改配置")
		}

		if err = whereSetting.Update(&vos.DbSetting{
			Val: valStr,
		}).Error; err != nil {
			return errors.New("修改配置信息失败")
		}

		switch srcSetting.Name {
		case consts.DbSettingLogDir:
			fallthrough
		case consts.DbSettingJdkSaveDir:
			fallthrough
		case consts.DbSettingAppSaveDir:
			_ = os.MkdirAll(srcSetting.Val, 0777)
			if err = os.Rename(srcSetting.Val, valStr); err != nil {
				return errors.New("目录移动失败")
			}
		}

		return nil
	})

}
