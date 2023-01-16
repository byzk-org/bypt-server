package helper

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/byzk-org/bypt-server/consts"
	"github.com/byzk-org/bypt-server/db"
	"github.com/byzk-org/bypt-server/utils"
	"github.com/byzk-org/bypt-server/vos"
	"github.com/jinzhu/gorm"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var AppStatusMgr = newAppRunMgr()

// appRunMgr app管理器
type appRunMgr struct {
	sync.RWMutex
	startAppMap map[string]*AppStatusInfo
}

// newAppRunMgr 创建一个app管理器
func newAppRunMgr() *appRunMgr {
	return &appRunMgr{
		startAppMap: make(map[string]*AppStatusInfo),
	}
}

// NowStartNum 现在启动的数量
func (a *appRunMgr) NowStartNum() int {
	return len(a.startAppMap)
}

// QueryStartAppInfo 查询App启动信息
func (a *appRunMgr) QueryStartAppInfo(appName string) ([]byte, error) {
	a.Lock()
	defer a.Unlock()

	appStatusInfo, ok := a.startAppMap[appName]
	if !ok {
		return nil, errors.New("app未启动")
	}
	appStatusInfo.convertPluginsOutPut()
	marshal, _ := json.Marshal(appStatusInfo)
	return marshal, nil
}

// QueryStartAppPluginInfo 查询启动插件信息
func (a *appRunMgr) QueryStartAppPluginInfo(appName string, appPluginName string) ([]byte, error) {
	a.Lock()
	defer a.Unlock()
	appStatusInfo, ok := a.startAppMap[appName]
	if !ok {
		return nil, errors.New("app未启动")
	}

	pluginList := appStatusInfo.VersionInfo.PluginInfo
	endList := make([]map[string]interface{}, 0, len(pluginList))
	for _, plugin := range pluginList {
		if !strings.HasPrefix(plugin.Name, appPluginName) {
			continue
		}
		returnData := map[string]interface{}{
			"name": plugin.Name,
			"desc": plugin.Desc,
			"md5":  plugin.Md5,
			"sha1": plugin.Sha1,
		}

		buffer := appStatusInfo.pluginOutPutBuffer[plugin.Name]
		if buffer != nil && buffer.Len() > 0 {
			returnData["output"] = buffer.Bytes()
		}

		endList = append(endList, returnData)
	}

	marshal, err := json.Marshal(endList)
	if err != nil {
		return nil, errors.New("转换数据结构失败")
	}

	return marshal, nil
}

// StartAppList 启动列表
func (a *appRunMgr) StartAppList() []byte {
	a.Lock()
	defer a.Unlock()
	endList := make([]*AppStatusInfo, 0, len(a.startAppMap))
	for _, val := range a.startAppMap {
		endList = append(endList, val)
	}
	marshal, _ := json.Marshal(endList)
	return marshal
}

// IsStart 是否启动
func (a *appRunMgr) IsStart(appName string) bool {
	a.Lock()
	defer a.Unlock()
	status, ok := a.startAppMap[appName]
	if !ok || status.isClose {
		return false
	}
	return true
}

// StopAllApp 停止app
func (a *appRunMgr) StopAllApp() error {
	a.Lock()
	defer a.Unlock()
	appMap := a.startAppMap
	for name := range appMap {
		if err := a.StopApp(name); err != nil {
			return err
		}
	}
	return nil
}

// StopAppAndVersion 停止app
func (a *appRunMgr) StopAppAndVersion(name, version string) error {
	a.Lock()
	defer a.Unlock()
	appInfo, ok := a.startAppMap[name]
	if !ok {
		return nil
	}

	if appInfo.VersionStr != version {
		return nil
	}

	return a.StopApp(appInfo.Name)
}

// StopApp 停止app
func (a *appRunMgr) StopApp(appName string) error {
	a.Lock()
	defer a.Unlock()
	defer func() { recover() }()
	info, ok := a.startAppMap[appName]
	if !ok {
		return errors.New("app未启动")
	}

	return db.GetDb().Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&vos.DbAppStartInfo{}).Where(&vos.DbAppStartInfo{
			Name:    info.Name,
			Version: info.VersionStr,
		}).Delete(&vos.DbAppStartInfo{}).Error; err != nil {
			return errors.New("删除应用启动信息失败")
		}
		a.closeStopRestartChan(info)
		a.settingErrStatus("正常停止", info, appRunErrTypeData)
		delete(a.startAppMap, appName)
		return nil
	})

}

// StartAppByPrevConfig 根据上一次的配置启动
func (a *appRunMgr) StartAppByPrevConfig() {

	allAppStartInfo := make([]*vos.DbAppStartInfo, 0)
	if err := db.GetDb().Model(&vos.DbAppStartInfo{}).Find(&allAppStartInfo).Error; err != nil {
		return
	}

	if len(allAppStartInfo) > 0 {
		for _, s := range allAppStartInfo {
			if s.RunDir != "" {
				_ = os.RemoveAll(s.RunDir)
			}
		}
	}

	appStartInfos := make([]*vos.DbAppStartInfo, 0)
	if err := db.GetDb().Model(&vos.DbAppStartInfo{}).Where(&vos.DbAppStartInfo{
		Restart: vos.AppRestartTypeAlways,
	}).Or(&vos.DbAppStartInfo{
		Restart: vos.AppRestartTypeErrorAuto,
	}).Find(&appStartInfos).Error; err != nil {
		return
	}

	if len(appStartInfos) == 0 {
		return
	}

	for _, appStartInfo := range appStartInfos {
		_ = a.StartApp(appStartInfo)
	}

}

// RestartApp 重启app
func (a *appRunMgr) RestartApp(appName string) error {
	info, ok := a.startAppMap[appName]
	if !ok {
		return errors.New("应用未启动, 无法启动")
	}

	appStartInfo := &vos.DbAppStartInfo{}
	if err := db.GetDb().Model(&vos.DbAppStartInfo{}).Where(&vos.DbAppStartInfo{
		Name:    info.Name,
		Version: info.VersionStr,
	}).First(&appStartInfo).Error; err != nil {
		return errors.New("未识别的应用")
	}
	if err := a.StopApp(appName); err != nil {
		return err
	}
	return a.StartApp(appStartInfo)
}

func (a *appRunMgr) RestartAppWithStartInfo(startInfo *vos.DbAppStartInfo) error {
	_, ok := a.startAppMap[startInfo.Name]
	if !ok {
		return errors.New("应用未启动, 无法重启")
	}

	//appStartInfo := &vos.DbAppStartInfo{}
	//if err := db.GetDb().Model(&vos.DbAppStartInfo{}).Where(&vos.DbAppStartInfo{
	//	Name:    info.Name,
	//	Version: info.VersionStr,
	//}).First(&appStartInfo).Error; err != nil {
	//	return errors.New("未识别的应用")
	//}
	if err := a.StopApp(startInfo.Name); err != nil {
		return err
	}
	return a.StartApp(startInfo)
}

// StartApp 启动App
func (a *appRunMgr) StartApp(appStartInfo *vos.DbAppStartInfo) (returnErr error) {

	if appStartInfo == nil {
		return errors.New("获取应用启动信息失败")
	}

	if appStartInfo.Name == "" {
		return errors.New("要启动的应用名称不能为空")
	}

	if a.IsStart(appStartInfo.Name) {
		return errors.New("应用已经启动, 请勿重复启动")
	}

	a.Lock()
	defer a.Unlock()
	defer func() {
		e := recover()
		if e != nil {
			returnErr = errors.New("启动应用失败")
		}
	}()

	if appStartInfo.JdkPackName != "" {
		appStartInfo.JdkPackInfo = &vos.DbJdkInfo{}
		if err := db.GetDb().Where(&vos.DbJdkInfo{
			Name: appStartInfo.JdkPackName,
		}).First(&appStartInfo.JdkPackInfo).Error; err != nil {
			return errors.New("未识别的包内jdk名称")
		}
	}

	settingModel := db.GetDb().Model(&vos.DbSetting{})
	settingRunDir := &vos.DbSetting{}
	if err := settingModel.Where(&vos.DbSetting{
		Name: consts.DbSettingRunDir,
	}).First(&settingRunDir).Error; err != nil {
		return errors.New("获取运行目录失败")
	}

	settingLogDir := &vos.DbSetting{}
	if err := settingModel.Where(&vos.DbSetting{
		Name: consts.DbSettingLogDir,
	}).First(&settingLogDir).Error; err != nil {
		return errors.New("获取日志目录失败")
	}

	appInfo := &vos.DbAppInfo{}
	if err := db.GetDb().Model(&vos.DbAppInfo{}).Where(&vos.DbAppInfo{
		Name: appStartInfo.Name,
	}).First(&appInfo).Error; err != nil || appInfo.Name == "" {
		return errors.New("未查询到相关app信息")
	}

	appVersion := &vos.DbAppVersionInfo{}
	if err := db.GetDb().Model(&vos.DbAppVersionInfo{}).Where(&vos.DbAppVersionInfo{
		Name:    appStartInfo.Version,
		AppName: appInfo.Name,
	}).First(&appVersion).Error; err != nil && appVersion.Name == "" {
		return errors.New("获取要启动的版本名称失败")
	}

	srcStartInfo := &vos.DbAppStartInfo{}
	if err := db.GetDb().Model(&vos.DbAppStartInfo{
		Name:    appInfo.Name,
		Version: appVersion.Name,
	}).First(&srcStartInfo).Error; err != nil && err != gorm.ErrRecordNotFound {
		return errors.New("查询数据信息失败")
	}

	if srcStartInfo.RunDir != "" {
		_ = os.RemoveAll(srcStartInfo.RunDir)
	}

	plugins := make([]*vos.DbAppPlugin, 0)
	if err := db.GetDb().Model(&vos.DbAppPlugin{}).Where(&vos.DbAppPlugin{
		AppName:    appInfo.Name,
		AppVersion: appVersion.Name,
	}).Find(&plugins).Error; err != nil && err != gorm.ErrRecordNotFound {
		return errors.New("查询应用版本信息失败")
	}

	isHavePluginConfig := false
	if len(appStartInfo.PluginEnvConfig) > 0 {
		appStartInfo.PluginEnvConfigBytes, _ = json.Marshal(appStartInfo.PluginEnvConfigBytes)
		isHavePluginConfig = true
	}

	blockSize := md5.Size + sha1.Size
	pluginDataFlagLen := len(appVersion.Plugins)
	if pluginDataFlagLen > 0 {
		if pluginDataFlagLen%blockSize != 0 {
			return errors.New("解析插件信息失败, 数据可能已被篡改")
		}
		appVersion.PluginInfo = make([]*vos.DbAppPlugin, 0, pluginDataFlagLen/blockSize)
		for i := 0; i < pluginDataFlagLen; i += blockSize {
			hashBytes := appVersion.Plugins[i : i+blockSize]
			md5Bytes := hashBytes[:md5.Size]
			sha1Bytes := hashBytes[md5.Size:]
			plugin := &vos.DbAppPlugin{}
			if err := db.GetDb().Where(&vos.DbAppPlugin{
				Md5:        md5Bytes,
				Sha1:       sha1Bytes,
				AppName:    appInfo.Name,
				AppVersion: appVersion.Name,
			}).First(&plugin).Error; err != nil {
				return errors.New("获取插件信息失败")
			}

			if !utils.PubKeyVerifySign(consts.CaPubKey, plugin.Src(), plugin.Sign) {
				return errors.New("插件已被篡改, 请尝试重新导入应用")
			}

			if len(plugin.EnvConfigBytes) > 0 {
				_ = json.Unmarshal(plugin.EnvConfigBytes, &plugin.EnvConfig)
			}

			if len(plugin.EnvConfig) > 0 {
				for pi := range plugin.EnvConfig {
					p := plugin.EnvConfig[pi]
					if isHavePluginConfig {
						for k, v := range appStartInfo.PluginEnvConfig {
							if strings.HasPrefix(plugin.Name, k) {
								c, ok := v[p.Name]
								if ok {
									p.Val = c
								} else {
									p.Val = p.DefaultVal
								}
								goto ContinuePlugin
							}
						}
					}

					p.Val = p.DefaultVal
				ContinuePlugin:
					continue
				}
			}

			appVersion.PluginInfo = append(appVersion.PluginInfo, plugin)
		}
	}

	if ok := utils.PubKeyVerifySign(consts.CaPubKey, appVersion.SignSrc(), appVersion.Sign); !ok {
		return errors.New("数据可能已被篡改，请您重新导入进行尝试")
	}

	return db.GetDb().Transaction(func(tx *gorm.DB) error {
		appVersionModel := tx.Model(&vos.DbAppVersionInfo{})

		if appStartInfo.Version == "" {
			appStartInfo.Version = appInfo.CurrentVersion
		}

		if appStartInfo.Version == "" {
			return errors.New("获取要启动的版本信息失败")
		}

		appStartInfo.VersionInfo = appVersion

		if len(appVersion.EnvConfig) == 0 {
			appStartInfo.EnvConfig = nil
		} else {
			appConfigs := make([]*vos.AppConfig, 0)
			if err := json.Unmarshal(appVersion.EnvConfig, &appConfigs); err == nil {
				appConfigMap := make(map[string]*vos.AppConfig)
				for i := range appConfigs {
					tmpC := appConfigs[i]
					appConfigMap[tmpC.Name] = tmpC
				}

				endConfig := make([]*vos.AppConfig, 0, len(appConfigMap))
				if appStartInfo.EnvConfig != nil {
					for _, appStartConfig := range appStartInfo.EnvConfig {
						config, ok := appConfigMap[appStartConfig.Name]
						if !ok {
							continue
						}
						delete(appConfigMap, appStartConfig.Name)
						if appStartConfig.Val != "" {
							endConfig = append(endConfig, &vos.AppConfig{
								Name:       config.Name,
								Val:        appStartConfig.Val,
								Desc:       config.Desc,
								DefaultVal: config.DefaultVal,
							})
						} else {
							endConfig = append(endConfig, config)
						}
					}
				}

				for k := range appConfigMap {
					endConfig = append(endConfig, appConfigMap[k])
				}

				appStartInfo.EnvConfig = endConfig

			}

		}

		appInfo.CurrentVersion = appVersion.Name
		if err := tx.Model(&vos.DbAppInfo{}).Where(&vos.DbAppInfo{
			Name: appInfo.Name,
		}).Update(&vos.DbAppInfo{
			CurrentVersion: appVersion.Name,
		}).Error; err != nil {
			return errors.New("更新应用信息失败")
		}

		if err := appVersionModel.Where(&vos.DbAppVersionInfo{
			AppName: appInfo.Name,
			Name:    appVersion.Name,
		}).Update(&appVersion).Error; err != nil {
			return errors.New("更新应用配置失败")
		}

		javaCmd := "java"
		if appStartInfo.JdkPath != "" {
			javaCmd = appStartInfo.JdkPath
		}

		//tmpRunDir, err := ioutil.TempDir(settingRunDir.Val, "appRun*")
		//if err != nil {
		//	return errors.New("创建运行目录失败")
		//}
		appStartInfo.RunDir = filepath.Join(settingRunDir.Val, appInfo.Name, appVersion.Name)
		_ = os.RemoveAll(appStartInfo.RunDir)
		if err := os.MkdirAll(appStartInfo.RunDir, 0777); err != nil {
			return errors.New("创建运行目录失败")
		}

		appStartInfo.LogDir = settingLogDir.Val
		if stat, err := os.Stat(appStartInfo.LogDir); err != nil || !stat.IsDir() {
			if err = os.MkdirAll(appStartInfo.LogDir, 0777); err != nil {
				return errors.New("创建日志目录失败")
			}
		}

		if len(appStartInfo.EnvConfig) > 0 {
			marshal, _ := json.Marshal(appStartInfo.EnvConfig)
			appStartInfo.EnvConfigBytes = marshal
		}

		appStartInfo.JdkArgsBytes = appVersion.JdkStartArgsBytes
		if err := json.Unmarshal(appVersion.JdkStartArgsBytes, &appStartInfo.JdkArgs); err != nil {
			return errors.New("转换java启动参数失败")
		}

		if len(appStartInfo.Args) > 0 {
			marshal, _ := json.Marshal(appStartInfo.Args)
			appStartInfo.ArgsBytes = marshal
		}

		if len(appStartInfo.CopyFiles) > 0 {
			marshal, _ := json.Marshal(appStartInfo.CopyFiles)
			appStartInfo.CopyFileBytes = marshal
		}

		statusInfo := &AppStatusInfo{
			AppInfo:            appInfo,
			StartArgs:          appStartInfo,
			Name:               appInfo.Name,
			Desc:               appInfo.Desc,
			VersionStr:         appVersion.Name,
			VersionInfo:        appVersion,
			StartTime:          time.Now(),
			JavaCmd:            javaCmd,
			Status:             appRunStatusWaitRun,
			exitChannel:        make(chan string, 1),
			pluginsCmd:         make([]*exec.Cmd, 0, len(appVersion.PluginInfo)),
			isClose:            false,
			pluginOkChan:       make(chan bool, len(appVersion.PluginInfo)),
			pluginOutPutBuffer: make(map[string]*bytes.Buffer),
			runDir:             appStartInfo.RunDir,
		}

		memArgs := make([]string, 0, 5)
		if appStartInfo.Xmx != "" {
			memArgs = append(memArgs, "-Xmx"+appStartInfo.Xmx)
		}

		if appStartInfo.Xms != "" {
			memArgs = append(memArgs, "-Xms"+appStartInfo.Xms)
		}

		if appStartInfo.Xmn != "" {
			memArgs = append(memArgs, "-xmn"+appStartInfo.Xmn)
		}

		if appStartInfo.PermSize != "" {
			memArgs = append(memArgs, "-XX:PermSize="+appStartInfo.PermSize)
		}

		if appStartInfo.MaxPermSize != "" {
			memArgs = append(memArgs, "-XX:MaxPermSize="+appStartInfo.MaxPermSize)
		}

		if len(memArgs) > 0 {
			appStartInfo.JdkArgs = append(memArgs, appStartInfo.JdkArgs...)
		}
		a.startAppMap[appStartInfo.Name] = statusInfo
		if err := a.startAppExec(statusInfo); err != nil {
			return err
		}

		appStartInfoModel := tx.Model(&vos.DbAppStartInfo{})
		if err := appStartInfoModel.Where(&vos.DbAppStartInfo{
			Name: appStartInfo.Name,
		}).Delete(&vos.DbAppStartInfo{}).Error; err != nil {
			return errors.New("删除缓存数据失败")
		}

		if err := appStartInfoModel.Create(&statusInfo.StartArgs).Error; err != nil {
			return errors.New("保存配置缓存失败")
		}

		return nil
	})

}

func (a *appRunMgr) startAppExec(appStatusInfo *AppStatusInfo) error {
	startInfo := appStatusInfo.StartArgs

	contentSrcPath, sm4Key, err := decryptPath(startInfo.VersionInfo.Content)
	if err != nil {
		return errors.New(err.Error())
	}

	appExecName := "run"
	if appStatusInfo.StartArgs.SaveAppSuffix {
		appExecName += ".jar"
	}

	contentPath := filepath.Join(appStatusInfo.runDir, appExecName)
	file, err := os.OpenFile(contentSrcPath, os.O_RDONLY, 0666)
	if err != nil {
		return errors.New("程序源文件已经损坏或丢失, 请重新导入")
	}
	defer file.Close()
	if err = utils.Sm4Decrypt2File(sm4Key, file, contentPath); err != nil {
		return errors.New("写出运行文件失败")
	}

	md5Sum, err := utils.CalcMd5(contentPath)
	if err != nil {
		return errors.New("获取运行文件摘要失败")
	}

	if bytes.Compare(md5Sum, appStatusInfo.VersionInfo.ContentMd5) != 0 {
		a.settingErrStatus("", appStatusInfo, appRunErrTypeData)
		return errors.New("文件已被损坏")
	}

	sha1Sum, err := utils.CalcSha1(contentPath)
	if err != nil {
		return errors.New("获取运行文件摘要失败")
	}

	if bytes.Compare(sha1Sum, appStatusInfo.VersionInfo.ContentSha1) != 0 {
		return errors.New("文件已被损坏")
	}

	jarPass := appStatusInfo.VersionInfo.JarPass
	jarPass, err = utils.Sm2Decrypt(consts.CaPrivateKey, jarPass)
	if err != nil {
		return errors.New("转换运行密钥失败")
	}

	jarPassObj := make(map[string]string)
	if err = json.Unmarshal(jarPass, &jarPassObj); err != nil {
		return errors.New("获取运行密钥失败,请尝试重新导入")
	}

	algorithm, err := a.convertBase642byte(jarPassObj, "algorithm")
	if err != nil {
		return errors.New("获取运行密钥失败,请尝试重新导入")
	}

	ivSize, err := a.convertBase642byte(jarPassObj, "ivsize")
	if err != nil {
		return errors.New("获取运行密钥失败,请尝试重新导入")
	}

	keySize, err := a.convertBase642byte(jarPassObj, "keysize")
	if err != nil {
		return errors.New("获取运行密钥失败,请尝试重新导入")
	}

	password, err := a.convertBase642byte(jarPassObj, "password")
	if err != nil {
		return errors.New("获取运行密钥失败,请尝试重新导入")
	}

	xjarMd5, err := a.convertBase642byte(jarPassObj, "md5")
	if err != nil {
		return errors.New("获取运行密钥失败,请尝试重新导入")
	}

	xjarSha1, err := a.convertBase642byte(jarPassObj, "sha1")
	if err != nil {
		return errors.New("获取运行密钥失败,请尝试重新导入")
	}

	if bytes.Compare(md5Sum, xjarMd5) != 0 {
		return errors.New("文件已被损坏")
	}

	if bytes.Compare(sha1Sum, xjarSha1) != 0 {
		return errors.New("文件已被损坏")
	}

	runKey := bytes.Join([][]byte{
		algorithm, {13, 10},
		keySize, {13, 10},
		ivSize, {13, 10},
		password, {13, 10},
	}, []byte{})

	go a.startRun(contentPath, appStatusInfo, runKey)
	return nil
}

func (a *appRunMgr) settingErrStatus(errMsg string, appStatusInfo *AppStatusInfo, errType appRunErrType) {
	appStatusInfo.closeLock.Lock()
	defer appStatusInfo.closeLock.Unlock()
	//defer func() { recover() }()
	defer os.RemoveAll(appStatusInfo.runDir)
	if appStatusInfo.runCmd != nil && appStatusInfo.runCmd.Process != nil {
		_ = appStatusInfo.runCmd.Process.Kill()
	}

	if len(appStatusInfo.pluginsCmd) > 0 {
		for _, pluginCmd := range appStatusInfo.pluginsCmd {
			if pluginCmd != nil && pluginCmd.Process != nil {
				_ = pluginCmd.Process.Kill()
			}
		}
	}

	if appStatusInfo.isClose {
		return
	}
	appStatusInfo.Status = appRunStatusRunError
	appStatusInfo.HaveErr = true
	appStatusInfo.ErrMsg = errMsg
	appStatusInfo.isClose = true
	a.closeLogs(appStatusInfo.logCloser)
	a.closePluginOkChan(appStatusInfo)
	a.settingRestart(appStatusInfo, errType)
	close(appStatusInfo.exitChannel)

}

// settingRestart 设置重启
func (a *appRunMgr) settingRestart(appStatusInfo *AppStatusInfo, errType appRunErrType) {
	appStatusInfo.IsRestart = false
	restartMode := appStatusInfo.StartArgs.Restart
	if errType == appRunErrTypeData || restartMode == vos.AppRestartTypeErrorAuto {
		return
	}

	if restartMode == "" {
		return
	}

	if restartMode == vos.AppRestartTypeErrorApp && errType != appRunErrTypeApp {
		return
	}

	if restartMode == vos.AppRestartTypeErrorPlugin && errType != appRunErrTypePlugin {
		return
	}

	switch errType {
	case appRunErrTypePlugin:
		fallthrough
	case appRunErrTypeApp:
		appStatusInfo.IsRestart = true
		appStatusInfo.stopRestartChannel = make(chan bool, 1)
		appStatusInfo.Status = appRunStatusWaitRestart
		go func() {
			timeOut := time.NewTimer(60 * time.Second)
			defer timeOut.Stop()
			for {
				select {
				case <-appStatusInfo.stopRestartChannel:
					a.closeStopRestartChan(appStatusInfo)
					return
				case <-timeOut.C:
					if err := a.StartApp(appStatusInfo.StartArgs); err != nil {
						appStatusInfo.IsRestart = false
					}
					return
				}
			}

		}()
	default:
		appStatusInfo.IsRestart = false
	}
}

func (a *appRunMgr) closeStopRestartChan(appStatus *AppStatusInfo) {
	defer func() { recover() }()
	close(appStatus.stopRestartChannel)
}

func (a *appRunMgr) closeLogs(closer io.Closer) {
	defer func() { recover() }()
	_ = closer.Close()
}

func (a *appRunMgr) closePluginOkChan(appStatusInfo *AppStatusInfo) {
	defer func() { recover() }()
	close(appStatusInfo.pluginOkChan)
}

func (a *appRunMgr) convertBase642byte(obj map[string]string, name string) ([]byte, error) {
	val, ok := obj[name]
	if !ok {
		return nil, errors.New("获取名称失败")
	}
	decodeString, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		return nil, errors.New("解析base64字符串失败")
	}

	s := string(decodeString)
	split := strings.Split(s, ",")

	endBytes := make([]byte, 0, len(split))

	for _, str := range split {
		str = strings.TrimSpace(str)
		parseUint, err := strconv.ParseUint(str, 10, 8)
		if err != nil {
			return nil, errors.New("转换字节失败")
		}
		endBytes = append(endBytes, byte(parseUint))
	}

	return endBytes, nil
}

func (a *appRunMgr) startRun(contentPath string, appStatusInfo *AppStatusInfo, runKey []byte) {
	defer func() {
		e := recover()
		if e != nil {
			switch err := e.(type) {
			case error:
				a.settingErrStatus(err.Error(), appStatusInfo, appRunErrTypeApp)
			case string:
				a.settingErrStatus(err, appStatusInfo, appRunErrTypeData)
			default:
				a.settingErrStatus("运行中的未知异常", appStatusInfo, appRunErrTypeData)

			}
		}
	}()

	var (
		err      error
		abs      string
		fd       os.FileInfo
		javaPath string

		uid int
		gid int
	)

	runDir := filepath.Dir(contentPath)

	if appStatusInfo.StartArgs.JdkPackInfo != nil {
		javaPath, err = a.parsePackJdk(runDir, appStatusInfo.StartArgs.JdkPackInfo)
		if err != nil {
			a.settingErrStatus(err.Error(), appStatusInfo, appRunErrTypeData)
			return
		}
		appStatusInfo.JavaCmd = javaPath
	}

	//logsWriter := os.Stdout
	logsWriter, err := newAppLogs(appStatusInfo.StartArgs.Name, appStatusInfo.StartArgs.Version, appStatusInfo.StartArgs.LogDir)
	if err != nil {
		a.settingErrStatus(err.Error(), appStatusInfo, appRunErrTypeData)
		return
	}
	appStatusInfo.logCloser = logsWriter

	copyFiles := appStatusInfo.StartArgs.CopyFiles
	if len(copyFiles) > 0 {
		for _, fileName := range copyFiles {
			abs, err = filepath.Abs(fileName)
			if err != nil {
				a.settingErrStatus("获取文件 ["+fileName+"] 绝对路径失败", appStatusInfo, appRunErrTypeData)
				return
			}

			if fd, err = os.Stat(abs); err != nil {
				a.settingErrStatus("获取文件 ["+abs+"] 信息失败", appStatusInfo, appRunErrTypeData)
				return
			} else {
				_, name := filepath.Split(abs)
				if fd.IsDir() {
					if err = copyDir(abs, filepath.Join(appStatusInfo.runDir, name)); err != nil {
						a.settingErrStatus(err.Error(), appStatusInfo, appRunErrTypeData)
						return
					}
				} else {
					if err = copyFile(abs, filepath.Join(appStatusInfo.runDir, name)); err != nil {
						a.settingErrStatus(err.Error(), appStatusInfo, appRunErrTypeData)
						return
					}
				}
			}
		}
	}

	pluginTotalLen := len(appStatusInfo.VersionInfo.PluginInfo)
	beforePlugins := make([]*vos.DbAppPlugin, 0, pluginTotalLen)
	afterPlugins := make([]*vos.DbAppPlugin, 0, pluginTotalLen)
	listenerAndNormalPlugins := make([]*vos.DbAppPlugin, 0, pluginTotalLen)

	for _, plugin := range appStatusInfo.VersionInfo.PluginInfo {
		switch plugin.Type {
		case vos.AppPluginTypeNormal:
			fallthrough
		case vos.AppPluginTypeListener:
			listenerAndNormalPlugins = append(listenerAndNormalPlugins, plugin)
		case vos.AppPluginTypeBefore:
			beforePlugins = append(beforePlugins, plugin)
		case vos.AppPluginTypeAfter:
			afterPlugins = append(afterPlugins, plugin)
		default:
			a.settingErrStatus("无法处理的插件类型", appStatusInfo, appRunErrTypeData)
			return
		}
	}

	beforePluginLen := len(beforePlugins)
	if beforePluginLen > 0 {
		for _, plugin := range beforePlugins {
			if err = a.startSyncPlugin(plugin, appStatusInfo); err != nil {
				a.settingErrStatus(err.Error(), appStatusInfo, appRunErrTypePlugin)
				return
			}
		}
	}

	listenerAndNormalPluginLen := len(listenerAndNormalPlugins)
	if listenerAndNormalPluginLen > 0 {
		appStatusInfo.pluginOkChan = make(chan bool, listenerAndNormalPluginLen)
		for _, plugin := range listenerAndNormalPlugins {
			go func(p *vos.DbAppPlugin, appS *AppStatusInfo) { a.startPlugin(p, appS) }(plugin, appStatusInfo)
		}

		for i := 0; i < listenerAndNormalPluginLen; i++ {
			_, isOpen := <-appStatusInfo.pluginOkChan
			if !isOpen {
				return
			}
		}
	}

	env := os.Environ()
	config := appStatusInfo.StartArgs.EnvConfig
	if len(config) > 0 {
		for _, c := range config {
			if c.Val == "" {
				c.Val = c.DefaultVal
			}
			env = append(env, c.Name+"="+c.Val)
		}
	}

	env = append(env, "now_os="+runtime.GOOS)
	env = append(env, "now_arch="+runtime.GOARCH)
	env = append(env, "run_dir="+runDir)
	cmdArgs := make([]string, 0, len(appStatusInfo.StartArgs.JdkArgs)+len(appStatusInfo.StartArgs.Args)+1)
	cmdArgs = append(cmdArgs, appStatusInfo.StartArgs.JdkArgs...)
	cmdArgs = append(cmdArgs, contentPath)
	cmdArgs = append(cmdArgs, appStatusInfo.StartArgs.Args...)

	cmd := exec.Command(appStatusInfo.JavaCmd, cmdArgs...)
	cmd.Stdin = bytes.NewReader(runKey)
	if runtime.GOOS == "linux" {
		uid, err = strconv.Atoi(consts.User.Uid)
		gid, err = strconv.Atoi(consts.User.Gid)
		if err != nil {
			a.settingErrStatus("获取用户["+consts.User.Username+"]失败", appStatusInfo, appRunErrTypePlugin)
			return
		}
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{
				Uid: uint32(uid),
				Gid: uint32(gid),
			},
		}
	}
	cmd.Stdout = logsWriter
	cmd.Stderr = logsWriter
	cmd.Dir = runDir
	cmd.Env = env
	appStatusInfo.runCmd = cmd
	go func() {
		msg := <-appStatusInfo.exitChannel
		//fmt.Println("程序结束 => ", msg, isOpen)
		a.settingErrStatus(msg, appStatusInfo, appRunErrTypeApp)
	}()

	appStatusInfo.Status = appRunStatusRunner
	if err = cmd.Start(); err != nil {
		//fmt.Println("程序结束3 => " + err.Error())
		a.settingErrStatus("运行异常 => "+err.Error(), appStatusInfo, appRunErrTypeApp)
		return
	}

	defer func() {
		defer os.RemoveAll(appStatusInfo.runDir)
		if err = cmd.Wait(); err != nil {
			a.settingErrStatus("运行异常 => "+err.Error(), appStatusInfo, appRunErrTypeApp)
			return
		}
	}()

	afterPluginLen := len(afterPlugins)
	if afterPluginLen > 0 {
		for _, plugin := range afterPlugins {
			if err = a.startSyncPlugin(plugin, appStatusInfo); err != nil {
				a.settingErrStatus(err.Error(), appStatusInfo, appRunErrTypePlugin)
				return
			}
		}
	}

	//fmt.Println("程序结束2")
}

func (a *appRunMgr) parsePackJdk(runDir string, jdkInfo *vos.DbJdkInfo) (string, error) {
	dir, err := utils.TmpDir()
	if err != nil {
		return "", errors.New("创建临时存储目录失败")
	}
	defer os.RemoveAll(dir)
	if !utils.PubKeyVerifySign(consts.CaPubKey, jdkInfo.SignSrc(), jdkInfo.Sign) {
		return "", errors.New("jdk已被篡改, 请重新导入然后再次尝试")
	}

	path, key, err := utils.Sm4DecryptContentPath(consts.CaPrivateKey, jdkInfo.Content)
	if err != nil {
		return "", errors.New("获取jdk文件路径失败")
	}

	tmpDecryptFile := filepath.Join(dir, "j")
	file, err := os.OpenFile(path, os.O_RDONLY, 0666)
	if err != nil {
		return "", errors.New("打开jdk文件失败")
	}
	defer file.Close()
	if err = utils.Sm4Decrypt2File(key, file, tmpDecryptFile); err != nil {
		return "", errors.New("解析内部jdk失败")
	}

	jdkSavePath := filepath.Join(runDir, "._j")
	if err = utils.DeCompressGzip(tmpDecryptFile, jdkSavePath); err != nil {
		return "", errors.New("解压包内jdk到运行目录失败")
	}
	javaPath := filepath.Join(jdkSavePath, "bin", "java")
	if runtime.GOOS == "windows" {
		javaPath += ".exe"
	}
	os.Chmod(javaPath, 0777)
	return javaPath, nil
}

func (a *appRunMgr) startPlugin(plugin *vos.DbAppPlugin, appStatusInfo *AppStatusInfo) {
	pluginName := plugin.Name

	defer func() {
		e := recover()
		if e != nil {
			switch err := e.(type) {
			case error:
				a.settingErrStatus("插件("+pluginName+") 异常 =>"+err.Error(), appStatusInfo, appRunErrTypePlugin)
			case string:
				a.settingErrStatus("插件("+pluginName+") 异常 =>"+err, appStatusInfo, appRunErrTypePlugin)
			default:
				a.settingErrStatus("插件("+pluginName+")运行失败, 未知异常", appStatusInfo, appRunErrTypePlugin)
			}
		}
	}()
	//defer func() {
	//	appStatusInfo.pluginOkChan <- true
	//}()

	var (
		uid int
		gid int
	)

	if !utils.PubKeyVerifySign(consts.CaPubKey, plugin.Src(), plugin.Sign) {
		a.settingErrStatus("插件已被损坏", appStatusInfo, appRunErrTypeData)
		return
	}

	env := os.Environ()
	env = append(env, "now_os="+runtime.GOOS)
	env = append(env, "now_arch="+runtime.GOARCH)
	env = append(env, "run_dir="+appStatusInfo.runDir)
	env = append(env, "__cmd__=start")
	if len(plugin.EnvConfig) > 0 {
		for _, e := range plugin.EnvConfig {
			env = append(env, e.Name+"="+e.Val)
		}
	}

	if len(plugin.EnvConfig) > 0 {
		for _, pe := range plugin.EnvConfig {
			if pe.Val == "" {
				pe.Val = pe.DefaultVal
			}
			env = append(env, pe.Val)
		}
	}

	pluginDirs := filepath.Join(appStatusInfo.runDir, "p")
	_ = os.MkdirAll(pluginDirs, 0777)

	pluginSrcPath, sm4Key, err := decryptPath(plugin.Content)
	if err != nil {
		a.settingErrStatus("插件已被损坏, 请尝试重新导入", appStatusInfo, appRunErrTypeData)
		return
	}

	file, err := os.OpenFile(pluginSrcPath, os.O_RDONLY, 0666)
	if err != nil {
		a.settingErrStatus("插件源文件打开失败", appStatusInfo, appRunErrTypeData)
		return
	}
	defer file.Close()

	pluginFileName := filepath.Join(pluginDirs, utils.PathAddSuffix(pluginName))
	if err = utils.Sm4Decrypt2File(sm4Key, file, pluginFileName); err != nil {
		a.settingErrStatus("写出插件信息失败", appStatusInfo, appRunErrTypeData)
		return
	}

	pMd5, err := utils.CalcMd5(pluginFileName)
	if err != nil {
		a.settingErrStatus("获取运行插件MD5摘要失败", appStatusInfo, appRunErrTypeData)
		return
	}

	pSha1, err := utils.CalcSha1(pluginFileName)
	if err != nil {
		a.settingErrStatus("获取运行插件SHA1摘要失败", appStatusInfo, appRunErrTypeData)
		return
	}

	if !bytes.Equal(pSha1, plugin.Sha1) || !bytes.Equal(pMd5, plugin.Md5) {
		a.settingErrStatus("插件数据已被篡改", appStatusInfo, appRunErrTypeData)
		return
	}

	if err = os.Chmod(pluginFileName, 0777); err != nil {
		a.settingErrStatus("更改插件权限失败", appStatusInfo, appRunErrTypeData)
		return
	}

	buffer := &bytes.Buffer{}

	command := exec.Command(pluginFileName)
	appStatusInfo.pluginsCmd = append(appStatusInfo.pluginsCmd, command)
	if runtime.GOOS == "linux" {
		uid, err = strconv.Atoi(consts.User.Uid)
		gid, err = strconv.Atoi(consts.User.Gid)
		if err != nil {
			a.settingErrStatus("获取用户["+consts.User.Username+"]失败", appStatusInfo, appRunErrTypePlugin)
			return
		}
		command.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{
				Uid: uint32(uid),
				Gid: uint32(gid),
			},
		}
	}
	command.Dir = pluginDirs
	command.Env = env
	command.Stdout = buffer

	appStatusInfo.pluginOutPutBuffer[pluginName] = buffer

	if err = command.Start(); err != nil {
		a.settingErrStatus("插件("+pluginName+")启动失败 => "+err.Error(), appStatusInfo, appRunErrTypePlugin)
		return
	}

	appStatusInfo.pluginOkChan <- true
	switch plugin.Type {
	case vos.AppPluginTypeListener:
		errMsg := "监听插件(" + pluginName + ")提前退出"
		if err = command.Wait(); err != nil {
			errMsg += " => " + err.Error()
		}
		a.settingErrStatus(errMsg, appStatusInfo, appRunErrTypePlugin)
	case vos.AppPluginTypeNormal:
		if err = command.Wait(); err != nil {
			a.settingErrStatus("插件("+pluginName+")运行失败 => "+err.Error(), appStatusInfo, appRunErrTypePlugin)
		}
	default:
		a.settingErrStatus("未知的插件类型", appStatusInfo, appRunErrTypeData)
		return
	}

}

func (a *appRunMgr) startSyncPlugin(plugin *vos.DbAppPlugin, appStatusInfo *AppStatusInfo) (returnErr error) {
	pluginName := plugin.Name
	defer func() {
		e := recover()
		if e != nil {
			switch err := e.(type) {
			case error:
				returnErr = errors.New(err.Error())
			case string:
				returnErr = errors.New(err)
			default:
				returnErr = errors.New("插件运行失败, 未知异常")
			}
		}
	}()

	var (
		uid int
		gid int
	)

	isUnlock := false
	a.Lock()
	defer func() {
		if !isUnlock {
			a.Unlock()
		}
	}()

	if appStatusInfo.isClose {
		return errors.New("应用已关闭")
	}

	if !utils.PubKeyVerifySign(consts.CaPubKey, plugin.Src(), plugin.Sign) {
		return errors.New("插件已被损坏")
	}

	pluginDirs := filepath.Join(appStatusInfo.runDir, "p")
	_ = os.MkdirAll(pluginDirs, 0777)

	pluginSrcPath, sm4Key, err := decryptPath(plugin.Content)
	if err != nil {
		return errors.New("插件已被损坏, 请尝试重新导入")
	}

	file, err := os.OpenFile(pluginSrcPath, os.O_RDONLY, 0666)
	if err != nil {
		return errors.New("插件源文件打开失败")
	}
	defer file.Close()

	pluginFileName := filepath.Join(pluginDirs, utils.PathAddSuffix(pluginName))
	if err = utils.Sm4Decrypt2File(sm4Key, file, pluginFileName); err != nil {
		return errors.New("写出插件信息失败")
	}

	pMd5, err := utils.CalcMd5(pluginFileName)
	if err != nil {
		return errors.New("获取运行插件MD5摘要失败")
	}

	pSha1, err := utils.CalcSha1(pluginFileName)
	if err != nil {
		return errors.New("获取运行插件SHA1摘要失败")
	}

	if !bytes.Equal(pSha1, plugin.Sha1) || !bytes.Equal(pMd5, plugin.Md5) {
		return errors.New("插件数据已被篡改")
	}

	if err = os.Chmod(pluginFileName, 0777); err != nil {
		return errors.New("更改插件权限失败")
	}

	env := os.Environ()
	env = append(env, "now_os="+runtime.GOOS)
	env = append(env, "now_arch"+runtime.GOARCH)
	env = append(env, "run_dir"+appStatusInfo.runDir)
	env = append(env, "__cmd__=start")
	if len(plugin.EnvConfig) > 0 {
		for _, e := range plugin.EnvConfig {
			env = append(env, e.Name+"="+e.Val)
		}
	}

	buffer := &bytes.Buffer{}

	command := exec.Command(pluginFileName)
	if runtime.GOOS == "linux" {
		uid, err = strconv.Atoi(consts.User.Uid)
		gid, err = strconv.Atoi(consts.User.Gid)
		if err != nil {
			a.settingErrStatus("获取用户["+consts.User.Username+"]失败", appStatusInfo, appRunErrTypePlugin)
			return
		}
		command.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{
				Uid: uint32(uid),
				Gid: uint32(gid),
			},
		}
	}
	command.Dir = pluginDirs
	command.Env = env
	command.Stdout = buffer
	command.Stderr = buffer

	appStatusInfo.pluginOutPutBuffer[pluginName] = buffer
	isUnlock = true
	a.Unlock()

	if err = command.Run(); err != nil {
		return errors.New("插件(" + pluginName + ") 运行异常 =>" + err.Error())
	}

	return nil
}

func copyFile(src, dst string) error {
	var (
		err     error
		srcFd   *os.File
		dstFd   *os.File
		srcInfo os.FileInfo
	)

	if srcFd, err = os.Open(src); err != nil {
		return errors.New("打开文件 [" + src + "] 失败")
	}
	defer srcFd.Close()

	if dstFd, err = os.Create(dst); err != nil {
		return errors.New("创建文件 [" + dst + "] 失败")
	}
	defer dstFd.Close()

	if _, err = io.Copy(dstFd, srcFd); err != nil {
		return errors.New("复制文件 [" + src + "] 到 [" + dst + "] 失败")
	}

	if srcInfo, err = os.Stat(src); err != nil {
		return errors.New("获取原文件(" + src + ")信息失败")
	}

	if err = os.Chmod(dst, srcInfo.Mode()); err != nil {
		return errors.New("更改目标文件权限失败")
	}
	return nil
}

func copyDir(src, dst string) error {
	var (
		err     error
		fds     []os.FileInfo
		srcInfo os.FileInfo
	)

	if srcInfo, err = os.Stat(src); err != nil {
		return errors.New("获取原文件(" + src + ")信息失败")
	}

	if err = os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return errors.New("创建目标文件夹 [" + dst + "] 失败")
	}

	if fds, err = ioutil.ReadDir(src); err != nil {
		return errors.New("获取原目录[ " + src + " ]中的文件失败")
	}

	for _, fd := range fds {
		srcFd := filepath.Join(src, fd.Name())
		destFd := filepath.Join(dst, fd.Name())

		if fd.IsDir() {
			if err = copyDir(srcFd, destFd); err != nil {
				return err
			}
		} else {
			if err = copyFile(srcFd, destFd); err != nil {
				return err
			}
		}
	}

	return nil
}

func decryptPath(contentPath []byte) (string, []byte, error) {
	sm4EncryptKey := contentPath[:113]
	sm4Key, err := utils.Sm2Decrypt(consts.CaPrivateKey, sm4EncryptKey)
	if err != nil {
		return "", nil, errors.New("解析运行密钥失败")
	}

	contentPathBytes, err := utils.Sm4Decrypt(sm4Key, contentPath[113:])
	if err != nil {
		return "", nil, errors.New("解析文件路径失败")
	}
	return string(contentPathBytes), sm4Key, nil
}
