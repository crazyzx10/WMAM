# WMAM 多用户系统开发计划

## 概述
本文档详细规划了将 WMAM 从单机程序改造为多用户在线系统的开发步骤。

---

## 项目现状分析

### 现有资源
1. **后端** - Go + Gin 框架，已实现：
   - 微信广告数据拉取核心功能
   - 基础配置管理 API
   - 数据库连接与表初始化

2. **前端** - 现有深色主题界面，包含：
   - 数据库配置
   - 小程序管理
   - 数据拉取执行
   - 日志查看

3. **设计文档**
   - [PRD文档](file:///workspace/WMAM-MultiUser-PRD.md) - 功能需求详细描述
   - [UI设计文档](file:///workspace/WMAM-UI-Design-Doc.md) - Vercel风格界面设计规范
   - [原型文件](file:///workspace/WMAM-UI-Prototype.html) - 界面原型展示

---

## 开发阶段规划

### 第一阶段：后端基础框架搭建

#### 1.1 项目结构重组
```
/workspace/go-app/
├── main.go
├── config/
│   └── config.go
├── models/
│   ├── user.go
│   ├── operation_log.go
│   ├── fetch_lock.go
│   └── fetch_progress.go
├── middleware/
│   ├── auth.go
│   ├── logger.go
│   └── cors.go
├── handlers/
│   ├── auth.go
│   ├── users.go
│   ├── mini_programs.go
│   ├── fetch.go
│   ├── logs.go
│   └── database.go
├── utils/
│   ├── jwt.go
│   ├── password.go
│   └── response.go
└── frontend/
    └── (现有前端文件)
```

#### 1.2 新增数据库表设计
**1. 用户表 (users)**
```sql
CREATE TABLE users (
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
);
```

**2. 操作日志表 (operation_log)**
```sql
CREATE TABLE operation_log (
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
);
```

**3. 拉取锁表 (fetch_lock)**
```sql
CREATE TABLE fetch_lock (
    id INT PRIMARY KEY DEFAULT 1,
    locked_by VARCHAR(50) COMMENT '锁定者用户名',
    locked_at DATETIME COMMENT '锁定时间',
    expires_at DATETIME COMMENT '锁过期时间',
    CONSTRAINT single_row CHECK (id = 1)
);
```

**4. 拉取进度表 (fetch_progress)**
```sql
CREATE TABLE fetch_progress (
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
);
```

**5. 默认管理员账号初始化**
- 用户名: `admin`
- 密码: `admin123` (首次登录后强制修改)

#### 1.3 中间件实现
1. **认证中间件 (AuthMiddleware)**
   - JWT Token 验证
   - Token 过期检查
   - 用户状态检查（禁用用户无法登录）

2. **权限中间件 (AdminMiddleware)**
   - 检查用户角色是否为 admin
   - 仅允许管理员访问特定接口

3. **日志中间件 (LoggerMiddleware)**
   - 记录所有 API 请求
   - 记录操作日志到数据库

#### 1.4 工具函数
1. **JWT 工具**
   - Token 生成（有效期 24 小时）
   - Token 验证
   - Token 刷新机制

2. **密码工具**
   - 使用 bcrypt 加密密码
   - 密码验证

3. **响应封装**
   - 统一 API 响应格式
   - 错误处理

---

### 第二阶段：后端 API 开发

#### 2.1 认证相关接口
```
POST /api/auth/login
  - 用户名密码登录
  - 返回 JWT Token 和用户信息

POST /api/auth/logout
  - 用户登出

GET /api/auth/me
  - 获取当前登录用户信息
```

#### 2.2 用户管理接口（管理员专属）
```
GET /api/users
  - 获取用户列表
  - 支持分页

POST /api/users
  - 创建新用户

PUT /api/users/:id
  - 编辑用户信息

DELETE /api/users/:id
  - 删除用户

POST /api/users/:id/reset-password
  - 重置用户密码
```

#### 2.3 小程序管理接口
```
GET /api/mini-programs
  - 获取小程序列表

POST /api/mini-programs
  - 添加小程序（管理员）

PUT /api/mini-programs/:appid
  - 编辑小程序（管理员）

DELETE /api/mini-programs/:appid
  - 删除小程序（管理员）
```

#### 2.4 数据拉取接口
```
POST /api/fetch/execute
  - 开始执行数据拉取
  - SSE 实时返回日志

POST /api/fetch/interrupt
  - 中断当前拉取

POST /api/fetch/resume
  - 从断点继续拉取

POST /api/fetch/restart
  - 重新开始拉取

GET /api/fetch/status
  - 获取当前拉取状态

GET /api/fetch/progress
  - 获取拉取进度
```

#### 2.5 操作日志接口
```
GET /api/logs
  - 获取操作日志
  - 管理员可查看所有日志
  - 普通用户仅可查看自己的日志
  - 支持时间范围、操作类型筛选
```

#### 2.6 数据库配置接口（管理员专属）
```
GET /api/database/config
  - 获取数据库配置

POST /api/database/config
  - 保存数据库配置

POST /api/database/test
  - 测试数据库连接
```

#### 2.7 仪表盘接口
```
GET /api/dashboard/stats
  - 获取统计数据：
    - 用户总数
    - 小程序总数
    - 今日拉取次数
    - 最后拉取时间
```

---

### 第三阶段：前端重构

#### 3.1 界面改造要点
- 采用 Vercel 风格（亮色/暗色主题）
- 左侧导航栏 + 顶部栏布局
- 统一的卡片、按钮、表单组件

#### 3.2 页面结构

**1. 登录页**
- 登录表单
- 主题切换按钮
- 错误提示

**2. 主界面**
- 顶部栏：Logo、主题切换、用户头像（下拉菜单登出）
- 侧边栏：仪表盘、小程序、执行拉取、操作日志、数据库配置、用户管理（根据角色显示）
- 主内容区：动态切换各模块

**3. 仪表盘**
- 4个统计卡片（用户数、小程序数、今日拉取、最后拉取）
- 响应式网格布局（stats-grid）
- 快速操作（开始拉取）

**4. 小程序管理**
- 小程序列表（卡片式）
- 启用/禁用开关
- 编辑/删除按钮（管理员可见）
- 添加小程序弹窗

**5. 执行拉取**
- 小程序状态展示
- 进度条
- 操作按钮组（开始/中断/继续/重新开始）
- 实时日志区域

**6. 操作日志**
- 日志列表
- 筛选条件（时间范围、操作类型、用户-仅管理员）

**7. 用户管理（管理员专属）**
- 用户列表表格（用户名、角色、状态、操作）
- 角色徽章展示（管理员🛡️、普通用户👤）
- 启用/禁用切换开关
- 添加用户按钮（弹窗表单）
- 编辑用户信息
- 重置密码功能
- 删除/禁用用户操作

**8. 数据库配置（管理员专属）**
- 数据库配置表单（默认只读）
- 测试连接按钮
- 保存配置按钮
- 连接状态显示

#### 3.3 前端技术要点
1. **主题管理**
   - localStorage 保存用户偏好
   - 平滑过渡动画
   - 支持系统主题检测

2. **认证管理**
   - Token 存储在 localStorage
   - 请求时自动添加 Authorization 头
   - Token 过期自动跳转到登录页

3. **SSE 实时通信**
   - 处理数据拉取实时日志
   - 进度更新
   - 状态变更

---

## 详细开发步骤

### Step 1: 后端基础框架
1. 重组项目目录结构
2. 实现配置加载机制
3. 实现 JWT 工具和密码工具
4. 实现数据库连接与表初始化（包括新增表）
5. 初始化默认管理员账号
6. 实现中间件（认证、权限、日志）

### Step 2: 认证模块
1. 实现登录 API
2. 实现登出 API
3. 实现获取当前用户信息 API
4. 集成 JWT 认证中间件

### Step 3: 用户管理模块
1. 实现用户列表 API
2. 实现创建用户 API
3. 实现编辑用户 API
4. 实现删除用户 API
5. 实现重置密码 API

### Step 4: 操作日志模块
1. 实现操作日志记录功能
2. 实现日志查询 API
3. 根据角色过滤日志

### Step 5: 数据拉取增强
1. 实现拉取锁机制
2. 实现拉取进度保存
3. 实现中断功能
4. 实现断点续传功能
5. 实现重新开始功能
6. 实现拉取状态查询 API

### Step 6: 小程序管理改造
1. 将小程序配置从文件迁移到数据库（mini_program 表）
2. 改造小程序管理 API
3. 添加启用/禁用字段

### Step 7: 数据库配置模块
1. 实现数据库配置 API
2. 将配置从 .env 文件迁移到数据库（可选，或保持 .env 但通过 API 管理）

### Step 8: 仪表盘接口
1. 实现统计数据查询 API

### Step 9: 前端登录页
1. 实现登录界面（Vercel风格）
2. 实现主题切换
3. 实现登录逻辑
4. Token 存储与管理

### Step 10: 前端主布局
1. 实现顶部栏
2. 实现侧边栏导航
3. 实现主内容区框架
4. 实现路由/页面切换逻辑

### Step 11: 前端仪表盘
1. 实现4个统计卡片（用户数、小程序数、今日拉取、最后拉取）
2. 响应式网格布局（stats-grid）
3. 集成快速拉取按钮
4. 数据动态加载（调用 /api/dashboard/stats）

### Step 12: 前端小程序管理
1. 实现小程序列表
2. 实现添加/编辑/删除功能
3. 实现启用/禁用开关

### Step 13: 前端执行拉取
1. 实现状态展示
2. 实现进度条
3. 实现按钮组（开始/中断/继续/重新开始）
4. 集成 SSE 实时日志
5. 实现拉取状态动态更新

### Step 14: 前端操作日志
1. 实现日志列表
2. 实现筛选功能

### Step 15: 前端用户管理
1. 实现用户列表表格（用户名、角色、状态、操作）
2. 实现角色徽章展示（管理员🛡️、普通用户👤）
3. 实现启用/禁用切换开关
4. 实现添加用户弹窗表单
5. 实现编辑用户功能
6. 实现重置密码功能
7. 实现删除/禁用用户功能
8. 根据角色控制页面可见性

### Step 16: 前端数据库配置
1. 实现配置表单
2. 实现测试连接
3. 实现保存配置

### Step 17: 集成测试
1. 端到端测试
2. 权限测试
3. 并发拉取测试
4. 中断/续传测试
5. 主题切换测试

### Step 18: 文档与部署
1. 更新 README
2. 编写部署文档
3. 准备生产环境配置

---

## 技术注意事项

### 1. 数据迁移
- 现有的小程序配置需要从 .env 文件迁移到数据库的 mini_program 表
- 考虑写一个迁移脚本或在首次运行时自动迁移

### 2. 兼容性
- 保持现有数据库表结构不变
- 只新增表和字段
- 确保现有数据不受影响

### 3. 安全性
- 所有 API 需要认证
- 密码使用 bcrypt 加密存储
- 防止 SQL 注入（使用参数化查询）
- 防止 XSS 攻击（前端输出转义）
- Token 有效期限制

### 4. 互斥机制
- 使用数据库表 fetch_lock 实现拉取互斥
- 设置锁超时时间（30分钟）防止死锁
- 检查锁状态后再允许拉取

### 5. 断点续传
- 在 fetch_progress 表中保存进度
- 记录每个小程序的每个数据类型的拉取状态
- 中断后从上次停止的地方继续

---

## 时间预估

| 阶段 | 预计时间 |
|------|----------|
| 后端基础框架 | 4-6小时 |
| 认证模块 | 2-3小时 |
| 用户管理模块 | 2-3小时 |
| 操作日志模块 | 2小时 |
| 数据拉取增强 | 4-6小时 |
| 小程序管理改造 | 2-3小时 |
| 其他后端接口 | 2小时 |
| 前端登录页 + 主布局 | 3-4小时 |
| 前端各功能页面 | 6-8小时 |
| 集成测试 | 3-4小时 |
| 文档与部署 | 2小时 |
| **总计** | **30-40小时** |

---

## 风险与应对

| 风险 | 应对措施 |
|------|----------|
| 数据迁移问题 | 先备份数据，写迁移脚本，测试后再应用 |
| 并发拉取冲突 | 实现严格的锁机制，超时自动释放 |
| 前端复杂度 | 先完成核心功能，再优化细节 |
| 时间不足 | 优先实现核心功能（认证、拉取、日志），其他功能可后续迭代 |

---

## 后续扩展（可选）

1. 数据可视化 - 接入金山文档
2. 通知机制 - 拉取完成后邮件/微信通知
3. 定时任务 - 支持定时自动拉取
4. 数据导出 - 导出 Excel/PDF 报表
5. 多语言支持 - 中英文切换
