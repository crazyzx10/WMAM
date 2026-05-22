# WMAM - 微信小程序广告数据管理工具

一个功能强大的微信小程序广告数据拉取和管理工具，支持多小程序批量管理，自动拉取广告数据并存储到MySQL数据库。

## 功能特性

- ✅ **多小程序管理**：支持配置多个微信小程序，统一管理
- ✅ **数据自动拉取**：自动从微信广告API拉取各类广告数据
- ✅ **增量更新**：智能识别已拉取日期，只拉取新增数据
- ✅ **Web管理界面**：友好的图形化配置和执行界面
- ✅ **实时进度**：显示数据拉取进度和详细日志
- ✅ **MySQL存储**：结构化存储，支持数据分析

## 拉取的数据类型

1. **广告位清单**：所有广告位的配置信息
2. **广告汇总数据**：按日期和广告位类型的汇总统计
3. **广告细分数据**：按具体广告位的详细统计
4. **结算数据**：收入、已结算、罚金等结算信息

## 技术栈

- **后端**：Go 1.24+
- **数据库**：MySQL 5.7+
- **前端**：原生 HTML/CSS/JavaScript
- **API**：微信广告平台官方API

## 快速开始

### 前置要求

- Go 1.24 或更高版本
- MySQL 5.7 或更高版本
- 微信小程序 AppID 和 AppSecret

### 安装步骤

1. 克隆仓库：
```bash
git clone https://github.com/crazyzx10/addata.git
cd addata/go-app
```

2. 安装依赖：
```bash
go mod download
```

3. 编译运行：
```bash
go run main.go
```

或者直接运行编译好的二进制文件：
```bash
./ad-config-tool.exe  # Windows
./ad-config-tool      # Linux/Mac
```

4. 打开浏览器访问：
```
http://localhost:28384
```

## 使用说明

### 1. 数据库配置

- 访问 Web 界面，切换到「数据库」标签页
- 填写您的 MySQL 数据库连接信息
- 点击「测试连接」验证配置
- 保存配置

### 2. 小程序配置

- 切换到「小程序」标签页
- 点击「+ 添加小程序」
- 填写小程序名称、AppID 和 AppSecret
- 可以添加多个小程序
- 保存配置

### 3. 执行数据拉取

- 切换到「执行日志」标签页
- 点击「开始执行」
- 查看实时进度和日志输出

## 数据库结构

程序会自动创建以下 6 个数据表：

| 表名 | 说明 |
|------|------|
| `mini_program` | 小程序配置表 |
| `adunit_list` | 广告位清单表 |
| `publisher_adpos_general` | 广告汇总数据表 |
| `publisher_adunit_general` | 广告细分数据表 |
| `publisher_settlement` | 结算数据表 |
| `fetch_log` | 数据拉取日志表 |

## 配置文件

程序使用 `.env` 文件存储配置，位于可执行文件同目录下。

```env
# 数据库配置
DB_HOST=localhost
DB_PORT=3306
DB_USER=root
DB_PASSWORD=your_password
DB_DATABASE=ad_data

# API配置
API_BASE=https://api.weixin.qq.com/publisher/stat
START_DATE=2025-07-01

# 小程序配置
MINI_PROGRAM_1_NAME=小程序1
MINI_PROGRAM_1_APPID=wx1234567890abcdef
MINI_PROGRAM_1_APPSECRET=your_appsecret
```

## 项目结构

```
addata/
├── go-app/
│   ├── main.go                  # Go主程序
│   ├── go.mod                   # Go模块配置
│   ├── go.sum                   # 依赖锁定文件
│   ├── fetch_wechat_ad_data.js  # Node.js备用版本
│   ├── .env.example             # 配置示例
│   └── frontend/
│       └── index.html           # Web界面
└── README.md                    # 项目文档
```

## 注意事项

1. 请妥善保管 `.env` 文件，避免泄露敏感信息
2. 确保微信小程序有广告主权限
3. 首次拉取可能需要较长时间，请耐心等待
4. 数据拉取有频率限制，请合理安排拉取时间

## 许可证

MIT License

## 联系方式

如有问题，请提交 Issue。
