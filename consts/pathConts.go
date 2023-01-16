package consts

import (
	"errors"
	"github.com/sirupsen/logrus"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
)

var (
	User       *user.User
	HomeDir    string
	DbPathDir  string
	LogPathDir string
	AppSaveDir string
	JdkSaveDir string
)

const currentUser = "{{ .UserName }}"
//const currentUser = "slx"

func init() {
	var err error
	if currentUser == "" {
		if User, err = user.Current(); err != nil {
			logrus.Error("获取当前用户失败 => " + err.Error())
			os.Exit(1)
		}
	} else {
		if User, err = user.Lookup(currentUser); err != nil {
			logrus.Error("获取当前用户[" + currentUser + "]失败 => " + err.Error())
			os.Exit(1)
		}
	}

	HomeDir = User.HomeDir
	DbPathDir = filepath.Join(HomeDir, ".devTools", ".data")
	AppSaveDir = filepath.Join(HomeDir, ".devTools", "appData")
	JdkSaveDir = filepath.Join(HomeDir, ".devTools", "jdkData")
	LogPathDir = filepath.Join(HomeDir, ".devTools", "logs")

	initBashConfig()
}

func GetUidAndGid() (int, int, error) {
	var (
		uid int
		gid int
		err error
	)

	if uid, err = strconv.Atoi(User.Uid); err != nil {
		return 0, 0, errors.New("获取用户uid失败")
	}

	if gid, err = strconv.Atoi(User.Gid); err != nil {
		return 0, 0, errors.New("获取用户gid失败")
	}

	return uid, gid, err
}
