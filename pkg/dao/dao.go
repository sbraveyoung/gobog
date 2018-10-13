package dao

import (
	"errors"
	"logs"

	"github.com/SmartBrave/gobog/pkg/config"
	"github.com/SmartBrave/gobog/pkg/mysql"
	"github.com/SmartBrave/gobog/pkg/structs"
)

type Dao struct {
	mysql *mysql.MySQL
}

var d Dao

func Init(c *config.Config) error {
	d.mysql = &mysql.MySQL{}
	return d.mysql.Init(c)
}

func VerifyLogin(user, passwd string) error {
	var id int
	sql := "select id from user where name='" + user + "' and password='" + passwd + "'"
	row := d.mysql.QueryRow(sql)
	if err := row.Scan(&id); err != nil {
		logs.Error(err)
		//should not return err directly,can expost information of mysql
		//return err
		return errors.New("user or passwd error.")
	}
	return nil
}

func ExistUser(u *structs.User) bool {
	return true
}

func Register(u *structs.User) error {
	if ExistUser(u) {
		return errors.New("This user has exist!")
	} else {
		//return Register(u)
	}
	return nil
}
