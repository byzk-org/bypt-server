package services

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/byzk-org/bypt-server/consts"
	"github.com/byzk-org/bypt-server/db"
	"github.com/byzk-org/bypt-server/helper"
	socket "github.com/byzk-org/bypt-server/socket/client"
	"github.com/byzk-org/bypt-server/utils"
	"github.com/byzk-org/bypt-server/vos"
	"github.com/jinzhu/gorm"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

type appVersionInfo struct {
	AppName           string             `json:"appName,omitempty"`
	Name              string             `json:"name,omitempty"`
	Desc              string             `json:"desc,omitempty"`
	CreateTime        time.Time          `json:"createTime,omitempty"`
	ContentMd5        []byte             `json:"md5,omitempty"`
	ContentSha1       []byte             `json:"sha1,omitempty"`
	Sign              []byte             `json:"sign,omitempty"`
	EnvConfig         []byte             `json:"envConfig,omitempty"`
	EnvConfigInfos    []vos.AppConfig    `json:"envConfigInfos,omitempty"`
	JarPass           []byte             `json:"j,omitempty"`
	Content           []byte             `json:"c,omitempty"`
	PluginInfos       []*vos.DbAppPlugin `json:"ps,omitempty"`
	JdkStartArgsBytes []byte             `json:"ja,omitempty"`

	//Config        []*AppConfig
	//ExecType      AppExecType
	//CopyFiles     []*AppCopyFileInfo
}

func (d *appVersionInfo) SignSrc() []byte {
	return bytes.Join([][]byte{
		[]byte(d.Name),
		[]byte(d.AppName),
		[]byte(d.Desc),
		d.ContentSha1,
		d.ContentMd5,
		d.JarPass,
		d.Content,
	}, nil)
}

type syncInfoParam struct {
	All bool `json:"all,omitempty"`
	//StartInfo  bool   `json:"startInfo,omitempty"`
	Jdk        bool   `json:"jdk,omitempty"`
	App        bool   `json:"app,omitempty"`
	Version    bool   `json:"version,omitempty"`
	AppName    string `json:"appName,omitempty"`
	AppVersion string `json:"appVersion,omitempty"`
	GOOS       string `json:"os,omitempty"`
	GOARCH     string `json:"arch,omitempty"`
}

type syncInfoResult struct {
	AppInfos    []*vos.DbAppInfo  `json:"appInfos,omitempty"`
	AppVersions []*appVersionInfo `json:"appVersions,omitempty"`
	JdkInfos    []*vos.DbJdkInfo  `json:"jdkInfos,omitempty"`
}

var syncInfoService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {

	var (
		err error

		sign []byte
		msg  SliceBytes

		renameMap = make(map[string]string)

		appSaveDirSetting = &vos.DbSetting{}
		jdkSaveDirSetting = &vos.DbSetting{}
	)

	msg, err = socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	if err = db.GetDb().Model(&vos.DbSetting{}).Where(&vos.DbSetting{
		Name: consts.DbSettingAppSaveDir,
	}).First(&appSaveDirSetting).Error; err != nil || appSaveDirSetting.Val == "" {
		return errors.New("获取应用保存目录失败")
	}

	if err = db.GetDb().Model(&vos.DbSetting{}).Where(&vos.DbSetting{
		Name: consts.DbSettingJdkSaveDir,
	}).First(&jdkSaveDirSetting).Error; err != nil || jdkSaveDirSetting.Val == "" {
		return errors.New("获取jdk保存目录失败")
	}

	tmpDir, err := utils.TmpDir()
	if err != nil {
		return errors.New("创建临时存储目录失败")
	}
	defer os.RemoveAll(tmpDir)

	syncInfo := &syncInfoParam{}
	if err = json.Unmarshal(msg, &syncInfo); err != nil {
		return errors.New("转换同步信息失败")
	}

	syncInfo.GOOS = runtime.GOOS
	syncInfo.GOARCH = runtime.GOARCH

	msg, _ = json.Marshal(syncInfo)

	helper.AppStatusMgr.Lock()
	defer helper.AppStatusMgr.Unlock()
	if helper.AppStatusMgr.NowStartNum() > 0 {
		return errors.New("请先关闭所有已经启动的应用然后尝试同步")
	}

	conn, err := socket.GetClientConn()
	if err != nil {
		return err
	}
	defer conn.SendEndMsg()

	if err = conn.WriteDataStr("syncRemoteInfo"); err != nil {
		return err
	}

	if err = conn.Wait(); err != nil {
		return err
	}

	if err = conn.WriteData(msg); err != nil {
		return err
	}

	lenByte, err := conn.ReadMsg()
	if err != nil {
		return err
	}

	size, err := strconv.ParseInt(string(lenByte), 10, 64)
	if err != nil {
		return err
	}

	socketOperation.SendMsg([]byte("下载消息头"))
	socketOperation.SendMsg(lenByte)
	_, err = socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	err = conn.WriteDataStr("ok")
	if err != nil {
		return err
	}

	allBuffer, err := receiveData2byte(size, conn, socketOperation)
	if err != nil {
		return err
	}

	syncResult := &syncInfoResult{}
	if err = json.Unmarshal(allBuffer, &syncResult); err != nil {
		return errors.New("转换同步结果失败")
	}

	return db.GetDb().Transaction(func(tx *gorm.DB) error {

		appInfoModel := tx.Model(&vos.DbAppInfo{})
		appVersionModel := tx.Model(&vos.DbAppVersionInfo{})
		//appStartModel := tx.Model(&vos.DbAppStartInfo{})
		appPluginModel := tx.Model(&vos.DbAppPlugin{})
		jdkModel := tx.Model(&vos.DbJdkInfo{})

		if len(syncResult.AppInfos) > 0 {
			for _, appInfo := range syncResult.AppInfos {
				if err = appInfoModel.Where(&vos.DbAppInfo{
					Name: appInfo.Name,
				}).Delete(&vos.DbAppInfo{}).Error; err != nil {
					return errors.New("清除原有应用信息失败")
				}
				appInfo.CreateTime = time.Now()
				if err = appInfoModel.Create(&appInfo).Error; err != nil {
					return errors.New("保存同步应用数据失败")
				}
			}
		}

		if len(syncResult.AppVersions) > 0 {

			for _, appVersion := range syncResult.AppVersions {

				outName := fmt.Sprintf("[%s-%s]", appVersion.AppName, appVersion.Name)

				if !utils.PubKeyVerifySign(consts.CaPubKey, appVersion.SignSrc(), appVersion.Sign) {
					return errors.New(outName + ": 验证签名失败")
				}
				if err = appVersionModel.Where(&vos.DbAppVersionInfo{
					Name:    appVersion.Name,
					AppName: appVersion.AppName,
				}).Delete(&vos.DbAppVersionInfo{}).Error; err != nil {
					return errors.New("清除原有应用版本信息失败")
				}

				if err = appPluginModel.Where(&vos.DbAppPlugin{
					AppName:    appVersion.AppName,
					AppVersion: appVersion.Name,
				}).Delete(&vos.DbAppPlugin{}).Error; err != nil {
					return errors.New("清除原有应用插件失败")
				}

				versionPluginList := appVersion.PluginInfos

				pluginTmpSaveDir := filepath.Join(tmpDir, appVersion.AppName, appVersion.Name)
				_ = os.MkdirAll(pluginTmpSaveDir, 0777)

				appVersionSaveDir := filepath.Join(appSaveDirSetting.Val, appVersion.AppName, appVersion.Name)

				pluginSaveDir := filepath.Join(appVersionSaveDir, "p")

				appVersionPlugin := &bytes.Buffer{}
				for pj, plugin := range versionPluginList {
					if !utils.PubKeyVerifySign(consts.CaPubKey, plugin.Src(), plugin.Sign) {
						return errors.New("数据签名验证失败, 数据可能在传输过程中被篡改")
					}

					pluginSm4Key := plugin.Content[:113]
					pluginContentSizeBytes := plugin.Content[113:]

					pluginContentSizeStr := string(pluginContentSizeBytes)
					pluginContentSize, e := strconv.ParseInt(pluginContentSizeStr, 10, 64)
					if e != nil {
						return errors.New("获取" + outName + "插件内容大小失败")
					}

					pluginMd5Hex := hex.EncodeToString(plugin.Md5)
					pluginSha1Hex := hex.EncodeToString(plugin.Sha1)

					decryptPluginKey, e := utils.Sm2Decrypt(consts.CaPrivateKey, pluginSm4Key)
					if e != nil {
						return errors.New(outName + "插件: 解析运行密钥失败")
					}

					pluginRealPath := filepath.Join(pluginSaveDir, pluginMd5Hex+pluginSha1Hex)
					encryptPath, e := utils.Sm4Encrypt(decryptPluginKey, []byte(pluginRealPath))
					if e != nil {
						return errors.New(fmt.Sprintf("%s插件: 路径转换失败", outName))
					}

					plugin.Content = bytes.Join([][]byte{
						pluginSm4Key,
						encryptPath,
					}, nil)

					sign, err = utils.PrivateKeySign(consts.CaPrivateKey, plugin.Src())
					if err != nil {
						return errors.New("插件数据包签名失败")
					}

					plugin.Sign = sign

					pluginTmpSaveFile := filepath.Join(pluginTmpSaveDir, fmt.Sprintf("p%s%s", pluginMd5Hex, pluginSha1Hex))
					socketOperation.SendMsg([]byte(fmt.Sprintf("下载%s插件%d", outName, pj+1)))
					socketOperation.SendMsg([]byte(pluginContentSizeStr))
					if e = receiveData2File(pluginContentSize, pluginTmpSaveFile, conn, socketOperation); e != nil {
						return errors.New(fmt.Sprintf("%s: %s", outName, e.Error()))
					}

					if err = appPluginModel.Create(&plugin).Error; err != nil {
						return errors.New("保存应用插件信息失败")
					}
					renameMap[pluginTmpSaveFile] = pluginRealPath
					appVersionPlugin.Write(plugin.Md5)
					appVersionPlugin.Write(plugin.Sha1)
				}

				appVersionSm4EncKey := appVersion.Content[:113]
				appVersionContentSizeBytes := appVersion.Content[113:]
				appVersionContentSizeStr := string(appVersionContentSizeBytes)
				appVersionContentSize, e := strconv.ParseInt(appVersionContentSizeStr, 10, 64)
				if e != nil {
					return errors.New("获取" + outName + "版本内容失败")
				}

				appVersionMd5Hex := hex.EncodeToString(appVersion.ContentMd5)
				appVersionSha1Hex := hex.EncodeToString(appVersion.ContentMd5)

				decryptKey, e := utils.Sm2Decrypt(consts.CaPrivateKey, appVersionSm4EncKey)
				if e != nil {
					return errors.New(outName + ": 解析运行密钥失败")
				}

				appVersionRealPath := filepath.Join(appVersionSaveDir, "run")
				encryptPath, e := utils.Sm4Encrypt(decryptKey, []byte(appVersionRealPath))
				if e != nil {
					return errors.New(fmt.Sprintf("%s: 路径转换失败", outName))
				}

				appVersion.Content = bytes.Join([][]byte{
					appVersionSm4EncKey,
					encryptPath,
				}, nil)
				appVersionContentTmpSaveFile := filepath.Join(pluginTmpSaveDir, fmt.Sprintf("p%s%s", appVersionMd5Hex, appVersionSha1Hex))
				socketOperation.SendMsg([]byte(fmt.Sprintf("下载%s内容", outName)))
				socketOperation.SendMsg([]byte(appVersionContentSizeStr))

				if e = receiveData2File(appVersionContentSize, appVersionContentTmpSaveFile, conn, socketOperation); e != nil {
					return errors.New(fmt.Sprintf("%s: %s", outName, e.Error()))
				}

				renameMap[appVersionContentTmpSaveFile] = appVersionRealPath

				appVersion.CreateTime = time.Now()

				endVersion := &vos.DbAppVersionInfo{
					AppName:           appVersion.AppName,
					Name:              appVersion.Name,
					Desc:              appVersion.Desc,
					CreateTime:        appVersion.CreateTime,
					EndUpdateTime:     time.Now(),
					ContentMd5:        appVersion.ContentMd5,
					ContentSha1:       appVersion.ContentSha1,
					Sign:              appVersion.Sign,
					EnvConfig:         appVersion.EnvConfig,
					EnvConfigInfos:    appVersion.EnvConfigInfos,
					JarPass:           appVersion.JarPass,
					Content:           appVersion.Content,
					Plugins:           appVersionPlugin.Bytes(),
					JdkStartArgsBytes: appVersion.JdkStartArgsBytes,
				}

				sign, err = utils.PrivateKeySign(consts.CaPrivateKey, endVersion.SignSrc())
				if err != nil {
					return errors.New("制作摘要失败")
				}
				endVersion.Sign = sign

				if err = appVersionModel.Create(&endVersion).Error; err != nil {
					return errors.New("保存应用版本信息失败")
				}
			}
		}

		if len(syncResult.JdkInfos) > 0 {
			jdkSaveDir := jdkSaveDirSetting.Val
			jdkPluginTmpSaveDir := filepath.Join(tmpDir, "_j")
			_ = os.MkdirAll(jdkPluginTmpSaveDir, 0777)
			for i := range syncResult.JdkInfos {
				jdk := syncResult.JdkInfos[i]
				if !utils.PubKeyVerifySign(consts.CaPubKey, jdk.SignSrc(), jdk.Sign) {
					return errors.New("远程数据可能已被篡改,请先确认目标服务器数据正确")
				}

				outName := fmt.Sprintf("[%s]", jdk.Name)

				jdkSm4Key := jdk.Content[:113]
				jdkContentSizeBytes := jdk.Content[113:]
				jdkContentSizeStr := string(jdkContentSizeBytes)
				jdkContentSize, e := strconv.ParseInt(jdkContentSizeStr, 10, 64)
				if e != nil {
					return errors.New("获取" + outName + "插件内容大小失败")
				}

				jdkMd5Hex := hex.EncodeToString(jdk.MD5)
				jdkSha1Hex := hex.EncodeToString(jdk.SHA1)

				decryptJdkKey, e := utils.Sm2Decrypt(consts.CaPrivateKey, jdkSm4Key)
				if e != nil {
					return errors.New(outName + "插件: 解析运行密钥失败")
				}

				jdkRealPath := filepath.Join(jdkSaveDir, jdkMd5Hex+jdkSha1Hex)
				encryptPath, e := utils.Sm4Encrypt(decryptJdkKey, []byte(jdkRealPath))
				if e != nil {
					return errors.New(fmt.Sprintf("%s插件: 路径转换失败", outName))
				}

				jdk.Content = bytes.Join([][]byte{
					jdkSm4Key,
					encryptPath,
				}, nil)

				sign, err = utils.PrivateKeySign(consts.CaPrivateKey, jdk.SignSrc())
				if err != nil {
					return errors.New("jdk数据包签名失败")
				}
				jdk.Sign = sign

				jdkTmpSaveFile := filepath.Join(jdkPluginTmpSaveDir, fmt.Sprintf("%s%s", jdkMd5Hex, jdkSha1Hex))
				socketOperation.SendMsg([]byte(fmt.Sprintf("下载%sjdk包", outName)))
				socketOperation.SendMsg([]byte(jdkContentSizeStr))
				if e = receiveData2File(jdkContentSize, jdkTmpSaveFile, conn, socketOperation); e != nil {
					return errors.New(fmt.Sprintf("%s: %s", outName, e.Error()))
				}

				if err = jdkModel.Create(&jdk).Error; err != nil {
					return errors.New("保存jdk信息失败")
				}
				renameMap[jdkTmpSaveFile] = jdkRealPath
			}
		}

		renameMapSize := len(renameMap)
		handleSize := 0
		if renameMapSize > 0 {
			socketOperation.SendMsg([]byte("正在保存文件"))
			socketOperation.SendMsg([]byte(strconv.FormatInt(int64(renameMapSize), 10)))
			for k, v := range renameMap {
				handleSize += 1
				dir := filepath.Dir(v)
				_ = os.MkdirAll(dir, 0777)
				if err = copyFile(k, v); err != nil {
					return errors.New("文件保存失败")
				}
				socketOperation.SendMsg([]byte(fmt.Sprintf("%d", handleSize)))
				if _, err = socketOperation.ReadMsg(); err != nil {
					return err
				}
			}
		}

		socketOperation.SendMsg([]byte("!!!!!!"))

		return nil
	})

}

var syncRemoteService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	var (
		err                 error
		msg                 []byte
		syncInfo            = &syncInfoParam{}
		appName, appVersion string

		appInfoList    = make([]*vos.DbAppInfo, 0)
		appVersionList = make([]*vos.DbAppVersionInfo, 0)

		syncResult = &syncInfoResult{}

		appInfoModel      *gorm.DB
		appVersionModel   *gorm.DB
		appPluginModel    *gorm.DB
		appStartInfoModel *gorm.DB

		contentPath string

		fileContentFile []string

		stat os.FileInfo

		sign []byte
	)

	msg, err = socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	if err = json.Unmarshal(msg, &syncInfo); err != nil {
		return errors.New("转换同步信息失败")
	}

	if syncInfo.GOARCH != runtime.GOARCH || syncInfo.GOOS != runtime.GOOS {
		return errors.New(fmt.Sprintf("无法将 [%s - %s] 上的应用同步到 [%s - %s]", runtime.GOOS, runtime.GOARCH, syncInfo.GOOS, syncInfo.GOARCH))
	}

	appPluginModel = db.GetDb().Model(&vos.DbAppPlugin{})
	appStartInfoModel = db.GetDb().Model(&vos.DbAppStartInfo{})

	appName = syncInfo.AppName
	appVersion = syncInfo.AppVersion

	appInfoModel = db.GetDb().Model(&vos.DbAppInfo{})
	if appName != "" {
		appInfoModel = appInfoModel.Where(&vos.DbAppInfo{
			Name: appName,
		})
		appStartInfoModel = appStartInfoModel.Where(&vos.DbAppStartInfo{
			Name: appName,
		})
	}

	if err = appInfoModel.Find(&appInfoList).Error; err != nil || len(appInfoList) == 0 {
		return errors.New("查询应用列表失败")
	}

	appVersionModel = db.GetDb().Model(&vos.DbAppVersionInfo{})
	appVersionModel = appVersionModel.Where(&vos.DbAppVersionInfo{
		AppName: appName,
	})

	if appVersion != "" {
		appVersionModel = appVersionModel.Where(&vos.DbAppVersionInfo{
			Name: appVersion,
		})
		appStartInfoModel = appStartInfoModel.Where(&vos.DbAppStartInfo{
			Version: appVersion,
		})
	}
	if err = appVersionModel.Find(&appVersionList).Error; err != nil || len(appVersionList) == 0 {
		return errors.New("获取应用版本列表失败")
	}

	if syncInfo.All {
		syncInfo.App = true
		syncInfo.Version = true
		syncInfo.Jdk = true
	}

	if syncInfo.App {
		syncResult.AppInfos = appInfoList
	}

	if syncInfo.Version {

		versionList := make([]*appVersionInfo, 0, len(appVersionList))
		fileContentFile = make([]string, len(versionList))
		for i, version := range appVersionList {
			tmpPluginList := make([]*vos.DbAppPlugin, 0)
			if err = appPluginModel.Where(&vos.DbAppPlugin{
				AppName:    version.AppName,
				AppVersion: version.Name,
			}).Find(&tmpPluginList).Error; err != nil {
				continue
			}

			if len(tmpPluginList) > 0 {
				for pi, p := range tmpPluginList {
					if !utils.PubKeyVerifySign(consts.CaPubKey, p.Src(), p.Sign) {
						return errors.New("远程数据可能已被篡改,请先确认目标服务器数据正确")
					}

					contentPath, _, err = utils.Sm4DecryptContentPath(consts.CaPrivateKey, p.Content)
					if err != nil {
						return err
					}

					stat, err = os.Stat(contentPath)
					if err != nil {
						return errors.New("远程插件已丢失")
					}

					sizeStr := strconv.FormatInt(stat.Size(), 10)
					tmpPluginList[pi].Content = bytes.Join([][]byte{
						tmpPluginList[pi].Content[:113],
						[]byte(sizeStr),
					}, nil)
					sign, err = utils.PrivateKeySign(consts.CaPrivateKey, tmpPluginList[pi].Src())
					if err != nil {
						return errors.New("插件签名失败")
					}
					tmpPluginList[pi].Sign = sign
					fileContentFile = append(fileContentFile, contentPath)
				}
				appVersionList[i].PluginInfo = tmpPluginList
			}

			tmpVersionInfo := appVersionList[i]

			if !utils.PubKeyVerifySign(consts.CaPubKey, tmpVersionInfo.SignSrc(), tmpVersionInfo.Sign) {
				return errors.New("远程应用版本数据可能已被篡改, 请您先确认远程数据正确")
			}

			contentPath, _, err = utils.Sm4DecryptContentPath(consts.CaPrivateKey, tmpVersionInfo.Content)
			if err != nil {
				return err
			}

			stat, err = os.Stat(contentPath)
			if err != nil {
				return errors.New("远程应用已丢失")
			}

			sizeStr := strconv.FormatInt(stat.Size(), 10)
			tmpVersionInfo.Content = bytes.Join([][]byte{
				tmpVersionInfo.Content[:113],
				[]byte(sizeStr),
			}, nil)

			fileContentFile = append(fileContentFile, contentPath)

			tmpVersionInfo.PluginInfo = tmpPluginList
			appVer := &appVersionInfo{
				AppName:           tmpVersionInfo.AppName,
				Name:              tmpVersionInfo.Name,
				Desc:              tmpVersionInfo.Desc,
				CreateTime:        tmpVersionInfo.CreateTime,
				ContentMd5:        tmpVersionInfo.ContentMd5,
				ContentSha1:       tmpVersionInfo.ContentSha1,
				EnvConfig:         tmpVersionInfo.EnvConfig,
				EnvConfigInfos:    tmpVersionInfo.EnvConfigInfos,
				JarPass:           tmpVersionInfo.JarPass,
				Content:           tmpVersionInfo.Content,
				PluginInfos:       tmpVersionInfo.PluginInfo,
				JdkStartArgsBytes: tmpVersionInfo.JdkStartArgsBytes,
			}

			sign, err = utils.PrivateKeySign(consts.CaPrivateKey, appVer.SignSrc())
			if err != nil {
				return errors.New("版本信息签名失败")
			}
			appVer.Sign = sign

			versionList = append(versionList, appVer)
		}
		syncResult.AppVersions = versionList
	}

	if syncInfo.Jdk {
		jdkInfoList := make([]*vos.DbJdkInfo, 0)
		if len(fileContentFile) == 0 {
			fileContentFile = make([]string, 0)
		}

		if err = db.GetDb().Model(&vos.DbJdkInfo{}).Find(&jdkInfoList).Error; err != nil && err != gorm.ErrRecordNotFound {
			return errors.New("查询jdk列表失败")
		}

		if len(jdkInfoList) == 0 {
			goto End
		}

		for i, jdk := range jdkInfoList {
			if !utils.PubKeyVerifySign(consts.CaPubKey, jdk.SignSrc(), jdk.Sign) {
				return errors.New("远程数据可能已被篡改,请先确认目标服务器数据正确")
			}

			contentPath, _, err = utils.Sm4DecryptContentPath(consts.CaPrivateKey, jdk.Content)
			if err != nil {
				return err
			}

			stat, err = os.Stat(contentPath)
			if err != nil {
				return errors.New("远程JDK已丢失")
			}

			sizeStr := strconv.FormatInt(stat.Size(), 10)
			jdkInfoList[i].Content = bytes.Join([][]byte{
				jdkInfoList[i].Content[:113],
				[]byte(sizeStr),
			}, nil)

			sign, err = utils.PrivateKeySign(consts.CaPrivateKey, jdkInfoList[i].SignSrc())
			if err != nil {
				return errors.New("jdk签名失败")
			}
			jdkInfoList[i].Sign = sign
			fileContentFile = append(fileContentFile, contentPath)
		}

		syncResult.JdkInfos = jdkInfoList
	}
End:
	tmpDir, err := ioutil.TempDir("", "syncJsonFile*")
	if err != nil {
		return errors.New("创建临时目录失败")
	}
	defer os.RemoveAll(tmpDir)
	dataJsonFile := filepath.Join(tmpDir, "tmpData")
	create, err := os.Create(dataJsonFile)
	if err != nil {
		return errors.New("创建临时传输文件失败")
	}
	defer create.Close()

	encoder := json.NewEncoder(create)
	if err = encoder.Encode(syncResult); err != nil {
		return errors.New("转换传输数据失败")
	}

	stat, err = create.Stat()
	if err != nil {
		return errors.New("获取传输数据状态失败")
	}

	size := stat.Size()
	socketOperation.SendMsg([]byte(strconv.FormatInt(size, 10)))
	_, err = socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	buffer := make([]byte, 1024*1024)

	if err = sendSyncFile2Client(dataJsonFile, socketOperation, buffer); err != nil {
		return err
	}

	for _, p := range fileContentFile {
		if err = sendSyncFile2Client(p, socketOperation, buffer); err != nil {
			return err
		}
	}

	return nil
}

func sendSyncFile2Client(p string, socketOperation *SocketOperation, buffer []byte) error {
	var (
		file *os.File
		err  error
		read int
	)

	file, err = os.OpenFile(p, os.O_RDONLY, 0666)
	if err != nil {
		return errors.New("打开临时传输文件失败")
	}
	defer file.Close()

	for {
		read, err = file.Read(buffer)
		if err == io.EOF {
			return nil
		}

		if err != nil {
			return errors.New("读取传输文件失败")
		}

		socketOperation.SendMsg(buffer[:read])
		_, err = socketOperation.ReadMsg()
		if err != nil {
			return err
		}
	}
}

func receiveData2byte(readSize int64, conn *socket.Conn, socketOperation *SocketOperation) ([]byte, error) {
	var (
		receiveSize int64 = 0
		tmpData     []byte

		buffer = &bytes.Buffer{}

		err error
	)

	for receiveSize < readSize {
		tmpData, err = conn.ReadMsg()
		if err != nil {
			return nil, err
		}
		receiveSize += int64(len(tmpData))
		_, _ = buffer.Write(tmpData)
		if err = conn.WriteDataStr("ok"); err != nil {
			return nil, err
		}
		socketOperation.SendMsg([]byte(fmt.Sprintf("%d", receiveSize)))
		if _, err = socketOperation.ReadMsg(); err != nil {
			return nil, err
		}
	}

	return buffer.Bytes(), nil

}

func receiveData2File(readSize int64, path string, conn *socket.Conn, socketOperation *SocketOperation) error {
	var (
		receiveSize int64 = 0
		tmpData     []byte

		file *os.File

		err error
	)

	_ = os.RemoveAll(path)
	file, err = os.Create(path)
	if err != nil {
		return errors.New("创建文件失败")
	}
	defer file.Close()

	for receiveSize < readSize {
		tmpData, err = conn.ReadMsg()
		if err != nil {
			return err
		}
		receiveSize += int64(len(tmpData))
		_, _ = file.Write(tmpData)
		if err = conn.WriteDataStr("ok"); err != nil {
			return err
		}
		socketOperation.SendMsg([]byte(fmt.Sprintf("%d", receiveSize)))
		if _, err = socketOperation.ReadMsg(); err != nil {
			return err
		}
	}

	return nil

}

func copyFile(srcFilePath, distFilePath string) error {
	srcFile, err := os.OpenFile(srcFilePath, os.O_RDONLY, 0666)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	distFile, err := os.Create(distFilePath)
	if err != nil {
		return err
	}
	defer distFile.Close()

	_, err = io.Copy(distFile, srcFile)
	return err
}
