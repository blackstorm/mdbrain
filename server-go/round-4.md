# Server-Go Convergence Plan (Round 4)

## 当前系统问题

### 冗余辅助层（可删除）
- `internal/http/middleware/session_manager.go`
  - `sessionUserID` 仅有 1 处调用，语义只是 `c.Get("session.user_id")` + trim，属于无信息增益包装。
  - `redirect` 仅被 `session_middleware.go` 使用，语义只是 `c.Redirect(http.StatusFound, location)` 的透传。
- `internal/http/response/response.go`
  - `Unauthorized` 仅有 2 处调用，本质是 `Error(..., 401, ...)` 的薄封装。

### 隐式分支（可收敛）
- `internal/http/handlers/console_common.go`
  - `ServeConsoleAsset` 先读 `c.Param("path")` 再回退 `c.Param("*")`。
  - 实际路由为 `/console/storage/:id/*`，`path` 分支在当前系统中无有效来源，属于伪兼容分支。

## 本轮删减策略
- 删除“零信息增益”的一层包装函数，让调用点显式表达真实行为。
- 删除当前路由形态下不可达的参数分支，收敛为单一路径参数读取。
- 保持 HTTP 接口、状态码、响应体结构、路由与部署配置不变。

## 保留的最小闭环定义
- Console 会话链路保持：`SessionMiddleware -> ConsoleInitCheckMiddleware -> ConsoleAuthMiddleware`。
- Console 资源读取保持：`GET /console/storage/:id/*` 的鉴权、对象读取、响应头与响应体语义不变。
- Obsidian 同步鉴权保持：`Authorization: Bearer <sync_key>` 校验逻辑与 401 响应语义不变。

## 将被删除 / 合并 / 收敛的模块与内容

### 删除
- `internal/http/middleware/session_manager.go`
  - `sessionUserID`
  - `redirect`
- `internal/http/response/response.go`
  - `Unauthorized`

### 合并 / 收敛
- `internal/http/middleware/session_middleware.go`
  - 在 `ConsoleAuthMiddleware` 内联读取 `session.user_id`。
  - 在 `ConsoleAuthMiddleware` 与 `ConsoleInitCheckMiddleware` 内联 `c.Redirect(http.StatusFound, ...)`。
- `internal/http/handlers/sync.go`
  - 将 `response.Unauthorized(...)` 收敛为 `response.Error(..., http.StatusUnauthorized, ...)`。
- `internal/http/handlers/console_common.go`
  - 删除 `c.Param("path")` 分支，仅使用 `c.Param("*")`。

## 修改后的目标结构（简述）
- 会话与响应层仅保留“有独立语义价值”的函数，消除一次性包装。
- Console 资源路径解析只保留与真实路由一致的单一路径读取。
- 同步鉴权错误出口统一到基础 `response.Error` 原语，减少重复抽象层。
