# 飞书租户隔离功能 — 配置手册

## 功能概述

本功能在 Multica 中引入了飞书（Lark）多租户隔离机制，支持：

- **租户共享 Workspace**：同一飞书租户下的所有成员可自动访问指定 Workspace，无需手动邀请
- **租户管理员**：飞书租户下指定用户可管理该租户的所有 Workspace
- **私有 Workspace**：仅显式成员可访问，不受租户共享规则影响
- **飞书登录**：用户通过飞书 OAuth 直接登录 Multica

---

## 数据库变更

### 新增表

| 表名 | 说明 |
|------|------|
| `lark_tenant` | 飞书租户注册表，记录 `tenant_key` 与租户 UUID |
| `user_identity` | 用户身份映射表，关联本地用户与飞书 `open_id` |
| `lark_tenant_admin` | 租户管理员表，记录哪些用户是哪些租户的管理员 |

### Workspace 字段变更

| 字段 | 类型 | 说明 |
|------|------|------|
| `home_tenant_id` | UUID (nullable) | Workspace 所属主租户 |
| `visibility` | text | `tenant`（租户共享）或 `private`（私有） |

> **注意**：`visibility=private` 的 Workspace 不受租户共享规则影响。

### 迁移文件

- `server/migrations/109_lark_tenants_and_workspace_visibility.up.sql`
- `server/migrations/109_lark_tenants_and_workspace_visibility.down.sql`

---

## 新增环境变量

| 变量 | 必填 | 说明 |
|------|------|------|
| `LARK_APP_ID` | 是 | 飞书应用的 App ID |
| `LARK_APP_SECRET` | 是 | 飞书应用的 App Secret |
| `LARK_REDIRECT_URI` | 否 | 飞书 OAuth 回调地址，默认 `/auth/lark` |

在 `.env` 或 `.env.worktree` 中添加：

```bash
# 飞书应用配置
LARK_APP_ID=cli_xxxxxxxxxxxxxxxxxx
LARK_APP_SECRET=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
LARK_REDIRECT_URI=https://your-domain.com/auth/lark
```

---

## 新增 API 端点

### 1. 飞书登录回调

```
POST /auth/lark
```

飞书授权后回调此接口，完成用户登录/注册。

**请求参数**（URL Query）：

| 参数 | 类型 | 说明 |
|------|------|------|
| `code` | string | 飞书授权码 |
| `state` | string | 状态参数（包含 next 跳转信息） |

**响应**：登录成功后 JWT 写入 Cookie，跳转至前端。

### 2. 获取当前用户可见的 Workspace 列表

```
GET /api/lark/workspaces
```

返回当前飞书租户下所有 `visibility=tenant` 的 Workspace 列表（仅租户管理员可调用）。

**响应**：

```json
[
  {
    "id": "uuid",
    "name": "Design Ops",
    "slug": "design-ops",
    "description": null,
    "context": null,
    "settings": {},
    "repos": [],
    "issue_prefix": "DES",
    "created_at": "2026-01-01T00:00:00Z",
    "updated_at": "2026-01-01T00:00:00Z",
    "home_tenant_id": "uuid",
    "visibility": "tenant"
  }
]
```

---

## 新增前端页面

### 1. 飞书登录入口

**路径**：`/auth/lark`

用户点击"Continue with Lark"后跳转至此页面，完成飞书 OAuth 流程。

### 2. 租户管理员 Workspace 管理页

**路径**：`/admin/workspaces`

功能：
- 租户管理员可创建 `tenant`（租户共享）或 `private`（私有）Workspace
- 列出当前租户下所有 Workspace
- 点击"Open"进入对应 Workspace

> 普通成员不显示此页面。

---

## 权限模型

### 角色与权限

| 角色 | 说明 |
|------|------|
| `owner` | Workspace 所有者，可管理成员和所有设置 |
| `admin` | Workspace 管理员，可管理成员 |
| `member` | 普通成员，可访问 Workspace 内容 |

### 租户管理员

- 租户管理员自动获得该租户下所有 Workspace 的 `admin` 权限
- 租户管理员可访问 `/admin/workspaces` 页面
- 租户管理员可创建和管理 `visibility=tenant` 的 Workspace

### Workspace 访问规则

```
规则 1：显式 member → 始终可访问
规则 2：visibility=tenant 且用户 Lark 租户 ID == workspace.home_tenant_id → 可访问
规则 3：租户管理员 → 可管理该租户下所有 Workspace
```

---

## 前端配置

### 1. 设置飞书 App ID

在应用初始化时配置：

```typescript
import { useConfigStore } from "@multica/core/config";

const setAuthConfig = useConfigStore((s) => s.setAuthConfig);
setAuthConfig({
  allowSignup: true,
  larkAppId: process.env.NEXT_PUBLIC_LARK_APP_ID,
});
```

### 2. NEXT_PUBLIC_LARK_APP_ID

在 `.env` 中添加：

```bash
NEXT_PUBLIC_LARK_APP_ID=cli_xxxxxxxxxxxxxxxxxx
```

---

## 部署步骤

### 1. 数据库迁移

```bash
make migrate-up
# 或
cd server && go run ./cmd/migrate up
```

### 2. 生成数据库查询代码

```bash
make sqlc
# 或
cd server && ~/bin/sqlc generate
```

### 3. 启动后端

```bash
make dev
# 或
cd server && go run ./cmd/server
```

### 4. 启动前端

```bash
cd apps/web && pnpm dev
```

---

## 测试验证

### 后端测试

```bash
make test
# 或
cd server && go test ./...
```

### 前端类型检查

```bash
pnpm typecheck
```

### 前端单元测试（需 Node.js ≥ 20）

```bash
pnpm test
```

---

## 已知限制

1. **Node.js 版本**：前端单元测试需要 Node.js 20+，当前环境 Node.js 18 会导致 `rolldown` 兼容性问题
2. **飞书 API 字段**：飞书 OAuth 和用户信息接口字段按常见 Feishu OpenAPI 形态接入，实际字段名称可能需根据应用权限调整
3. **租户名称**：当前使用 `tenant_key` 作为租户标识，可后续扩展真实租户名称拉取

---

## 文件变更清单

### 新增文件

| 文件 | 说明 |
|------|------|
| `server/migrations/109_lark_tenants_and_workspace_visibility.up.sql` | 数据库迁移 |
| `server/migrations/109_lark_tenants_and_workspace_visibility.down.sql` | 回滚迁移 |
| `server/pkg/db/queries/lark_tenant.sql` | 租户相关查询 |
| `server/pkg/db/queries/user_identity.sql` | 用户身份查询 |
| `server/internal/handler/auth_lark.go` | 飞书登录 handler |
| `server/internal/handler/lark_workspace.go` | 租户 workspace 列表 handler |
| `apps/web/app/auth/lark/page.tsx` | 飞书登录页面 |
| `apps/web/app/(auth)/admin/workspaces/page.tsx` | 租户管理员 Workspace 管理页 |

### 修改文件

| 文件 | 主要变更 |
|------|---------|
| `server/pkg/db/queries/workspace.sql` | 新增 `visibility` 过滤逻辑 |
| `server/internal/handler/auth.go` | JWT 增加 `lark_tenant_id` |
| `server/internal/handler/handler.go` | 新增租户中间件 |
| `server/internal/handler/workspace.go` | 权限逻辑支持租户检查 |
| `server/internal/handler/invitation.go` | 邀请逻辑支持租户 |
| `server/internal/middleware/auth.go` | 认证中间件写入租户 ID |
| `server/internal/middleware/workspace.go` | Workspace 权限中间件支持租户 |
| `server/cmd/server/router.go` | 注册新路由 |
| `packages/core/config/index.ts` | 支持 `larkAppId` 配置 |
| `packages/core/platform/auth-initializer.tsx` | 初始化飞书 App ID |
| `packages/core/api/client.ts` | 新增 `/api/lark/workspaces` 接口 |
| `packages/core/auth/store.ts` | 支持 `lark_tenant_id` 状态 |
| `packages/core/types/workspace.ts` | `Workspace` 类型新增字段 |
| `packages/core/workspace/mutations.ts` | 创建 Workspace 支持 visibility |
| `packages/core/workspace/queries.ts` | 列表查询支持租户过滤 |
| `packages/core/paths/resolve.ts` | 登录后跳转逻辑支持 Lark |
| `packages/views/issues/hooks/issue-delete-mutations.test.tsx` | 测试 fixture 更新 |
| `packages/views/workspace/paths-hooks.test.tsx` | 测试 fixture 更新 |
| `packages/core/paths/resolve.test.ts` | 测试 fixture 更新 |
| `packages/core/realtime/use-realtime-sync.test.ts` | 测试 fixture 更新 |
| `apps/web/test/helpers.tsx` | 测试 fixture 更新 |
| `apps/web/app/(auth)/admin/workspaces/page.tsx` | 修复 type import |

---

## 故障排查

### "column home_tenant_id does not exist"

**原因**：迁移未执行

**解决**：
```bash
make migrate-up
```

### 飞书登录页面空白

**检查**：
1. `LARK_APP_ID` 和 `LARK_APP_SECRET` 是否正确配置
2. 飞书应用是否已开通对应权限（`contact:user.base:readonly`, `contact:user.email:readonly`）
3. `LARK_REDIRECT_URI` 是否与飞书应用配置一致

### "tenant session required"

**原因**：请求未携带正确的 Lark 租户会话信息

**解决**：确认用户通过 `/auth/lark` 登录，而非普通邮箱登录