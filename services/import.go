package services

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/byzk-org/bypt-server/consts"
	"github.com/byzk-org/bypt-server/db"
	"github.com/byzk-org/bypt-server/helper"
	"github.com/byzk-org/bypt-server/utils"
	"github.com/byzk-org/bypt-server/vos"
	"github.com/jinzhu/gorm"
	"github.com/tjfoc/gmsm/sm2"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

var dataSplitByte = []byte(";")

var importService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	var (
		err                 error
		contentMd5          SliceBytes
		contentSha1         SliceBytes
		contentDataFilePath SliceBytes
		contentFilePath     string

		appInfo    *vos.DbAppInfo
		appVersion *vos.DbAppVersionInfo
	)

	contentMd5, err = socketOperation.ReadMsg()
	if err != nil {
		return errors.New("获取消息摘要失败")
	}
	contentSha1, err = socketOperation.ReadMsg()
	if err != nil {
		return errors.New("获取消息摘要失败")
	}
	contentDataFilePath, err = socketOperation.ReadMsg()
	if err != nil {
		return errors.New("获取程序文件失败")
	}

	contentFilePath = contentDataFilePath.String()
	md5Byte, err := utils.CalcMd5(contentFilePath)
	if err != nil {
		return errors.New("获取文件md5摘要失败")
	}

	sha1Byte, err := utils.CalcSha1(contentFilePath)
	if err != nil {
		return errors.New("获取文件sha1摘要失败")
	}

	if bytes.Equal(contentMd5, md5Byte) || bytes.Equal(sha1Byte, contentSha1) {
		return errors.New("数据摘要验证失败")
	}

	dir, err := utils.TmpDir()
	if err != nil {
		return errors.New("创建临时存储目录失败,请确保当前用户有对tmp目录的操作权限")
	}
	defer os.RemoveAll(dir)
	tmpFile := filepath.Join(dir, "appRunner")

	if err = utils.DeCompressGzip(contentFilePath, tmpFile); err != nil {
		return errors.New("解压数据文件失败")
	}

	contentEncFile, err := os.OpenFile(tmpFile, os.O_RDONLY, 0666)
	if err != nil {
		return errors.New("打开内容文件失败")
	}
	defer contentEncFile.Close()

	contentMd5 = make([]byte, md5.Size)
	contentSha1 = make([]byte, sha1.Size)
	contentSm4KeyEncrypt := make([]byte, 113)

	if err = readFileBySize(contentMd5, contentEncFile); err != nil {
		return err
	}

	if err = readFileBySize(contentSha1, contentEncFile); err != nil {
		return err
	}

	if err = readFileBySize(contentSm4KeyEncrypt, contentEncFile); err != nil {
		return err
	}

	contentSm4Key, err := utils.Sm2Decrypt(consts.CaPrivateKey, contentSm4KeyEncrypt)
	if err != nil {
		return errors.New("解析数据密钥失败")
	}

	contentFilePath = filepath.Join(dir, "tmpC")
	if err = utils.Sm4Decrypt2File(contentSm4Key, contentEncFile, contentFilePath); err != nil {
		return err
	}

	contentSumMd5, err := utils.CalcMd5(contentFilePath)
	if err != nil {
		return errors.New("获取数据集MD5摘要失败")
	}

	contentSumSha1, err := utils.CalcSha1(contentFilePath)
	if err != nil {
		return errors.New("获取数据集SHA1摘要失败")
	}

	if !bytes.Equal(contentMd5, contentSumMd5) ||
		!bytes.Equal(contentSha1, contentSumSha1) {
		return errors.New("数据可能已被损坏")
	}

	contentFile, err := os.OpenFile(contentFilePath, os.O_RDONLY, 0666)
	if err != nil {
		return errors.New("打开数据包文件失败")
	}
	defer contentFile.Close()

	cmdBytes, err := nextInfo(contentFile)
	if err != nil {
		return err
	}
	cmd := string(cmdBytes)

	hexAppInfoBytes, err := nextInfo(contentFile)
	if err != nil {
		return err
	}

	hexAppVersionBytes, err := nextInfo(contentFile)
	if err != nil {
		return err
	}

	appInfoJsonBytes, err := base64.StdEncoding.DecodeString(string(hexAppInfoBytes))
	if err != nil {
		return errors.New("转换应用信息失败")
	}

	appVersionJsonBytes, err := base64.StdEncoding.DecodeString(string(hexAppVersionBytes))
	if err != nil {
		return errors.New("转换应用版本信息失败")
	}

	appInfo = &vos.DbAppInfo{}
	appVersion = &vos.DbAppVersionInfo{}

	if err = json.Unmarshal(appInfoJsonBytes, &appInfo); err != nil {
		return errors.New("转换App信息失败")
	}

	if err = json.Unmarshal(appVersionJsonBytes, &appVersion); err != nil {
		return errors.New("转换App版本信息失败")
	}

	if helper.AppStatusMgr.IsStart(appInfo.Name) {
		return errors.New("应用已经启动，请关闭应用后在做尝试")
	}

	if appVersion.OS != runtime.GOOS || appVersion.ARCH != runtime.GOARCH {
		return errors.New("应用运行平台不正确, 此应用只能运行在 " + appVersion.OS + " : " + appVersion.ARCH + " 的平台架构下")
	}

	switch cmd {
	case "install-app":
		if err = installAppPack(contentFile, appInfo, appVersion); err != nil {
			return err
		}
	case "install-plugin":
		if err = installPlugin(contentFile, appInfo, appVersion); err != nil {
			return err
		}
	case "install-jdk":
		if err = installJdk(contentFile, appInfo); err != nil {
			return err
		}
	default:
		return errors.New("未识别的操作码")
	}
	socketOperation.SendMsg([]byte("ok"))
	return nil
}

// readFileBySize 读取文件通过大小
func readFileBySize(data []byte, file *os.File) error {
	dataLen := len(data)
	if dataLen == 0 {
		return errors.New("要读取的数据长度不能为空")
	}
	readSize := 0
	for {
		read, err := file.Read(data)
		if err != nil {
			return errors.New("文件读取失败 => " + err.Error())
		}
		readSize += read
		if readSize < dataLen {
			continue
		}
		return nil
	}
}

// installJdk 安装jdk
func installJdk(contentFile *os.File, jdkInfo *vos.DbAppInfo) error {
	GlobalOperationLock.Lock()
	defer GlobalOperationLock.Unlock()
	jdkSavePathSetting := &vos.DbSetting{}
	if err := db.GetDb().Where(&vos.DbSetting{
		Name: consts.DbSettingJdkSaveDir,
	}).First(&jdkSavePathSetting).Error; err != nil {
		return errors.New("获取jdk文件保存目录失败")
	}
	_ = os.MkdirAll(jdkSavePathSetting.Val, 0777)

	dir, err := utils.TmpDir()
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	cmd, err := nextInfo(contentFile)
	if err != nil {
		return errors.New("获取包指令失败")
	}

	if string(cmd) != "jdk" {
		return errors.New("非法指令")
	}

	jdkPath := filepath.Join(dir, "j")
	if err = decryptFrame2File(contentFile, jdkPath); err != nil {
		return err
	}

	jdkFileMd5Sum, err := utils.CalcMd5(jdkPath)
	if err != nil {
		return errors.New("计算jdk文件md5摘要失败")
	}

	jdkFileSha1Sum, err := utils.CalcSha1(jdkPath)
	if err != nil {
		return errors.New(" 计算jdk文件sha1摘要失败")
	}

	key := utils.Sm4RandomKey()

	jdkSavePath := filepath.Join(jdkSavePathSetting.Val, hex.EncodeToString(jdkFileMd5Sum)+hex.EncodeToString(jdkFileSha1Sum))

	if err = utils.Sm4Encrypt2File(key, jdkPath, jdkSavePath); err != nil {
		return errors.New("保存jdk文件失败")
	}

	encryptPath, err := utils.Sm4Encrypt(key, []byte(jdkSavePath))
	if err != nil {
		return errors.New("生成保护密钥失败")
	}

	encryptKey, err := utils.Sm2Encrypt(consts.CaPubKey, key)
	if err != nil {
		return errors.New("生成保护密钥失败")
	}

	srcJdkInfo := &vos.DbJdkInfo{}

	if err = db.GetDb().Where(&vos.DbJdkInfo{
		Name: jdkInfo.Name,
	}).First(&srcJdkInfo).Error; err != nil && err != gorm.ErrRecordNotFound {
		return errors.New("查询原数据失败")
	}

	saveJdkInfo := &vos.DbJdkInfo{
		Name:          jdkInfo.Name,
		Desc:          jdkInfo.Desc,
		EndUpdateTime: time.Now(),
		MD5:           jdkFileMd5Sum,
		SHA1:          jdkFileSha1Sum,
		Content: bytes.Join([][]byte{
			encryptKey,
			encryptPath,
		}, nil),
	}

	sign, err := utils.PrivateKeySign(consts.CaPrivateKey, saveJdkInfo.SignSrc())
	if err != nil {
		return errors.New("生成数据签名失败")
	}

	saveJdkInfo.Sign = sign
	saveJdkInfo.CreateTime = saveJdkInfo.EndUpdateTime

	if srcJdkInfo.Name != "" {
		saveJdkInfo.CreateTime = srcJdkInfo.CreateTime
		if err = db.GetDb().Model(&vos.DbJdkInfo{}).Where(&vos.DbJdkInfo{
			Name: jdkInfo.Name,
		}).Update(&saveJdkInfo).Error; err != nil {
			return errors.New("保存jdk信息失败")
		}
		return nil
	}

	if err = db.GetDb().Create(&saveJdkInfo).Error; err != nil {
		return errors.New("保存jdk信息失败")
	}
	return nil
}

// installPlugin 安装插件
func installPlugin(contentFile *os.File, appInfo *vos.DbAppInfo, appVersionInfo *vos.DbAppVersionInfo) error {
	GlobalOperationLock.Lock()
	defer GlobalOperationLock.Unlock()
	var (
		sign []byte
	)

	return db.GetDb().Transaction(func(tx *gorm.DB) error {
		appInfoModel := tx.Model(&vos.DbAppInfo{})
		appVersionInfoModel := tx.Model(&vos.DbAppVersionInfo{})

		srcAppInfo := &vos.DbAppInfo{}
		if err := appInfoModel.Where(&vos.DbAppInfo{
			Name: appInfo.Name,
		}).First(&srcAppInfo).Error; err != nil || srcAppInfo.Name == "" {
			return errors.New("为找到对应应用，请您确认应用已导入")
		}

		srcAppVersionInfo := &vos.DbAppVersionInfo{}
		if err := appVersionInfoModel.Where(&vos.DbAppVersionInfo{
			AppName: appInfo.Name,
			Name:    appVersionInfo.Name,
		}).First(&srcAppVersionInfo).Error; err != nil || srcAppVersionInfo.Name == "" {
			return errors.New("为找到对应应用，请您确认应用已导入")
		}

		if !utils.PubKeyVerifySign(consts.CaPubKey, srcAppVersionInfo.SignSrc(), srcAppVersionInfo.Sign) {
			return errors.New("数据校验失败, 数据可能已被篡改")
		}

		saveDirSetting := &vos.DbSetting{}
		if err := tx.Where(&vos.DbSetting{
			Name: consts.DbSettingAppSaveDir,
		}).First(&saveDirSetting).Error; err != nil {
			return errors.New("查询保存目录失败")
		}

		saveDir := filepath.Join(saveDirSetting.Val, srcAppInfo.Name, srcAppVersionInfo.Name)

		plugins, err := parsePlugin(contentFile, saveDir, appInfo.Name, appVersionInfo.Name, consts.CaPubKey)
		if err != nil {
			return err
		}

		if err = tx.Where(&vos.DbAppPlugin{
			AppName:    appInfo.Name,
			AppVersion: appVersionInfo.Name,
		}).Delete(&vos.DbAppPlugin{}).Error; err != nil {
			return errors.New("删除原有插件失败")
		}
		if len(plugins) == 0 {
			return nil
		}

		srcAppVersionInfo.Plugins = make([]byte, 0, len(plugins)*(md5.Size+sha1.Size))
		for _, plugin := range plugins {
			srcAppVersionInfo.Plugins = append(srcAppVersionInfo.Plugins, plugin.Md5...)
			srcAppVersionInfo.Plugins = append(srcAppVersionInfo.Plugins, plugin.Sha1...)
			sign, err = utils.PrivateKeySign(consts.CaPrivateKey, plugin.Src())
			if err != nil {
				return errors.New("插件制作签名失败")
			}
			plugin.Sign = sign
			if err = tx.Create(&plugin).Error; err != nil {
				return errors.New("保存插件信息是失败")
			}
		}

		srcAppInfo.EndUpdateTime = time.Now()
		srcAppVersionInfo.EndUpdateTime = time.Now()

		sign, err = utils.PrivateKeySign(consts.CaPrivateKey, srcAppVersionInfo.SignSrc())
		if err != nil {
			return errors.New("生成数据签名失败")
		}

		srcAppVersionInfo.Sign = sign

		if err = appInfoModel.Where(&vos.DbAppInfo{
			Name: appInfo.Name,
		}).Update(&srcAppInfo).Error; err != nil {
			return errors.New("更新应用信息失败")
		}
		if err = appVersionInfoModel.Where(&vos.DbAppVersionInfo{
			AppName: appInfo.Name,
			Name:    appVersionInfo.Name,
		}).Update(&srcAppVersionInfo).Error; err != nil {
			return errors.New("更新应用信息失败")
		}
		return nil
	})
	//parsePlugin(contentFile)
}

// installAppPack 安装应用包
func installAppPack(contentFile *os.File, appInfo *vos.DbAppInfo, appVersionInfo *vos.DbAppVersionInfo) error {
	GlobalOperationLock.Lock()
	defer GlobalOperationLock.Unlock()
	var (
		err                    error
		tmpDir                 string
		hexJarPassBytes        []byte
		jarPassJsonBytes       []byte
		jarPassEnc             []byte
		cmdByte                []byte
		encryptContentFilePath []byte
		encryptAppContentKey   []byte
		plugins                []*vos.DbAppPlugin
		contentFileMd5         []byte
		contentFileSha1        []byte
		sign                   []byte
	)
	saveDirSetting := &vos.DbSetting{}

	if err = db.GetDb().Where(&vos.DbSetting{
		Name: consts.DbSettingAppSaveDir,
	}).First(&saveDirSetting).Error; err != nil {
		return errors.New("获取程序保存目录失败")
	}

	_ = os.MkdirAll(saveDirSetting.Val, 0777)

	tmpDir, err = utils.TmpDir()
	if err != nil {
		return errors.New("创建临时目录失败")
	}
	defer os.RemoveAll(tmpDir)

	return db.GetDb().Transaction(func(tx *gorm.DB) error {

		appInfoModel := tx.Model(&vos.DbAppInfo{})
		appVersionInfoModel := tx.Model(&vos.DbAppVersionInfo{})

		srcAppInfo := &vos.DbAppInfo{}

		if err = appInfoModel.Where(&vos.DbAppInfo{
			Name: appInfo.Name,
		}).First(&srcAppInfo).Error; err != nil && err != gorm.ErrRecordNotFound {
			return errors.New("获取源版本信息失败")
		}

		appVersionInfoModel.Where(&vos.DbAppVersionInfo{
			Name:    appVersionInfo.Name,
			AppName: appInfo.Name,
		}).Delete(&vos.DbAppVersionInfo{
			Name:    appVersionInfo.Name,
			AppName: appInfo.Name,
		})

		hexJarPassBytes, err = nextInfo(contentFile)
		if err != nil {
			return err
		}

		jarPassJsonBytes, err = base64.StdEncoding.DecodeString(string(hexJarPassBytes))
		if err != nil {
			return errors.New("获取运行密钥失败")
		}

		jarPassEnc, err = utils.Sm2Encrypt(consts.CaPubKey, jarPassJsonBytes)
		if err != nil {
			return errors.New("加密运行密钥失败")
		}

		appVersionInfo.JarPass = jarPassEnc

		cmdByte, err = nextInfo(contentFile)
		if err != nil {
			return errors.New("获取包命令失败")
		}

		cmdStr := string(cmdByte)
		if cmdStr != "jar" {
			return errors.New("获取运行程序失败")
		}

		contentTmpFilePath := filepath.Join(tmpDir, "dc")

		err = decryptFrame2File(contentFile, contentTmpFilePath)
		if err != nil {
			return err
		}

		contentFileMd5, err = utils.CalcMd5(contentTmpFilePath)
		if err != nil {
			return errors.New("获取文件内容MD5摘要失败")
		}

		contentFileSha1, err = utils.CalcSha1(contentTmpFilePath)
		if err != nil {
			return errors.New("获取文件内容SHA1摘要失败")
		}

		contentFileDir := filepath.Join(saveDirSetting.Val, appInfo.Name, appVersionInfo.Name)
		_ = os.MkdirAll(contentFileDir, 0777)

		contentFilePath := filepath.Join(contentFileDir, "run")

		contentSm4Key := utils.Sm4RandomKey()
		if err = utils.Sm4Encrypt2File(contentSm4Key, contentTmpFilePath, contentFilePath); err != nil {
			return errors.New("转换源文件格式失败")
		}
		_ = os.RemoveAll(contentTmpFilePath)

		encryptContentFilePath, err = utils.Sm4Encrypt(contentSm4Key, []byte(contentFilePath))
		if err != nil {
			return errors.New("转换路径失败")
		}

		encryptAppContentKey, err = utils.Sm2Encrypt(consts.CaPubKey, contentSm4Key)
		if err != nil {
			return errors.New("保护密钥生成失败")
		}

		appVersionInfo.Content = bytes.Join([][]byte{
			encryptAppContentKey,
			encryptContentFilePath,
		}, nil)

		plugins, err = parsePlugin(contentFile, contentFileDir, appInfo.Name, appVersionInfo.Name, consts.CaPubKey)
		if err != nil {
			return err
		}

		if len(plugins) > 0 {
			if err = tx.Where(&vos.DbAppPlugin{
				AppName:    appInfo.Name,
				AppVersion: appVersionInfo.Name,
			}).Delete(&vos.DbAppPlugin{}).Error; err != nil {
				return errors.New("删除原有插件列表失败")
			}
			appVersionInfo.Plugins = make([]byte, 0, len(plugins)*(md5.Size+sha1.Size))
			for _, plugin := range plugins {
				appVersionInfo.Plugins = append(appVersionInfo.Plugins, plugin.Md5...)
				appVersionInfo.Plugins = append(appVersionInfo.Plugins, plugin.Sha1...)
				if err = tx.Create(&plugin).Error; err != nil {
					return errors.New("保存插件信息失败")
				}
			}

		}

		appVersionInfo.AppName = appInfo.Name
		appVersionInfo.ContentMd5 = contentFileMd5
		appVersionInfo.ContentSha1 = contentFileSha1
		appVersionInfo.CreateTime = time.Now()
		appVersionInfo.EndUpdateTime = appVersionInfo.CreateTime

		if len(appVersionInfo.EnvConfigInfos) > 0 {
			appVersionInfo.EnvConfig, err = json.Marshal(appVersionInfo.EnvConfigInfos)
			if err != nil {
				return errors.New("转换插件信息失败")
			}
		}

		if len(appVersionInfo.JdkStartArgs) == 0 {
			appVersionInfo.JdkStartArgs = []string{"-jar"}
		}

		appVersionInfo.JdkStartArgsBytes, err = json.Marshal(appVersionInfo.JdkStartArgs)
		if err != nil {
			return errors.New("转换java启动参数失败")
		}

		sign, err = utils.PrivateKeySign(consts.CaPrivateKey, appVersionInfo.SignSrc())
		if err != nil {
			return errors.New("生成数据签名失败")
		}
		appVersionInfo.Sign = sign
		if err = appVersionInfoModel.Create(&appVersionInfo).Error; err != nil {
			return errors.New("保存应用版本信息失败")
		}

		if srcAppInfo.Name == "" {
			appInfo.CreateTime = appVersionInfo.CreateTime
			appInfo.EndUpdateTime = appVersionInfo.CreateTime
			appInfo.CurrentVersion = appVersionInfo.Name
			if err = appInfoModel.Create(&appInfo).Error; err != nil {
				return errors.New("保存应用信息失败")
			}
		} else {
			appInfo.EndUpdateTime = appVersionInfo.CreateTime
			if err = appInfoModel.Where(&vos.DbAppInfo{
				Name: srcAppInfo.Name,
			}).Update(&srcAppInfo).Error; err != nil {
				return errors.New("修改应用信息失败")
			}
		}

		return nil
	})

}

// nextInfo 读取下一段插件
func nextInfo(contentFile *os.File) ([]byte, error) {

	//lenBytes := make([]byte, 0, 8)
	nextInfoBytes := &bytes.Buffer{}
	tmpByte := make([]byte, 1, 1)
	for {
		s, err := contentFile.Read(tmpByte)
		if err == io.EOF {
			return nil, io.EOF
		}
		if err != nil {
			return nil, errors.New("读取数据失败")
		}

		if bytes.Equal(dataSplitByte, tmpByte[:s]) {
			break
		}

		nextInfoBytes.WriteByte(tmpByte[0])
	}
	return nextInfoBytes.Bytes(), nil
}

//decryptFrame2File 解密块到文件
func decryptFrame2File(contentFile *os.File, destFile string) error {
	var (
		s   int
		err error
	)
	sm4keyEnc := make([]byte, 113)
	_, err = contentFile.Read(sm4keyEnc)
	if err != nil {
		return errors.New("读取包长度失败")
	}

	sm4Key, err := utils.Sm2Decrypt(consts.CaPrivateKey, sm4keyEnc)
	if err != nil {
		return errors.New("解析保护密钥失败")
	}

	tmpDir, err := utils.TmpDir()
	if err != nil {
		return errors.New("创建临时目录失败")
	}
	defer os.RemoveAll(tmpDir)

	lenBytes := make([]byte, 0, 8)

	tmpByte := make([]byte, 1, 1)
	for {
		s, err = contentFile.Read(tmpByte)
		if err != nil {
			return errors.New("读取数据失败")
		}

		if bytes.Equal(dataSplitByte, tmpByte[:s]) {
			break
		}

		lenBytes = append(lenBytes, tmpByte[0])
	}

	dataLen64, err := strconv.ParseInt(string(lenBytes), 10, 64)
	if err != nil {
		return errors.New("转换长度失败")
	}
	dataLen := int(dataLen64)

	tmpEncFilePath := filepath.Join(tmpDir, "tmpEnc")
	tmpEncFile, err := os.Create(tmpEncFilePath)
	if err != nil {
		return errors.New("创建临时文件失败")
	}
	defer tmpEncFile.Close()

	writeSize := 0
	tmpBuffer := make([]byte, consts.Sm4EncDataLen)
	for {
		s, err = contentFile.Read(tmpBuffer)
		if err != nil {
			return errors.New("读取数据内容失败")
		}

		writeSize += s
		if dataLen < writeSize {
			d := writeSize - dataLen
			s = s - d
			writeSize = dataLen
			_, err = contentFile.Seek(int64(-d), 1)
			if err != nil {
				return errors.New("移动文件指针失败")
			}
		}

		_, err = tmpEncFile.Write(tmpBuffer[:s])
		if err != nil {
			return errors.New("写出数据文件失败")
		}

		if dataLen == writeSize {
			break
		}
	}

	_, err = tmpEncFile.Seek(0, 0)
	if err != nil {
		return errors.New("移动文件指针到开始失败")
	}

	err = utils.Sm4Decrypt2File(sm4Key, tmpEncFile, destFile)
	if err != nil {
		return errors.New("写出文件失败")
	}

	//return utils.Base64Decoder2File(base64TmpFilePath, destFile)
	return nil
}

// parsePlugin  转换插件
func parsePlugin(contentFile *os.File, pluginSaveDir, appName, appVersionName string, pubKey *sm2.PublicKey) ([]*vos.DbAppPlugin, error) {
	_ = os.MkdirAll(pluginSaveDir, 0777)
	var (
		allPlugins = make([]*vos.DbAppPlugin, 0)
		err        error
		dir        string
		cmdInfo    []byte
		plugin     *vos.DbAppPlugin
	)

	dir, err = utils.TmpDir()
	if err != nil {
		return nil, errors.New("创建插件临时中转目录失败")
	}
	defer os.RemoveAll(dir)

	tmpFile := filepath.Join(dir, "p")
	for {
		cmdInfo, err = nextInfo(contentFile)
		if err == io.EOF {
			return allPlugins, nil
		}

		s := string(cmdInfo)
		if "plugin" != s {
			return nil, errors.New("非法的命令集")
		}

		if err = decryptFrame2File(contentFile, tmpFile); err != nil {
			return nil, err
		}

		plugin, err = handlerPluginFile(appName, appVersionName, tmpFile, pluginSaveDir, pubKey)
		if err != nil {
			return nil, err
		}

		allPlugins = append(allPlugins, plugin)
	}
}

// handlerPluginFile 处理插件文件
func handlerPluginFile(appName, appVersionName, pluginFilePath, pluginSaveDir string, pubKey *sm2.PublicKey) (*vos.DbAppPlugin, error) {
	dir, err := utils.TmpDir()
	if err != nil {
		return nil, errors.New("创建程序临时运行目录失败")
	}
	defer os.RemoveAll(dir)

	saveDir := filepath.Join(pluginSaveDir, "p")
	_ = os.MkdirAll(saveDir, 0777)

	file, err := os.OpenFile(pluginFilePath, os.O_RDONLY, 0666)
	if err != nil {
		return nil, errors.New("打开插件文件失败")
	}
	defer file.Close()

	pluginInfoLenBytes, err := nextInfo(file)
	if err != nil {
		return nil, errors.New("读取插件信息失败")
	}

	pluginInfoLen, err := strconv.ParseInt(string(pluginInfoLenBytes), 10, 64)
	if err != nil {
		return nil, errors.New("获取插件描述信息长度失败")
	}

	pluginInfoJsonBytes := make([]byte, pluginInfoLen, pluginInfoLen)
	if err = readFileBySize(pluginInfoJsonBytes, file); err != nil {
		return nil, errors.New("获取插件描述信息失败")
	}

	plugin := &vos.PluginInfo{}
	if err = json.Unmarshal(pluginInfoJsonBytes, plugin); err != nil {
		return nil, errors.New("获取插件描述信息失败")
	}

	tmpRunFilePath := filepath.Join(dir, utils.PathAddSuffix("r"))
	tmpRunFile, err := os.Create(tmpRunFilePath)
	if err != nil {
		return nil, errors.New("创建插件运行文件失败")
	}
	defer tmpRunFile.Close()

	_, err = io.Copy(tmpRunFile, file)
	if err != nil {
		return nil, errors.New("写出文件失败")
	}
	tmpRunFile.Close()

	if err = os.Chmod(tmpRunFilePath, 0777); err != nil {
		return nil, errors.New("修改插件可执行权失败")
	}
	env := os.Environ()
	env = append(env, "__cmd__=info")

	cmd := exec.Command(tmpRunFilePath)
	cmd.Env = env
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, errors.New("插件预运行失败")
	}
	output = output[:len(output)-2]
	outputJsonInfo, err := hex.DecodeString(string(output))
	if err != nil {
		return nil, errors.New("解析插件信息失败")
	}

	outputJson := make(map[string]string)
	if err = json.Unmarshal(outputJsonInfo, &outputJson); err != nil {
		return nil, errors.New("获取插件信息失败")
	}

	pluginType, ok := outputJson["type"]
	if !ok {
		return nil, errors.New("获取插件类型失败")
	}

	switch plugin.Type {
	case vos.AppPluginTypeListener:
	case vos.AppPluginTypeNormal:
	case vos.AppPluginTypeBefore:
	case vos.AppPluginTypeAfter:
	default:
		return nil, errors.New("未被支持的插件类型")
	}

	//if plugin.Type != vos.AppPluginTypeListener && plugin.Type != vos.AppPluginTypeNormal {
	//	return nil, errors.New("未被支持的插件类型")
	//}

	pluginDesc, ok := outputJson["desc"]
	if !ok {
		return nil, errors.New("获取插件描述失败")
	}

	if vos.AppPluginType(pluginType) != plugin.Type {
		return nil, errors.New("插件类别校验失败")
	}

	md5Sum, err := utils.CalcMd5(tmpRunFilePath)
	if err != nil {
		return nil, errors.New("获取插件md5摘要失败")
	}

	sha1Sum, err := utils.CalcSha1(tmpRunFilePath)
	if err != nil {
		return nil, errors.New("获取插件sha1摘要失败")
	}

	md5Str := hex.EncodeToString(md5Sum)
	sha1Str := hex.EncodeToString(sha1Sum)
	saveFilePath := filepath.Join(saveDir, md5Str+sha1Str)

	key := utils.Sm4RandomKey()
	if err = utils.Sm4Encrypt2File(key, tmpRunFilePath, saveFilePath); err != nil {
		return nil, err
	}

	encryptFilePath, err := utils.Sm4Encrypt(key, []byte(saveFilePath))
	if err != nil {
		return nil, errors.New("插件路径格式转换失败")
	}

	encryptKey, err := utils.Sm2Encrypt(pubKey, key)
	if err != nil {
		return nil, errors.New("生成数据保护密钥失败")
	}

	var envConfigBytes []byte = nil

	if len(plugin.EnvConfig) > 0 {
		envConfigBytes, _ = json.Marshal(plugin.EnvConfig)
	}

	endData := &vos.DbAppPlugin{
		AppName:    appName,
		AppVersion: appVersionName,
		Name:       hex.EncodeToString(md5Sum) + hex.EncodeToString(sha1Sum),
		Desc:       pluginDesc,
		Content: bytes.Join([][]byte{
			encryptKey,
			encryptFilePath,
		}, nil),
		Md5:            md5Sum,
		Sha1:           sha1Sum,
		Type:           plugin.Type,
		EnvConfig:      plugin.EnvConfig,
		EnvConfigBytes: envConfigBytes,
	}

	sign, err := utils.PrivateKeySign(consts.CaPrivateKey, endData.Src())
	if err != nil {
		return nil, errors.New("插件签名失败")
	}
	endData.Sign = sign
	return endData, nil
}
