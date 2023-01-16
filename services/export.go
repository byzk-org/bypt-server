package services

import (
	"encoding/json"
	"errors"
	"github.com/byzk-org/bypt-server/consts"
	"github.com/byzk-org/bypt-server/db"
	"github.com/byzk-org/bypt-server/vos"
	"github.com/jinzhu/gorm"
	"gopkg.in/yaml.v2"
	"os"
	"path/filepath"
)

var exportService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	GlobalOperationLock.Lock()
	defer GlobalOperationLock.Unlock()

	msg, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	uid, gid, err := consts.GetUidAndGid()
	if err != nil {
		return err
	}

	data := make([]*vos.DbAppStartInfo, 0)
	if err = db.GetDb().Model(&vos.DbAppStartInfo{}).Find(&data).Error; err != nil && err != gorm.ErrRecordNotFound {
		return errors.New("查询启动信息失败")
	}

	if len(data) == 0 {
		return errors.New("未查询到启动信息")
	}

	endData := make(map[string]*vos.DbAppStartInfo)
	for _, d := range data {
		if len(d.EnvConfigBytes) > 0 {
			_ = json.Unmarshal(d.EnvConfigBytes, &d.EnvConfig)
		}

		if len(d.EnvConfig) > 0 {
			envConfig := make([]*vos.AppConfig, 0, len(d.EnvConfig))
			for i := range d.EnvConfig {
				config := d.EnvConfig[i]
				if config.Val == "" {
					config.Val = config.DefaultVal
				}

				if config.Val != "" {
					envConfig = append(envConfig, config)
				}
			}
		}

		if len(d.ArgsBytes) > 0 {
			_ = json.Unmarshal(d.ArgsBytes, &d.Args)
		}

		if len(d.CopyFileBytes) > 0 {
			_ = json.Unmarshal(d.CopyFileBytes, &d.CopyFiles)
		}

		endData[d.Name] = d
	}

	exportDir := msg.String()
	_ = os.MkdirAll(exportDir, 0777)
	os.Chown(exportDir, uid, gid)
	exportFilePath := filepath.Join(msg.String(), "byptStart.yaml")
	file, err := os.Create(exportFilePath)
	if err != nil {
		return errors.New("创建导出文件失败")
	}
	file.Chown(uid, gid)
	defer file.Close()

	encoder := yaml.NewEncoder(file)
	defer encoder.Close()

	if err = encoder.Encode(endData); err != nil {
		return errors.New("转换配置文件失败")
	}

	socketOperation.SendMsg([]byte(exportFilePath))
	return nil
}
