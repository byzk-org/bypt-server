package services

import "sync"

var GlobalOperationLock = sync.Mutex{}

type ReadMsg func() (SliceBytes, error)
type SendSuccessMsg func(content []byte)

type ServiceInterfaceFn func(socketOperation *SocketOperation) error

type SocketOperation struct {
	ReadMsg ReadMsg
	SendMsg SendSuccessMsg
}

type SliceBytes []byte

func (s SliceBytes) String() string {
	return string(s)
}

var (
	ServiceMap = map[string]ServiceInterfaceFn{
		"start":                      startService,
		"startWithConfig":            startYamlConfigService,
		"stop":                       stopAppService,
		"stopWithConfig":             stopYamlConfigService,
		"import":                     importService,
		"configSetting":              configService,
		"configList":                 configListService,
		"appList":                    appList,
		"appListByAppName":           appListByAppName,
		"appListByAppNameAndVersion": appListByAppNameAndVersion,
		"psList":                     psListService,
		"psApp":                      psAppNameService,
		"psAppPlugin":                psAppNamePluginService,
		"rmAll":                      rmAllService,
		"rmByName":                   rmAppByNameService,
		"rmByNameAndVersion":         rmAppByNameAndVersionService,
		"logByName":                  logByNameService,
		"logByNameAndVersion":        logByNameAndVersionService,
		"syncInfo":                   syncInfoService,
		"syncRemoteInfo":             syncRemoteService,
		"restart":                    restartService,
		"restartWithConfig":          restartWithConfigService,
		"jdkLs":                      jdkListService,
		"jdkLsName":                  jdkListNameService,
		"jdkRm":                      jdkRmName,
		"jdkRmAll":                   jdkRmAll,
		"jdkRename":                  jdkRename,
		"infoBanner":                 infoBannerService,
		"infoLogClear":               infoClearLogService,
		"export":                     exportService,
	}
)
