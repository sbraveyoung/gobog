package mysql

import (
	"database/sql"

	"github.com/SmartBrave/gobog/pkg/config"
	_ "github.com/go-sql-driver/mysql"
)

type MySQL struct {
	db *sql.DB
}

func (m *MySQL) Init(c *config.Config) error {
	var err error
	//m.db, err = sql.Open("mysql", c.Mysql.User+":"+c.Mysql.Passwd+"@/"+c.Mysql.Db)
	return err
}

func (m *MySQL) QueryRow(query string) *sql.Row {
	return m.db.QueryRow(query)
}
