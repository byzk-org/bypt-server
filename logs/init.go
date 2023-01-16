package logs

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/byzk-org/bypt-server/consts"
	"github.com/byzk-org/bypt-server/db"
	"github.com/byzk-org/bypt-server/vos"
	"github.com/jinzhu/gorm"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

var (
	clearSpace int64 = 24
	clearUnit        = time.Hour
)

const (
	dbPrevClearLogTimeKey = "prevClearTime"
	dbClearSpaceKey       = "clearSpace"
	dbClearSpaceUnit      = "clearSpaceUnit"
)

var (
	prevClearLogTime time.Time
	nextClearLogTime time.Time
)

var ClearLogMsg = &bytes.Buffer{}

func init() {
	log.SetFormatter(&log.JSONFormatter{})
	log.SetLevel(log.DebugLevel)
	//homeDir, err := os.UserHomeDir()
	//if err != nil {
	//	_, _ = os.Stderr.WriteString("获取用户家目录失败")
	//	os.Exit(-1)
	//}
	//path := filepath.Join(homeDir, ".devTools", "logs", "server")
	//
	///* 日志轮转相关函数
	//`WithLinkName` 为最新的日志建立软连接
	//`WithRotationTime` 设置日志分割的时间，隔多久分割一次
	//WithMaxAge 和 WithRotationCount二者只能设置一个
	//  `WithMaxAge` 设置文件清理前的最长保存时间
	//  `WithRotationCount` 设置文件清理前最多保存的个数
	//*/
	//writer, _ := rotatelogs.New(
	//	path+".%Y%m%d%H%M",
	//	rotatelogs.WithLinkName(path+".log"),
	//	rotatelogs.WithMaxAge(15*12*time.Hour),
	//	rotatelogs.WithRotationTime(12*time.Hour),
	//)
	//log.SetOutput(writer)
}

func NextClearLogTime() time.Time {
	return nextClearLogTime
}

func PrevClearLogTime() time.Time {
	return prevClearLogTime
}

func setPrevClearLogTime(t time.Time) error {
	timeStr := t.Format("2006-01-02 15:04:05")
	if err := saveClearSetting(dbPrevClearLogTimeKey, timeStr); err != nil {
		return err
	}

	prevClearLogTime = t
	return nil
}

func saveClearSetting(key, val string) error {
	count := 0
	if err := db.GetDb().Model(&vos.DbLogClearInfo{}).Where(&vos.DbLogClearInfo{
		Name: key,
	}).Count(&count).Error; err != nil {
		return errors.New("查询日志配置失败")
	}

	if count == 0 {
		if err := db.GetDb().Model(&vos.DbLogClearInfo{}).Create(&vos.DbLogClearInfo{
			Name: key,
			Val:  val,
		}).Error; err != nil {
			return errors.New("保存配置失败")
		}
	} else {
		if err := db.GetDb().Model(&vos.DbLogClearInfo{}).Where(&vos.DbLogClearInfo{
			Name: key,
		}).Update(&vos.DbLogClearInfo{
			Val: val,
		}).Error; err != nil {
			return errors.New("更新配置失败")
		}
	}
	return nil
}

func GetTimeSpace() int64 {
	return clearSpace
}

func GetTimeUnit() time.Duration {
	return clearUnit
}

func SetTimeSpace(space int64) error {
	if err := saveClearSetting(dbClearSpaceKey, strconv.FormatInt(space, 10)); err != nil {
		return err
	}
	clearSpace = space
	return nil
}

func SetTimeSpaceUnit(unit time.Duration) error {
	switch unit {
	case time.Nanosecond:
	case time.Microsecond:
	case time.Millisecond:
	case time.Second:
	case time.Minute:
	case time.Hour:
	default:
		return errors.New("非法的时间单位")
	}
	if err := saveClearSetting(dbClearSpaceUnit, strconv.FormatInt(int64(unit), 10)); err != nil {
		return err
	}
	clearUnit = unit
	return nil
}

func InitClearLogListener() {

	go func() {
		isHavePrevClear := false
		dbLogClearInfoModel := db.GetDb().Model(&vos.DbLogClearInfo{})
		prevTimeStrSetting := &vos.DbLogClearInfo{}
		if err := dbLogClearInfoModel.Where(&vos.DbLogClearInfo{
			Name: dbPrevClearLogTimeKey,
		}).First(&prevTimeStrSetting).Error; err == nil && prevTimeStrSetting.Val != "" {
			PrevClearLogTimeTmp, e := time.ParseInLocation("2006-01-02 15:04:05", prevTimeStrSetting.Val, time.Local)
			if e == nil {
				prevClearLogTime = PrevClearLogTimeTmp
				isHavePrevClear = true
			}
		}

		clearSpaceInfo := &vos.DbLogClearInfo{}
		if err := dbLogClearInfoModel.Where(&vos.DbLogClearInfo{
			Name: dbClearSpaceKey,
		}).First(&clearSpaceInfo).Error; err == nil && clearSpaceInfo.Val != "" {
			parseInt, e := strconv.ParseInt(clearSpaceInfo.Val, 10, 64)
			if e != nil {
				clearSpace = parseInt
			}
		}

		clearSpaceUnitInfo := &vos.DbLogClearInfo{}
		if err := dbLogClearInfoModel.Where(&vos.DbLogClearInfo{
			Name: dbClearSpaceUnit,
		}).First(&clearSpaceUnitInfo).Error; err == nil && clearSpaceUnitInfo.Val != "" {
			parseInt, e := strconv.ParseInt(clearSpaceUnitInfo.Val, 10, 64)
			if e != nil {
				clearSpace = parseInt
			}
			clearUnit = time.Duration(parseInt)
			switch clearUnit {
			case time.Nanosecond:
			case time.Microsecond:
			case time.Millisecond:
			case time.Second:
			case time.Minute:
			case time.Hour:
			default:
				clearUnit = time.Hour
			}
		}

		ticker := time.NewTicker(time.Duration(clearSpace * int64(clearUnit)))

		if isHavePrevClear {
			for {
				nextClearLogTime = prevClearLogTime.Add(time.Duration(clearSpace * int64(clearUnit)))
				now := time.Now()
				if now.After(nextClearLogTime) {
					ClearLogMsg.Reset()
					clearLog()
					continue
				}
				break
			}
		} else {
			nextClearLogTime = time.Now().Add(time.Duration(clearSpace * int64(clearUnit)))
		}

		for {
			<-ticker.C
			ClearLogMsg.Reset()
			clearLog()

		}
	}()
}

func clearLog() {
	defer func() { recover() }()
	defer func() {
		setPrevClearLogTime(nextClearLogTime)
		nextClearLogTime = time.Now().Add(time.Duration(clearSpace * int64(clearUnit)))
	}()
	clearTimeBefore := nextClearLogTime.UnixNano()
	clearTimeStr := nextClearLogTime.Format("2006-01-02 15:04:05")

	appVersionModel := db.GetDb().Model(&vos.DbAppVersionInfo{})

	appVersionList := make([]*vos.DbAppVersionInfo, 0)
	if err := appVersionModel.Find(&appVersionList).Error; err != nil && err != gorm.ErrRecordNotFound {
		ClearLogMsg.WriteString("查询程序日志失败\n")
		return
	}

	if len(appVersionList) == 0 {
		ClearLogMsg.WriteString("程序没有记录日志, 本次清除完成\n")
		return
	}

	logSetting := vos.DbSetting{}
	if err := db.GetDb().Where(&vos.DbSetting{
		Name: consts.DbSettingLogDir,
	}).First(&logSetting).Error; err != nil || logSetting.Name == "" {
		ClearLogMsg.WriteString("查找日志配置路径失败")
		return
	}

	for _, appVersion := range appVersionList {
		clearAppVersionLog(logSetting.Val, appVersion, clearTimeBefore, clearTimeStr)
	}

}

func clearAppVersionLog(dbLogPath string, appVersion *vos.DbAppVersionInfo, timeBefore int64, timeBeforeStr string) {
	defer func() { recover() }()
	var (
		fileStat os.FileInfo
		err      error
	)

	logAppName := fmt.Sprintf("[%s-%s]\n", appVersion.AppName, appVersion.Name)

	logFilePath := filepath.Join(dbLogPath, appVersion.AppName, appVersion.Name, "main.log")
	fileStat, err = os.Stat(logFilePath)
	if err != nil {
		ClearLogMsg.WriteString(fmt.Sprintf("获取%s日志文件失败\n", logAppName))
		return
	}

	if fileStat.IsDir() {
		ClearLogMsg.WriteString(fmt.Sprintf("获取%s日志文件失败\n", logAppName))
		return
	}

	conn, err := gorm.Open("sqlite3", fmt.Sprintf("file:%s?auto_vacuum=1", logFilePath))
	if err != nil {
		ClearLogMsg.WriteString(fmt.Sprintf("打开%s日志文件失败\n", logAppName))
		return
	}
	defer conn.Close()

	if err = conn.Where("at_date <= ?", timeBefore).Delete(&vos.DbLog{}).Error; err != nil {
		ClearLogMsg.WriteString(fmt.Sprintf("删除%s在%s之前的日志失败\n", logAppName, timeBeforeStr))
		return
	}

	ClearLogMsg.WriteString(fmt.Sprintf("成功删除%s在%s之前的日志\n", logAppName, timeBeforeStr))

}
