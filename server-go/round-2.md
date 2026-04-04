# Server-Go Convergence Plan (Round 2)

## Current Problems

### Redundancy / Dead Paths
- Console handlers (`console_auth.go`, `console_vaults.go`) contain `valueFromJSON` fallback paths, but no production path writes those context keys; only form inputs are effective.
- Repository has an alias method `GetNoteForApp` that only forwards to `GetNoteByClientID`.

### Hidden Coupling / Layer Leak
- `ConsoleLogoHandler` depends on `ConsoleVaultHandler` internals by constructing a temporary `ConsoleVaultHandler` only to call `authorizedVault`.
- Shared helpers (`stringValue`, `firstNonEmpty`) are defined in feature files (`sync.go`, `console_auth.go`) but used across multiple handlers, creating non-obvious file coupling.

### Implicit Behavior / Error Surface
- `ConsoleVaultHandler.enrichVaults` ignores repository errors (`notes, _ := ...`, `storageSize, _ := ...`), turning data failures into silent partial state.
- `ConsoleInitCheckMiddleware` uses `context.Background()` instead of request context.
- `SyncHandler.syncChanges` uses repeated nested scans to compare client/server hashes, increasing code path complexity and mismatch risk.

## Round-2 Reduction Strategy
- Delete ineffective input paths and remove fake flexibility.
- Replace hidden cross-file dependencies with explicit shared helper location.
- Remove inter-handler coupling by using a shared vault authorization helper.
- Propagate data access errors instead of silently swallowing them.
- Simplify sync diff computation using explicit hash maps.

## Minimum Closed Loop To Keep
- Console init/login/password change/vault management/logo management remain functional.
- Sync endpoints (`/obsidian/sync/*`) keep response semantics unchanged.
- App note rendering and vault lookup flow remain unchanged.
- Existing storage modes (`local`, `s3`) and auth compatibility behavior remain unchanged.

## Modules/Files To Delete, Merge, Converge

### Delete
- `handlers.valueFromJSON`.
- `repository.Repository.GetNoteForApp`.

### Converge
- Input parsing in console handlers to form-only:
  - `console_auth.go`
  - `console_vaults.go`
- Move shared helper ownership to one place (`console_helpers.go`):
  - `firstNonEmpty`
  - `stringValue`
- Replace inter-handler authorization coupling:
  - add package-level `authorizedVaultForSession` helper (used by both console vault and console logo handlers)
  - remove `ConsoleVaultHandler.authorizedVault` method usage.
- `ConsoleVaultHandler.enrichVaults` -> return `([]any, error)` and propagate errors to callers.
- `ConsoleInitCheckMiddleware` to request-scoped context.
- `SyncHandler.syncChanges` to map-based diff logic with unchanged output contract.

## Target Structure (Post Round-2)
- Console handlers no longer contain ineffective JSON fallback branches.
- Shared handler helpers live in one helper file instead of feature-file leakage.
- Vault authorization logic is shared without cross-handler object construction.
- Vault list rendering path fails fast on repository errors rather than silently degrading.
- Sync diff logic is linear, explicit, and easier to verify.
