package services

import (
	"encoding/json"
	"github.com/byzk-org/bypt-server/logs"
	"time"
)

var bannerText = []byte(` ____  _  _  ____  ____ 
(  _ \( \/ )(  _ \(_  _)
 ) _ < \  /  )___/  )(  
(____/ (__) (__)   (__)
         Version: 2.0.0
`)

var infoBannerService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	socketOperation.SendMsg(bannerText)
	return nil
}

var infoClearLogService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {

	clearLogInfo := &struct {
		NextClearTime time.Time
		PrevClearTime time.Time
		TimeSpace     int64
		TimeUnit      time.Duration
		ClearLogMsg   []byte
	}{
		NextClearTime: logs.NextClearLogTime(),
		PrevClearTime: logs.PrevClearLogTime(),
		TimeSpace:     logs.GetTimeSpace(),
		TimeUnit:      logs.GetTimeUnit(),
		ClearLogMsg:   logs.ClearLogMsg.Bytes(),
	}

	marshal, _ := json.Marshal(clearLogInfo)
	socketOperation.SendMsg(marshal)
	return nil
}
