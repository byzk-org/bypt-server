package helper

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/byzk-org/bypt-server/vos"
	"github.com/jinzhu/gorm"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	splitRune = []byte{'\n'}
)

func newAppLogs(appName, appVersion, dbLogPath string) (w *appLogs, returnErr error) {
	defer func() {
		e := recover()
		if e != nil {
			switch err := e.(type) {
			case error:
				returnErr = err
			case string:
				returnErr = errors.New(err)
			default:
				returnErr = errors.New("未知的异常")
			}
		}
	}()
	logPath := filepath.Join(dbLogPath, appName, appVersion)
	//if err != nil {
	//	return nil, errors.New("创建应用日志路径失败")
	//}

	_ = os.MkdirAll(logPath, 0777)
	stat, err := os.Stat(logPath)
	if err != nil {
		return nil, errors.New("获取日志目录失败")
	}

	if !stat.IsDir() {
		return nil, errors.New("获取日志存放目录失败")
	}

	logFilePath := filepath.Join(logPath, "main.log")
	fileStat, err := os.Stat(logFilePath)
	if err != nil {
		file, err := os.Create(logFilePath)
		if err != nil {
			return nil, errors.New("创建日志存储文件失败")
		}
		defer file.Close()
		fileStat, err = os.Stat(logFilePath)
		if err != nil {
			return nil, errors.New("获取文件状态失败")
		}
	}

	if fileStat.IsDir() {
		return nil, errors.New("创建日志文件失败")
	}

	openPath := fmt.Sprintf("file:%s?auto_vacuum=1", logFilePath)
	if logDb, e := gorm.Open("sqlite3", openPath); e != nil {
		return nil, errors.New("创建日志存储集失败")
	} else {
		logDb.Exec("PRAGMA auto_vacuum = 1;")
		logDb.AutoMigrate(&vos.DbLog{})
		logDb.AutoMigrate(&vos.DbLog{})
		return &appLogs{
			dbLog:    logDb,
			openPath: openPath,
			tmpLogInfo: &vos.DbLog{
				AppName:    appName,
				AppVersion: appVersion,
			},
		}, nil
	}

}

type appLogs struct {
	sync.Mutex
	openPath       string
	dbLog          *gorm.DB
	tmpBuf         []byte
	tmpLogInfo     *vos.DbLog
	LogRefreshChan chan time.Time
}

func (a *appLogs) Write(p []byte) (int, error) {
	a.Lock()
	defer a.Unlock()
	dataIsClose := false
	if a.dbLog.DB().Stats().OpenConnections == 0 {
		open, err := gorm.Open("sqlite3", a.openPath)
		if err != nil {
			dataIsClose = true
		} else {
			a.dbLog = open
			defer a.dbLog.Close()
		}
	}
	split := bytes.Split(p, splitRune)
	endLen := len(split) - 1
	if len(a.tmpBuf) > 0 {
		if len(a.tmpBuf) > 0 {
			split[0] = bytes.Join([][]byte{
				a.tmpBuf,
				split[0],
			}, nil)
			a.tmpBuf = nil
		}
	}

	if len(split[len(split)-1]) != 0 {
		a.tmpBuf = bytes.Join([][]byte{
			a.tmpBuf,
			split[len(split)-1],
		}, nil)
	}

	a.dbLog.Transaction(func(tx *gorm.DB) error {
		logModel := tx.Model(&vos.DbLog{})
		for i := 0; i < endLen; i++ {
			content := split[i]
			a.tmpLogInfo.Content = content
			a.tmpLogInfo.AtDate = time.Now().UnixNano()
			if dataIsClose {
				logrus.Error("日志保存失败, 原日志信息 => ", string(content))
				continue
			}
			if err := logModel.Create(&a.tmpLogInfo).Error; err != nil {
				logrus.Error("日志保存失败, 原日志信息 => ", string(content))
				continue
			}
			//time.Sleep(time.Millisecond * 10)
		}
		return nil
	})

	return len(p), nil
}

func (a *appLogs) Close() error {
	go func() {
		a.Lock()
		defer a.Unlock()
		a.dbLog.Close()
	}()
	return nil
	//return nil
}
