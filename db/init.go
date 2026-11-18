package db

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/go-sql-driver/mysql"
)

var dbInstance *sql.DB

var dbNameRe = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// Init 连接云托管 MySQL（环境变量沿用模板：MYSQL_ADDRESS、MYSQL_USERNAME、MYSQL_PASSWORD、MYSQL_DATABASE），
// 库和表不存在时自动建。
func Init() error {
	dataBase := os.Getenv("MYSQL_DATABASE")
	if dataBase == "" {
		dataBase = "xytx_mini"
	}
	if !dbNameRe.MatchString(dataBase) {
		return fmt.Errorf("MYSQL_DATABASE 只能包含字母、数字、下划线")
	}

	cfg := mysql.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = os.Getenv("MYSQL_ADDRESS")
	cfg.User = os.Getenv("MYSQL_USERNAME")
	cfg.Passwd = os.Getenv("MYSQL_PASSWORD")
	cfg.Timeout = 3 * time.Second
	cfg.ReadTimeout = 3 * time.Second
	cfg.WriteTimeout = 3 * time.Second
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Params = map[string]string{"charset": "utf8mb4", "time_zone": "'+00:00'"}

	// 先不指定库连一次，把库建出来
	boot, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return err
	}
	_, err = boot.Exec("CREATE DATABASE IF NOT EXISTS `" + dataBase + "` DEFAULT CHARSET utf8mb4")
	boot.Close()
	if err != nil {
		return fmt.Errorf("建库 %s 失败：%v", dataBase, err)
	}

	cfg.DBName = dataBase
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(time.Hour)
	if _, err := db.Exec(createSessionTable); err != nil {
		db.Close()
		return fmt.Errorf("建表 qrlogin_session 失败：%v", err)
	}
	dbInstance = db
	fmt.Printf("MySQL 已连接：%s/%s\n", cfg.Addr, dataBase)
	return nil
}

// Get 返回连接池
func Get() *sql.DB {
	return dbInstance
}

// 扫码会话表。时间一律用数据库的 UTC_TIMESTAMP(3) 写和比较，多个实例之间不受各自时钟影响。
const createSessionTable = `CREATE TABLE IF NOT EXISTS qrlogin_session (
  state        CHAR(32)     NOT NULL COMMENT '会话号，即小程序码的 scene',
  status       VARCHAR(16)  NOT NULL COMMENT 'waiting / scanned / done',
  env_version  VARCHAR(16)  NOT NULL DEFAULT '' COMMENT '出码时的小程序版本',
  created_at   DATETIME(3)  NOT NULL,
  expires_at   DATETIME(3)  NOT NULL,
  scanned_at   DATETIME(3)  NULL,
  done_at      DATETIME(3)  NULL,
  phone_masked VARCHAR(32)  NOT NULL DEFAULT '' COMMENT '只存打码后的号码',
  country_code VARCHAR(8)   NOT NULL DEFAULT '',
  openid_tail  VARCHAR(16)  NOT NULL DEFAULT '',
  last_error   VARCHAR(512) NOT NULL DEFAULT '',
  PRIMARY KEY (state),
  KEY idx_expires_at (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='扫码手机号登录联调测试的会话'`
