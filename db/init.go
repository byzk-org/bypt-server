package db

import (
	"fmt"
	"github.com/byzk-org/bypt-server/consts"
	"github.com/byzk-org/bypt-server/vos"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
)

var (
	mainSqlite3Db *gorm.DB
)

func InitDb() {
	var (
		file *os.File
		db   *gorm.DB
	)
	_ = os.MkdirAll(consts.DbPathDir, 0777)
	dbFile, err := filepath.Abs(filepath.Join(consts.DbPathDir, ".main.data"))
	if err != nil {
		logrus.Error("获取数据文件失败 => ", err.Error())
		panic("")
	}

	_, err = os.Stat(dbFile)
	if err != nil {
		file, err = os.Create(dbFile)
		if err == nil {
			_ = file.Close()
		}
	}

	if db, err = gorm.Open("sqlite3", fmt.Sprintf("file:%s?auto_vacuum=1", dbFile)); err != nil {
		logrus.Error("创建主数据库失败 => ", err.Error())
		panic("")
	} else {
		//mainSqlite3Db = db.Debug()
		mainSqlite3Db = db
		mainSqlite3Db.Exec("PRAGMA auto_vacuum = 1;")
	}

	initDataTable()
}

func initDataTable() {
	mainSqlite3Db.AutoMigrate(&vos.DbAppInfo{})
	mainSqlite3Db.AutoMigrate(&vos.DbAppVersionInfo{})
	mainSqlite3Db.AutoMigrate(&vos.DbSetting{})
	mainSqlite3Db.AutoMigrate(&vos.DbAppPlugin{})
	mainSqlite3Db.AutoMigrate(&vos.DbAppStartInfo{})
	mainSqlite3Db.AutoMigrate(&vos.DbJdkInfo{})
	mainSqlite3Db.AutoMigrate(&vos.DbLogClearInfo{})

	dbSettingModel := mainSqlite3Db.Model(&vos.DbSetting{})
	whereSetting := dbSettingModel.Where(&vos.DbSetting{
		Name: consts.DbSettingLogDir,
	}).Or(&vos.DbSetting{
		Name: consts.DbSettingRunDir,
	}).Or(&vos.DbSetting{
		Name: consts.DbSettingSyncServer,
	}).Or(&vos.DbSetting{
		Name: consts.DbSettingAppSaveDir,
	}).Or(&vos.DbSetting{
		Name: consts.DbSettingJdkSaveDir,
	})

	count := 0
	if err := whereSetting.Count(&count).Error; err != nil || count != 5 {
		tmpDir := os.TempDir()
		dbSettingModel.Delete(&vos.DbSetting{})
		dbSettingModel.Create(&vos.DbSetting{
			Name:    consts.DbSettingRunDir,
			Desc:    "程序运行目录",
			Val:     tmpDir,
			StopApp: true,
		})
		dbSettingModel.Create(&vos.DbSetting{
			Name:    consts.DbSettingLogDir,
			Desc:    "日志存放目录",
			Val:     consts.LogPathDir,
			StopApp: true,
		})
		dbSettingModel.Create(&vos.DbSetting{
			Name: consts.DbSettingSyncServer,
			Desc: "远程同步服务地址, 格式: IP:PORT 例: 127.0.0.1:8080",
			Val:  "",
		})
		dbSettingModel.Create(&vos.DbSetting{
			Name:    consts.DbSettingAppSaveDir,
			Desc:    "程序文件存放目录",
			Val:     consts.AppSaveDir,
			StopApp: true,
		})
		dbSettingModel.Create(&vos.DbSetting{
			Name:    consts.DbSettingJdkSaveDir,
			Desc:    "jdk文件存放目录",
			Val:     consts.JdkSaveDir,
			StopApp: true,
		})
	}
}

func GetDb() *gorm.DB {
	return mainSqlite3Db
}
