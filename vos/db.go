package vos

import (
	"bytes"
	"time"
)

type AppExecType int

const (
	AppExecTypePlugin AppExecType = iota
	AppExecTypeCmd
)

type AppRestartType string

const (
	AppRestartTypeAlways      AppRestartType = "always"
	AppRestartTypeErrorAuto   AppRestartType = "auto"
	AppRestartTypeErrorAny    AppRestartType = "error-any"
	AppRestartTypeErrorApp    AppRestartType = "error-app"
	AppRestartTypeErrorPlugin AppRestartType = "error-plugin"
)

type AppPluginType string

const (
	AppPluginTypeListener AppPluginType = "listener"
	AppPluginTypeNormal   AppPluginType = "normal"
	AppPluginTypeBefore   AppPluginType = "before"
	AppPluginTypeAfter    AppPluginType = "after"
)

// DbAppStartInfo app启动信息
type DbAppStartInfo struct {
	// Name app信息
	Name string `json:"name,omitempty" yaml:"-"`
	// Version 启动的版本号
	Version string `json:"version,omitempty" yaml:"version"`
	// versionInfo 版本信息
	VersionInfo *DbAppVersionInfo `gorm:"-" json:"-" yaml:"-"`
	// EnvConfig 环境配置
	EnvConfig      []*AppConfig `gorm:"-" json:"envConfig,omitempty" yaml:"envConfigs,omitempty"`
	EnvConfigBytes []byte       `json:"-" yaml:"-"`
	// JdkPath 本地jdk路径
	JdkPath string `json:"jdkPath,omitempty" yaml:"jdkPath,omitempty"`
	// JdkPackName 管理器内jdk的名称
	JdkPackName string     `json:"jdkPackName,omitempty" yaml:"jdkPackName,omitempty"`
	JdkPackInfo *DbJdkInfo `gorm:"-" json:"-" yaml:"-"`
	// JdkArgs jar包之前的jdk命令
	JdkArgs      []string `gorm:"-" json:"jdkFlags,omitempty" yaml:"-"`
	JdkArgsBytes []byte   `json:"-" yaml:"-"`
	// Args 应用参数
	Args      []string `gorm:"-" json:"args,omitempty" yaml:"args,omitempty"`
	ArgsBytes []byte   `json:"-" yaml:"-"`
	// Restart 是否跟随服务重启
	Restart AppRestartType `json:"restart,omitempty" yaml:"restart,omitempty"`
	// CopyFiles 要拷贝的文件
	CopyFiles     []string `gorm:"-" json:"copyFiles,omitempty" yaml:"copyFile,omitempty"`
	CopyFileBytes []byte   `json:"-" yaml:"-"`
	// SaveAppSuffix 保留应用文件后缀
	SaveAppSuffix        bool                         `json:"saveAppSuffix,omitempty" yaml:"saveAppSuffix,omitempty"`
	RunDir               string                       `json:"-" yaml:"-"`
	LogDir               string                       `gorm:"-" json:"-" yaml:"-"`
	Xmx                  string                       `json:"xmx,omitempty" yaml:"xmx,omitempty"`
	Xms                  string                       `json:"xms,omitempty" yaml:"xms,omitempty"`
	Xmn                  string                       `json:"xmn,omitempty" yaml:"xmn,omitempty"`
	PermSize             string                       `json:"permSize,omitempty" yaml:"permSize,omitempty"`
	MaxPermSize          string                       `json:"maxPermSize,omitempty" yaml:"maxPermSize,omitempty"`
	PluginEnvConfig      map[string]map[string]string `gorm:"-" json:"pluginEnvConfig,omitempty"`
	PluginEnvConfigBytes []byte                       `json:"-"`
}

type PluginInfo struct {
	Type      AppPluginType `json:"type,omitempty"`
	Md5       []byte        `json:"md5,omitempty"`
	Sha1      []byte        `json:"sha1,omitempty"`
	Sign      []byte        `json:"sign,omitempty"`
	EnvConfig []*AppConfig  `gorm:"-" json:"envConfig,omitempty"`
}

type AppConfig struct {
	Name       string `json:"name,omitempty" yaml:"name,omitempty"`
	Val        string `json:"val,omitempty" yaml:"val,omitempty"`
	Desc       string `json:"desc,omitempty" yaml:"-"`
	DefaultVal string `json:"defaultVal,omitempty" yaml:"-"`
}

type DbAppPlugin struct {
	AppName        string
	AppVersion     string
	Name           string
	Desc           string
	Content        []byte
	Md5            []byte
	Sha1           []byte
	Sign           []byte
	Type           AppPluginType
	EnvConfig      []*AppConfig `gorm:"-" json:"envConfig,omitempty"`
	EnvConfigBytes []byte       ` json:"-"`
}

func (d *DbAppPlugin) Src() []byte {
	return bytes.Join([][]byte{
		[]byte(d.AppName),
		[]byte(d.AppVersion),
		[]byte(d.Name),
		d.Content,
		d.Md5,
		d.Sha1,
		[]byte(d.Type),
		d.EnvConfigBytes,
	}, nil)
}

type DbSetting struct {
	Name    string `gorm:"primary_key" json:"name,omitempty"`
	Desc    string `json:"desc,omitempty"`
	Val     string `json:"val,omitempty"`
	StopApp bool   `json:"stopApp,omitempty"`
}

type DbAppInfo struct {
	// Name 应用名称
	Name               string              `gorm:"primary_key" json:"name,omitempty"`
	Desc               string              `json:"desc,omitempty"`
	CreateTime         time.Time           `json:"createTime,omitempty"`
	EndUpdateTime      time.Time           `json:"endUpdateTime,omitempty"`
	CurrentVersion     string              `json:"currentVersion,omitempty"`
	CurrentVersionInfo *DbAppVersionInfo   `gorm:"-" json:"currentVersionInfo,omitempty"`
	Versions           []*DbAppVersionInfo `gorm:"-" json:"versions,omitempty"`
}

type DbAppVersionInfo struct {
	AppName           string         `json:"-"`
	Name              string         `json:"name,omitempty"`
	Desc              string         `json:"desc,omitempty"`
	CreateTime        time.Time      `json:"createTime,omitempty"`
	EndUpdateTime     time.Time      `json:"endUpdateTime,omitempty"`
	ContentMd5        []byte         `json:"md5,omitempty"`
	ContentSha1       []byte         `json:"sha1,omitempty"`
	Sign              []byte         `json:"-"`
	EnvConfig         []byte         `json:"-"`
	EnvConfigInfos    []AppConfig    `gorm:"-" json:"envConfigInfos,omitempty"`
	JarPass           []byte         `json:"-"`
	Content           []byte         `json:"-"`
	Plugins           []byte         `json:"-"`
	PluginInfo        []*DbAppPlugin `gorm:"-" json:"pluginInfo,omitempty"`
	JdkStartArgs      []string       `gorm:"-" json:"jdkStartArgs,omitempty"`
	JdkStartArgsBytes []byte         `json:"-"`
	OS                string         `gorm:"-" json:"os,omitempty"`
	ARCH              string         `gorm:"-" json:"arch,omitempty"`
	//Config        []*AppConfig
	//ExecType      AppExecType
	//CopyFiles     []*AppCopyFileInfo
}

type DbLog struct {
	AppName    string `json:"appName,omitempty"`
	AppVersion string `json:"appVersion,omitempty"`
	Content    []byte `json:"content,omitempty"`
	AtDate     int64  `json:"atDate,omitempty"`
}

func (d *DbAppVersionInfo) SignSrc() []byte {
	return bytes.Join([][]byte{
		[]byte(d.Name),
		[]byte(d.AppName),
		[]byte(d.Desc),
		d.Content,
		d.ContentSha1,
		d.ContentMd5,
		d.JarPass,
		d.Plugins,
		d.JdkStartArgsBytes,
	}, nil)
}

type DbJdkInfo struct {
	Name          string    `json:"name,omitempty"`
	Desc          string    `json:"desc,omitempty"`
	CreateTime    time.Time `json:"createTime,omitempty"`
	EndUpdateTime time.Time `json:"endUpdateTime,omitempty"`
	MD5           []byte    `json:"md5,omitempty"`
	SHA1          []byte    `json:"sha1,omitempty"`
	Content       []byte    `json:"content,omitempty"`
	Sign          []byte    `json:"sign,omitempty"`
}

func (d *DbJdkInfo) SignSrc() []byte {
	return bytes.Join([][]byte{
		[]byte(d.Name),
		[]byte(d.Desc),
		d.MD5,
		d.SHA1,
		d.Content,
	}, nil)
}

type DbLogClearInfo struct {
	Name string
	Val  string
}
