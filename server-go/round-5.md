# Server-Go Convergence Plan (Round 5)

## 当前系统存在的问题

### 1) 同类错误处理仍有分叉（sync 入口）
- `internal/http/handlers/sync.go`
  - `SyncChanges` 的 JSON 绑定失败已走 `badRequestResult("Invalid JSON body")`。
  - `SyncNote` 与 `SyncAsset` 仍手写 `handlerResult{Status: 400, Body: ...}`。
- 同一类错误在 3 个入口存在两套写法，增加漂移风险。

### 2) 伪扩展点/无效参数（sync 内部错误结果）
- `internal/http/handlers/sync.go`
  - `internalErrorResult(_ error)` 接收参数但完全不使用。
- 该签名暗示“可基于 error 分支输出”，但当前并未实现，属于无效抽象面。

### 3) 资源响应头与响应体类型来源重复（潜在不一致）
- `internal/http/handlers/console_common.go`
- `internal/http/handlers/console_logo.go`
  - 同一请求里分别两次调用 `firstNonEmpty(object.ContentType, "application/octet-stream")`：
    - 一次用于 `Content-Type` header
    - 一次用于 `c.Blob(...)`
- 现状结果一致，但存在未来修改只改一处导致隐式分叉的风险。

## 本轮删减策略
- 将 sync 入口的“无效 JSON”错误统一到既有 `badRequestResult` 原语。
- 删除 `internalErrorResult` 的无效参数，收敛为零参数函数。
- 在资源响应路径内将 `contentType` 单点求值后复用，避免同义表达分叉。

## 保留的最小闭环定义
- `/obsidian/sync/changes|notes/:id|assets/:id` 的状态码与响应体语义保持不变。
- Console 资源读取与 logo/favicon 读取的返回体、缓存头、权限校验语义保持不变。
- 不修改任何路由、请求参数、环境变量、部署步骤。

## 将被删除 / 合并 / 收敛的模块或文件

### 收敛
- `internal/http/handlers/sync.go`
  - `SyncNote` 与 `SyncAsset` 的 bind 失败改为 `badRequestResult("Invalid JSON body")`。
  - `internalErrorResult(_ error)` -> `internalErrorResult()`，并更新调用点。

- `internal/http/handlers/console_common.go`
  - 资源返回时统一为单个 `contentType` 变量复用（header + blob）。

- `internal/http/handlers/console_logo.go`
  - `serveVaultObject` 返回时统一为单个 `contentType` 变量复用（header + blob）。

## 修改后的目标结构（简述）
- Sync 错误出口以 `badRequestResult` / `internalErrorResult` 两个稳定原语表达，去掉重复字面量与伪参数。
- Console 资源响应类型来源收敛为单点求值，降低未来维护时的隐式漂移风险。
