package helper

import (
	"bytes"
	"github.com/byzk-org/bypt-server/vos"
	"io"
	"os/exec"
	"sync"
	"time"
)

type appRunStatus string

const (
	appRunStatusWaitRun     appRunStatus = "正在启动"
	appRunStatusRunner      appRunStatus = "正在运行"
	appRunStatusRunError    appRunStatus = "运行异常"
	appRunStatusWaitRestart appRunStatus = "等待重启"
	appRunStatusRunRestart  appRunStatus = "正在重启"
)

type appRunErrType int

const (
	_ appRunErrType = iota
	// appRunErrTypeData 业务数据异常
	appRunErrTypeData
	appRunErrTypeApp
	appRunErrTypePlugin
)

type AppStatusInfo struct {
	StartArgs          *vos.DbAppStartInfo   `json:"startArgs,omitempty"`
	Name               string                `json:"name,omitempty"`
	Desc               string                `json:"desc,omitempty"`
	AppInfo            *vos.DbAppInfo        `json:"appInfo,omitempty"`
	VersionStr         string                `json:"versionStr,omitempty"`
	VersionInfo        *vos.DbAppVersionInfo `json:"versionInfo,omitempty"`
	StartTime          time.Time             `json:"startTime,omitempty"`
	HaveErr            bool                  `json:"haveErr,omitempty"`
	ErrMsg             string                `json:"errMsg,omitempty"`
	JavaCmd            string                `json:"javaCmd,omitempty"`
	PluginOutPutBuffer map[string][]byte     `json:"pluginOutPutBuffer,omitempty"`
	Status             appRunStatus          `gorm:"-" json:"status,omitempty"`
	IsRestart          bool                  `gorm:"-" json:"isRestart,omitempty"`
	exitChannel        chan string
	runCmd             *exec.Cmd
	pluginsCmd         []*exec.Cmd
	runDir             string
	closeLock          sync.Mutex
	isClose            bool
	pluginOkChan       chan bool
	pluginOutPutBuffer map[string]*bytes.Buffer
	logCloser          io.Closer
	stopRestartChannel chan bool
}

func (a *AppStatusInfo) convertPluginsOutPut() {
	if a.PluginOutPutBuffer == nil {
		a.PluginOutPutBuffer = make(map[string][]byte)
	}
	if len(a.pluginOutPutBuffer) == 0 {
		return
	}

	for k, v := range a.pluginOutPutBuffer {
		if v != nil && v.Len() > 0 {
			a.PluginOutPutBuffer[k] = v.Bytes()
		}
	}
}
