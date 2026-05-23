package main

import (
	"context"
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
	"strings"
	"sync"
	"time"

	"database/sql"
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
			// 返回默认配置
			return Config{
				Database: DatabaseConfig{
					Host: "",
					Port: 3306,
					User: "",
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

	// 解析.env文件
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

	// 解析数据库配置
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

	// 解析基础配置
	cfg.Settings.APIBase = kvMap["API_BASE"]
	if cfg.Settings.APIBase == "" {
		cfg.Settings.APIBase = "https://api.weixin.qq.com/publisher/stat"
	}
	cfg.Settings.StartDate = kvMap["START_DATE"]

	// 解析小程序配置
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

		// 验证小程序配置完整性
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

	// 添加数据库配置
	lines = append(lines, "# Database Configuration")
	lines = append(lines, fmt.Sprintf("DB_HOST=%s", cfg.Database.Host))
	lines = append(lines, fmt.Sprintf("DB_PORT=%d", cfg.Database.Port))
	lines = append(lines, fmt.Sprintf("DB_USER=%s", cfg.Database.User))
	lines = append(lines, fmt.Sprintf("DB_PASSWORD=%s", cfg.Database.Password))
	lines = append(lines, fmt.Sprintf("DB_DATABASE=%s", cfg.Database.Database))
	lines = append(lines, "")

	// 添加基础配置
	lines = append(lines, "# API Configuration")
	lines = append(lines, fmt.Sprintf("API_BASE=%s", cfg.Settings.APIBase))
	lines = append(lines, fmt.Sprintf("START_DATE=%s", cfg.Settings.StartDate))
	lines = append(lines, "")

	// 添加小程序配置
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

	// 写入文件
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
	
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Minute * 30)
	db.SetConnMaxIdleTime(time.Minute * 10)
	
	return db, nil
}

func testDatabaseConnection(w http.ResponseWriter, r *http.Request) {
	var cfg DatabaseConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	config, _ := loadConfig()
	config.Database = cfg

	db, err := getDBConnection(config)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "连接成功",
	})
}

func getConfig(w http.ResponseWriter, r *http.Request) {
	cfg, _ := loadConfig()
	json.NewEncoder(w).Encode(cfg)
}

func validateMiniProgramConfig(programs []MiniProgramConfig) ([]MiniProgramConfig, error) {
	var validPrograms []MiniProgramConfig
	for i, p := range programs {
		name := strings.TrimSpace(p.Name)
		appid := strings.TrimSpace(p.AppID)
		secret := strings.TrimSpace(p.AppSecret)

		if name == "" {
			return nil, fmt.Errorf("第 %d 个小程序名称不能为空", i+1)
		}
		if appid == "" {
			return nil, fmt.Errorf("第 %d 个小程序AppID不能为空", i+1)
		}
		if secret == "" {
			return nil, fmt.Errorf("第 %d 个小程序AppSecret不能为空", i+1)
		}

		validPrograms = append(validPrograms, MiniProgramConfig{
			Name:     name,
			AppID:    appid,
			AppSecret: secret,
		})
	}
	return validPrograms, nil
}

func saveConfigHandler(w http.ResponseWriter, r *http.Request) {
	var newCfg Config
	if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	oldCfg, _ := loadConfig()

	// 合并数据库配置
	if newCfg.Database.Host == "" {
		newCfg.Database = oldCfg.Database
	} else {
		if newCfg.Database.Port == 0 {
			newCfg.Database.Port = 3306
		}
	}

	// 验证并合并小程序配置
	if len(newCfg.MiniPrograms) == 0 {
		newCfg.MiniPrograms = oldCfg.MiniPrograms
	} else {
		validPrograms, err := validateMiniProgramConfig(newCfg.MiniPrograms)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		newCfg.MiniPrograms = validPrograms
	}

	// 基础配置（起始日期硬编码为2025-07-01）
	newCfg.Settings.StartDate = "2025-07-01"
	newCfg.Settings.APIBase = oldCfg.Settings.APIBase
	if newCfg.Settings.APIBase == "" {
		newCfg.Settings.APIBase = "https://api.weixin.qq.com/publisher/stat"
	}

	if err := saveConfig(newCfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='小程序配置表';`,
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='广告位清单表';`,
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='广告汇总数据表';`,
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='广告细分数据表';`,
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='结算数据表';`,
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='数据拉取日志表';`,
	}

	for _, table := range tables {
		_, err := db.Exec(table)
		if err != nil {
			return err
		}
	}
	return nil
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

func cleanupExpiredTokens() {
	now := time.Now().UnixNano() / 1e6
	tokenCaches.Range(func(key, value interface{}) bool {
		cache := value.(tokenCache)
		if now >= cache.expireTime {
			tokenCaches.Delete(key)
			log.Printf("[cleanupExpiredTokens] 已清理过期Token - AppID: %v", key)
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
	log.Println("[TokenCleanup] Token清理任务已启动（每5分钟执行一次）")
}

func fetchDataWithRetry(token, action, appid, appsecret, apiBase string, extraParams map[string]string) (map[string]interface{}, error) {
	currentToken := token
	retryCount := 0
	tokenRefreshCount := 0
	maxRetries := 3
	maxTokenRefresh := 5
	
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
				if tokenRefreshCount >= maxTokenRefresh {
					return nil, fmt.Errorf("Token刷新次数超过限制")
				}
				currentToken, err = getToken(appid, appsecret)
				if err != nil {
					return nil, err
				}
				tokenRefreshCount++
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

var AD_SLOT_NAMES = map[string]string{
	"SLOT_ID_WEAPP_VIDEO_BEGIN":   "视频贴片",
	"SLOT_ID_WEAPP_INTERSTITIAL":  "插屏",
	"SLOT_ID_WEAPP_BANNER":        "Banner",
	"SLOT_ID_WEAPP_REWARD_VIDEO":  "激励视频",
	"SLOT_ID_WEAPP_TEMPLATE":      "原生模版",
	"SLOT_ID_WEAPP_COVER":         "封面",
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

func saveMiniProgram(db *sql.DB, name, appid, appsecret string) error {
	_, err := db.Exec(`
		INSERT INTO mini_program (名称, 小程序ID, 小程序Secret)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE
			名称 = VALUES(名称),
			小程序Secret = VALUES(小程序Secret),
			更新时间 = CURRENT_TIMESTAMP
	`, name, appid, appsecret)
	return err
}

func saveAdunitList(db *sql.DB, name, appid string, list []map[string]interface{}) error {
	if len(list) == 0 {
		return nil
	}
	
	batchSize := 100
	for i := 0; i < len(list); i += batchSize {
		end := i + batchSize
		if end > len(list) {
			end = len(list)
		}
		batch := list[i:end]
		
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
				templTypeList = ""
				for i, t := range types {
					if i > 0 {
						templTypeList += ","
					}
					templTypeList += t
				}
			}
			
			valueStrings = append(valueStrings, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
			valueArgs = append(valueArgs, name, appid, adUnitID, adUnitName, adSlot, adSlotName, adUnitType, adUnitStatus, statusName, slotID, adUnitSize, allowPlayable, videoMin, videoMax, templTypeList)
		}
		
		query := `INSERT INTO adunit_list (小程序名称, 小程序ID, 广告位唯一ID, 广告位名称, 广告位类型枚举, 广告位类型,
			广告位类型值, 状态, 状态名称, 广告位数字ID, 广告尺寸, 是否允许可播放,
			视频最短时长, 视频最长时长, 模版类型列表)
			VALUES ` + strings.Join(valueStrings, ",") + `
			ON DUPLICATE KEY UPDATE
				广告位名称 = VALUES(广告位名称),
				广告位类型枚举 = VALUES(广告位类型枚举),
				广告位类型 = VALUES(广告位类型),
				状态 = VALUES(状态),
				状态名称 = VALUES(状态名称),
				更新时间 = CURRENT_TIMESTAMP`
		
		_, err := db.Exec(query, valueArgs...)
		if err != nil {
			return err
		}
	}
	
	return nil
}

func saveSummaryData(db *sql.DB, name, appid string, list []map[string]interface{}) error {
	if len(list) == 0 {
		return nil
	}

	batchSize := 100
	for i := 0; i < len(list); i += batchSize {
		end := i + batchSize
		if end > len(list) {
			end = len(list)
		}
		batch := list[i:end]
		
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
			valueArgs = append(valueArgs, name, appid, dateVal, adSlot, adSlotName, slotStr, reqSuccCount, exposureCount, formatRate(exposureRate), clickCount, formatRate(clickRate), income, fenToYuan(float64(income)), ecpm, fenToYuan(ecpm))
		}
		
		query := `INSERT INTO publisher_adpos_general (小程序名称, 小程序ID, 日期, 广告位类型枚举, 广告位类型, 广告位数字ID,
			成功请求次数, 曝光量, 曝光率, 点击量, 点击率, 总收入分, 总收入元, 千次曝光收入分, 千次曝光收入元)
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
		
		_, err := db.Exec(query, valueArgs...)
		if err != nil {
			return err
		}
	}
	
	return nil
}

func saveDetailData(db *sql.DB, name, appid string, list []map[string]interface{}) error {
	if len(list) == 0 {
		return nil
	}
	
	batchSize := 100
	for i := 0; i < len(list); i += batchSize {
		end := i + batchSize
		if end > len(list) {
			end = len(list)
		}
		batch := list[i:end]
		
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
			valueArgs = append(valueArgs, name, appid, adUnitID, adUnitName, dateVal, adSlot, adSlotName, slotStr, reqSuccCount, exposureCount, formatRate(exposureRate), clickCount, formatRate(clickRate), income, fenToYuan(float64(income)), publisherIncome, fenToYuan(float64(publisherIncome)), agencyIncome, ecpm, fenToYuan(ecpm), isSmartAds, parentTemplType)
		}
		
		query := `INSERT INTO publisher_adunit_general (小程序名称, 小程序ID, 广告位唯一ID, 广告位名称, 日期,
			广告位类型枚举, 广告位类型, 广告位数字ID, 成功请求次数, 曝光量, 曝光率, 点击量, 点击率,
			总收入分, 总收入元, 流量主收入分, 流量主收入元, 代理商收入分, 千次曝光收入分, 千次曝光收入元, 是否智能广告, 父模版类型)
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
		
		_, err := db.Exec(query, valueArgs...)
		if err != nil {
			return err
		}
	}
	
	return nil
}

func saveSettlementData(db *sql.DB, name, appid string, data map[string]interface{}) error {
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
	
	_, err := db.Exec(`
		INSERT INTO publisher_settlement (小程序名称, 小程序ID, 总预估收入分, 总预估收入元,
			总已结算收入分, 总已结算收入元, 总罚金分, 总罚金元,
			微信云开发总预估收入分, 微信云开发总已结算收入分, 微信云开发总罚金分, 数据拉取日期)
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
	`, name, appid, revenueAll, fenToYuan(float64(revenueAll)), settledRevenueAll, fenToYuan(float64(settledRevenueAll)), penaltyAll, fenToYuan(float64(penaltyAll)), wywRevenueAll, wywSettledRevenueAll, wywPenaltyAll)
	return err
}

func logFetch(db *sql.DB, name, appid, fetchType, fetchDate, status string, totalCount int, errorMessage string) error {
	if fetchDate == "" {
		fetchDate = time.Now().Format("2006-01-02")
	}
	_, err := db.Exec(`
		INSERT INTO fetch_log (小程序名称, 小程序ID, 拉取类型, 拉取日期, 状态, 记录数, 错误信息)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, name, appid, fetchType, fetchDate, status, totalCount, errorMessage)
	return err
}

var allowedTables = map[string]bool{
	"publisher_adpos_general":    true,
	"publisher_adunit_general":   true,
	"publisher_settlement":       true,
	"adunit_list":                true,
	"mini_program":               true,
	"fetch_log":                  true,
}

var allowedColumns = map[string]bool{
	"日期":         true,
	"data拉取日期": true,
}

func getLatestDataDate(db *sql.DB, tableName, dateColumn, appid string, defaultDate string) string {
	if !allowedTables[tableName] {
		log.Printf("[getLatestDataDate] 不允许的数据表名: %s", tableName)
		return defaultDate
	}
	if !allowedColumns[dateColumn] {
		log.Printf("[getLatestDataDate] 不允许的字段名: %s", dateColumn)
		return defaultDate
	}
	
	var latestDate sql.NullString
	query := fmt.Sprintf(`SELECT MAX(%s) as latest_date FROM %s WHERE 小程序ID = ?`, dateColumn, tableName)
	
	err := db.QueryRow(query, appid).Scan(&latestDate)
	
	if err != nil {
		log.Printf("[getLatestDataDate] 查询失败 - 小程序ID: %s, 数据表: %s, 错误: %v", appid, tableName, err)
		return defaultDate
	}
	
	if !latestDate.Valid || latestDate.String == "" {
		log.Printf("[getLatestDataDate] 无数据 - 小程序ID: %s, 数据表: %s", appid, tableName)
		return defaultDate
	}
	
	dateStr := latestDate.String
	if len(dateStr) > 10 {
		dateStr = dateStr[:10]
	}
	
	d, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		log.Printf("[getLatestDataDate] 日期解析失败 - 小程序ID: %s, 原始日期: %s, 错误: %v", appid, latestDate.String, err)
		return defaultDate
	}
	
	d = d.AddDate(0, 0, 1)
	log.Printf("[getLatestDataDate] 查询成功 - 小程序ID: %s, 最新日期: %s, 拉取起始日期: %s", appid, dateStr, d.Format("2006-01-02"))
	return d.Format("2006-01-02")
}

func syncAdUnitList(db *sql.DB, miniProgramName, appid, appsecret, accessToken, apiBase string, logChan chan<- string) error {
	logChan <- fmt.Sprintf("✅ [%s] 正在获取广告位列表...", miniProgramName)
	
	data, err := fetchDataWithRetry(accessToken, "get_adunit_list", appid, appsecret, apiBase, map[string]string{})
	if err != nil {
		return fmt.Errorf("获取广告位列表失败: %v", err)
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
		if err := saveAdunitList(db, miniProgramName, appid, adUnits); err != nil {
			return fmt.Errorf("保存广告位列表失败: %v", err)
		}
		logChan <- fmt.Sprintf("✅ [%s] 广告位清单已保存", miniProgramName)
	}
	
	if err := logFetch(db, miniProgramName, appid, "adunit_list", "", "success", len(adUnits), ""); err != nil {
		logChan <- fmt.Sprintf("⚠️ [%s] 记录日志失败: %v", miniProgramName, err)
	}
	
	logChan <- fmt.Sprintf("✅ [%s] 广告位列表同步完成，共 %d 个广告位", miniProgramName, len(adUnits))
	return nil
}

func syncSummaryData(ctx context.Context, db *sql.DB, miniProgramName, appid, appsecret, accessToken, apiBase, startDate, endDate string, logChan chan<- string) (int, error) {
	logChan <- fmt.Sprintf("✅ [%s] 正在获取 %s 至 %s 的汇总数据...", miniProgramName, startDate, endDate)
	
	ranges, err := getMonthRanges(startDate, endDate)
	if err != nil {
		return 0, fmt.Errorf("生成日期范围失败: %v", err)
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
			if err := saveSummaryData(db, miniProgramName, appid, data); err != nil {
				logChan <- fmt.Sprintf("❌ [%s] %s ~ %s: 保存失败 - %v", miniProgramName, r["start"], r["end"], err)
			} else {
				totalCount += len(data)
				logChan <- fmt.Sprintf("✅ [%s] %s ~ %s: %d 条", miniProgramName, r["start"], r["end"], len(data))
			}
		} else {
			logChan <- fmt.Sprintf("ℹ️ [%s] %s ~ %s: 无数据", miniProgramName, r["start"], r["end"])
		}
		
		time.Sleep(500 * time.Millisecond)
	}
	
	return totalCount, nil
}

func syncDetailData(ctx context.Context, db *sql.DB, miniProgramName, appid, appsecret, accessToken, apiBase, startDate, endDate string, logChan chan<- string) (int, error) {
	logChan <- fmt.Sprintf("✅ [%s] 正在获取 %s 至 %s 的细分数据...", miniProgramName, startDate, endDate)
	
	ranges, err := getMonthRanges(startDate, endDate)
	if err != nil {
		return 0, fmt.Errorf("生成日期范围失败: %v", err)
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
			if err := saveDetailData(db, miniProgramName, appid, data); err != nil {
				logChan <- fmt.Sprintf("❌ [%s] %s ~ %s: 保存失败 - %v", miniProgramName, r["start"], r["end"], err)
			} else {
				totalCount += len(data)
				logChan <- fmt.Sprintf("✅ [%s] %s ~ %s: %d 条", miniProgramName, r["start"], r["end"], len(data))
			}
		} else {
			logChan <- fmt.Sprintf("ℹ️ [%s] %s ~ %s: 无数据", miniProgramName, r["start"], r["end"])
		}
		
		time.Sleep(500 * time.Millisecond)
	}
	
	return totalCount, nil
}

func syncSettlementData(db *sql.DB, miniProgramName, appid, appsecret, accessToken, apiBase string, logChan chan<- string) error {
	logChan <- fmt.Sprintf("✅ [%s] 正在获取结算数据...", miniProgramName)
	
	data, err := fetchDataWithRetry(accessToken, "publisher_settlement", appid, appsecret, apiBase, map[string]string{})
	if err != nil {
		return fmt.Errorf("获取结算数据失败: %v", err)
	}
	
	if err := saveSettlementData(db, miniProgramName, appid, data); err != nil {
		return fmt.Errorf("保存结算数据失败: %v", err)
	}
	
	revenueAll := 0.0
	if v, ok := data["revenue_all"].(float64); ok {
		revenueAll = v
	}
	
	settledRevenueAll := 0.0
	if v, ok := data["settled_revenue_all"].(float64); ok {
		settledRevenueAll = v
	}
	
	logChan <- fmt.Sprintf("✅ [%s] 结算数据已保存", miniProgramName)
	logChan <- fmt.Sprintf("ℹ️ [%s] 总预估收入: %.2f 元", miniProgramName, fenToYuan(revenueAll))
	logChan <- fmt.Sprintf("ℹ️ [%s] 总已结算收入: %.2f 元", miniProgramName, fenToYuan(settledRevenueAll))
	
	if err := logFetch(db, miniProgramName, appid, "publisher_settlement", time.Now().Format("2006-01-02"), "success", 1, ""); err != nil {
		logChan <- fmt.Sprintf("⚠️ [%s] 记录日志失败: %v", miniProgramName, err)
	}
	
	return nil
}

type ProgressState struct {
	CurrentProgram  int
	TotalPrograms int
	CurrentStep  int
	TotalSteps  int
	Message string
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

func executeFetch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	ctx := r.Context()

	// 创建日志通道
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
		db, err := getDBConnection(cfg)
		if err != nil {
			logChan <- fmt.Sprintf("❌ 连接数据库失败: %v", err)
			return
		}
		defer db.Close()

		if err := db.Ping(); err != nil {
			logChan <- fmt.Sprintf("❌ 数据库连接测试失败: %v", err)
			return
		}
		logChan <- "✅ 数据库连接成功"

		logChan <- "📝 正在初始化数据库表..."
		if err := initDatabase(db); err != nil {
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
				return
			default:
			}

			programStartTime := time.Now().UnixMilli()
			logChan <- fmt.Sprintf("\n====== 处理小程序: %s (%s) ======", mp.Name, mp.AppID)

			// 步骤1: 准备
			currentStep++
			sendProgress(logChan, ProgressState{
				CurrentProgram: programIdx + 1,
				TotalPrograms: len(cfg.MiniPrograms),
				CurrentStep: currentStep,
				TotalSteps: totalSteps,
				Message: fmt.Sprintf("正在初始化 %s...", mp.Name),
			})

			if err := saveMiniProgram(db, mp.Name, mp.AppID, mp.AppSecret); err != nil {
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

			// 步骤2: 拉取广告位清单
			currentStep++
			sendProgress(logChan, ProgressState{
				CurrentProgram: programIdx + 1,
				TotalPrograms: len(cfg.MiniPrograms),
				CurrentStep: currentStep,
				TotalSteps: totalSteps,
				Message: fmt.Sprintf("正在获取 %s 的广告位清单...", mp.Name),
			})

			logChan <- "\n----- 1. 拉取广告位清单 -----"
			if err := syncAdUnitList(db, mp.Name, mp.AppID, mp.AppSecret, token, apiBase, logChan); err != nil {
				logChan <- fmt.Sprintf("❌ [%s] 同步广告位列表失败: %v", mp.Name, err)
			}

			endDate := time.Now().Format("2006-01-02")
			startDate := cfg.Settings.StartDate
			if startDate == "" {
				startDate = "2025-07-01"
			}
			logChan <- fmt.Sprintf("起始日期: %s", startDate)

			// 步骤3: 拉取汇总数据
			currentStep++
			sendProgress(logChan, ProgressState{
				CurrentProgram: programIdx + 1,
				TotalPrograms: len(cfg.MiniPrograms),
				CurrentStep: currentStep,
				TotalSteps: totalSteps,
				Message: fmt.Sprintf("正在获取 %s 的汇总数据...", mp.Name),
			})

			logChan <- "\n----- 2. 拉取汇总数据 -----"
			lastSummaryDate := getLatestDataDate(db, "publisher_adpos_general", "日期", mp.AppID, startDate)
			if lastSummaryDate == startDate {
				logChan <- fmt.Sprintf("📊 首次拉取汇总数据，从 %s 开始...", lastSummaryDate)
			} else {
				logChan <- fmt.Sprintf("📊 增量拉取汇总数据，从 %s 开始（已有数据至前一天）...", lastSummaryDate)
			}
			summaryCount, err := syncSummaryData(ctx, db, mp.Name, mp.AppID, mp.AppSecret, token, apiBase, lastSummaryDate, endDate, logChan)
			if err != nil {
				logChan <- fmt.Sprintf("❌ [%s] 同步汇总数据失败: %v", mp.Name, err)
			} else {
				logChan <- fmt.Sprintf("汇总数据拉取完成, 共 %d 条", summaryCount)
				if err := logFetch(db, mp.Name, mp.AppID, "publisher_adpos_general", endDate, "success", summaryCount, ""); err != nil {
					logChan <- fmt.Sprintf("⚠️ [%s] 记录日志失败: %v", mp.Name, err)
				}
			}

			// 步骤4: 拉取细分数据
			currentStep++
			sendProgress(logChan, ProgressState{
				CurrentProgram: programIdx + 1,
				TotalPrograms: len(cfg.MiniPrograms),
				CurrentStep: currentStep,
				TotalSteps: totalSteps,
				Message: fmt.Sprintf("正在获取 %s 的细分数据...", mp.Name),
			})

			logChan <- "\n----- 3. 拉取细分数据 -----"
			lastDetailDate := getLatestDataDate(db, "publisher_adunit_general", "日期", mp.AppID, startDate)
			if lastDetailDate == startDate {
				logChan <- fmt.Sprintf("📊 首次拉取细分数据，从 %s 开始...", lastDetailDate)
			} else {
				logChan <- fmt.Sprintf("📊 增量拉取细分数据，从 %s 开始（已有数据至前一天）...", lastDetailDate)
			}
			detailCount, err := syncDetailData(ctx, db, mp.Name, mp.AppID, mp.AppSecret, token, apiBase, lastDetailDate, endDate, logChan)
			if err != nil {
				logChan <- fmt.Sprintf("❌ [%s] 同步细分数据失败: %v", mp.Name, err)
			} else {
				logChan <- fmt.Sprintf("细分数据拉取完成, 共 %d 条", detailCount)
				if err := logFetch(db, mp.Name, mp.AppID, "publisher_adunit_general", endDate, "success", detailCount, ""); err != nil {
					logChan <- fmt.Sprintf("⚠️ [%s] 记录日志失败: %v", mp.Name, err)
				}
			}

			// 步骤1（下一个小程序）之前：拉取结算数据
			currentStep++
			sendProgress(logChan, ProgressState{
				CurrentProgram: programIdx + 1,
				TotalPrograms: len(cfg.MiniPrograms),
				CurrentStep: currentStep,
				TotalSteps: totalSteps,
				Message: fmt.Sprintf("正在获取 %s 的结算数据...", mp.Name),
			})

			logChan <- "\n----- 4. 拉取结算数据 -----"
			if err := syncSettlementData(db, mp.Name, mp.AppID, mp.AppSecret, token, apiBase, logChan); err != nil {
				logChan <- fmt.Sprintf("❌ [%s] 同步结算数据失败: %v", mp.Name, err)
				if err := logFetch(db, mp.Name, mp.AppID, "publisher_settlement", endDate, "failed", 0, err.Error()); err != nil {
					logChan <- fmt.Sprintf("⚠️ [%s] 记录日志失败: %v", mp.Name, err)
				}
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
		
		// 发送100%进度更新
		sendProgress(logChan, ProgressState{
			CurrentProgram: len(cfg.MiniPrograms),
			TotalPrograms:  len(cfg.MiniPrograms),
			CurrentStep:    totalSteps,
			TotalSteps:     totalSteps,
			Message:        "任务完成",
		})
	}()

	// 实时发送日志
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

func main() {
	// 启动Token清理任务
	startTokenCleanupJob()
	
	// 配置路由
	http.Handle("/", http.FileServer(http.FS(frontendFS)))
	http.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			getConfig(w, r)
		} else if r.Method == http.MethodPost {
			saveConfigHandler(w, r)
		}
	})
	http.HandleFunc("/api/test-connection", testDatabaseConnection)
	http.HandleFunc("/api/execute", executeFetch)

	port := 28384
	addr := fmt.Sprintf(":%d", port)
	url := fmt.Sprintf("http://localhost:%d/frontend/", port)

	// 自动打开浏览器
	go func() {
		time.Sleep(500 * time.Millisecond)
		openBrowser(url)
	}()

	log.Printf("🚀 服务器已启动: %s", url)
	log.Printf("💡 按 Ctrl+C 停止服务器")
	log.Fatal(http.ListenAndServe(addr, nil))
}
