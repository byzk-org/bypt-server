package services

import (
	"encoding/json"
	"errors"
	"github.com/byzk-org/bypt-server/consts"
	"github.com/byzk-org/bypt-server/db"
	"github.com/byzk-org/bypt-server/utils"
	"github.com/byzk-org/bypt-server/vos"
	"github.com/jinzhu/gorm"
	"os"
)

var jdkListService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	jdkList := make([]*vos.DbJdkInfo, 0)
	if err := db.GetDb().Find(&jdkList).Error; err != nil && err != gorm.ErrRecordNotFound {
		return errors.New("查询jdk信息列表失败")
	}
	marshal, _ := json.Marshal(jdkList)
	socketOperation.SendMsg(marshal)
	return nil
}

var jdkListNameService ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	msg, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}
	jdkInfo := &vos.DbJdkInfo{}
	if err = db.GetDb().Where(&vos.DbJdkInfo{
		Name: msg.String(),
	}).First(&jdkInfo).Error; err != nil {
		return errors.New("未识别的jdk信息")
	}
	marshal, _ := json.Marshal(jdkInfo)
	socketOperation.SendMsg(marshal)
	return err
}

var jdkRename ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	srcName, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	distName, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}

	srcJdkInfo := &vos.DbJdkInfo{}
	jdkInfoWhere := db.GetDb().Model(&vos.DbJdkInfo{}).Where(&vos.DbJdkInfo{
		Name: srcName.String(),
	})
	if err = jdkInfoWhere.First(&srcJdkInfo).Error; err != nil {
		return errors.New("未查询到原jdk信息")
	}

	if !utils.PubKeyVerifySign(consts.CaPubKey, srcJdkInfo.SignSrc(), srcJdkInfo.Sign) {
		return errors.New("数据可能已被篡改，请您尝试重新导入之后在尝试重命名功能")
	}

	srcJdkInfo.Name = distName.String()
	sign, err := utils.PrivateKeySign(consts.CaPrivateKey, srcJdkInfo.SignSrc())
	if err != nil {
		return errors.New("重新签名数据包失败")
	}
	srcJdkInfo.Sign = sign

	if err = jdkInfoWhere.Update(&srcJdkInfo).Error; err != nil {
		return errors.New("更新jdk名称失败")
	}
	return nil
}

var jdkRmAll ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	return db.GetDb().Transaction(func(tx *gorm.DB) error {
		jdkModel := tx.Model(&vos.DbJdkInfo{})
		if err := jdkModel.Delete(&vos.DbJdkInfo{}).Error; err != nil {
			return errors.New("删除现有jdk信息失败")
		}

		jdkSavePath := &vos.DbSetting{}
		if err := tx.Model(&vos.DbSetting{}).Where(&vos.DbSetting{
			Name: consts.DbSettingJdkSaveDir,
		}).First(&jdkSavePath).Error; err != nil {
			return errors.New("查询jdk保存目录失败")
		}

		if err := os.RemoveAll(jdkSavePath.Val); err != nil {
			return errors.New("删除jdk文件失败")
		}
		return nil
	})
}

var jdkRmName ServiceInterfaceFn = func(socketOperation *SocketOperation) error {
	var (
		p   string
		err error
	)
	name, err := socketOperation.ReadMsg()
	if err != nil {
		return err
	}
	return db.GetDb().Transaction(func(tx *gorm.DB) error {

		jdkModel := tx.Model(&vos.DbJdkInfo{})
		jdkInfoWhere := jdkModel.Where(&vos.DbJdkInfo{
			Name: name.String(),
		})
		srcJdkInfo := &vos.DbJdkInfo{}
		if err = jdkInfoWhere.First(&srcJdkInfo).Error; err != nil || srcJdkInfo.Name == "" {
			return errors.New("查询原jdk信息失败")
		}

		if err = jdkInfoWhere.Delete(&vos.DbJdkInfo{}).Error; err != nil {
			return errors.New("删除jdk信息失败")
		}

		if !utils.PubKeyVerifySign(consts.CaPubKey, srcJdkInfo.SignSrc(), srcJdkInfo.Sign) {
			return errors.New("数据被篡改，无法被删除，请尝试重新导入或者清除全部")
		}

		p, _, err = utils.Sm4DecryptContentPath(consts.CaPrivateKey, srcJdkInfo.Content)
		if err != nil {
			return err
		}

		if err = os.RemoveAll(p); err != nil {
			return errors.New("删除jdk文件失败")
		}

		return nil
	})
}
