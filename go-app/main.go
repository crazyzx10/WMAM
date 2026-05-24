package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-app/middleware"
	"go-app/models"
	"go-app/utils"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)

//go:embed frontend/*
var frontendFS embed.FS

type Config struct {
	Database      DatabaseConfig      `json:"database"`
	Settings      SettingsConfig      `json:"settings"`
	MiniPrograms  []MiniProgramConfig `json:"miniPrograms"`
}

type DatabaseConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

type SettingsConfig struct {
	StartDate string `json:"startDate"`
	APIBase   string `json:"apiBase"`
}

type MiniProgramConfig struct {
	Name     string `json:"name"`
	AppID    string `json:"appid"`
	AppSecret string `json:"appsecret"`
}

var (
	configPath   string
	mu           sync.Mutex
	httpClient   = &http.Client{Timeout: 30 * time.Second}
	tokenCaches  = sync.Map{}
	db           *sql.DB
)

func init() {
	exePath, err := os.Executable()
	if err != nil {
		exePath = os.Args[0]
	}
	exeDir := filepath.Dir(exePath)
	configPath = filepath.Join(exeDir, ".env")
}

func loadConfig() (Config, error) {
	var cfg Config
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{
				Database: DatabaseConfig{
					Host:     "",
					Port:     3306,
					User:     "",
					Password: "",
					Database: "",
				},
				Settings: SettingsConfig{
					StartDate: "",
					APIBase:   "https://api.weixin.qq.com/publisher/stat",
				},
				MiniPrograms: []MiniProgramConfig{},
			}, nil
		}
		return cfg, err
	}

	lines := strings.Split(string(data), "\n")
	kvMap := make(map[string]string)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eqIdx := strings.Index(line, "=")
		if eqIdx == -1 {
			continue
		}
		key := strings.TrimSpace(line[:eqIdx])
		value := strings.TrimSpace(line[eqIdx+1:])
		value = strings.Trim(value, "\"'")
		kvMap[key] = value
	}

	cfg.Database.Host = kvMap["DB_HOST"]
	if portStr, ok := kvMap["DB_PORT"]; ok {
		fmt.Sscanf(portStr, "%d", &cfg.Database.Port)
	}
	if cfg.Database.Port == 0 {
		cfg.Database.Port = 3306
	}
	cfg.Database.User = kvMap["DB_USER"]
	cfg.Database.Password = kvMap["DB_PASSWORD"]
	cfg.Database.Database = kvMap["DB_DATABASE"]

	cfg.Settings.APIBase = kvMap["API_BASE"]
	if cfg.Settings.APIBase == "" {
		cfg.Settings.APIBase = "https://api.weixin.qq.com/publisher/stat"
	}
	cfg.Settings.StartDate = kvMap["START_DATE"]

	var miniPrograms []MiniProgramConfig
	for i := 1; ; i++ {
		nameKey := fmt.Sprintf("MINI_PROGRAM_%d_NAME", i)
		appidKey := fmt.Sprintf("MINI_PROGRAM_%d_APPID", i)
		secretKey := fmt.Sprintf("MINI_PROGRAM_%d_APPSECRET", i)

		name, hasName := kvMap[nameKey]
		appid, hasAppid := kvMap[appidKey]
		secret, hasSecret := kvMap[secretKey]

		if !hasName || !hasAppid || !hasSecret {
			break
		}

		name = strings.TrimSpace(name)
		appid = strings.TrimSpace(appid)
		secret = strings.TrimSpace(secret)

		if name == "" || appid == "" || secret == "" {
			log.Printf("[loadConfig] 跳过无效的小程序配置 (索引: %d)", i)
			continue
		}

		miniPrograms = append(miniPrograms, MiniProgramConfig{
			Name:     name,
			AppID:    appid,
			AppSecret: secret,
		})
	}
	cfg.MiniPrograms = miniPrograms

	return cfg, nil
}

func saveConfig(cfg Config) error {
	var lines []string

	lines = append(lines, "# Database Configuration")
	lines = append(lines, fmt.Sprintf("DB_HOST=%s", cfg.Database.Host))
	lines = append(lines, fmt.Sprintf("DB_PORT=%d", cfg.Database.Port))
	lines = append(lines, fmt.Sprintf("DB_USER=%s", cfg.Database.User))
	lines = append(lines, fmt.Sprintf("DB_PASSWORD=%s", cfg.Database.Password))
	lines = append(lines, fmt.Sprintf("DB_DATABASE=%s", cfg.Database.Database))
	lines = append(lines, "")

	lines = append(lines, "# API Configuration")
	lines = append(lines, fmt.Sprintf("API_BASE=%s", cfg.Settings.APIBase))
	lines = append(lines, fmt.Sprintf("START_DATE=%s", cfg.Settings.StartDate))
	lines = append(lines, "")

	lines = append(lines, "# Mini Programs")
	for i, mp := range cfg.MiniPrograms {
		idx := i + 1
		lines = append(lines, fmt.Sprintf("MINI_PROGRAM_%d_NAME=%s", idx, mp.Name))
		lines = append(lines, fmt.Sprintf("MINI_PROGRAM_%d_APPID=%s", idx, mp.AppID))
		lines = append(lines, fmt.Sprintf("MINI_PROGRAM_%d_APPSECRET=%s", idx, mp.AppSecret))
		if i < len(cfg.MiniPrograms)-1 {
			lines = append(lines, "")
		}
	}

	data := strings.Join(lines, "\n")
	return os.WriteFile(configPath, []byte(data), 0644)
}

func getDBConnection(cfg Config) (*sql.DB, error) {
	port := 3306
	if cfg.Database.Port > 0 {
		port = cfg.Database.Port
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		port,
		cfg.Database.Database,
	)

	database, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	database.SetMaxOpenConns(10)
	database.SetMaxIdleConns(5)
	database.SetConnMaxLifetime(time.Minute * 30)
	database.SetConnMaxIdleTime(time.Minute * 10)

	return database, nil
}

func initDatabase(db *sql.DB) error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS mini_program (
			名称 VARCHAR(64) NOT NULL COMMENT '小程序名称',
			小程序ID VARCHAR(32) NOT NULL COMMENT '小程序AppID',
			小程序Secret VARCHAR(64) NOT NULL COMMENT '小程序AppSecret',
			是否启用 TINYINT DEFAULT 1 COMMENT '是否启用',
			创建时间 DATETIME DEFAULT CURRENT_TIMESTAMP,
			更新时间 DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (小程序ID)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='小程序配置表'`,
		`CREATE TABLE IF NOT EXISTS adunit_list (
			小程序名称 VARCHAR(64) NOT NULL COMMENT '小程序名称',
			小程序ID VARCHAR(32) NOT NULL COMMENT '小程序AppID',
			广告位唯一ID VARCHAR(64) NOT NULL COMMENT '广告位唯一ID',
			广告位名称 VARCHAR(128) COMMENT '广告位名称',
			广告位类型枚举 VARCHAR(64) COMMENT '广告位类型枚举',
			广告位类型 VARCHAR(32) COMMENT '广告位类型中文名',
			广告位类型值 VARCHAR(64) COMMENT '广告位类型',
			状态 VARCHAR(32) COMMENT '状态：ON=正常，OFF=暂停',
			状态名称 VARCHAR(16) COMMENT '状态中文名',
			广告位数字ID VARCHAR(64) COMMENT '广告位数字ID',
			广告尺寸 VARCHAR(128) COMMENT '广告尺寸',
			是否允许可播放 TINYINT COMMENT '是否允许可播放',
			视频最短时长 INT COMMENT '视频最短时长(秒)',
			视频最长时长 INT COMMENT '视频最长时间(秒)',
			模版类型列表 VARCHAR(256) COMMENT '模版类型列表',
			创建时间 DATETIME DEFAULT CURRENT_TIMESTAMP,
			更新时间 DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (小程序名称, 小程序ID, 广告位唯一ID)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='广告位清单表'`,
		`CREATE TABLE IF NOT EXISTS publisher_adpos_general (
			小程序名称 VARCHAR(64) NOT NULL COMMENT '小程序名称',
			小程序ID VARCHAR(32) NOT NULL COMMENT '小程序AppID',
			日期 DATE NOT NULL COMMENT '日期',
			广告位类型枚举 VARCHAR(64) COMMENT '广告位类型枚举',
			广告位类型 VARCHAR(32) COMMENT '广告位类型中文名',
			广告位数字ID VARCHAR(64) COMMENT '广告位数字ID',
			成功请求次数 INT COMMENT '成功请求次数',
			曝光量 INT COMMENT '曝光量',
			曝光率 VARCHAR(10) COMMENT '曝光率(%)',
			点击量 INT COMMENT '点击量',
			点击率 VARCHAR(10) COMMENT '点击率(%)',
			总收入分 INT COMMENT '总收入(分)',
			总收入元 DECIMAL(12,2) COMMENT '总收入(元)',
			千次曝光收入分 DECIMAL(10,2) COMMENT '千次曝光收入(分)',
			千次曝光收入元 DECIMAL(10,2) COMMENT '千次曝光收入(元)',
			创建时间 DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (小程序名称, 小程序ID, 日期, 广告位数字ID),
			KEY idx_appid_date (小程序ID, 日期)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='广告汇总数据表'`,
		`CREATE TABLE IF NOT EXISTS publisher_adunit_general (
			小程序名称 VARCHAR(64) NOT NULL COMMENT '小程序名称',
			小程序ID VARCHAR(32) NOT NULL COMMENT '小程序AppID',
			广告位唯一ID VARCHAR(64) NOT NULL COMMENT '广告位唯一ID',
			广告位名称 VARCHAR(128) COMMENT '广告位名称',
			日期 DATE NOT NULL COMMENT '日期',
			广告位类型枚举 VARCHAR(64) COMMENT '广告位类型枚举',
			广告位类型 VARCHAR(32) COMMENT '广告位类型中文名',
			广告位数字ID VARCHAR(64) COMMENT '广告位数字ID',
			成功请求次数 INT COMMENT '成功请求次数',
			曝光量 INT COMMENT '曝光量',
			曝光率 VARCHAR(10) COMMENT '曝光率(%)',
			点击量 INT COMMENT '点击量',
			点击率 VARCHAR(10) COMMENT '点击率(%)',
			总收入分 INT COMMENT '总收入(分)',
			总收入元 DECIMAL(12,2) COMMENT '总收入(元)',
			流量主收入分 INT COMMENT '流量主收入(分)',
			流量主收入元 DECIMAL(12,2) COMMENT '流量主收入(元)',
			代理商收入分 INT COMMENT '代理商收入(分)',
			千次曝光收入分 DECIMAL(10,2) COMMENT '千次曝光收入(分)',
			千次曝光收入元 DECIMAL(10,2) COMMENT '千次曝光收入(元)',
			是否智能广告 TINYINT COMMENT '是否智能广告',
			父模版类型 VARCHAR(32) COMMENT '父模版类型',
			创建时间 DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (小程序名称, 小程序ID, 广告位唯一ID, 日期),
			KEY idx_appid_date (小程序ID, 日期)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='广告细分数据表'`,
		`CREATE TABLE IF NOT EXISTS publisher_settlement (
			小程序名称 VARCHAR(64) NOT NULL COMMENT '小程序名称',
			小程序ID VARCHAR(32) NOT NULL COMMENT '小程序AppID',
			总预估收入分 BIGINT COMMENT '总预估收入(分)',
			总预估收入元 DECIMAL(14,2) COMMENT '总预估收入(元)',
			总已结算收入分 BIGINT COMMENT '总已结算收入(分)',
			总已结算收入元 DECIMAL(14,2) COMMENT '总已结算收入(元)',
			总罚金分 BIGINT COMMENT '总罚金(分)',
			总罚金元 DECIMAL(14,2) COMMENT '总罚金(元)',
			微信云开发总预估收入分 BIGINT COMMENT '微信云开发总预估收入(分)',
			微信云开发总已结算收入分 BIGINT COMMENT '微信云开发总已结算收入(分)',
			微信云开发总罚金分 BIGINT COMMENT '微信云开发总罚金(分)',
			数据拉取日期 DATE COMMENT '数据拉取日期',
			创建时间 DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (小程序名称, 小程序ID, 数据拉取日期),
			KEY idx_appid_fetch_date (小程序ID, 数据拉取日期)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='结算数据表'`,
		`CREATE TABLE IF NOT EXISTS fetch_log (
			小程序名称 VARCHAR(64) COMMENT '小程序名称',
			小程序ID VARCHAR(32) COMMENT '小程序AppID',
			拉取类型 VARCHAR(32) NOT NULL COMMENT '拉取类型',
			拉取日期 DATE COMMENT '拉取的数据日期',
			状态 VARCHAR(16) NOT NULL COMMENT '状态',
			记录数 INT COMMENT '拉取的记录数',
			错误信息 TEXT COMMENT '错误信息',
			创建时间 DATETIME DEFAULT CURRENT_TIMESTAMP,
			KEY idx_fetch_type (拉取类型),
			KEY idx_create_time (创建时间)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='数据拉取日志表'`,
		`CREATE TABLE IF NOT EXISTS users (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			username VARCHAR(50) NOT NULL UNIQUE COMMENT '用户名',
			password_hash VARCHAR(255) NOT NULL COMMENT '密码哈希',
			role ENUM('admin', 'user') NOT NULL DEFAULT 'user' COMMENT '角色',
			status ENUM('active', 'disabled') NOT NULL DEFAULT 'active' COMMENT '状态',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			last_login_at DATETIME COMMENT '最后登录时间',
			INDEX idx_username (username),
			INDEX idx_role (role)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户表'`,
		`CREATE TABLE IF NOT EXISTS operation_log (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			user_id BIGINT NOT NULL COMMENT '操作用户ID',
			username VARCHAR(50) NOT NULL COMMENT '操作用户名',
			operation_type VARCHAR(50) NOT NULL COMMENT '操作类型',
			operation_desc TEXT COMMENT '操作描述',
			ip_address VARCHAR(45) COMMENT 'IP地址',
			user_agent TEXT COMMENT '浏览器信息',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_user_id (user_id),
			INDEX idx_operation_type (operation_type),
			INDEX idx_created_at (created_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='操作日志表'`,
		`CREATE TABLE IF NOT EXISTS fetch_lock (
			id INT PRIMARY KEY DEFAULT 1,
			locked_by VARCHAR(50) COMMENT '锁定者用户名',
			locked_at DATETIME COMMENT '锁定时间',
			expires_at DATETIME COMMENT '锁过期时间',
			CONSTRAINT single_row CHECK (id = 1)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='拉取锁表'`,
		`CREATE TABLE IF NOT EXISTS fetch_progress (
			id INT PRIMARY KEY DEFAULT 1,
			current_program_index INT DEFAULT 0 COMMENT '当前处理的小程序索引',
			program_names TEXT COMMENT '逗号分隔的小程序名称列表',
			program_ids TEXT COMMENT '逗号分隔的小程序ID列表',
			adunit_list_status VARCHAR(50) DEFAULT 'pending' COMMENT 'pending/completed',
			summary_status VARCHAR(50) DEFAULT 'pending' COMMENT 'pending/completed',
			detail_status VARCHAR(50) DEFAULT 'pending' COMMENT 'pending/completed',
			settlement_status VARCHAR(50) DEFAULT 'pending' COMMENT 'pending/completed',
			current_data_type VARCHAR(50) COMMENT '当前正在拉取的数据类型',
			locked_by VARCHAR(50) COMMENT '当前锁定者',
			locked_at DATETIME COMMENT '锁定时间',
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			CONSTRAINT single_row CHECK (id = 1)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='拉取进度表'`,
	}

	for _, table := range tables {
		_, err := db.Exec(table)
		if err != nil {
			return err
		}
	}

	return nil
}

func initDefaultAdmin() error {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		passwordHash, err := utils.HashPassword("admin123")
		if err != nil {
			return err
		}
		_, err = db.Exec(`
			INSERT INTO users (username, password_hash, role, status)
			VALUES ('admin', ?, 'admin', 'active')
		`, passwordHash)
		if err != nil {
			return err
		}
		log.Println("✅ 默认管理员账户已创建: admin / admin123")
	}

	return nil
}

func loginHandler(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "参数错误")
		return
	}

	user, err := models.GetUserByUsername(db, req.Username)
	if err != nil {
		utils.Error(c, 401, "用户名或密码错误")
		return
	}

	if user.Status == "disabled" {
		utils.Error(c, 403, "账户已被禁用")
		return
	}

	if !utils.CheckPassword(req.Password, user.PasswordHash) {
		utils.Error(c, 401, "用户名或密码错误")
		_ = models.CreateOperationLog(db, user.ID, user.Username, "LOGIN_FAILED", "登录失败-密码错误", c.ClientIP(), c.GetHeader("User-Agent"))
		return
	}

	token, err := utils.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		utils.Error(c, 500, "生成Token失败")
		return
	}

	_ = models.UpdateLastLogin(db, user.ID)
	_ = models.CreateOperationLog(db, user.ID, user.Username, "LOGIN", "登录成功", c.ClientIP(), c.GetHeader("User-Agent"))

	utils.Success(c, gin.H{
		"token": token,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
		},
	})
}

func getCurrentUserHandler(c *gin.Context) {
	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")
	role, _ := c.Get("role")

	utils.Success(c, gin.H{
		"user": gin.H{
			"id":       userID,
			"username": username,
			"role":     role,
		},
	})
}

func logoutHandler(c *gin.Context) {
	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")
	_ = models.CreateOperationLog(db, userID.(int64), username.(string), "LOGOUT", "登出", c.ClientIP(), c.GetHeader("User-Agent"))
	utils.Success(c, nil)
}

func getUsersHandler(c *gin.Context) {
	users, err := models.GetAllUsers(db)
	if err != nil {
		utils.Error(c, 500, "获取用户列表失败")
		return
	}
	utils.Success(c, gin.H{"users": users})
}

func createUserHandler(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		Role     string `json:"role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "参数错误")
		return
	}

	passwordHash, err := utils.HashPassword(req.Password)
	if err != nil {
		utils.Error(c, 500, "密码加密失败")
		return
	}

	user, err := models.CreateUser(db, req.Username, passwordHash, req.Role)
	if err != nil {
		utils.Error(c, 500, "创建用户失败")
		return
	}

	adminID, _ := c.Get("user_id")
	adminName, _ := c.Get("username")
	_ = models.CreateOperationLog(db, adminID.(int64), adminName.(string), "USER_CREATE", fmt.Sprintf("创建用户: %s", req.Username), c.ClientIP(), c.GetHeader("User-Agent"))

	utils.Success(c, gin.H{"user": user})
}

func updateUserHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		utils.Error(c, 400, "无效的用户ID")
		return
	}

	var req struct {
		Username string `json:"username" binding:"required"`
		Status   string `json:"status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "参数错误")
		return
	}

	user, err := models.UpdateUser(db, id, req.Username, req.Status)
	if err != nil {
		utils.Error(c, 500, "更新用户失败")
		return
	}

	adminID, _ := c.Get("user_id")
	adminName, _ := c.Get("username")
	_ = models.CreateOperationLog(db, adminID.(int64), adminName.(string), "USER_UPDATE", fmt.Sprintf("更新用户: %s", req.Username), c.ClientIP(), c.GetHeader("User-Agent"))

	utils.Success(c, gin.H{"user": user})
}

func deleteUserHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		utils.Error(c, 400, "无效的用户ID")
		return
	}

	user, _ := models.GetUserByID(db, id)

	err = models.DeleteUser(db, id)
	if err != nil {
		utils.Error(c, 500, "删除用户失败")
		return
	}

	adminID, _ := c.Get("user_id")
	adminName, _ := c.Get("username")
	_ = models.CreateOperationLog(db, adminID.(int64), adminName.(string), "USER_DELETE", fmt.Sprintf("删除用户: %s", user.Username), c.ClientIP(), c.GetHeader("User-Agent"))

	utils.Success(c, nil)
}

func getDashboardStatsHandler(c *gin.Context) {
	userCount, _ := models.GetUserCount(db)
	
	miniProgramCount := int64(0)
	_ = db.QueryRow("SELECT COUNT(*) FROM mini_program").Scan(&miniProgramCount)

	todayFetchCount, _ := models.GetTodayFetchCount(db)

	lastFetchTime, _ := models.GetLastFetchTime(db)
	var lastFetchStr string
	if lastFetchTime != nil {
		lastFetchStr = lastFetchTime.Format("2006-01-02 15:04:05")
	}

	utils.Success(c, gin.H{
		"user_count":        userCount,
		"mini_program_count": miniProgramCount,
		"today_fetch_count": todayFetchCount,
		"last_fetch_time":   lastFetchStr,
	})
}

func getMiniProgramsHandler(c *gin.Context) {
	rows, err := db.Query("SELECT 名称, 小程序ID, 是否启用, 创建时间 FROM mini_program ORDER BY 创建时间 DESC")
	if err != nil {
		utils.Error(c, 500, "获取小程序列表失败")
		return
	}
	defer rows.Close()

	var programs []gin.H
	for rows.Next() {
		var name, appID string
		var enabled int
		var createdAt time.Time
		_ = rows.Scan(&name, &appID, &enabled, &createdAt)
		programs = append(programs, gin.H{
			"name":      name,
			"appid":     appID,
			"enabled":   enabled,
			"createdAt": createdAt,
		})
	}

	utils.Success(c, gin.H{"programs": programs})
}

func getLogsHandler(c *gin.Context) {
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")
	isAdmin := role == "admin"

	var offset int
	var limit int = 20
	if pageStr := c.Query("page"); pageStr != "" {
		page, _ := strconv.Atoi(pageStr)
		offset = (page - 1) * limit
	}

	logs, _, err := models.GetOperationLogs(db, userID.(int64), isAdmin, offset, limit)
	if err != nil {
		utils.Error(c, 500, "获取日志失败")
		return
	}

	utils.Success(c, gin.H{"logs": logs})
}

func getConfigHandler(c *gin.Context) {
	cfg, _ := loadConfig()
	utils.Success(c, cfg)
}

func saveConfigHandler(c *gin.Context) {
	var newCfg Config
	if err := c.ShouldBindJSON(&newCfg); err != nil {
		utils.Error(c, 400, "参数错误")
		return
	}

	oldCfg, _ := loadConfig()

	if newCfg.Database.Host == "" {
		newCfg.Database = oldCfg.Database
	} else {
		if newCfg.Database.Port == 0 {
			newCfg.Database.Port = 3306
		}
	}

	if len(newCfg.MiniPrograms) == 0 {
		newCfg.MiniPrograms = oldCfg.MiniPrograms
	} else {
		var validPrograms []MiniProgramConfig
		for _, p := range newCfg.MiniPrograms {
			name := strings.TrimSpace(p.Name)
			appid := strings.TrimSpace(p.AppID)
			secret := strings.TrimSpace(p.AppSecret)

			if name == "" || appid == "" || secret == "" {
				continue
			}
			validPrograms = append(validPrograms, MiniProgramConfig{Name: name, AppID: appid, AppSecret: secret})
		}
		newCfg.MiniPrograms = validPrograms
	}

	newCfg.Settings.StartDate = "2025-07-01"
	newCfg.Settings.APIBase = oldCfg.Settings.APIBase
	if newCfg.Settings.APIBase == "" {
		newCfg.Settings.APIBase = "https://api.weixin.qq.com/publisher/stat"
	}

	if err := saveConfig(newCfg); err != nil {
		utils.Error(c, 500, "保存配置失败")
		return
	}

	uid, _ := c.Get("user_id")
	uname, _ := c.Get("username")
	_ = models.CreateOperationLog(db, uid.(int64), uname.(string), "CONFIG_UPDATE", "更新配置", c.ClientIP(), c.GetHeader("User-Agent"))

	utils.Success(c, nil)
}

func testConnectionHandler(c *gin.Context) {
	var cfg DatabaseConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		utils.Error(c, 400, "参数错误")
		return
	}

	config, _ := loadConfig()
	config.Database = cfg

	database, err := getDBConnection(config)
	if err != nil {
		utils.Error(c, 500, err.Error())
		return
	}
	defer database.Close()

	if err := database.Ping(); err != nil {
		utils.Error(c, 500, err.Error())
		return
	}

	utils.Success(c, nil)
}

type ProgressState struct {
	CurrentProgram int
	TotalPrograms  int
	CurrentStep    int
	TotalSteps     int
	Message        string
}

func sendProgress(logChan chan<- string, state ProgressState) {
	percent := 0
	if state.TotalSteps > 0 {
		percent = (state.CurrentStep * 100) / state.TotalSteps
		if percent > 100 {
			percent = 100
		}
	}
	logChan <- fmt.Sprintf("[PROGRESS] %d %s", percent, state.Message)
}

func executeFetchHandler(c *gin.Context) {
	w := c.Writer
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	ctx := c.Request.Context()

	logChan := make(chan string, 100)
	doneChan := make(chan struct{})

	go func() {
		defer close(logChan)
		defer close(doneChan)

		mainStartTime := time.Now().UnixMilli()
		logChan <- "========== 微信小程序广告数据拉取开始 =========="
		logChan <- fmt.Sprintf("开始时间: %s", time.Now().Format("2006-01-02 15:04:05"))

		select {
		case <-ctx.Done():
			logChan <- "⚠️ 任务已中断"
			return
		default:
		}

		userID, _ := c.Get("user_id")
		username, _ := c.Get("username")

		locked, err := models.AcquireFetchLock(db, username.(string))
		if err != nil || !locked {
			lockInfo, _ := models.GetFetchLock(db)
			if lockInfo != nil && lockInfo.LockedBy != "" && lockInfo.ExpiresAt.After(time.Now()) {
				logChan <- fmt.Sprintf("❌ 数据拉取被 %s 锁定，请稍后再试", lockInfo.LockedBy)
			} else {
				logChan <- "❌ 获取拉取锁失败"
			}
			return
		}
		defer models.ReleaseFetchLock(db, username.(string))

		_ = models.CreateOperationLog(db, userID.(int64), username.(string), "FETCH_START", "开始数据拉取", c.ClientIP(), c.GetHeader("User-Agent"))

		cfg, err := loadConfig()
		if err != nil {
			logChan <- fmt.Sprintf("❌ 加载配置失败: %v", err)
			return
		}

		if cfg.Database.Host == "" {
			logChan <- "❌ 数据库配置为空，请先配置数据库"
			return
		}

		logChan <- "🔌 正在连接数据库..."
		database, err := getDBConnection(cfg)
		if err != nil {
			logChan <- fmt.Sprintf("❌ 连接数据库失败: %v", err)
			return
		}
		defer database.Close()

		if err := database.Ping(); err != nil {
			logChan <- fmt.Sprintf("❌ 数据库连接测试失败: %v", err)
			return
		}
		logChan <- "✅ 数据库连接成功"

		logChan <- "📝 正在初始化数据库表..."
		if err := initDatabase(database); err != nil {
			logChan <- fmt.Sprintf("❌ 初始化数据库失败: %v", err)
			return
		}
		logChan <- "✅ 数据库初始化完成"

		logChan <- fmt.Sprintf("小程序数量: %d", len(cfg.MiniPrograms))
		if len(cfg.MiniPrograms) == 0 {
			logChan <- "⚠️ 没有配置小程序，任务结束"
			return
		}

		totalSteps := len(cfg.MiniPrograms) * 5
		currentStep := 0

		for programIdx, mp := range cfg.MiniPrograms {
			select {
			case <-ctx.Done():
				logChan <- "⚠️ 任务已中断"
				_ = models.CreateOperationLog(db, userID.(int64), username.(string), "FETCH_ABORT", "数据拉取中断", c.ClientIP(), c.GetHeader("User-Agent"))
				return
			default:
			}

			programStartTime := time.Now().UnixMilli()
			logChan <- fmt.Sprintf("\n====== 处理小程序: %s (%s) ======", mp.Name, mp.AppID)

			currentStep++
			sendProgress(logChan, ProgressState{
				CurrentProgram: programIdx + 1,
				TotalPrograms:  len(cfg.MiniPrograms),
				CurrentStep:    currentStep,
				TotalSteps:     totalSteps,
				Message:        fmt.Sprintf("正在初始化 %s...", mp.Name),
			})

			if _, err := database.Exec(`
				INSERT INTO mini_program (名称, 小程序ID, 小程序Secret)
				VALUES (?, ?, ?)
				ON DUPLICATE KEY UPDATE
					名称 = VALUES(名称),
					小程序Secret = VALUES(小程序Secret),
					更新时间 = CURRENT_TIMESTAMP
			`, mp.Name, mp.AppID, mp.AppSecret); err != nil {
				logChan <- fmt.Sprintf("⚠️ [%s] 保存小程序配置失败: %v", mp.Name, err)
			}

			logChan <- fmt.Sprintf("🔑 [%s] 正在获取 Access Token...", mp.Name)
			token, err := getTokenWithCache(mp.AppID, mp.AppSecret)
			if err != nil {
				logChan <- fmt.Sprintf("❌ [%s] 获取Token失败: %v", mp.Name, err)
				continue
			}
			logChan <- fmt.Sprintf("✅ [%s] 获取Token成功", mp.Name)

			apiBase := cfg.Settings.APIBase
			if apiBase == "" {
				apiBase = "https://api.weixin.qq.com/publisher/stat"
			}

			currentStep++
			sendProgress(logChan, ProgressState{
				CurrentProgram: programIdx + 1,
				TotalPrograms:  len(cfg.MiniPrograms),
				CurrentStep:    currentStep,
				TotalSteps:     totalSteps,
				Message:        fmt.Sprintf("正在获取 %s 的广告位清单...", mp.Name),
			})

			logChan <- "\n----- 1. 拉取广告位清单 -----"
			if err := syncAdUnitList(database, mp.Name, mp.AppID, mp.AppSecret, token, apiBase, logChan); err != nil {
				logChan <- fmt.Sprintf("❌ [%s] 同步广告位列表失败: %v", mp.Name, err)
			}

			endDate := time.Now().Format("2006-01-02")
			startDate := cfg.Settings.StartDate
			if startDate == "" {
				startDate = "2025-07-01"
			}
			logChan <- fmt.Sprintf("起始日期: %s", startDate)

			currentStep++
			sendProgress(logChan, ProgressState{
				CurrentProgram: programIdx + 1,
				TotalPrograms:  len(cfg.MiniPrograms),
				CurrentStep:    currentStep,
				TotalSteps:     totalSteps,
				Message:        fmt.Sprintf("正在获取 %s 的汇总数据...", mp.Name),
			})

			logChan <- "\n----- 2. 拉取汇总数据 -----"
			lastSummaryDate := getLatestDataDate(database, "publisher_adpos_general", "日期", mp.AppID, startDate)
			summaryCount, err := syncSummaryData(ctx, database, mp.Name, mp.AppID, mp.AppSecret, token, apiBase, lastSummaryDate, endDate, logChan)
			if err != nil {
				logChan <- fmt.Sprintf("❌ [%s] 同步汇总数据失败: %v", mp.Name, err)
			} else {
				logChan <- fmt.Sprintf("汇总数据拉取完成, 共 %d 条", summaryCount)
			}

			currentStep++
			sendProgress(logChan, ProgressState{
				CurrentProgram: programIdx + 1,
				TotalPrograms:  len(cfg.MiniPrograms),
				CurrentStep:    currentStep,
				TotalSteps:     totalSteps,
				Message:        fmt.Sprintf("正在获取 %s 的细分数据...", mp.Name),
			})

			logChan <- "\n----- 3. 拉取细分数据 -----"
			lastDetailDate := getLatestDataDate(database, "publisher_adunit_general", "日期", mp.AppID, startDate)
			detailCount, err := syncDetailData(ctx, database, mp.Name, mp.AppID, mp.AppSecret, token, apiBase, lastDetailDate, endDate, logChan)
			if err != nil {
				logChan <- fmt.Sprintf("❌ [%s] 同步细分数据失败: %v", mp.Name, err)
			} else {
				logChan <- fmt.Sprintf("细分数据拉取完成, 共 %d 条", detailCount)
			}

			currentStep++
			sendProgress(logChan, ProgressState{
				CurrentProgram: programIdx + 1,
				TotalPrograms:  len(cfg.MiniPrograms),
				CurrentStep:    currentStep,
				TotalSteps:     totalSteps,
				Message:        fmt.Sprintf("正在获取 %s 的结算数据...", mp.Name),
			})

			logChan <- "\n----- 4. 拉取结算数据 -----"
			if err := syncSettlementData(database, mp.Name, mp.AppID, mp.AppSecret, token, apiBase, logChan); err != nil {
				logChan <- fmt.Sprintf("❌ [%s] 同步结算数据失败: %v", mp.Name, err)
			}

			programDuration := time.Now().UnixMilli() - programStartTime
			logChan <- fmt.Sprintf("\n====== %s 数据拉取完成 ======", mp.Name)
			logChan <- fmt.Sprintf("耗时: %s", formatDuration(programDuration))

			time.Sleep(500 * time.Millisecond)
		}

		mainDuration := time.Now().UnixMilli() - mainStartTime
		logChan <- "\n========== 全部数据拉取完成 =========="
		logChan <- fmt.Sprintf("完成时间: %s", time.Now().Format("2006-01-02 15:04:05"))
		logChan <- fmt.Sprintf("总耗时: %s", formatDuration(mainDuration))

		_ = models.CreateOperationLog(db, userID.(int64), username.(string), "FETCH_SUCCESS", "数据拉取成功", c.ClientIP(), c.GetHeader("User-Agent"))

		sendProgress(logChan, ProgressState{
			CurrentProgram: len(cfg.MiniPrograms),
			TotalPrograms:  len(cfg.MiniPrograms),
			CurrentStep:    totalSteps,
			TotalSteps:     totalSteps,
			Message:        "任务完成",
		})
	}()

	for {
		select {
		case msg, ok := <-logChan:
			if !ok {
				return
			}
			fmt.Fprintf(w, "%s\n", msg)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-ctx.Done():
			return
		}
	}
}

type tokenCache struct {
	token      string
	expireTime int64
}

func getTokenWithCache(appid, appsecret string) (string, error) {
	now := time.Now().UnixNano() / 1e6

	if cached, ok := tokenCaches.Load(appid); ok {
		cache := cached.(tokenCache)
		if now < cache.expireTime-300000 {
			return cache.token, nil
		}
	}

	token, err := getToken(appid, appsecret)
	if err != nil {
		return "", err
	}

	tokenCaches.Store(appid, tokenCache{
		token:      token,
		expireTime: now + 7200000,
	})

	return token, nil
}

func getToken(appID, secret string) (string, error) {
	url := fmt.Sprintf("https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=%s&secret=%s", appID, secret)
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Errcode     int    `json:"errcode"`
		Errmsg      string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.Errcode != 0 {
		return "", fmt.Errorf("%s", result.Errmsg)
	}
	return result.AccessToken, nil
}

var allowedTables = map[string]bool{
	"publisher_adpos_general":   true,
	"publisher_adunit_general":  true,
	"publisher_settlement":      true,
	"adunit_list":               true,
	"mini_program":              true,
	"fetch_log":                 true,
}

var allowedColumns = map[string]bool{
	"日期":         true,
	"data拉取日期": true,
}

func getLatestDataDate(database *sql.DB, tableName, dateColumn, appid string, defaultDate string) string {
	if !allowedTables[tableName] {
		return defaultDate
	}
	if !allowedColumns[dateColumn] {
		return defaultDate
	}

	var latestDate sql.NullString
	query := fmt.Sprintf(`SELECT MAX(%s) as latest_date FROM %s WHERE 小程序ID = ?`, dateColumn, tableName)
	err := database.QueryRow(query, appid).Scan(&latestDate)

	if err != nil || !latestDate.Valid || latestDate.String == "" {
		return defaultDate
	}

	dateStr := latestDate.String
	if len(dateStr) > 10 {
		dateStr = dateStr[:10]
	}
	d, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return defaultDate
	}
	d = d.AddDate(0, 0, 1)
	return d.Format("2006-01-02")
}

var AD_SLOT_NAMES = map[string]string{
	"SLOT_ID_WEAPP_VIDEO_BEGIN":  "视频贴片",
	"SLOT_ID_WEAPP_INTERSTITIAL": "插屏",
	"SLOT_ID_WEAPP_BANNER":       "Banner",
	"SLOT_ID_WEAPP_REWARD_VIDEO": "激励视频",
	"SLOT_ID_WEAPP_TEMPLATE":     "原生模版",
	"SLOT_ID_WEAPP_COVER":        "封面",
}

var AD_STATUS_NAMES = map[string]string{
	"AD_UNIT_STATUS_ON":  "正常",
	"AD_UNIT_STATUS_OFF": "暂停",
}

func fenToYuan(fen float64) float64 {
	return fen / 100
}

func formatRate(rate float64) string {
	return fmt.Sprintf("%.2f%%", rate*100)
}

func formatDuration(ms int64) string {
	seconds := ms / 1000
	minutes := seconds / 60
	hours := minutes / 60

	if hours > 0 {
		return fmt.Sprintf("%d 小时 %d 分钟", hours, minutes%60)
	} else if minutes > 0 {
		return fmt.Sprintf("%d 分钟 %d 秒", minutes, seconds%60)
	}
	return fmt.Sprintf("%d 秒", seconds)
}

func syncAdUnitList(database *sql.DB, miniProgramName, appid, appsecret, accessToken, apiBase string, logChan chan<- string) error {
	logChan <- fmt.Sprintf("✅ [%s] 正在获取广告位清单...", miniProgramName)

	data, err := fetchDataWithRetry(accessToken, "get_adunit_list", appid, appsecret, apiBase, map[string]string{})
	if err != nil {
		return err
	}

	var adUnits []map[string]interface{}
	if adUnitList, ok := data["ad_unit"].([]interface{}); ok {
		for _, item := range adUnitList {
			if m, ok := item.(map[string]interface{}); ok {
				adUnits = append(adUnits, m)
			}
		}
	}

	if len(adUnits) > 0 {
		batchSize := 100
		for i := 0; i < len(adUnits); i += batchSize {
			end := i + batchSize
			if end > len(adUnits) {
				end = len(adUnits)
			}
			batch := adUnits[i:end]

			valueStrings := make([]string, 0, len(batch))
			valueArgs := make([]interface{}, 0, len(batch)*15)

			for _, item := range batch {
				adUnitID := ""
				if v, ok := item["ad_unit_id"].(string); ok {
					adUnitID = v
				}
				adUnitName := ""
				if v, ok := item["ad_unit_name"].(string); ok {
					adUnitName = v
				}
				adSlot := ""
				if v, ok := item["ad_slot"].(string); ok {
					adSlot = v
				}
				adSlotName := AD_SLOT_NAMES[adSlot]
				if adSlotName == "" {
					adSlotName = adSlot
				}
				adUnitType := ""
				if v, ok := item["ad_unit_type"].(string); ok {
					adUnitType = v
				}
				adUnitStatus := ""
				if v, ok := item["ad_unit_status"].(string); ok {
					adUnitStatus = v
				}
				statusName := AD_STATUS_NAMES[adUnitStatus]
				if statusName == "" {
					statusName = adUnitStatus
				}
				slotID := ""
				if v, ok := item["slot_id"].(string); ok {
					slotID = v
				}
				adUnitSize := ""
				if v, ok := item["ad_unit_size"].([]interface{}); ok {
					sizeData, _ := json.Marshal(v)
					adUnitSize = string(sizeData)
				}
				allowPlayable := 0
				if v, ok := item["is_allow_playable"].(bool); ok && v {
					allowPlayable = 1
				}
				videoMin := 0
				if v, ok := item["video_duration_min"].(float64); ok {
					videoMin = int(v)
				}
				videoMax := 0
				if v, ok := item["video_duration_max"].(float64); ok {
					videoMax = int(v)
				}
				templTypeList := ""
				if v, ok := item["templ_type_list"].([]interface{}); ok {
					var types []string
					for _, t := range v {
						if s, ok := t.(string); ok {
							types = append(types, s)
						}
					}
					for i, t := range types {
						if i > 0 {
							templTypeList += ","
						}
						templTypeList += t
					}
				}
				valueStrings = append(valueStrings, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
				valueArgs = append(valueArgs, miniProgramName, appid, adUnitID, adUnitName, adSlot, adSlotName, adUnitType, adUnitStatus, statusName, slotID, adUnitSize, allowPlayable, videoMin, videoMax, templTypeList)
			}

			query := `INSERT INTO adunit_list (小程序名称, 小程序ID, 广告位唯一ID, 广告位名称, 广告位类型枚举, 广告位类型, 广告位类型值, 状态, 状态名称, 广告位数字ID, 广告尺寸, 是否允许可播放, 视频最短时长, 视频最长时长, 模版类型列表)
					VALUES ` + strings.Join(valueStrings, ",") + `
					ON DUPLICATE KEY UPDATE
						广告位名称 = VALUES(广告位名称),
						广告位类型枚举 = VALUES(广告位类型枚举),
						广告位类型 = VALUES(广告位类型),
						状态 = VALUES(状态),
						状态名称 = VALUES(状态名称),
						更新时间 = CURRENT_TIMESTAMP`
			if _, err := database.Exec(query, valueArgs...); err != nil {
				return err
			}
		}

		logChan <- fmt.Sprintf("✅ [%s] 广告位清单已保存", miniProgramName)
	}

	if _, err := database.Exec(`
		INSERT INTO fetch_log (小程序名称, 小程序ID, 拉取类型, 拉取日期, 状态, 记录数, 错误信息)
		VALUES (?, ?, 'adunit_list', ?, 'success', ?, '')
	`, miniProgramName, appid, time.Now().Format("2006-01-02"), len(adUnits)); err != nil {
		logChan <- fmt.Sprintf("⚠️ [%s] 记录日志失败: %v", miniProgramName, err)
	}

	logChan <- fmt.Sprintf("✅ [%s] 广告位列表同步完成, 共 %d 个广告位", miniProgramName, len(adUnits))
	return nil
}

func getMonthRanges(startDate, endDate string) ([]map[string]string, error) {
	var ranges []map[string]string

	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, err
	}

	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, err
	}

	for start.Before(end) || start.Equal(end) {
		rangeStart := start.Format("2006-01-02")
		lastDay := time.Date(start.Year(), start.Month()+1, 0, 0, 0, 0, 0, time.Local)
		if lastDay.After(end) {
			lastDay = end
		}
		rangeEnd := lastDay.Format("2006-01-02")

		ranges = append(ranges, map[string]string{
			"start": rangeStart,
			"end":   rangeEnd,
		})

		start = time.Date(start.Year(), start.Month()+1, 1, 0, 0, 0, 0, time.Local)
	}

	return ranges, nil
}

func fetchDataWithRetry(token, action, appid, appsecret, apiBase string, extraParams map[string]string) (map[string]interface{}, error) {
	currentToken := token
	retryCount := 0
	maxRetries := 3

	for retryCount < maxRetries {
		params := url.Values{}
		params.Set("access_token", currentToken)
		params.Set("action", action)
		params.Set("page", "1")
		params.Set("page_size", "90")

		for k, v := range extraParams {
			params.Set(k, v)
		}

		apiURL := apiBase + "?" + params.Encode()
		resp, err := httpClient.Get(apiURL)
		if err != nil {
			retryCount++
			time.Sleep(time.Duration(retryCount) * time.Second)
			continue
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("JSON解析失败: %v", err)
		}

		if errcode, ok := result["errcode"].(float64); ok && errcode != 0 {
			if errcode == 40001 || errcode == 42001 {
				currentToken, err = getToken(appid, appsecret)
				if err != nil {
					return nil, err
				}
				retryCount++
				continue
			}
			errMsg := ""
			if em, ok := result["err_msg"].(string); ok {
				errMsg = em
			} else if em, ok := result["errmsg"].(string); ok {
				errMsg = em
			}
			return nil, fmt.Errorf("API错误 [%d]: %s", int(errcode), errMsg)
		}

		if ret, ok := result["ret"].(float64); ok && ret != 0 {
			if ret == 45009 {
				time.Sleep(2 * time.Second)
				retryCount++
				continue
			}
			errMsg := ""
			if em, ok := result["err_msg"].(string); ok {
				errMsg = em
			} else if em, ok := result["errmsg"].(string); ok {
				errMsg = em
			}
			return nil, fmt.Errorf("API错误 [%d]: %s", int(ret), errMsg)
		}

		return result, nil
	}

	return nil, fmt.Errorf("重试次数超过限制")
}

func fetchAllPagesByDateRange(token, action, startDate, endDate, appid, appsecret, apiBase string) ([]map[string]interface{}, error) {
	var allList []map[string]interface{}
	page := 1
	pageSize := 90
	hasMore := true

	for hasMore {
		params := map[string]string{
			"start_date": startDate,
			"end_date":   endDate,
			"page":       fmt.Sprintf("%d", page),
			"page_size":  fmt.Sprintf("%d", pageSize),
		}

		data, err := fetchDataWithRetry(token, action, appid, appsecret, apiBase, params)
		if err != nil {
			return nil, err
		}

		var list []map[string]interface{}
		if dataList, ok := data["list"].([]interface{}); ok {
			for _, item := range dataList {
				if m, ok := item.(map[string]interface{}); ok {
					list = append(list, m)
				}
			}
		}

		allList = append(allList, list...)

		totalNum := 0
		if tn, ok := data["total_num"].(float64); ok {
			totalNum = int(tn)
		}

		if page*pageSize >= totalNum || len(list) < pageSize {
			hasMore = false
		} else {
			page++
			time.Sleep(500 * time.Millisecond)
		}
	}

	return allList, nil
}

func syncSummaryData(ctx context.Context, database *sql.DB, miniProgramName, appid, appsecret, accessToken, apiBase, startDate, endDate string, logChan chan<- string) (int, error) {
	ranges, err := getMonthRanges(startDate, endDate)
	if err != nil {
		return 0, err
	}

	totalCount := 0
	for _, r := range ranges {
		select {
		case <-ctx.Done():
			return totalCount, fmt.Errorf("任务已中断")
		default:
		}

		data, err := fetchAllPagesByDateRange(accessToken, "publisher_adpos_general", r["start"], r["end"], appid, appsecret, apiBase)
		if err != nil {
			logChan <- fmt.Sprintf("❌ [%s] %s ~ %s: 拉取失败 - %v", miniProgramName, r["start"], r["end"], err)
			continue
		}

		if len(data) > 0 {
			batchSize := 100
			for i := 0; i < len(data); i += batchSize {
				endIdx := i + batchSize
				if endIdx > len(data) {
					endIdx = len(data)
				}
				batch := data[i:endIdx]

				valueStrings := make([]string, 0, len(batch))
				valueArgs := make([]interface{}, 0, len(batch)*15)

				for _, item := range batch {
					dateVal := ""
					if v, ok := item["date"].(string); ok {
						if len(v) >= 10 {
							dateVal = v[:10]
						} else {
							dateVal = v
						}
					}
					adSlot := ""
					if v, ok := item["ad_slot"].(string); ok {
						adSlot = v
					}
					adSlotName := AD_SLOT_NAMES[adSlot]
					if adSlotName == "" {
						adSlotName = adSlot
					}
					slotStr := ""
					if v, ok := item["slot_str"].(string); ok {
						slotStr = v
					} else if v, ok := item["slot_id"].(float64); ok {
						slotStr = fmt.Sprintf("%d", int(v))
					}
					reqSuccCount := 0
					if v, ok := item["req_succ_count"].(float64); ok {
						reqSuccCount = int(v)
					}
					exposureCount := 0
					if v, ok := item["exposure_count"].(float64); ok {
						exposureCount = int(v)
					}
					exposureRate := 0.0
					if v, ok := item["exposure_rate"].(float64); ok {
						exposureRate = v
					}
					clickCount := 0
					if v, ok := item["click_count"].(float64); ok {
						clickCount = int(v)
					}
					clickRate := 0.0
					if v, ok := item["click_rate"].(float64); ok {
						clickRate = v
					}
					income := 0
					if v, ok := item["income"].(float64); ok {
						income = int(v)
					}
					ecpm := 0.0
					if v, ok := item["ecpm"].(float64); ok {
						ecpm = v
					}

					valueStrings = append(valueStrings, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
					valueArgs = append(valueArgs, miniProgramName, appid, dateVal, adSlot, adSlotName, slotStr, reqSuccCount, exposureCount, formatRate(exposureRate), clickCount, formatRate(clickRate), income, fenToYuan(float64(income)), ecpm, fenToYuan(ecpm))
				}

				query := `INSERT INTO publisher_adpos_general (小程序名称, 小程序ID, 日期, 广告位类型枚举, 广告位类型, 广告位数字ID, 成功请求次数, 曝光量, 曝光率, 点击量, 点击率, 总收入分, 总收入元, 千次曝光收入分, 千次曝光收入元)
					VALUES ` + strings.Join(valueStrings, ",") + `
					ON DUPLICATE KEY UPDATE
						广告位类型 = VALUES(广告位类型),
						成功请求次数 = VALUES(成功请求次数),
						曝光量 = VALUES(曝光量),
						曝光率 = VALUES(曝光率),
						点击量 = VALUES(点击量),
						点击率 = VALUES(点击率),
						总收入分 = VALUES(总收入分),
						总收入元 = VALUES(总收入元),
						千次曝光收入分 = VALUES(千次曝光收入分),
						千次曝光收入元 = VALUES(千次曝光收入元)`
				if _, err := database.Exec(query, valueArgs...); err != nil {
					logChan <- fmt.Sprintf("❌ [%s] %s ~ %s: 保存失败 - %v", miniProgramName, r["start"], r["end"], err)
				} else {
					totalCount += len(batch)
				}
			}

			logChan <- fmt.Sprintf("✅ [%s] %s ~ %s: %d 条", miniProgramName, r["start"], r["end"], len(data))
		} else {
			logChan <- fmt.Sprintf("ℹ️ [%s] %s ~ %s: 无数据", miniProgramName, r["start"], r["end"])
		}

		time.Sleep(500 * time.Millisecond)
	}

	if _, err := database.Exec(`
		INSERT INTO fetch_log (小程序名称, 小程序ID, 拉取类型, 拉取日期, 状态, 记录数, 错误信息)
		VALUES (?, ?, 'publisher_adpos_general', ?, 'success', ?, '')
	`, miniProgramName, appid, endDate, totalCount); err != nil {
		logChan <- fmt.Sprintf("⚠️ [%s] 记录日志失败: %v", miniProgramName, err)
	}

	return totalCount, nil
}

func syncDetailData(ctx context.Context, database *sql.DB, miniProgramName, appid, appsecret, accessToken, apiBase, startDate, endDate string, logChan chan<- string) (int, error) {
	ranges, err := getMonthRanges(startDate, endDate)
	if err != nil {
		return 0, err
	}

	totalCount := 0
	for _, r := range ranges {
		select {
		case <-ctx.Done():
			return totalCount, fmt.Errorf("任务已中断")
		default:
		}

		data, err := fetchAllPagesByDateRange(accessToken, "publisher_adunit_general", r["start"], r["end"], appid, appsecret, apiBase)
		if err != nil {
			logChan <- fmt.Sprintf("❌ [%s] %s ~ %s: 拉取失败 - %v", miniProgramName, r["start"], r["end"], err)
			continue
		}

		if len(data) > 0 {
			batchSize := 100
			for i := 0; i < len(data); i += batchSize {
				endIdx := i + batchSize
				if endIdx > len(data) {
					endIdx = len(data)
				}
				batch := data[i:endIdx]

				valueStrings := make([]string, 0, len(batch))
				valueArgs := make([]interface{}, 0, len(batch)*22)

				for _, item := range batch {
					adUnitID := ""
					if v, ok := item["ad_unit_id"].(string); ok {
						adUnitID = v
					}
					adUnitName := ""
					if v, ok := item["ad_unit_name"].(string); ok {
						adUnitName = v
					}
					var statItem map[string]interface{}
					if v, ok := item["stat_item"].(map[string]interface{}); ok {
						statItem = v
					}
					dateVal := ""
					if statItem != nil {
						if v, ok := statItem["date"].(string); ok {
							if len(v) >= 10 {
								dateVal = v[:10]
							} else {
								dateVal = v
							}
						}
					}
					adSlot := ""
					if statItem != nil {
						if v, ok := statItem["ad_slot"].(string); ok {
							adSlot = v
						}
					}
					adSlotName := AD_SLOT_NAMES[adSlot]
					if adSlotName == "" {
						adSlotName = adSlot
					}
					slotStr := ""
					if statItem != nil {
						if v, ok := statItem["slot_str"].(string); ok {
							slotStr = v
						}
					}
					reqSuccCount := 0
					if statItem != nil {
						if v, ok := statItem["req_succ_count"].(float64); ok {
							reqSuccCount = int(v)
						}
					}
					exposureCount := 0
					if statItem != nil {
						if v, ok := statItem["exposure_count"].(float64); ok {
							exposureCount = int(v)
						}
					}
					exposureRate := 0.0
					if statItem != nil {
						if v, ok := statItem["exposure_rate"].(float64); ok {
							exposureRate = v
						}
					}
					clickCount := 0
					if statItem != nil {
						if v, ok := statItem["click_count"].(float64); ok {
							clickCount = int(v)
						}
					}
					clickRate := 0.0
					if statItem != nil {
						if v, ok := statItem["click_rate"].(float64); ok {
							clickRate = v
						}
					}
					income := 0
					if statItem != nil {
						if v, ok := statItem["income"].(float64); ok {
							income = int(v)
						}
					}
					publisherIncome := 0
					if statItem != nil {
						if v, ok := statItem["publisher_income"].(float64); ok {
							publisherIncome = int(v)
						}
					}
					agencyIncome := 0
					if statItem != nil {
						if v, ok := statItem["agency_income"].(float64); ok {
							agencyIncome = int(v)
						}
					}
					ecpm := 0.0
					if statItem != nil {
						if v, ok := statItem["ecpm"].(float64); ok {
							ecpm = v
						}
					}
					isSmartAds := 0
					if statItem != nil {
						if v, ok := statItem["is_smart_ads"].(float64); ok && v != 0 {
							isSmartAds = 1
						}
					}
					parentTemplType := ""
					if statItem != nil {
						if v, ok := statItem["parent_templ_type"].(string); ok {
							parentTemplType = v
						}
					}

					valueStrings = append(valueStrings, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
					valueArgs = append(valueArgs, miniProgramName, appid, adUnitID, adUnitName, dateVal, adSlot, adSlotName, slotStr, reqSuccCount, exposureCount, formatRate(exposureRate), clickCount, formatRate(clickRate), income, fenToYuan(float64(income)), publisherIncome, fenToYuan(float64(publisherIncome)), agencyIncome, ecpm, fenToYuan(ecpm), isSmartAds, parentTemplType)
				}

				query := `INSERT INTO publisher_adunit_general (小程序名称, 小程序ID, 广告位唯一ID, 广告位名称, 日期, 广告位类型枚举, 广告位类型, 广告位数字ID, 成功请求次数, 曝光量, 曝光率, 点击量, 点击率, 总收入分, 总收入元, 流量主收入分, 流量主收入元, 代理商收入分, 千次曝光收入分, 千次曝光收入元, 是否智能广告, 父模版类型)
					VALUES ` + strings.Join(valueStrings, ",") + `
					ON DUPLICATE KEY UPDATE
						广告位名称 = VALUES(广告位名称),
						广告位类型 = VALUES(广告位类型),
						成功请求次数 = VALUES(成功请求次数),
						曝光量 = VALUES(曝光量),
						曝光率 = VALUES(曝光率),
						点击量 = VALUES(点击量),
						点击率 = VALUES(点击率),
						总收入分 = VALUES(总收入分),
						总收入元 = VALUES(总收入元),
						流量主收入分 = VALUES(流量主收入分),
						流量主收入元 = VALUES(流量主收入元),
						代理商收入分 = VALUES(代理商收入分),
						千次曝光收入分 = VALUES(千次曝光收入分),
						千次曝光收入元 = VALUES(千次曝光收入元),
						是否智能广告 = VALUES(是否智能广告),
						父模版类型 = VALUES(父模版类型)`
				if _, err := database.Exec(query, valueArgs...); err != nil {
					logChan <- fmt.Sprintf("❌ [%s] %s ~ %s: 保存失败 - %v", miniProgramName, r["start"], r["end"], err)
				} else {
					totalCount += len(batch)
				}
			}

			logChan <- fmt.Sprintf("✅ [%s] %s ~ %s: %d 条", miniProgramName, r["start"], r["end"], len(data))
		} else {
			logChan <- fmt.Sprintf("ℹ️ [%s] %s ~ %s: 无数据", miniProgramName, r["start"], r["end"])
		}

		time.Sleep(500 * time.Millisecond)
	}

	if _, err := database.Exec(`
		INSERT INTO fetch_log (小程序名称, 小程序ID, 拉取类型, 拉取日期, 状态, 记录数, 错误信息)
		VALUES (?, ?, 'publisher_adunit_general', ?, 'success', ?, '')
	`, miniProgramName, appid, endDate, totalCount); err != nil {
		logChan <- fmt.Sprintf("⚠️ [%s] 记录日志失败: %v", miniProgramName, err)
	}

	return totalCount, nil
}

func syncSettlementData(database *sql.DB, miniProgramName, appid, appsecret, accessToken, apiBase string, logChan chan<- string) error {
	logChan <- fmt.Sprintf("✅ [%s] 正在获取结算数据...", miniProgramName)

	data, err := fetchDataWithRetry(accessToken, "publisher_settlement", appid, appsecret, apiBase, map[string]string{})
	if err != nil {
		return err
	}

	revenueAll := int64(0)
	if v, ok := data["revenue_all"].(float64); ok {
		revenueAll = int64(v)
	}
	settledRevenueAll := int64(0)
	if v, ok := data["settled_revenue_all"].(float64); ok {
		settledRevenueAll = int64(v)
	}
	penaltyAll := int64(0)
	if v, ok := data["penalty_all"].(float64); ok {
		penaltyAll = int64(v)
	}
	wywRevenueAll := int64(0)
	wywSettledRevenueAll := int64(0)
	wywPenaltyAll := int64(0)
	if wyw, ok := data["wyw_settled_summary"].(map[string]interface{}); ok {
		if v, ok := wyw["wyw_revenue_all"].(float64); ok {
			wywRevenueAll = int64(v)
		}
		if v, ok := wyw["wyw_settled_revenue_all"].(float64); ok {
			wywSettledRevenueAll = int64(v)
		}
		if v, ok := wyw["wyw_penalty_all"].(float64); ok {
			wywPenaltyAll = int64(v)
		}
	}

	if _, err := database.Exec(`
		INSERT INTO publisher_settlement (小程序名称, 小程序ID, 总预估收入分, 总预估收入元, 总已结算收入分, 总已结算收入元, 总罚金分, 总罚金元, 微信云开发总预估收入分, 微信云开发总已结算收入分, 微信云开发总罚金分, 数据拉取日期)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURDATE())
		ON DUPLICATE KEY UPDATE
			总预估收入分 = VALUES(总预估收入分),
			总预估收入元 = VALUES(总预估收入元),
			总已结算收入分 = VALUES(总已结算收入分),
			总已结算收入元 = VALUES(总已结算收入元),
			总罚金分 = VALUES(总罚金分),
			总罚金元 = VALUES(总罚金元),
			微信云开发总预估收入分 = VALUES(微信云开发总预估收入分),
			微信云开发总已结算收入分 = VALUES(微信云开发总已结算收入分),
			微信云开发总罚金分 = VALUES(微信云开发总罚金分),
			创建时间 = CURRENT_TIMESTAMP
	`, miniProgramName, appid, revenueAll, fenToYuan(float64(revenueAll)), settledRevenueAll, fenToYuan(float64(settledRevenueAll)), penaltyAll, fenToYuan(float64(penaltyAll)), wywRevenueAll, wywSettledRevenueAll, wywPenaltyAll); err != nil {
		return err
	}

	logChan <- fmt.Sprintf("✅ [%s] 结算数据已保存", miniProgramName)
	logChan <- fmt.Sprintf("ℹ️ [%s] 总预估收入: %.2f 元", miniProgramName, fenToYuan(float64(revenueAll)))
	logChan <- fmt.Sprintf("ℹ️ [%s] 总已结算收入: %.2f 元", miniProgramName, fenToYuan(float64(settledRevenueAll)))

	if _, err := database.Exec(`
		INSERT INTO fetch_log (小程序名称, 小程序ID, 拉取类型, 拉取日期, 状态, 记录数, 错误信息)
		VALUES (?, ?, 'publisher_settlement', ?, 'success', 1, '')
	`, miniProgramName, appid, time.Now().Format("2006-01-02")); err != nil {
		logChan <- fmt.Sprintf("⚠️ [%s] 记录日志失败: %v", miniProgramName, err)
	}

	return nil
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func cleanupExpiredTokens() {
	now := time.Now().UnixNano() / 1e6
	tokenCaches.Range(func(key, value interface{}) bool {
		cache := value.(tokenCache)
		if now >= cache.expireTime {
			tokenCaches.Delete(key)
		}
		return true
	})
}

func startTokenCleanupJob() {
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		for range ticker.C {
			cleanupExpiredTokens()
		}
	}()
}

func main() {
	var err error
	cfg, _ := loadConfig()
	if cfg.Database.Host != "" {
		db, err = getDBConnection(cfg)
		if err == nil {
			_ = initDatabase(db)
			_ = initDefaultAdmin()
		}
	}

	startTokenCleanupJob()

	r := gin.Default()

	r.StaticFS("/", http.FS(frontendFS))
	r.NoRoute(func(c *gin.Context) {
		c.FileFromFS("frontend/index.html", http.FS(frontendFS))
	})

	auth := r.Group("/api")
	{
		auth.POST("/auth/login", loginHandler)
		auth.POST("/auth/logout", middleware.AuthMiddleware(), logoutHandler)
		auth.GET("/auth/me", middleware.AuthMiddleware(), getCurrentUserHandler)
	}

	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	{
		api.GET("/dashboard/stats", getDashboardStatsHandler)
		api.GET("/mini-programs", getMiniProgramsHandler)
		api.POST("/fetch/execute", executeFetchHandler)
		api.GET("/logs", getLogsHandler)
		api.GET("/config", getConfigHandler)
		api.POST("/config", saveConfigHandler)
		api.POST("/test-connection", testConnectionHandler)
	}

	admin := r.Group("/api")
	admin.Use(middleware.AuthMiddleware(), middleware.AdminMiddleware())
	{
		admin.GET("/users", getUsersHandler)
		admin.POST("/users", createUserHandler)
		admin.PUT("/users/:id", updateUserHandler)
		admin.DELETE("/users/:id", deleteUserHandler)
	}

	port := 28384
	addr := fmt.Sprintf(":%d", port)
	webUrl := fmt.Sprintf("http://localhost:%d/", port)

	go func() {
		time.Sleep(500 * time.Millisecond)
		openBrowser(webUrl)
	}()

	log.Printf("🚀 服务器已启动: %s", webUrl)
	log.Printf("💡 默认管理员: admin / admin123")
	log.Printf("💡 按 Ctrl+C 停止服务器")
	log.Fatal(r.Run(addr))
}