# Server-Go Convergence Plan (Round 6)

## 当前系统存在的问题

### 1) 同一常量语义通过“可变参数 helper”表达（伪扩展点）
- `internal/http/handlers/sync.go`
  - `truncate(value, n int)` 仅在 `publishErrorMessage` 中以固定值 `400` 调用一次。
- `n` 参数在当前系统不是业务变量，而是固定规则，保留可变参数会放大理解面。

### 2) 一次性逻辑使用“通用 helper / map 选择”表达（过度工程化）
- `internal/http/handlers/console_logo.go`
  - `sha256Hex(content, length)` 仅一处调用，且 `length` 固定为 `16`。
  - 扩展名选择使用 `map[string]string{...}[contentType]`，当前仅有 PNG/JPEG 两条固定分支。
- 这两处都在表达“固定业务规则”，却使用了“可扩展形态”。

## 本轮删减策略
- 删除 `truncate` helper，把固定 400 长度限制内聚到 `publishErrorMessage` 单点。
- 删除 `sha256Hex(..., length)` 的“可变长度”参数，改为上传路径中的固定哈希规则（16 字节前缀）。
- 将 logo 扩展名选择从运行时 map 查询收敛为显式分支（PNG/JPEG）。

## 保留的最小闭环定义
- Sync 错误响应结构保持：`{"success": false, "error": ...}`，发布错误消息截断上限仍为 400。
- Logo 上传能力保持：仍只允许 PNG/JPEG，logo key 仍基于内容哈希前缀生成。
- 不修改任何接口路径、请求参数、响应字段、配置项或部署方式。

## 将被删除 / 合并 / 收敛的模块或文件

### 删除
- `internal/http/handlers/sync.go`
  - 删除 `truncate(value, n int)`。
- `internal/http/handlers/console_logo.go`
  - 删除 `sha256Hex(content, length)`。

### 收敛
- `internal/http/handlers/sync.go`
  - 在 `publishErrorMessage` 内联固定 400 长度截断逻辑。
- `internal/http/handlers/console_logo.go`
  - 在 `UploadVaultLogo` 内联固定 16 字节 hash 前缀规则。
  - 扩展名选择改为显式分支，移除 map 查询。
- `internal/http/handlers/console_logo_more_test.go`
  - 断言中不再依赖被删除的生产 helper，改为测试内本地计算固定 16 字节 hash 前缀。

## 修改后的目标结构（简述）
- 仅保留“当前已证明需要”的固定规则表达，不保留未使用的可变参数入口。
- 让错误消息截断与 logo key 生成规则都变成单点、直读、可验证的实现。
