# WMAM 部署与运行说明

本文档说明 WMAM 多人版第一版的部署方式。第一版不需要 Docker，不需要单独部署前端服务；Go 可执行程序会内嵌已经构建好的 React 前端页面。

## 1. 部署形态

推荐生产部署：

```text
用户浏览器
  -> HTTPS 域名
  -> Nginx / 宝塔 / 其他反向代理
  -> 127.0.0.1:28384 上运行的 WMAM Go 程序
  -> 执行拉取时连接广告数据 MySQL
```

WMAM 可以运行在云服务器 A，广告数据 MySQL 可以运行在服务器 B。WMAM 启动时不会主动连接 MySQL，只有管理员测试配置、保存可用配置或用户执行拉取时才会连接。

本机运行也可以：

```text
管理员电脑
  -> 运行 WMAM Go 程序
  -> 浏览器访问 http://127.0.0.1:28384/
```

如果别人无法访问管理员电脑，那么只有管理员自己能使用。

## 2. 文件结构

部署目录建议如下：

```text
wmam/
  wmam-server        # Linux/macOS 可执行文件，Windows 下为 wmam-server.exe
  config.yaml        # 可选，建议保留
  data/              # 首次运行后自动生成，必须持久保存
```

`data/` 目录非常重要：

```text
data/
  wmam-system.db     # 用户、权限、小程序配置、MySQL 配置、任务摘要、审计日志
  secret.key         # 本地字段加密密钥
```

不要删除 `data/`。如果迁移服务器，需要把 `wmam-system.db` 和 `secret.key` 一起迁移，或者使用网页端“系统配置”里的加密备份导出/导入。

## 3. config.yaml

如果没有 `config.yaml`，程序会使用默认值：

```yaml
server:
  host: "127.0.0.1"
  port: 28384

data:
  dir: "./data"
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `server.host` | 程序监听地址。生产环境建议保持 `127.0.0.1`，由反向代理对外提供 HTTPS。 |
| `server.port` | 程序监听端口，默认 `28384`。 |
| `data.dir` | WMAM 本地系统存储目录。 |

## 4. 编译

在 `go-app/web` 下构建前端：

```bash
npm install
npm run build
```

在 `go-app` 下编译 Go 程序：

```bash
go build -o wmam-server .
```

如果在 Windows 上给 Linux 服务器编译：

```powershell
$env:GOOS="linux"
$env:GOARCH="amd64"
go build -o wmam-server .
```

如果在 Windows 本机运行：

```powershell
go build -o wmam-server.exe .
.\wmam-server.exe
```

## 5. 首次使用

首次启动后访问：

```text
http://127.0.0.1:28384/
```

默认管理员：

```text
admin / admin123
```

首次登录会要求修改密码。管理员恢复码会在首次启动日志中显示一次，请保存好。普通用户忘记密码时由管理员重置；管理员忘记密码时使用恢复码重置。

首次配置顺序建议：

1. 管理员登录并修改默认密码。
2. 保存管理员恢复码。
3. 进入“系统配置”填写 MySQL 地址、端口、库名、用户名、密码。
4. 测试 MySQL 连接并保存。
5. 进入“小程序配置”添加小程序 AppID 和 AppSecret。
6. 进入“用户管理”创建普通用户。
7. 进入“执行拉取”运行一次任务。

## 6. 反向代理示例

Nginx 示例：

```nginx
server {
    listen 443 ssl;
    server_name your-domain.com;

    ssl_certificate     /path/to/fullchain.pem;
    ssl_certificate_key /path/to/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:28384;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
    }
}
```

如果执行拉取页面需要长时间保持实时日志连接，反向代理不要设置过短的读取超时。

## 7. systemd 示例

Linux 服务器建议用 `systemd` 托管：

```ini
[Unit]
Description=WMAM
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/wmam
ExecStart=/opt/wmam/wmam-server
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

常用命令：

```bash
sudo systemctl daemon-reload
sudo systemctl enable wmam
sudo systemctl start wmam
sudo systemctl status wmam
```

## 8. 备份和迁移

推荐使用网页端“系统配置”里的加密备份导出：

1. 输入备份文件密码。
2. 导出 `.wmam` 备份文件。
3. 在新环境导入该文件。

备份包含用户、权限、小程序配置、数据库连接配置、任务摘要、审计日志和本地字段加密密钥。备份不包含登录状态，也不包含执行详细日志。

备份文件密码无法找回。

## 9. 常见问题

### 浏览器打不开

确认程序是否运行，并确认端口：

```bash
curl http://127.0.0.1:28384/
```

### 普通用户看不到配置页面

这是预期行为。普通用户只能使用执行拉取和操作日志。

### 程序启动后没有连接 MySQL

这是预期行为。WMAM 启动后不会自动连接 MySQL，只有测试配置、保存配置或执行拉取时才连接。

### 迁移后配置无法解密

确认 `data/secret.key` 是否随 `wmam-system.db` 一起迁移。更推荐使用网页端加密备份导出/导入。
