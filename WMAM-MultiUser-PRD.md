# WMAM 多用户系统 PRD

**版本**：v1.1  
**日期**：2026-05-22  
**作者**：WMAM 开发团队  
**状态**：草稿

---

## 1. 项目概述

### 1.1 项目背景

WMAM（微信小程序广告数据管理工具）目前是一个单文件本地 Go 程序，用于从微信广告平台 API 拉取小程序广告数据并存储到远程 MySQL 数据库。

随着用户团队规模的扩大，需要将现有系统从**单机单人使用**扩展为**在线多用户协作平台**，以支持小团队内部共享使用。

### 1.2 项目目标

将 WMAM 从单机本地程序改造为基于 Web 的多用户在线系统，支持：
- 管理员统一管理小程序配置
- 多个普通用户协作执行数据拉取
- 完整的用户权限管理和操作审计

### 1.3 核心价值

- ✅ **集中管理**：管理员统一配置，降低配置错误风险
- ✅ **团队协作**：多个团队成员可共享使用
- ✅ **操作安全**：互斥机制防止并发冲突
- ✅ **审计追溯**：完整操作日志，记录所有关键操作

---

## 2. 用户角色与权限

### 2.1 角色定义

| 角色 | 数量 | 说明 |
|------|------|------|
| 管理员 | 1人 | 系统唯一管理员，负责系统配置 |
| 普通用户 | N人 | 团队成员，仅能执行数据拉取操作 |

### 2.2 权限矩阵

| 功能 | 管理员 | 普通用户 |
|------|--------|----------|
| 系统登录 | ✅ | ✅ |
| 查看小程序列表 | ✅ | ✅ |
| 添加小程序 | ✅ | ❌ |
| 编辑小程序 | ✅ | ❌ |
| 删除小程序 | ✅ | ❌ |
| 执行数据拉取 | ✅ | ✅ |
| 中断拉取 | ✅ | ✅ |
| 继续拉取 | ✅ | ✅ |
| 重新拉取 | ✅ | ✅ |
| 查看操作日志 | ✅ | ✅（仅自己） |
| 用户管理 | ✅ | ❌ |
| 查看他人操作日志 | ✅ | ❌ |

### 2.3 用户流转

```
管理员
  ↓ 创建账号
普通用户A、B、C...
  ↓ 登录
执行数据拉取
  ↓
查看操作日志
```

---

## 3. 功能需求

### 3.1 用户管理系统

#### 3.1.1 用户注册与登录

**功能描述**：管理员开通账号，用户使用账号密码登录

**详细设计**：
- ❌ **关闭注册功能**：禁止普通用户自行注册
- ✅ **管理员开通**：管理员后台创建用户账号
- ✅ **初始密码**：创建时设置初始密码
- ✅ **密码修改**：用户首次登录后强制修改密码
- ✅ **会话管理**：JWT Token 认证，Token 有效期 24 小时

**数据表设计**：

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

#### 3.1.2 用户管理（管理员）

**功能描述**：管理员对普通用户进行 CRUD 操作

**操作列表**：
- 创建用户：设置用户名、初始密码、角色
- 编辑用户：修改用户名、状态（启用/禁用）
- 重置密码：重置为随机密码或指定密码
- 删除用户：软删除或直接删除
- 查看用户列表：显示所有用户及状态

### 3.2 小程序配置管理

#### 3.2.1 小程序列表

**功能描述**：展示所有已配置的小程序信息

**展示信息**：
- 小程序名称
- AppID
- 是否启用
- 配置时间
- 更新时间

**数据表**（复用现有）：

```sql
CREATE TABLE mini_program (
    名称 VARCHAR(64) NOT NULL,
    小程序ID VARCHAR(32) NOT NULL,
    小程序Secret VARCHAR(64) NOT NULL,
    是否启用 TINYINT DEFAULT 1,
    创建时间 DATETIME DEFAULT CURRENT_TIMESTAMP,
    更新时间 DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (小程序ID)
);
```

#### 3.2.2 小程序配置（管理员）

**功能描述**：管理员添加、编辑、删除小程序配置

**操作列表**：
- 添加小程序：输入名称、AppID、AppSecret
- 编辑小程序：修改名称、AppSecret、启用状态
- 删除小程序：确认后删除
- 批量导入/导出：（可选）批量管理小程序

### 3.3 数据拉取功能

#### 3.3.1 执行拉取

**功能描述**：用户触发数据拉取任务

**执行流程**：
1. 用户点击"执行拉取"按钮
2. 系统检查互斥锁状态
3. 如已锁定，提示"数据拉取中，请勿重复操作"
4. 如未锁定，获取锁，开始拉取
5. 实时推送日志到前端
6. 拉取完成，释放锁
7. 记录操作日志

**技术实现**：

```go
// 互斥锁实现（Redis 或数据库）
type FetchLock struct {
    lockedBy   string    // 锁定者用户名
    lockedAt   time.Time // 锁定时间
    isLocked   bool
}

// 获取锁
func (l *FetchLock) Acquire(username string) bool {
    if l.isLocked {
        return false // 已被锁定
    }
    l.isLocked = true
    l.lockedBy = username
    l.lockedAt = time.Now()
    return true
}

// 释放锁
func (l *FetchLock) Release(username string) bool {
    if l.lockedBy == username {
        l.isLocked = false
        l.lockedBy = ""
        return true
    }
    return false
}
```

#### 3.3.2 拉取中断功能

**功能描述**：用户或管理员可在拉取过程中随时中断，支持断点续传

**中断流程**：
1. 用户点击"中断执行"按钮
2. 系统发送中断信号，停止当前拉取
3. 记录中断点和已完成的进度
4. 释放锁，提示中断成功
5. 用户可再次点击"继续拉取"，从断点继续

**断点续传机制**：
1. 记录当前处理的小程序索引
2. 每个小程序记录4种数据的拉取状态：
   - 广告位清单（adunit_list）
   - 汇总数据（publisher_adpos_general）
   - 细分数据（publisher_adunit_general）
   - 结算数据（publisher_settlement）
3. 中断后再次拉取，跳过已完成的步骤

**数据表设计**：

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

**中断场景示例**：
```
场景：配置了3个小程序，正在拉取第2个小程序的汇总数据时中断

中断前的进度：
✅ 小程序1 - 全部完成
⏸️ 小程序2 - 汇总数据（进行中）← 中断点
❌ 小程序3 - 未开始

继续拉取后的行为：
✅ 小程序1 - 跳过（已完成）
⏸️ 小程序2 - 从汇总数据继续
❌ 小程序3 - 从头开始
```

**用户体验**：
- 拉取中：显示"中断执行"按钮（红色）
- 中断成功：提示"已中断，可继续拉取"
- 继续拉取：按钮显示"继续执行"（橙色）
- 重新拉取：用户可选择"重新拉取"（清空进度，从头开始）
- 所有用户均可使用中断、继续、重新拉取功能

**API 设计**：

```
POST /api/fetch/interrupt
  Headers: Authorization: Bearer <token>
  Description: 中断当前拉取
  Response: { 
    success: true, 
    message: "已成功中断拉取",
    canResume: true 
  }

POST /api/fetch/resume
  Headers: Authorization: Bearer <token>
  Description: 从断点继续拉取
  Response: (SSE Stream) - 同 execute

POST /api/fetch/restart
  Headers: Authorization: Bearer <token>
  Description: 重新开始拉取（清空进度）
  Response: (SSE Stream) - 同 execute

GET /api/fetch/progress
  Headers: Authorization: Bearer <token>
  Response: { 
    isLocked: true,
    lockedBy: "user1",
    currentProgram: 2,
    totalPrograms: 3,
    currentDataType: "summary",
    canResume: true
  }
```

#### 3.3.3 拉取状态展示

**功能描述**：实时展示拉取进度和日志

**界面元素**：
- 进度条：显示当前进度百分比
- 状态文本：如"正在拉取 XX 小程序数据..."
- 实时日志：
  - ✅ 成功日志（绿色）
  - ❌ 错误日志（红色）
  - ⚠️ 警告日志（黄色）
  - ℹ️ 信息日志（灰色）

**日志格式示例**：
```
========== 微信小程序广告数据拉取开始 ==========
开始时间: 2026-05-22 10:30:00

====== 处理小程序: 测试小程序1 (wx123456) ======
✅ [测试小程序1] 正在获取 Access Token...
✅ [测试小程序1] 获取Token成功
✅ [测试小程序1] 正在获取广告位列表...
✅ [测试小程序1] 广告位清单已保存

========== 全部数据拉取完成 ==========
完成时间: 2026-05-22 10:35:00
总耗时: 5 分钟 0 秒
```

#### 3.3.4 并发控制

**功能描述**：防止多人同时执行拉取操作

**实现策略**：
1. **前端拦截**：点击后立即禁用按钮，显示加载状态
2. **后端锁机制**：
   - 首次点击时尝试获取锁
   - 获取成功：执行拉取
   - 获取失败：返回错误提示
3. **超时释放**：锁超时时间 30 分钟，防止意外未释放
4. **主动释放**：拉取完成后立即释放锁

**用户体验**：
- 锁定状态：按钮显示"拉取中..."，不可点击
- 未锁定：按钮显示"开始执行"
- 其他用户看到：提示"管理员正在执行拉取，请稍后再试"

### 3.4 操作日志系统

#### 3.4.1 日志记录

**功能描述**：记录所有关键操作，便于审计追溯

**记录的操作**：
- 用户登录/登出
- 执行数据拉取（开始、结束、失败）
- 小程序配置变更（增删改）
- 用户管理操作

**日志字段**：

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

**操作类型枚举**：
- `LOGIN` - 用户登录
- `LOGOUT` - 用户登出
- `FETCH_START` - 开始拉取
- `FETCH_SUCCESS` - 拉取成功
- `FETCH_FAILED` - 拉取失败
- `FETCH_ABORT` - 拉取中断
- `FETCH_RESUME` - 拉取继续
- `FETCH_RESTART` - 拉取重启
- `MINI_PROGRAM_ADD` - 添加小程序
- `MINI_PROGRAM_EDIT` - 编辑小程序
- `MINI_PROGRAM_DELETE` - 删除小程序
- `USER_CREATE` - 创建用户
- `USER_DISABLE` - 禁用用户
- `USER_ENABLE` - 启用用户
- `PASSWORD_RESET` - 重置密码

#### 3.4.2 日志查看

**功能描述**：管理员和用户查看操作历史

**权限差异**：
- **管理员**：查看所有用户的操作日志，可筛选
- **普通用户**：仅查看自己的操作日志

**筛选条件**：
- 时间范围
- 操作类型
- 用户名（管理员专用）

---

## 4. 技术架构

### 4.1 系统架构图

```
┌─────────────────────────────────────────────────────────────┐
│                      客户端浏览器                            │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    Web 服务器 (Go)                          │
│  ┌───────────┐  ┌───────────┐  ┌───────────┐  ┌──────────┐ │
│  │  用户模块  │  │ 小程序模块 │  │ 拉取模块  │  │ 日志模块  │ │
│  └───────────┘  └───────────┘  └───────────┘  └──────────┘ │
│  ┌─────────────────────────────────────────────────────────┐│
│  │                   中间件层                              ││
│  │  ┌─────────┐  ┌──────────┐  ┌─────────────────────┐   ││
│  │  │ JWT认证 │  │ 权限校验  │  │   操作日志记录       │   ││
│  │  └─────────┘  └──────────┘  └─────────────────────┘   ││
│  └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│   MySQL 用户库   │  │  MySQL 数据存储  │  │  微信广告 API   │
│   (系统配置)    │  │  (广告数据)     │  │                │
└─────────────────┘  └─────────────────┘  └─────────────────┘
```

### 4.2 技术栈

| 组件 | 技术选型 | 说明 |
|------|---------|------|
| 后端框架 | Go + Gin | 复用现有 Go 代码 |
| 前端框架 | 原生 HTML/CSS/JS | 复用现有前端 |
| 数据库 | MySQL | 复用现有数据表，新增用户表 |
| 认证 | JWT | 无状态认证 |
| 互斥锁 | Redis / MySQL | 防止并发拉取 |
| 会话管理 | Redis / 内存 | Token 存储 |

### 4.3 API 设计

#### 4.3.1 认证相关

```
POST /api/auth/login
  Request: { username, password }
  Response: { token, user: { id, username, role } }

POST /api/auth/logout
  Headers: Authorization: Bearer <token>
  Response: { success: true }

GET /api/auth/me
  Headers: Authorization: Bearer <token>
  Response: { user: { id, username, role } }
```

#### 4.3.2 用户管理（管理员）

```
GET /api/users
  Headers: Authorization: Bearer <token>
  Query: ?page=1&pageSize=20
  Response: { users: [...], total: 100 }

POST /api/users
  Headers: Authorization: Bearer <token>
  Request: { username, password, role }
  Response: { user: {...} }

PUT /api/users/:id
  Headers: Authorization: Bearer <token>
  Request: { username, status }
  Response: { user: {...} }

DELETE /api/users/:id
  Headers: Authorization: Bearer <token>
  Response: { success: true }

POST /api/users/:id/reset-password
  Headers: Authorization: Bearer <token>
  Request: { newPassword }
  Response: { success: true }
```

#### 4.3.3 小程序管理

```
GET /api/mini-programs
  Headers: Authorization: Bearer <token>
  Response: { programs: [...] }

POST /api/mini-programs         # 管理员
  Headers: Authorization: Bearer <token>
  Request: { name, appid, appsecret }
  Response: { program: {...} }

PUT /api/mini-programs/:id      # 管理员
  Headers: Authorization: Bearer <token>
  Request: { name, appsecret, enabled }
  Response: { program: {...} }

DELETE /api/mini-programs/:id   # 管理员
  Headers: Authorization: Bearer <token>
  Response: { success: true }
```

#### 4.3.4 数据拉取

```
POST /api/fetch/execute
  Headers: Authorization: Bearer <token>
  Description: 开始执行数据拉取
  Response: (SSE Stream)
    data: {"type": "log", "content": "..."}
    data: {"type": "progress", "percent": 50}
    data: {"type": "complete", "success": true}

POST /api/fetch/interrupt
  Headers: Authorization: Bearer <token>
  Description: 中断当前拉取
  Response: { 
    success: true, 
    message: "已成功中断拉取",
    canResume: true 
  }

POST /api/fetch/resume
  Headers: Authorization: Bearer <token>
  Description: 从断点继续拉取
  Response: (SSE Stream) - 同 execute

POST /api/fetch/restart
  Headers: Authorization: Bearer <token>
  Description: 重新开始拉取（清空进度）
  Response: (SSE Stream) - 同 execute

GET /api/fetch/status
  Headers: Authorization: Bearer <token>
  Response: { 
    isLocked: true, 
    lockedBy: "user1", 
    lockedAt: "2026-05-22T10:00:00Z" 
  }

GET /api/fetch/progress
  Headers: Authorization: Bearer <token>
  Response: { 
    isLocked: true,
    lockedBy: "user1",
    currentProgram: 2,
    totalPrograms: 3,
    currentDataType: "summary",
    canResume: true,
    programStatuses: [
      { name: "小程序1", status: "completed" },
      { name: "小程序2", status: "in_progress" },
      { name: "小程序3", status: "pending" }
    ]
  }
```

#### 4.3.5 操作日志

```
GET /api/logs
  Headers: Authorization: Bearer <token>
  Query: ?page=1&pageSize=20&type=LOGIN&startDate=2026-05-01&endDate=2026-05-22
  Response: { logs: [...], total: 100 }
```

---

## 5. 数据库设计

### 5.1 新增数据表

#### 5.1.1 用户表（users）

见 3.1.1 章节。

#### 5.1.2 操作日志表（operation_log）

见 3.4.1 章节。

#### 5.1.3 拉取锁表（fetch_lock）

```sql
CREATE TABLE fetch_lock (
    id INT PRIMARY KEY DEFAULT 1,
    locked_by VARCHAR(50) COMMENT '锁定者用户名',
    locked_at DATETIME COMMENT '锁定时间',
    expires_at DATETIME COMMENT '锁过期时间',
    CONSTRAINT single_row CHECK (id = 1)
);
```

#### 5.1.3 拉取进度表（fetch_progress）

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

### 5.2 现有表保持不变

- `mini_program` - 小程序配置表
- `adunit_list` - 广告位清单表
- `publisher_adpos_general` - 广告汇总数据表
- `publisher_adunit_general` - 广告细分数据表
- `publisher_settlement` - 结算数据表
- `fetch_log` - 拉取日志表

---

## 6. 前端界面设计

### 6.1 页面结构

```
├── 登录页 (login.html)
│   └── 用户名、密码输入
│
├── 主页面 (index.html)
│   ├── 顶部导航栏
│   │   ├── Logo + 系统名称
│   │   ├── 当前用户信息
│   │   └── 退出登录按钮
│   │
│   ├── 侧边栏菜单
│   │   ├── 仪表盘（首页）
│   │   ├── 小程序管理（管理员可见）
│   │   ├── 执行拉取
│   │   ├── 操作日志
│   │   └── 用户管理（管理员可见）
│   │
│   └── 主内容区
│
└── 模态框
    ├── 添加/编辑小程序（管理员）
    ├── 添加/编辑用户（管理员）
    └── 确认对话框
```

### 6.2 页面功能

#### 6.2.1 登录页

- 用户名输入框
- 密码输入框
- 登录按钮
- 错误提示信息
- 首次登录强制修改密码（待扩展）

#### 6.2.2 仪表盘

- 系统概览信息
  - 用户总数
  - 小程序总数
  - 今日拉取次数
  - 最后拉取时间
- 快捷操作入口

#### 6.2.3 小程序管理（管理员）

- 小程序列表表格
  - 名称、AppID、状态、操作
- 添加小程序按钮
- 编辑/删除操作
- 分页

#### 6.2.4 执行拉取

- 拉取状态显示
  - 当前状态（空闲/拉取中/锁定者信息）
  - 锁定时间
  - 当前处理的小程序和进度
- 拉取按钮组
  - 空闲时："开始执行"（蓝色主按钮）
  - 拉取中："中断执行"（红色按钮）
  - 中断后："继续执行"（橙色按钮）/ "重新拉取"（灰色按钮）
  - 被锁定："数据拉取中，请勿重复操作"（提示信息）
- 进度条
- 实时日志区
- 每个小程序的拉取状态指示：
  - ✅ 已完成（绿色勾选）
  - ⏸️ 进行中（橙色旋转）
  - ❌ 失败（红色叉号）
  - ⏳ 待处理（灰色圆圈）

#### 6.2.5 操作日志

- 筛选条件
  - 时间范围
  - 操作类型
  - 用户名（管理员）
- 日志列表
- 分页

#### 6.2.6 用户管理（管理员）

- 用户列表表格
  - 用户名、角色、状态、创建时间、最后登录、操作
- 添加用户按钮
- 编辑/禁用/启用/重置密码操作

---

## 7. 安全设计

### 7.1 认证安全

- **密码加密**：使用 bcrypt 算法哈希存储
- **Token 安全**：
  - JWT Token 签名验证
  - Token 有效期 24 小时
  - 支持 Token 刷新（可选）

### 7.2 权限控制

- **接口鉴权**：所有 API 需要携带有效 Token
- **角色校验**：管理员接口需验证 role=admin
- **数据隔离**：
  - 普通用户无法访问管理员接口
  - 操作日志按用户隔离

### 7.3 输入校验

- 所有用户输入进行合法性校验
- SQL 注入防护（使用参数化查询）
- XSS 防护（输出转义）

### 7.4 操作安全

- **删除确认**：所有删除操作需二次确认
- **敏感操作日志**：记录所有管理员操作
- **拉取互斥**：防止并发冲突

---

## 8. 部署架构

### 8.1 部署模式

**推荐模式：单机部署**

```
┌─────────────────────────────────────┐
│           云服务器                   │
│  ┌─────────────────────────────┐   │
│  │      Go Web 应用            │   │
│  │      (单实例)               │   │
│  └─────────────────────────────┘   │
│              │                      │
│  ┌─────────────────────────────┐   │
│  │      MySQL 数据库            │   │
│  │      (广告数据存储)          │   │
│  └─────────────────────────────┘   │
└─────────────────────────────────────┘
```

### 8.2 服务器配置

**最低配置**：
- CPU: 1 核
- 内存: 1 GB
- 带宽: 1 Mbps
- 系统: Ubuntu 20.04 / CentOS 7

**推荐配置**：
- CPU: 2 核
- 内存: 2 GB
- 带宽: 3 Mbps
- 系统: Ubuntu 20.04 / CentOS 7

### 8.3 环境要求

- Go 1.26.3+
- MySQL 5.7+
- Redis（可选，用于 Token 存储和锁）

---

## 9. 项目里程碑

### Phase 1：基础框架搭建
- [ ] 用户认证模块（登录、登出）
- [ ] 用户管理模块（管理员 CRUD）
- [ ] 基础权限控制

### Phase 2：核心功能开发
- [ ] 小程序管理模块（管理员 CRUD）
- [ ] 数据拉取模块（带互斥锁）
- [ ] 前端界面改造（适配多用户）

### Phase 3：日志与审计
- [ ] 操作日志记录
- [ ] 日志查看功能
- [ ] 日志筛选与导出

### Phase 4：测试与部署
- [ ] 功能测试
- [ ] 安全测试
- [ ] 部署文档
- [ ] 上线部署

---

## 10. 附录

### 10.1 术语表

| 术语 | 说明 |
|------|------|
| WMAM | 微信小程序广告数据管理工具 |
| JWT | JSON Web Token，无状态认证标准 |
| SSE | Server-Sent Events，服务端推送技术 |
| 拉取锁 | 防止多人同时执行数据拉取的互斥机制 |

### 10.2 参考资料

- 微信广告平台 API 文档
- Go Gin 框架文档
- JWT 官方规范

### 10.3 后续扩展建议

（暂不实现，仅记录）

1. **数据可视化**：接入金山文档，展示拉取的数据
2. **通知机制**：拉取完成后发送邮件/微信通知
3. **定时任务**：支持定时自动拉取
4. **数据导出**：导出 Excel/PDF 报表
5. **多语言支持**：中英文切换

---

**文档版本历史**：

| 版本 | 日期 | 修改内容 | 作者 |
|------|------|---------|------|
| v1.0 | 2026-05-22 | 初始版本 | WMAM 开发团队 |
