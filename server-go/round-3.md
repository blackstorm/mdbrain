# Server-Go Convergence Plan (Round 3)

## Current Problems

### Redundancy / Dead Code
- `internal/http/middleware/session_manager.go` still contains unused helpers/methods:
  - `Session.Authenticated`
  - `Session.Clone`
  - `sessionTenantID`
  - `valueAsString`
  - `nowUTC`
- These paths are not used in production flow and increase mental/maintenance surface.

### State Fork / Implicit Behavior
- `SyncHandler` has split invalid-JSON handling paths:
  - `SyncChanges` uses `response.BadRequest` + manual publish error record.
  - `SyncNote`/`SyncAsset` use local `handlerResult` + `recordAndWrite`.
- Same error class is handled by multiple paths, increasing drift risk.

### Overly Narrow Helper Surface
- `internal/http/response/response.go` contains `BadRequest`, but after converging sync error handling it becomes redundant (only one call site currently).

## Round-3 Reduction Strategy
- Delete proven-dead middleware helpers.
- Converge sync invalid-JSON handling into one local path (`badRequestResult` + `recordAndWrite`).
- Remove redundant `response.BadRequest` helper and keep response package minimal (`Error` + `Unauthorized`).

## Minimum Closed Loop To Keep
- Session middleware still loads/saves session and CSRF behavior unchanged.
- Sync endpoints (`/obsidian/sync/changes`, `/obsidian/sync/notes/:id`, `/obsidian/sync/assets/:id`) keep response contract and publish status recording behavior.
- Console/App primary flow unchanged.

## Modules/Files To Delete, Merge, Converge

### Delete
- In `internal/http/middleware/session_manager.go`:
  - `Session.Authenticated`
  - `Session.Clone`
  - `sessionTenantID`
  - `valueAsString`
  - `nowUTC`
- In `internal/http/response/response.go`:
  - `BadRequest`

### Converge
- In `internal/http/handlers/sync.go`:
  - `SyncChanges` bind-failure path uses `badRequestResult("Invalid JSON body")` + `recordAndWrite`.
  - Keep `SyncNote` and `SyncAsset` bind-failure path in same pattern (already aligned).
  - Remove dependency on `response.BadRequest` in sync bind handling.

## Target Structure (Post Round-3)
- Middleware session module contains only active session/CSRF logic.
- Sync invalid-JSON behavior follows one internal path and one publish-status recording path.
- Response helper package is trimmed to active primitives only.
