package services

import (
	"encoding/json"
	"errors"
	"github.com/byzk-org/bypt-server/db"
	"github.com/byzk-org/bypt-server/logs"
	"github.com/byzk-org/bypt-server/vos"
	"strconv"
	"time"
)

const (
	settingLogsClearSpaceKey     = "logClearTimeSpace"
	settingLogsClearSpaceUnitKey = "logClearTimeSpaceUnit"
)

var configListService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	settings := make([]vos.DbSetting, 2)
	if err := db.GetDb().Model(&vos.DbSetting{}).Find(&settings).Error; err != nil {
		return errors.New("查询配置列表失败")
	}

	settings = append(settings, vos.DbSetting{
		Name: settingLogsClearSpaceKey,
		Val:  strconv.FormatInt(logs.GetTimeSpace(), 10),
		Desc: "日志清除时间间隔",
	})

	unitStr := "H"
	switch logs.GetTimeUnit() {
	case time.Millisecond:
		unitStr = "MS"
	case time.Second:
		unitStr = "S"
	case time.Minute:
		unitStr = "M"
	case time.Hour:
		unitStr = "H"
	}
	settings = append(settings, vos.DbSetting{
		Name: settingLogsClearSpaceUnitKey,
		Val:  unitStr,
		Desc: "日志清除时间间隔单位, MS: 毫秒, S: 秒, M: 分钟, H: 小时",
	})

	marshal, _ := json.Marshal(settings)
	socketOperation.SendMsg(marshal)
	return nil
}
