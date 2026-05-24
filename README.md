# WMAM - 微信小程序广告数据管理工具

WMAM 是一个微信小程序广告数据拉取和管理工具，支持多小程序批量管理、多人登录、权限隔离、手动拉取任务、实时进度、任务历史摘要和操作审计日志。

部署和运行请优先查看 [WMAM-Deployment-Guide.md](WMAM-Deployment-Guide.md)。

## 功能特性

- 多小程序配置和启用/禁用管理
- 微信广告数据增量拉取
- MySQL 存储广告结果数据
- 本地 SQLite 存储 WMAM 自身用户、权限、配置、任务摘要和审计日志
- 管理员和普通用户权限隔离
- 拉取任务开始、中断、继续、结束
- 当前页面实时详细日志，刷新后清空
- 加密备份导出和导入覆盖
- 浅色/暗色模式、折叠侧边栏、Toast 操作反馈

## 拉取的数据类型

1. 广告位清单
2. 广告汇总数据
3. 广告细分数据
4. 结算数据

## 技术栈

- 后端：Go 1.26.3 + Gin
- 前端：React + Vite + Tailwind CSS + Lucide
- WMAM 本地系统存储：SQLite
- 广告数据存储：MySQL 5.7+
- API：微信广告平台官方 API

## 快速开始

### 前置要求

- Go 1.26.3 或更高版本
- Node.js 和 npm
- MySQL 5.7 或更高版本
- 微信小程序 AppID 和 AppSecret

### 本地运行

```bash
git clone https://github.com/crazyzx10/WMAM.git
cd WMAM/go-app
go mod download
```

构建前端：

```bash
cd web
npm install
npm run build
cd ..
```

运行后端：

```bash
go run .
```

打开浏览器访问：

```text
http://127.0.0.1:28384/
```

默认管理员：

```text
admin / admin123
```

首次登录后请立刻修改密码，并保存首次启动日志中显示的管理员恢复码。

## 使用流程

1. 管理员登录并修改默认密码。
2. 进入“系统配置”，填写 MySQL 地址、端口、库名、用户名、密码。
3. 测试连接并保存配置。
4. 进入“小程序配置”，添加小程序 AppID 和 AppSecret。
5. 进入“用户管理”，创建普通用户。
6. 进入“执行拉取”，开始拉取任务。
7. 进入“操作日志”，查看历史任务摘要和审计日志。

普通用户只能进入“执行拉取”和“操作日志”，不能管理数据库、小程序、用户或系统备份。

## 配置文件

运行级配置使用 `config.yaml`。如果不存在，程序会使用默认值。

```yaml
server:
  host: "127.0.0.1"
  port: 28384

data:
  dir: "./data"
```

MySQL 连接配置、小程序配置、用户、权限、任务摘要和审计日志保存在 `data/wmam-system.db`。字段加密密钥保存在 `data/secret.key`。

## 项目结构

```text
WMAM/
├── go-app/
│   ├── main.go
│   ├── internal/
│   ├── middleware/
│   ├── models/
│   ├── utils/
│   ├── web/              # React 前端源码
│   ├── frontend/         # 前端构建产物，由 Go 内嵌
│   └── config.yaml.example
├── WMAM-Deployment-Guide.md
└── README.md
```

## 注意事项

1. 妥善保管 `data/` 目录，特别是 `wmam-system.db` 和 `secret.key`。
2. 生产环境建议通过 Nginx、宝塔或其他反向代理提供 HTTPS。
3. WMAM 启动后不会主动连接 MySQL，只有测试配置、保存配置或执行拉取时才连接。
4. 迁移服务器时，推荐使用网页端“系统配置”的加密备份导出/导入。
5. 确保微信小程序有广告主权限。

## 许可证

MIT License
