package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
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

	"go-app/internal/appconfig"
	jobstate "go-app/internal/jobs"
	"go-app/internal/security"
	"go-app/internal/storage"
	"go-app/middleware"
	"go-app/models"
	"go-app/utils"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)

//go:embed frontend/*
var frontendFS embed.FS

type Config struct {
	Database     DatabaseConfig      `json:"database"`
	Settings     SettingsConfig      `json:"settings"`
	MiniPrograms []MiniProgramConfig `json:"miniPrograms"`
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
	Name      string `json:"name"`
	AppID     string `json:"appid"`
	AppSecret string `json:"appsecret"`
}

var (
	configPath  string
	mu          sync.Mutex
	httpClient  = &http.Client{Timeout: 30 * time.Second}
	tokenCaches = sync.Map{}
	db          *sql.DB
	systemDB    *sql.DB
	fieldKey    []byte
	appDataDir  string
	jobEvents   = newJobEventBroker()
)

const authCookieName = "wmam_session"

type jobEventBroker struct {
	mu          sync.Mutex
	subscribers map[int64]map[chan string]struct{}
}

func newJobEventBroker() *jobEventBroker {
	return &jobEventBroker{subscribers: make(map[int64]map[chan string]struct{})}
}

func (b *jobEventBroker) subscribe(jobID int64) chan string {
	ch := make(chan string, 100)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subscribers[jobID] == nil {
		b.subscribers[jobID] = make(map[chan string]struct{})
	}
	b.subscribers[jobID][ch] = struct{}{}
	return ch
}

func (b *jobEventBroker) unsubscribe(jobID int64, ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if subscribers := b.subscribers[jobID]; subscribers != nil {
		delete(subscribers, ch)
		if len(subscribers) == 0 {
			delete(b.subscribers, jobID)
		}
	}
	close(ch)
}

func (b *jobEventBroker) publish(jobID int64, event gin.H) {
	if _, ok := event["time"]; !ok {
		event["time"] = time.Now().Format("15:04:05")
	}
	raw, _ := json.Marshal(event)
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subscribers[jobID] {
		select {
		case ch <- string(raw):
		default:
		}
	}
}

func publishJobStateEvent(jobID int64) {
	job, err := storage.GetFetchJobByID(systemDB, jobID)
	if err != nil || job == nil {
		return
	}
	jobEvents.publish(jobID, gin.H{
		"type":            "job",
		"status":          job.Status,
		"progressPercent": job.ProgressPercent,
		"currentProgram":  job.CurrentProgramName,
		"currentStep":     job.CurrentStep,
	})
}

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
			Name:      name,
			AppID:     appid,
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

func hashAuthToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func setAuthCookie(c *gin.Context, token string, duration time.Duration, persistent bool) {
	maxAge := 0
	if persistent {
		maxAge = int(duration.Seconds())
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(authCookieName, token, maxAge, "/", "", false, true)
}

func clearAuthCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(authCookieName, "", -1, "/", "", false, true)
}

func bearerToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) == 2 && parts[0] == "Bearer" {
		return parts[1]
	}

	token, err := c.Cookie(authCookieName)
	if err != nil {
		return ""
	}
	return token
}

func localAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c)
		if token == "" {
			utils.ErrorWithStatus(c, http.StatusUnauthorized, 401, "请先登录")
			c.Abort()
			return
		}

		claims, err := utils.ParseToken(token)
		if err != nil {
			utils.ErrorWithStatus(c, http.StatusUnauthorized, 401, "登录状态已失效")
			c.Abort()
			return
		}

		sessionActive, err := storage.ValidateSession(systemDB, hashAuthToken(token))
		if err != nil || !sessionActive {
			utils.ErrorWithStatus(c, http.StatusUnauthorized, 401, "登录状态已失效")
			c.Abort()
			return
		}

		user, err := storage.GetUserByID(systemDB, claims.UserID)
		if err != nil || user.Status != "active" {
			utils.ErrorWithStatus(c, http.StatusUnauthorized, 401, "账号不可用")
			c.Abort()
			return
		}

		c.Set("user_id", user.ID)
		c.Set("username", user.Username)
		c.Set("role", user.Role)
		c.Set("must_change_password", user.MustChangePassword)
		c.Next()
	}
}

func localLoginHandler(c *gin.Context) {
	var req struct {
		Username         string `json:"username" binding:"required"`
		Password         string `json:"password" binding:"required"`
		RememberPassword bool   `json:"rememberPassword"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "参数错误")
		return
	}

	user, err := storage.GetUserByUsername(systemDB, req.Username)
	if err != nil {
		utils.Error(c, 401, "用户名或密码错误")
		return
	}

	if user.Status == "disabled" {
		utils.Error(c, 403, "账号已被禁用")
		return
	}

	if !utils.CheckPassword(req.Password, user.PasswordHash) {
		utils.Error(c, 401, "用户名或密码错误")
		_ = storage.CreateAuditLog(systemDB, &user.ID, user.Username, "LOGIN_FAILED", "user", fmt.Sprintf("%d", user.ID), "登录失败：密码错误", "failed", c.ClientIP(), c.GetHeader("User-Agent"))
		return
	}

	tokenDuration := 24 * time.Hour
	if req.RememberPassword {
		tokenDuration = 30 * 24 * time.Hour
	}

	token, err := utils.GenerateTokenWithDuration(user.ID, user.Username, user.Role, tokenDuration)
	if err != nil {
		utils.Error(c, 500, "生成登录凭证失败")
		return
	}

	expiresAt := time.Now().Add(tokenDuration)
	_ = storage.CreateSession(systemDB, user.ID, hashAuthToken(token), req.RememberPassword, expiresAt, c.ClientIP(), c.GetHeader("User-Agent"))
	_ = storage.UpdateLastLogin(systemDB, user.ID)
	_ = storage.CreateAuditLog(systemDB, &user.ID, user.Username, "LOGIN", "user", fmt.Sprintf("%d", user.ID), "登录成功", "success", c.ClientIP(), c.GetHeader("User-Agent"))
	setAuthCookie(c, token, tokenDuration, req.RememberPassword)

	utils.Success(c, gin.H{
		"expires_at": expiresAt.Format(time.RFC3339),
		"user": gin.H{
			"id":                   user.ID,
			"username":             user.Username,
			"role":                 user.Role,
			"must_change_password": user.MustChangePassword,
		},
	})
}

func localRecoverAdminHandler(c *gin.Context) {
	var req struct {
		RecoveryCode string `json:"recoveryCode" binding:"required"`
		NewPassword  string `json:"newPassword" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "参数错误")
		return
	}
	if len(req.NewPassword) < 8 {
		utils.Error(c, 400, "新密码至少需要 8 位")
		return
	}

	recoveryHash, err := storage.GetAdminRecoveryHash(systemDB)
	if err != nil || !utils.CheckPassword(req.RecoveryCode, recoveryHash) {
		utils.Error(c, 403, "恢复码无效")
		return
	}

	admin, err := storage.GetAdminUser(systemDB)
	if err != nil {
		utils.Error(c, 500, "管理员账号不存在")
		return
	}
	passwordHash, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		utils.Error(c, 500, "密码加密失败")
		return
	}
	if err := storage.UpdateUserPassword(systemDB, admin.ID, passwordHash, false); err != nil {
		utils.Error(c, 500, "重置管理员密码失败")
		return
	}
	_ = storage.RevokeUserSessions(systemDB, admin.ID)

	nextRecoveryCode, err := security.NewRecoveryCode()
	if err != nil {
		utils.Error(c, 500, "生成新恢复码失败")
		return
	}
	nextRecoveryHash, err := utils.HashPassword(nextRecoveryCode)
	if err != nil {
		utils.Error(c, 500, "保存新恢复码失败")
		return
	}
	if err := storage.ReplaceAdminRecoveryHash(systemDB, nextRecoveryHash); err != nil {
		utils.Error(c, 500, "保存新恢复码失败")
		return
	}

	_ = storage.CreateAuditLog(systemDB, &admin.ID, admin.Username, "ADMIN_RECOVERY_RESET", "user", fmt.Sprintf("%d", admin.ID), "使用恢复码重置管理员密码", "success", c.ClientIP(), c.GetHeader("User-Agent"))

	utils.Success(c, gin.H{"newRecoveryCode": nextRecoveryCode})
}

func localCurrentUserHandler(c *gin.Context) {
	userID, _ := c.Get("user_id")
	id, ok := userID.(int64)
	if !ok {
		utils.ErrorWithStatus(c, http.StatusUnauthorized, 401, "登录状态无效")
		return
	}

	user, err := storage.GetUserByID(systemDB, id)
	if err != nil || user.Status != "active" {
		utils.ErrorWithStatus(c, http.StatusUnauthorized, 401, "登录状态无效")
		return
	}

	utils.Success(c, gin.H{
		"user": gin.H{
			"id":                   user.ID,
			"username":             user.Username,
			"role":                 user.Role,
			"must_change_password": user.MustChangePassword,
		},
	})
}

func localLogoutHandler(c *gin.Context) {
	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")
	id, _ := userID.(int64)
	name, _ := username.(string)

	if token := bearerToken(c); token != "" {
		_ = storage.RevokeSession(systemDB, hashAuthToken(token))
	}
	clearAuthCookie(c)
	_ = storage.CreateAuditLog(systemDB, &id, name, "LOGOUT", "user", fmt.Sprintf("%d", id), "退出登录", "success", c.ClientIP(), c.GetHeader("User-Agent"))

	utils.Success(c, nil)
}

func localChangePasswordHandler(c *gin.Context) {
	var req struct {
		OldPassword string `json:"oldPassword" binding:"required"`
		NewPassword string `json:"newPassword" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "参数错误")
		return
	}
	if len(req.NewPassword) < 8 {
		utils.Error(c, 400, "新密码至少需要 8 位")
		return
	}

	userID, _ := c.Get("user_id")
	id, _ := userID.(int64)
	user, err := storage.GetUserByID(systemDB, id)
	if err != nil {
		utils.Error(c, 404, "用户不存在")
		return
	}
	if !utils.CheckPassword(req.OldPassword, user.PasswordHash) {
		utils.Error(c, 400, "旧密码错误")
		return
	}

	hash, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		utils.Error(c, 500, "密码加密失败")
		return
	}
	if err := storage.UpdateUserPassword(systemDB, user.ID, hash, false); err != nil {
		utils.Error(c, 500, "修改密码失败")
		return
	}
	_ = storage.RevokeUserSessions(systemDB, user.ID)
	clearAuthCookie(c)
	_ = storage.CreateAuditLog(systemDB, &user.ID, user.Username, "PASSWORD_CHANGE", "user", fmt.Sprintf("%d", user.ID), "修改自己的密码", "success", c.ClientIP(), c.GetHeader("User-Agent"))

	utils.Success(c, nil)
}

func localGetUsersHandler(c *gin.Context) {
	users, err := storage.ListUsers(systemDB)
	if err != nil {
		utils.Error(c, 500, "获取用户列表失败")
		return
	}
	utils.Success(c, gin.H{"users": users})
}

func localResetUserPasswordHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.Error(c, 400, "无效的用户 ID")
		return
	}

	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "参数错误")
		return
	}
	if len(req.Password) < 8 {
		utils.Error(c, 400, "新密码至少需要 8 位")
		return
	}

	user, err := storage.GetUserByID(systemDB, id)
	if err != nil {
		utils.Error(c, 404, "用户不存在")
		return
	}
	if user.Role == "admin" {
		utils.Error(c, 403, "不能在用户管理中重置管理员密码")
		return
	}

	hash, err := utils.HashPassword(req.Password)
	if err != nil {
		utils.Error(c, 500, "密码加密失败")
		return
	}
	if err := storage.UpdateUserPassword(systemDB, user.ID, hash, true); err != nil {
		utils.Error(c, 500, "重置密码失败")
		return
	}
	_ = storage.RevokeUserSessions(systemDB, user.ID)

	adminID, _ := c.Get("user_id")
	adminName, _ := c.Get("username")
	operatorID, _ := adminID.(int64)
	operatorName, _ := adminName.(string)
	_ = storage.CreateAuditLog(systemDB, &operatorID, operatorName, "USER_PASSWORD_RESET", "user", fmt.Sprintf("%d", user.ID), "重置用户密码："+user.Username, "success", c.ClientIP(), c.GetHeader("User-Agent"))

	utils.Success(c, nil)
}

func localCreateUserHandler(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
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

	user, err := storage.CreateUser(systemDB, req.Username, passwordHash, "user")
	if err != nil {
		utils.Error(c, 500, "创建用户失败："+err.Error())
		return
	}

	adminID, _ := c.Get("user_id")
	adminName, _ := c.Get("username")
	id, _ := adminID.(int64)
	name, _ := adminName.(string)
	_ = storage.CreateAuditLog(systemDB, &id, name, "USER_CREATE", "user", fmt.Sprintf("%d", user.ID), "创建用户："+user.Username, "success", c.ClientIP(), c.GetHeader("User-Agent"))

	utils.Success(c, gin.H{"user": user})
}

func localUpdateUserHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.Error(c, 400, "无效的用户 ID")
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

	user, err := storage.UpdateUser(systemDB, id, req.Username, req.Status)
	if err != nil {
		utils.Error(c, 500, "更新用户失败："+err.Error())
		return
	}
	if user.Status == "disabled" {
		_ = storage.RevokeUserSessions(systemDB, user.ID)
	}

	adminID, _ := c.Get("user_id")
	adminName, _ := c.Get("username")
	operatorID, _ := adminID.(int64)
	operatorName, _ := adminName.(string)
	action := "USER_ENABLE"
	if user.Status == "disabled" {
		action = "USER_DISABLE"
	}
	_ = storage.CreateAuditLog(systemDB, &operatorID, operatorName, action, "user", fmt.Sprintf("%d", user.ID), "更新用户："+user.Username, "success", c.ClientIP(), c.GetHeader("User-Agent"))

	utils.Success(c, gin.H{"user": user})
}

func localDeleteUserHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.Error(c, 400, "无效的用户 ID")
		return
	}

	user, err := storage.GetUserByID(systemDB, id)
	if err != nil {
		utils.Error(c, 404, "用户不存在")
		return
	}

	if err := storage.DeleteUser(systemDB, id); err != nil {
		utils.Error(c, 500, "删除用户失败："+err.Error())
		return
	}

	adminID, _ := c.Get("user_id")
	adminName, _ := c.Get("username")
	operatorID, _ := adminID.(int64)
	operatorName, _ := adminName.(string)
	_ = storage.CreateAuditLog(systemDB, &operatorID, operatorName, "USER_DELETE", "user", fmt.Sprintf("%d", user.ID), "删除用户："+user.Username, "success", c.ClientIP(), c.GetHeader("User-Agent"))

	utils.Success(c, nil)
}

func localDashboardStatsHandler(c *gin.Context) {
	userCount, _ := storage.CountUsers(systemDB)
	miniProgramCount, _ := storage.CountMiniPrograms(systemDB)
	todayFetchCount, _ := storage.CountTodayFetchJobs(systemDB)
	lastFetchTime, _ := storage.LastFetchTime(systemDB)

	utils.Success(c, gin.H{
		"user_count":         userCount,
		"mini_program_count": miniProgramCount,
		"today_fetch_count":  todayFetchCount,
		"last_fetch_time":    lastFetchTime,
	})
}

func localGetLogsHandler(c *gin.Context) {
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")
	id, _ := userID.(int64)
	isAdmin := role == "admin"

	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		if parsed, err := strconv.Atoi(pageStr); err == nil && parsed > 0 {
			page = parsed
		}
	}
	limit := 20
	offset := (page - 1) * limit

	logs, total, err := storage.ListAuditLogs(systemDB, id, isAdmin, offset, limit)
	if err != nil {
		utils.Error(c, 500, "获取日志失败")
		return
	}

	utils.Success(c, gin.H{"logs": logs, "total": total})
}

func testStoredMySQLConfig(cfg storage.MySQLConfig) error {
	database, err := getDBConnection(Config{
		Database: DatabaseConfig{
			Host:     cfg.Host,
			Port:     cfg.Port,
			User:     cfg.Username,
			Password: cfg.Password,
			Database: cfg.Database,
		},
	})
	if err != nil {
		return err
	}
	defer database.Close()
	return database.Ping()
}

func requireAdminPassword(c *gin.Context, password string) bool {
	if strings.TrimSpace(password) == "" {
		utils.Error(c, 400, "请输入管理员密码")
		return false
	}

	userID, _ := c.Get("user_id")
	id, _ := userID.(int64)
	user, err := storage.GetUserByID(systemDB, id)
	if err != nil || !utils.CheckPassword(password, user.PasswordHash) {
		utils.Error(c, 403, "管理员密码错误")
		return false
	}
	return true
}

func localGetMySQLConfigHandler(c *gin.Context) {
	cfg, err := storage.GetMySQLConfig(systemDB, fieldKey, false)
	if err != nil {
		utils.Error(c, 500, "读取数据库配置失败")
		return
	}
	utils.Success(c, gin.H{"mysql": cfg})
}

func localTestMySQLConfigHandler(c *gin.Context) {
	var req storage.MySQLConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "参数错误")
		return
	}

	current, _ := storage.GetMySQLConfig(systemDB, fieldKey, true)
	if strings.TrimSpace(req.Password) == "" {
		req.Password = current.Password
	}
	if req.Port == 0 {
		req.Port = 3306
	}

	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")
	id, _ := userID.(int64)
	name, _ := username.(string)

	if err := testStoredMySQLConfig(req); err != nil {
		_ = storage.CreateAuditLog(systemDB, &id, name, "SYSTEM_MYSQL_TEST", "system", "mysql", "测试 MySQL 连接失败", "failed", c.ClientIP(), c.GetHeader("User-Agent"))
		utils.Error(c, 500, "连接失败："+err.Error())
		return
	}

	_ = storage.CreateAuditLog(systemDB, &id, name, "SYSTEM_MYSQL_TEST", "system", "mysql", "测试 MySQL 连接成功", "success", c.ClientIP(), c.GetHeader("User-Agent"))
	utils.Success(c, gin.H{"message": "连接成功"})
}

func localSaveMySQLConfigHandler(c *gin.Context) {
	var req struct {
		storage.MySQLConfig
		AdminPassword string `json:"adminPassword"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "参数错误")
		return
	}
	if !requireAdminPassword(c, req.AdminPassword) {
		return
	}

	next := req.MySQLConfig
	current, _ := storage.GetMySQLConfig(systemDB, fieldKey, true)
	if strings.TrimSpace(next.Password) == "" {
		next.Password = current.Password
	}
	if next.Port == 0 {
		next.Port = 3306
	}
	if err := testStoredMySQLConfig(next); err != nil {
		utils.Error(c, 500, "保存前测试连接失败："+err.Error())
		return
	}

	if err := storage.SaveMySQLConfig(systemDB, fieldKey, next); err != nil {
		utils.Error(c, 500, "保存数据库配置失败")
		return
	}

	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")
	id, _ := userID.(int64)
	name, _ := username.(string)
	_ = storage.CreateAuditLog(systemDB, &id, name, "SYSTEM_MYSQL_UPDATE", "system", "mysql", "修改 MySQL 配置", "success", c.ClientIP(), c.GetHeader("User-Agent"))

	utils.Success(c, nil)
}

func localRestoreMySQLConfigHandler(c *gin.Context) {
	var req struct {
		AdminPassword string `json:"adminPassword"`
	}
	_ = c.ShouldBindJSON(&req)
	if !requireAdminPassword(c, req.AdminPassword) {
		return
	}

	if _, err := storage.RestoreLastGoodMySQLConfig(systemDB, fieldKey); err != nil {
		utils.Error(c, 500, "恢复配置失败："+err.Error())
		return
	}

	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")
	id, _ := userID.(int64)
	name, _ := username.(string)
	_ = storage.CreateAuditLog(systemDB, &id, name, "SYSTEM_MYSQL_RESTORE", "system", "mysql", "恢复上一份可用 MySQL 配置", "success", c.ClientIP(), c.GetHeader("User-Agent"))

	utils.Success(c, nil)
}

func localExportBackupHandler(c *gin.Context) {
	var req struct {
		AdminPassword  string `json:"adminPassword"`
		BackupPassword string `json:"backupPassword" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "参数错误")
		return
	}
	if !requireAdminPassword(c, req.AdminPassword) {
		return
	}

	data, err := storage.ExportEncryptedBackup(systemDB, fieldKey, req.BackupPassword)
	if err != nil {
		utils.Error(c, 500, "导出备份失败："+err.Error())
		return
	}

	userID, username, _ := currentIdentity(c)
	_ = storage.CreateAuditLog(systemDB, &userID, username, "BACKUP_EXPORT", "system", "backup", "导出系统配置", "success", c.ClientIP(), c.GetHeader("User-Agent"))

	filename := fmt.Sprintf("wmam-backup-%s.wmam", time.Now().Format("20060102-150405"))
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(http.StatusOK, "application/octet-stream", data)
}

func localImportBackupHandler(c *gin.Context) {
	backupPassword := c.PostForm("backupPassword")
	adminPassword := c.PostForm("adminPassword")
	if !requireAdminPassword(c, adminPassword) {
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		utils.Error(c, 400, "请选择备份文件")
		return
	}
	opened, err := file.Open()
	if err != nil {
		utils.Error(c, 400, "读取备份文件失败")
		return
	}
	defer opened.Close()

	data, err := io.ReadAll(opened)
	if err != nil {
		utils.Error(c, 400, "读取备份文件失败")
		return
	}

	importedFieldKey, err := storage.ImportEncryptedBackup(systemDB, data, backupPassword)
	if err != nil {
		utils.Error(c, 400, "导入备份失败："+err.Error())
		return
	}
	fieldKey = importedFieldKey
	if err := os.MkdirAll(appDataDir, 0750); err != nil {
		utils.Error(c, 500, "保存字段密钥失败")
		return
	}
	if err := os.WriteFile(filepath.Join(appDataDir, "secret.key"), fieldKey, 0600); err != nil {
		utils.Error(c, 500, "保存字段密钥失败")
		return
	}

	userID, username, _ := currentIdentity(c)
	_ = storage.CreateAuditLog(systemDB, &userID, username, "BACKUP_IMPORT", "system", "backup", "导入系统配置", "success", c.ClientIP(), c.GetHeader("User-Agent"))
	clearAuthCookie(c)

	utils.Success(c, gin.H{"message": "导入成功"})
}

func localListProgramsHandler(c *gin.Context) {
	programs, err := storage.ListMiniPrograms(systemDB, false, fieldKey)
	if err != nil {
		utils.Error(c, 500, "获取小程序列表失败")
		return
	}
	utils.Success(c, gin.H{"programs": programs})
}

func localCreateProgramHandler(c *gin.Context) {
	var req struct {
		Name      string `json:"name" binding:"required"`
		AppID     string `json:"appId" binding:"required"`
		AppSecret string `json:"appSecret" binding:"required"`
		Enabled   *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "参数错误")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	program, err := storage.CreateMiniProgram(systemDB, fieldKey, req.Name, req.AppID, req.AppSecret, enabled)
	if err != nil {
		utils.Error(c, 500, "创建小程序失败："+err.Error())
		return
	}

	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")
	id, _ := userID.(int64)
	name, _ := username.(string)
	_ = storage.CreateAuditLog(systemDB, &id, name, "PROGRAM_CREATE", "program", fmt.Sprintf("%d", program.ID), "创建小程序："+program.Name, "success", c.ClientIP(), c.GetHeader("User-Agent"))

	utils.Success(c, gin.H{"program": program})
}

func localUpdateProgramHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.Error(c, 400, "无效的小程序 ID")
		return
	}
	var req struct {
		Name      string `json:"name" binding:"required"`
		AppSecret string `json:"appSecret"`
		Enabled   bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "参数错误")
		return
	}

	program, err := storage.UpdateMiniProgram(systemDB, fieldKey, id, req.Name, req.AppSecret, req.Enabled)
	if err != nil {
		utils.Error(c, 500, "更新小程序失败："+err.Error())
		return
	}

	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")
	operatorID, _ := userID.(int64)
	operatorName, _ := username.(string)
	_ = storage.CreateAuditLog(systemDB, &operatorID, operatorName, "PROGRAM_UPDATE", "program", fmt.Sprintf("%d", program.ID), "更新小程序："+program.Name, "success", c.ClientIP(), c.GetHeader("User-Agent"))

	utils.Success(c, gin.H{"program": program})
}

func localSetProgramStatusHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.Error(c, 400, "无效的小程序 ID")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "参数错误")
		return
	}

	program, err := storage.SetMiniProgramEnabled(systemDB, id, req.Enabled)
	if err != nil {
		utils.Error(c, 500, "更新状态失败："+err.Error())
		return
	}

	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")
	operatorID, _ := userID.(int64)
	operatorName, _ := username.(string)
	action := "PROGRAM_DISABLE"
	if req.Enabled {
		action = "PROGRAM_ENABLE"
	}
	_ = storage.CreateAuditLog(systemDB, &operatorID, operatorName, action, "program", fmt.Sprintf("%d", program.ID), "更新小程序状态："+program.Name, "success", c.ClientIP(), c.GetHeader("User-Agent"))

	utils.Success(c, gin.H{"program": program})
}

func localDeleteProgramHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.Error(c, 400, "无效的小程序 ID")
		return
	}
	program, err := storage.GetMiniProgramByID(systemDB, id, false, fieldKey)
	if err != nil {
		utils.Error(c, 404, "小程序不存在")
		return
	}
	if err := storage.DeleteMiniProgram(systemDB, id); err != nil {
		utils.Error(c, 500, "删除小程序失败："+err.Error())
		return
	}

	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")
	operatorID, _ := userID.(int64)
	operatorName, _ := username.(string)
	_ = storage.CreateAuditLog(systemDB, &operatorID, operatorName, "PROGRAM_DELETE", "program", fmt.Sprintf("%d", program.ID), "删除小程序："+program.Name, "success", c.ClientIP(), c.GetHeader("User-Agent"))

	utils.Success(c, nil)
}

func currentIdentity(c *gin.Context) (int64, string, string) {
	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")
	role, _ := c.Get("role")
	id, _ := userID.(int64)
	name, _ := username.(string)
	roleName, _ := role.(string)
	return id, name, roleName
}

func redactRuntimeError(err error, secrets ...string) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[redacted]")
		}
	}
	return message
}

func runFetchJob(jobID int64, operatorID int64, operatorName string) {
	failures := make([]string, 0)
	failJob := func(errorSummary string, auditDescription string) {
		_, _ = storage.SetFetchJobTerminal(systemDB, jobID, string(jobstate.JobFailed), errorSummary)
		jobEvents.publish(jobID, gin.H{
			"type":    "complete",
			"status":  string(jobstate.JobFailed),
			"message": errorSummary,
		})
		publishJobStateEvent(jobID)
		if auditDescription != "" {
			_ = storage.CreateAuditLog(systemDB, &operatorID, operatorName, "JOB_FAILED", "job", fmt.Sprintf("%d", jobID), auditDescription, "failed", "", "")
		}
	}

	if err := storage.HeartbeatFetchJobLock(systemDB, jobID, storage.FetchJobLockTTL); err != nil {
		failJob("任务锁不可用", "任务锁不可用")
		return
	}
	stopHeartbeat := make(chan struct{})
	var stopHeartbeatOnce sync.Once
	defer stopHeartbeatOnce.Do(func() { close(stopHeartbeat) })
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := storage.HeartbeatFetchJobLock(systemDB, jobID, storage.FetchJobLockTTL); err != nil {
					jobEvents.publish(jobID, gin.H{
						"type":    "log",
						"message": "任务锁续期失败：" + err.Error(),
					})
					return
				}
			case <-stopHeartbeat:
				return
			}
		}
	}()

	mysqlCfg, err := storage.GetMySQLConfig(systemDB, fieldKey, true)
	if err != nil || mysqlCfg.Host == "" {
		failJob("MySQL 配置不可用", "MySQL 配置不可用")
		return
	}

	database, err := getDBConnection(Config{
		Database: DatabaseConfig{
			Host:     mysqlCfg.Host,
			Port:     mysqlCfg.Port,
			User:     mysqlCfg.Username,
			Password: mysqlCfg.Password,
			Database: mysqlCfg.Database,
		},
	})
	if err != nil {
		message := redactRuntimeError(err, mysqlCfg.Password)
		failJob(message, "连接 MySQL 失败")
		return
	}
	defer database.Close()

	if err := database.Ping(); err != nil {
		message := redactRuntimeError(err, mysqlCfg.Password)
		failJob(message, "MySQL 连接测试失败")
		return
	}
	if err := initDatabase(database); err != nil {
		message := redactRuntimeError(err, mysqlCfg.Password)
		failJob(message, "初始化 MySQL 表失败")
		return
	}

	programs, err := storage.ListEnabledMiniProgramsWithSecret(systemDB, fieldKey)
	if err != nil {
		failJob("读取小程序配置失败", "读取小程序配置失败")
		return
	}
	programByID := make(map[int64]storage.MiniProgram, len(programs))
	for _, program := range programs {
		programByID[program.ID] = program
	}

	steps, err := storage.ListFetchJobSteps(systemDB, jobID)
	if err != nil {
		failJob("读取任务步骤失败", "读取任务步骤失败")
		return
	}

	apiBase := "https://api.weixin.qq.com/publisher/stat"
	startDate := "2025-07-01"
	endDate := time.Now().Format("2006-01-02")
	logChan := make(chan string, 100)
	defer close(logChan)
	go func() {
		for line := range logChan {
			if strings.TrimSpace(line) == "" {
				continue
			}
			jobEvents.publish(jobID, gin.H{
				"type":    "log",
				"message": line,
			})
		}
	}()
	jobEvents.publish(jobID, gin.H{"type": "log", "message": "开始执行拉取任务"})

	for _, step := range steps {
		if step.Status == string(jobstate.StepSuccess) {
			continue
		}
		stillRunning, err := storage.IsFetchJobRunning(systemDB, jobID)
		if err != nil || !stillRunning {
			return
		}

		program, ok := programByID[step.ProgramID]
		if !ok {
			message := "小程序已禁用或不存在"
			_ = storage.MarkFetchJobStepFinished(systemDB, jobID, step.ID, string(jobstate.StepFailed), 0, message)
			jobEvents.publish(jobID, gin.H{
				"type":        "step",
				"status":      string(jobstate.StepFailed),
				"programId":   step.ProgramID,
				"programName": step.ProgramName,
				"stepType":    step.StepType,
				"error":       message,
			})
			publishJobStateEvent(jobID)
			failures = append(failures, fmt.Sprintf("%s: %s", step.ProgramName, message))
			continue
		}

		if err := storage.MarkFetchJobStepRunning(systemDB, step.ID, program.ID, program.Name, step.StepType); err != nil {
			jobEvents.publish(jobID, gin.H{
				"type":    "log",
				"message": fmt.Sprintf("%s: 更新步骤状态失败", step.ProgramName),
			})
			failures = append(failures, fmt.Sprintf("%s: 更新步骤状态失败", step.ProgramName))
			continue
		}
		jobEvents.publish(jobID, gin.H{
			"type":        "step",
			"status":      string(jobstate.StepRunning),
			"programId":   program.ID,
			"programName": program.Name,
			"stepType":    step.StepType,
		})
		publishJobStateEvent(jobID)

		token, err := getTokenWithCache(program.AppID, program.AppSecret)
		if err != nil {
			message := redactRuntimeError(err, program.AppSecret)
			_ = storage.MarkFetchJobStepFinished(systemDB, jobID, step.ID, string(jobstate.StepFailed), 0, message)
			jobEvents.publish(jobID, gin.H{
				"type":        "step",
				"status":      string(jobstate.StepFailed),
				"programId":   program.ID,
				"programName": program.Name,
				"stepType":    step.StepType,
				"error":       message,
			})
			publishJobStateEvent(jobID)
			failures = append(failures, fmt.Sprintf("%s/%s: %s", program.Name, step.StepType, message))
			continue
		}

		recordCount := 0
		switch step.StepType {
		case string(jobstate.StepAdunitList):
			err = syncAdUnitList(database, program.Name, program.AppID, program.AppSecret, token, apiBase, logChan)
		case string(jobstate.StepSummary):
			lastDate := getLatestDataDate(database, "publisher_adpos_general", "鏃ユ湡", program.AppID, startDate)
			recordCount, err = syncSummaryData(context.Background(), database, program.Name, program.AppID, program.AppSecret, token, apiBase, lastDate, endDate, logChan)
		case string(jobstate.StepDetail):
			lastDate := getLatestDataDate(database, "publisher_adunit_general", "鏃ユ湡", program.AppID, startDate)
			recordCount, err = syncDetailData(context.Background(), database, program.Name, program.AppID, program.AppSecret, token, apiBase, lastDate, endDate, logChan)
		case string(jobstate.StepSettlement):
			err = syncSettlementData(database, program.Name, program.AppID, program.AppSecret, token, apiBase, logChan)
			if err == nil {
				recordCount = 1
			}
		default:
			err = fmt.Errorf("unknown step type %s", step.StepType)
		}

		if err != nil {
			message := redactRuntimeError(err, mysqlCfg.Password, program.AppSecret)
			_ = storage.MarkFetchJobStepFinished(systemDB, jobID, step.ID, string(jobstate.StepFailed), recordCount, message)
			jobEvents.publish(jobID, gin.H{
				"type":        "step",
				"status":      string(jobstate.StepFailed),
				"programId":   program.ID,
				"programName": program.Name,
				"stepType":    step.StepType,
				"recordCount": recordCount,
				"error":       message,
			})
			publishJobStateEvent(jobID)
			failures = append(failures, fmt.Sprintf("%s/%s: %s", program.Name, step.StepType, message))
			continue
		}
		_ = storage.MarkFetchJobStepFinished(systemDB, jobID, step.ID, string(jobstate.StepSuccess), recordCount, "")
		jobEvents.publish(jobID, gin.H{
			"type":        "step",
			"status":      string(jobstate.StepSuccess),
			"programId":   program.ID,
			"programName": program.Name,
			"stepType":    step.StepType,
			"recordCount": recordCount,
		})
		publishJobStateEvent(jobID)
	}

	stillRunning, err := storage.IsFetchJobRunning(systemDB, jobID)
	if err != nil || !stillRunning {
		return
	}

	finalStatus := string(jobstate.JobCompleted)
	action := "JOB_COMPLETE"
	result := "success"
	errorSummary := ""
	if len(failures) > 0 {
		finalStatus = string(jobstate.JobFailed)
		action = "JOB_FAILED"
		result = "failed"
		errorSummary = strings.Join(failures, "; ")
	}
	_, _ = storage.SetFetchJobTerminal(systemDB, jobID, finalStatus, errorSummary)
	jobEvents.publish(jobID, gin.H{
		"type":    "complete",
		"status":  finalStatus,
		"message": storage.JobStatusLabel(finalStatus),
	})
	publishJobStateEvent(jobID)
	_ = storage.CreateAuditLog(systemDB, &operatorID, operatorName, action, "job", fmt.Sprintf("%d", jobID), storage.JobStatusLabel(finalStatus), result, "", "")
}

func localCurrentJobHandler(c *gin.Context) {
	userID, _, role := currentIdentity(c)
	job, err := storage.GetLatestFetchJob(systemDB)
	if err != nil {
		utils.Error(c, 500, "获取任务状态失败")
		return
	}
	if job == nil {
		utils.Success(c, gin.H{
			"job":         nil,
			"permissions": storage.ComputeJobPermissions(nil, userID, role),
			"steps":       []storage.FetchJobStep{},
		})
		return
	}

	steps := []storage.FetchJobStep{}
	if storage.CanOperateJob(job, userID, role) {
		loadedSteps, err := storage.ListFetchJobSteps(systemDB, job.ID)
		if err != nil {
			utils.Error(c, 500, "获取任务步骤失败")
			return
		}
		steps = loadedSteps
	}

	utils.Success(c, gin.H{
		"job":         job,
		"permissions": storage.ComputeJobPermissions(job, userID, role),
		"steps":       steps,
	})
}

func localStartJobHandler(c *gin.Context) {
	userID, username, _ := currentIdentity(c)
	programs, err := storage.ListEnabledMiniProgramsWithSecret(systemDB, fieldKey)
	if err != nil {
		utils.Error(c, 500, "读取小程序配置失败")
		return
	}

	job, err := storage.CreateFetchJob(systemDB, userID, username, programs)
	if err != nil {
		utils.Error(c, 409, "创建任务失败："+err.Error())
		return
	}

	_ = storage.CreateAuditLog(systemDB, &userID, username, "JOB_START", "job", fmt.Sprintf("%d", job.ID), "开始拉取任务", "success", c.ClientIP(), c.GetHeader("User-Agent"))
	jobEvents.publish(job.ID, gin.H{"type": "log", "message": "任务已创建"})
	go runFetchJob(job.ID, userID, username)
	utils.Success(c, gin.H{"job": job})
}

func localInterruptJobHandler(c *gin.Context) {
	jobID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.Error(c, 400, "无效的任务 ID")
		return
	}
	userID, username, role := currentIdentity(c)
	job, err := storage.GetFetchJobByID(systemDB, jobID)
	if err != nil || job == nil {
		utils.Error(c, 404, "任务不存在")
		return
	}
	if !storage.CanOperateJob(job, userID, role) {
		utils.Error(c, 403, "无权操作该任务")
		return
	}

	job, err = storage.InterruptFetchJob(systemDB, jobID)
	if err != nil {
		utils.Error(c, 409, "中断任务失败："+err.Error())
		return
	}
	_ = storage.CreateAuditLog(systemDB, &userID, username, "JOB_INTERRUPT", "job", fmt.Sprintf("%d", job.ID), "中断拉取任务", "success", c.ClientIP(), c.GetHeader("User-Agent"))
	jobEvents.publish(jobID, gin.H{
		"type":    "complete",
		"status":  job.Status,
		"message": "任务已中断",
	})
	publishJobStateEvent(jobID)
	utils.Success(c, gin.H{"job": job})
}

func localResumeJobHandler(c *gin.Context) {
	jobID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.Error(c, 400, "无效的任务 ID")
		return
	}
	userID, username, role := currentIdentity(c)
	job, err := storage.GetFetchJobByID(systemDB, jobID)
	if err != nil || job == nil {
		utils.Error(c, 404, "任务不存在")
		return
	}
	if !storage.CanOperateJob(job, userID, role) {
		utils.Error(c, 403, "无权操作该任务")
		return
	}

	job, err = storage.ResumeFetchJob(systemDB, jobID, userID, username)
	if err != nil {
		utils.Error(c, 409, "继续任务失败："+err.Error())
		return
	}
	_ = storage.CreateAuditLog(systemDB, &userID, username, "JOB_RESUME", "job", fmt.Sprintf("%d", job.ID), "继续拉取任务", "success", c.ClientIP(), c.GetHeader("User-Agent"))
	jobEvents.publish(jobID, gin.H{"type": "log", "message": "任务继续执行"})
	publishJobStateEvent(jobID)
	go runFetchJob(job.ID, userID, username)
	utils.Success(c, gin.H{"job": job})
}

func localEndJobHandler(c *gin.Context) {
	jobID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.Error(c, 400, "无效的任务 ID")
		return
	}
	userID, username, role := currentIdentity(c)
	job, err := storage.GetFetchJobByID(systemDB, jobID)
	if err != nil || job == nil {
		utils.Error(c, 404, "任务不存在")
		return
	}
	if !storage.CanOperateJob(job, userID, role) {
		utils.Error(c, 403, "无权操作该任务")
		return
	}

	job, err = storage.EndFetchJob(systemDB, jobID)
	if err != nil {
		utils.Error(c, 409, "结束任务失败："+err.Error())
		return
	}
	_ = storage.CreateAuditLog(systemDB, &userID, username, "JOB_END", "job", fmt.Sprintf("%d", job.ID), "结束拉取任务", "success", c.ClientIP(), c.GetHeader("User-Agent"))
	jobEvents.publish(jobID, gin.H{
		"type":    "complete",
		"status":  job.Status,
		"message": "任务已结束",
	})
	publishJobStateEvent(jobID)
	utils.Success(c, gin.H{"job": job})
}

func localListJobsHandler(c *gin.Context) {
	userID, _, role := currentIdentity(c)
	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		if parsed, err := strconv.Atoi(pageStr); err == nil && parsed > 0 {
			page = parsed
		}
	}
	limit := 20
	offset := (page - 1) * limit

	jobs, total, err := storage.ListFetchJobs(systemDB, userID, role == "admin", offset, limit)
	if err != nil {
		utils.Error(c, 500, "获取任务列表失败")
		return
	}
	utils.Success(c, gin.H{"jobs": jobs, "total": total})
}

func localJobDetailHandler(c *gin.Context) {
	jobID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.Error(c, 400, "无效的任务 ID")
		return
	}
	userID, _, role := currentIdentity(c)
	job, err := storage.GetFetchJobByID(systemDB, jobID)
	if err != nil || job == nil {
		utils.Error(c, 404, "任务不存在")
		return
	}
	if !storage.CanOperateJob(job, userID, role) {
		utils.Error(c, 403, "无权查看该任务")
		return
	}
	steps, err := storage.ListFetchJobSteps(systemDB, job.ID)
	if err != nil {
		utils.Error(c, 500, "获取任务步骤失败")
		return
	}
	utils.Success(c, gin.H{"job": job, "steps": steps})
}

func localJobEventsHandler(c *gin.Context) {
	jobID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.Error(c, 400, "无效的任务 ID")
		return
	}
	userID, _, role := currentIdentity(c)
	job, err := storage.GetFetchJobByID(systemDB, jobID)
	if err != nil || job == nil {
		utils.Error(c, 404, "任务不存在")
		return
	}
	if !storage.CanOperateJob(job, userID, role) {
		utils.Error(c, 403, "无权查看该任务")
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		utils.Error(c, 500, "当前连接不支持实时日志")
		return
	}

	ch := jobEvents.subscribe(jobID)
	defer jobEvents.unsubscribe(jobID, ch)

	c.Writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	c.Writer.Header().Set("Cache-Control", "no-cache, no-transform")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Status(http.StatusOK)

	ready, _ := json.Marshal(gin.H{
		"type":            "ready",
		"status":          job.Status,
		"progressPercent": job.ProgressPercent,
		"time":            time.Now().Format("15:04:05"),
	})
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", ready); err != nil {
		return
	}
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := fmt.Fprint(c.Writer, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func serveFrontendIndex(c *gin.Context) {
	indexHTML, err := frontendFS.ReadFile("frontend/index.html")
	if err != nil {
		c.String(http.StatusInternalServerError, "加载前端页面失败")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
}

func loginHandler(c *gin.Context) {
	var req struct {
		Username         string `json:"username" binding:"required"`
		Password         string `json:"password" binding:"required"`
		RememberPassword bool   `json:"rememberPassword"`
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
		"user_count":         userCount,
		"mini_program_count": miniProgramCount,
		"today_fetch_count":  todayFetchCount,
		"last_fetch_time":    lastFetchStr,
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
	"publisher_adpos_general":  true,
	"publisher_adunit_general": true,
	"publisher_settlement":     true,
	"adunit_list":              true,
	"mini_program":             true,
	"fetch_log":                true,
}

var allowedColumns = map[string]bool{
	"日期":       true,
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
	appCfg, err := appconfig.Load("config.yaml")
	if err != nil {
		log.Fatalf("读取配置失败: %v", err)
	}
	appDataDir = appCfg.Data.Dir

	fieldKey, err = security.LoadOrCreateFieldKey(appCfg.Data.Dir)
	if err != nil {
		log.Fatalf("初始化字段加密密钥失败: %v", err)
	}

	systemDB, err = storage.OpenSystemDB(appCfg.Data.Dir)
	if err != nil {
		log.Fatalf("初始化本地系统存储失败: %v", err)
	}
	defer systemDB.Close()

	defaultAdminHash, err := utils.HashPassword("admin123")
	if err != nil {
		log.Fatalf("初始化默认管理员失败: %v", err)
	}
	adminCreated, err := storage.EnsureDefaultAdmin(systemDB, defaultAdminHash)
	if err != nil {
		log.Fatalf("初始化默认管理员失败: %v", err)
	}
	if adminCreated {
		log.Println("默认管理员账号已创建: admin / admin123")
	}
	recoveryCode, err := security.NewRecoveryCode()
	if err != nil {
		log.Fatalf("生成管理员恢复码失败: %v", err)
	}
	recoveryHash, err := utils.HashPassword(recoveryCode)
	if err != nil {
		log.Fatalf("保存管理员恢复码失败: %v", err)
	}
	recoveryCreated, err := storage.EnsureAdminRecoveryHash(systemDB, recoveryHash)
	if err != nil {
		log.Fatalf("保存管理员恢复码失败: %v", err)
	}
	if recoveryCreated {
		log.Printf("管理员恢复码仅显示一次，请保存: %s", recoveryCode)
	}

	startTokenCleanupJob()

	r := gin.Default()

	auth := r.Group("/api")
	{
		auth.POST("/auth/login", localLoginHandler)
		auth.POST("/auth/recover-admin", localRecoverAdminHandler)
		auth.POST("/auth/logout", localAuthMiddleware(), localLogoutHandler)
		auth.GET("/auth/me", localAuthMiddleware(), localCurrentUserHandler)
		auth.POST("/auth/change-password", localAuthMiddleware(), localChangePasswordHandler)
	}

	api := r.Group("/api")
	api.Use(localAuthMiddleware())
	{
		api.GET("/dashboard/stats", localDashboardStatsHandler)
		api.GET("/programs", localListProgramsHandler)
		api.GET("/mini-programs", localListProgramsHandler)
		api.GET("/jobs/current", localCurrentJobHandler)
		api.GET("/jobs", localListJobsHandler)
		api.GET("/jobs/:id/events", localJobEventsHandler)
		api.GET("/jobs/:id", localJobDetailHandler)
		api.POST("/jobs/start", localStartJobHandler)
		api.POST("/jobs/:id/interrupt", localInterruptJobHandler)
		api.POST("/jobs/:id/resume", localResumeJobHandler)
		api.POST("/jobs/:id/end", localEndJobHandler)
		api.POST("/fetch/execute", localStartJobHandler)
		api.GET("/logs", localGetLogsHandler)
		api.GET("/audit-logs", localGetLogsHandler)
	}

	admin := r.Group("/api")
	admin.Use(localAuthMiddleware(), middleware.AdminMiddleware())
	{
		admin.GET("/users", localGetUsersHandler)
		admin.POST("/users", localCreateUserHandler)
		admin.PUT("/users/:id", localUpdateUserHandler)
		admin.POST("/users/:id/reset-password", localResetUserPasswordHandler)
		admin.DELETE("/users/:id", localDeleteUserHandler)
		admin.GET("/system/mysql", localGetMySQLConfigHandler)
		admin.POST("/system/mysql/test", localTestMySQLConfigHandler)
		admin.PUT("/system/mysql", localSaveMySQLConfigHandler)
		admin.POST("/system/mysql/restore-last-good", localRestoreMySQLConfigHandler)
		admin.POST("/system/backup/export", localExportBackupHandler)
		admin.POST("/system/backup/import", localImportBackupHandler)
		admin.GET("/config", localGetMySQLConfigHandler)
		admin.POST("/test-connection", localTestMySQLConfigHandler)
		admin.POST("/config", localSaveMySQLConfigHandler)
		admin.POST("/programs", localCreateProgramHandler)
		admin.PUT("/programs/:id", localUpdateProgramHandler)
		admin.POST("/programs/:id/status", localSetProgramStatusHandler)
		admin.DELETE("/programs/:id", localDeleteProgramHandler)
	}

	assetsFS, err := fs.Sub(frontendFS, "frontend/assets")
	if err != nil {
		log.Fatalf("加载前端资源失败: %v", err)
	}
	r.StaticFS("/assets", http.FS(assetsFS))
	r.GET("/", serveFrontendIndex)
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			utils.Error(c, http.StatusNotFound, "接口不存在")
			return
		}
		serveFrontendIndex(c)
	})

	port := appCfg.Server.Port
	host := strings.TrimSpace(appCfg.Server.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	browserHost := host
	if browserHost == "0.0.0.0" {
		browserHost = "localhost"
	}
	webUrl := fmt.Sprintf("http://%s:%d/", browserHost, port)

	go func() {
		time.Sleep(500 * time.Millisecond)
		openBrowser(webUrl)
	}()

	log.Printf("🚀 服务器已启动: %s", webUrl)
	log.Printf("💡 默认管理员: admin / admin123")
	log.Printf("💡 按 Ctrl+C 停止服务器")
	log.Fatal(r.Run(addr))
}
