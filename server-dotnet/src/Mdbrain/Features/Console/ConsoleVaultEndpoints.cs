using Mdbrain.Data;
using Mdbrain.Rendering;
using Mdbrain.Security;
using Mdbrain.Storage;
using Microsoft.EntityFrameworkCore;

namespace Mdbrain.Features.Console;

public static class ConsoleVaultEndpoints
{
    public static void MapConsoleVaultEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/", () => Results.Redirect("/console"));
        app.MapGet("/console", ConsoleHome).AddEndpointFilter(ConsoleEndpointFilters.RequireConsoleAuthAsync);
        app.MapGet("/console/vaults", ListVaults).AddEndpointFilter(ConsoleEndpointFilters.RequireConsoleAuthAsync);
        app.MapPost("/console/vaults", CreateVault).AddEndpointFilter(ConsoleEndpointFilters.RequireConsoleAuthAsync);
        app.MapPut("/console/vaults/{id}", UpdateVault).AddEndpointFilter(ConsoleEndpointFilters.RequireConsoleAuthAsync);
        app.MapDelete("/console/vaults/{id}", DeleteVault).AddEndpointFilter(ConsoleEndpointFilters.RequireConsoleAuthAsync);
        app.MapGet("/console/vaults/{id}/notes", SearchVaultNotes).AddEndpointFilter(ConsoleEndpointFilters.RequireConsoleAuthAsync);
        app.MapPut("/console/vaults/{id}/root-note", UpdateVaultRootNote).AddEndpointFilter(ConsoleEndpointFilters.RequireConsoleAuthAsync);
        app.MapGet("/console/vaults/{id}/root-note-selector", GetRootNoteSelector).AddEndpointFilter(ConsoleEndpointFilters.RequireConsoleAuthAsync);
        app.MapPost("/console/vaults/{id}/renew-sync-key", RenewVaultSyncKey).AddEndpointFilter(ConsoleEndpointFilters.RequireConsoleAuthAsync);
        app.MapPut("/console/vaults/{id}/custom-head-html", UpdateCustomHeadHtml).AddEndpointFilter(ConsoleEndpointFilters.RequireConsoleAuthAsync);
    }

    private static async Task<IResult> ConsoleHome(
        HttpContext context,
        MdbrainDbContext db,
        TemplateRenderer renderer,
        IObjectStore store,
        CancellationToken cancellationToken)
    {
        var tenantId = ConsoleMiddleware.CurrentSession(context).TenantId ?? string.Empty;
        var tenant = await db.Tenants.AsNoTracking().FirstAsync(item => item.Id == tenantId, cancellationToken);
        var vaults = await EnrichedVaultsAsync(db, store, tenantId, cancellationToken);
        var html = await renderer.RenderAsync("templates/console/vaults.html", new Dictionary<string, object?>
        {
            ["tenant"] = new Dictionary<string, object?> { ["id"] = tenant.Id, ["name"] = tenant.Name },
            ["vaults"] = vaults,
            ["csrf-token"] = ConsoleMiddleware.CurrentSession(context).CsrfToken ?? string.Empty
        }, cancellationToken);
        return Results.Content(html, "text/html; charset=utf-8");
    }

    private static async Task<IResult> ListVaults(MdbrainDbContext db, HttpContext context, TemplateRenderer renderer, IObjectStore store, CancellationToken cancellationToken)
    {
        var tenantId = ConsoleMiddleware.CurrentSession(context).TenantId ?? string.Empty;
        var html = await renderer.RenderAsync("templates/console/vault-list.html", new Dictionary<string, object?>
        {
            ["vaults"] = await EnrichedVaultsAsync(db, store, tenantId, cancellationToken)
        }, cancellationToken);
        return Results.Content(html, "text/html; charset=utf-8");
    }

    private static async Task<IResult> CreateVault(HttpContext context, HttpRequest request, MdbrainDbContext db, CancellationToken cancellationToken)
    {
        var form = await request.ReadFormAsync(cancellationToken);
        var tenantId = ConsoleMiddleware.CurrentSession(context).TenantId ?? string.Empty;
        var name = ((string?)form["name"].FirstOrDefault() ?? string.Empty).Trim();
        var domain = ((string?)form["domain"].FirstOrDefault() ?? string.Empty).Trim();
        if (name.Length == 0 || domain.Length == 0)
            return Results.Json(new { success = false, error = "Missing required fields" });
        if (await db.Vaults.AnyAsync(item => item.Domain == domain, cancellationToken))
            return Results.Json(new { success = false, error = "Domain already in use" });

        var vaultId = Guid.NewGuid().ToString();
        var syncKey = Guid.NewGuid().ToString();
        db.Vaults.Add(new Data.Entities.Vault
        {
            Id = vaultId,
            TenantId = tenantId,
            Name = name,
            Domain = domain,
            SyncKey = syncKey
        });
        await db.SaveChangesAsync(cancellationToken);

        return Results.Json(new
        {
            success = true,
            vault = new Dictionary<string, object?>
            {
                ["id"] = vaultId,
                ["name"] = name,
                ["domain"] = domain,
                ["sync-key"] = syncKey
            }
        });
    }

    private static async Task<IResult> DeleteVault(string id, HttpContext context, MdbrainDbContext db, IObjectStore store, CancellationToken cancellationToken)
    {
        var (vault, tenantId) = await ConsoleEndpointFilters.AuthorizedVaultForSessionAsync(context, db, id, cancellationToken);
        if (vault is null) return Results.Json(new { success = false, error = "Vault not found" });
        if (vault.TenantId != tenantId) return Results.Json(new { success = false, error = "Permission denied" });

        await store.DeleteVaultObjectsAsync(vault.Id, cancellationToken);
        db.Vaults.Remove(vault);
        await db.SaveChangesAsync(cancellationToken);
        return Results.Json(new { success = true, message = "Vault deleted" });
    }

    private static async Task<IResult> UpdateVault(string id, HttpContext context, HttpRequest request, MdbrainDbContext db, CancellationToken cancellationToken)
    {
        var (vault, tenantId) = await ConsoleEndpointFilters.AuthorizedVaultForSessionAsync(context, db, id, cancellationToken);
        if (vault is null) return Results.Json(new { success = false, error = "Vault not found" });
        if (vault.TenantId != tenantId) return Results.Json(new { success = false, error = "Permission denied" });

        var form = await request.ReadFormAsync(cancellationToken);
        var name = ((string?)form["name"].FirstOrDefault() ?? string.Empty).Trim();
        var domain = ((string?)form["domain"].FirstOrDefault() ?? string.Empty).Trim();
        if (name.Length == 0) return Results.Json(new { success = false, error = "Vault name is required" });
        if (domain.Length == 0) return Results.Json(new { success = false, error = "Domain is required" });
        if (await db.Vaults.AnyAsync(item => item.Domain == domain && item.Id != vault.Id, cancellationToken))
            return Results.Json(new { success = false, error = "Domain already in use" });

        vault.Name = name;
        vault.Domain = domain;
        await db.SaveChangesAsync(cancellationToken);
        return Results.Json(new { success = true, vault = new { id = vault.Id, name, domain } });
    }

    private static async Task<IResult> SearchVaultNotes(string id, string? q, HttpContext context, MdbrainDbContext db, CancellationToken cancellationToken)
    {
        var (vault, tenantId) = await ConsoleEndpointFilters.AuthorizedVaultForSessionAsync(context, db, id, cancellationToken);
        if (vault is null) return Results.Json(new { success = false, error = "Vault not found" });
        if (vault.TenantId != tenantId) return Results.Json(new { success = false, error = "Permission denied" });
        q = (q ?? string.Empty).Trim();
        var notesQuery = db.Notes.AsNoTracking().Where(note => note.VaultId == vault.Id && note.DeletedAt == null);
        if (q.Length > 0)
        {
            notesQuery = notesQuery.Where(note => note.Path.Contains(q) || (note.Content != null && note.Content.Contains(q)));
        }
        var notes = await notesQuery.OrderBy(note => note.Path).Take(50).Select(note => new
        {
            client_id = note.ClientId,
            path = note.Path,
            mtime = note.Mtime
        }).ToListAsync(cancellationToken);
        return Results.Json(new { success = true, notes });
    }

    private static async Task<IResult> UpdateVaultRootNote(string id, HttpContext context, HttpRequest request, MdbrainDbContext db, CancellationToken cancellationToken)
    {
        var (vault, tenantId) = await ConsoleEndpointFilters.AuthorizedVaultForSessionAsync(context, db, id, cancellationToken);
        if (vault is null) return Results.Json(new { success = false, error = "Vault not found" });
        if (vault.TenantId != tenantId) return Results.Json(new { success = false, error = "Permission denied" });
        var form = await request.ReadFormAsync(cancellationToken);
        var rootNoteId = ((string?)form["rootNoteId"].FirstOrDefault() ?? string.Empty).Trim();
        if (rootNoteId.Length == 0) return Results.Json(new { success = false, error = "Missing rootNoteId" });
        if (!await db.Notes.AnyAsync(note => note.VaultId == vault.Id && note.ClientId == rootNoteId && note.DeletedAt == null, cancellationToken))
            return Results.Json(new { success = false, error = "Root note not found" });
        vault.RootNoteId = rootNoteId;
        await db.SaveChangesAsync(cancellationToken);
        return Results.Json(new Dictionary<string, object?> { ["success"] = true, ["message"] = "Root note updated", ["root-note-id"] = rootNoteId });
    }

    private static async Task<IResult> RenewVaultSyncKey(string id, HttpContext context, MdbrainDbContext db, CancellationToken cancellationToken)
    {
        var (vault, tenantId) = await ConsoleEndpointFilters.AuthorizedVaultForSessionAsync(context, db, id, cancellationToken);
        if (vault is null) return Results.Json(new { success = false, error = "Vault not found" });
        if (vault.TenantId != tenantId) return Results.Json(new { success = false, error = "Permission denied" });
        var syncKey = Guid.NewGuid().ToString();
        vault.SyncKey = syncKey;
        await db.SaveChangesAsync(cancellationToken);
        return Results.Json(new Dictionary<string, object?> { ["success"] = true, ["message"] = "Publish key renewed", ["sync-key"] = syncKey });
    }

    private static async Task<IResult> GetRootNoteSelector(
        string id,
        HttpContext context,
        MdbrainDbContext db,
        TemplateRenderer renderer,
        CancellationToken cancellationToken)
    {
        var (vault, tenantId) = await ConsoleEndpointFilters.AuthorizedVaultForSessionAsync(context, db, id, cancellationToken);
        if (vault is null) return Results.Content("""<div class="alert alert-error"><span>Vault not found</span></div>""", "text/html; charset=utf-8", statusCode: StatusCodes.Status404NotFound);
        if (vault.TenantId != tenantId) return Results.Content("""<div class="alert alert-error"><span>Permission denied</span></div>""", "text/html; charset=utf-8", statusCode: StatusCodes.Status403Forbidden);

        var notes = await db.Notes.AsNoTracking()
            .Where(note => note.VaultId == vault.Id && note.DeletedAt == null)
            .OrderBy(note => note.Path)
            .Take(50)
            .Select(note => new { note.ClientId, note.Path, note.Mtime })
            .ToListAsync(cancellationToken);
        var selectedNotePath = string.Empty;
        var noteItems = notes.Select(note =>
        {
            var selected = note.ClientId == (vault.RootNoteId ?? string.Empty);
            if (selected)
            {
                selectedNotePath = note.Path;
            }

            return new Dictionary<string, object?>
            {
                ["client_id"] = note.ClientId,
                ["path"] = note.Path,
                ["mtime"] = note.Mtime,
                ["vault_id"] = vault.Id,
                ["selected"] = selected
            };
        }).ToArray();
        var html = await renderer.RenderAsync("templates/console/root-note-selector.html", new Dictionary<string, object?>
        {
            ["notes"] = noteItems,
            ["vault_id"] = vault.Id,
            ["selected_note_path"] = selectedNotePath
        }, cancellationToken);
        return Results.Content(html, "text/html; charset=utf-8");
    }

    private static async Task<IResult> UpdateCustomHeadHtml(string id, HttpContext context, HttpRequest request, MdbrainDbContext db, CancellationToken cancellationToken)
    {
        var (vault, tenantId) = await ConsoleEndpointFilters.AuthorizedVaultForSessionAsync(context, db, id, cancellationToken);
        if (vault is null) return Results.Json(new { success = false, error = "Vault not found" });
        if (vault.TenantId != tenantId) return Results.Json(new { success = false, error = "Permission denied" });
        var form = await request.ReadFormAsync(cancellationToken);
        var custom = (string?)form["customHeadHtml"].FirstOrDefault() ?? string.Empty;
        if (custom.Length > 65536) return Results.Json(new { success = false, error = "Custom HTML exceeds maximum size of 64KB" });
        vault.CustomHeadHtml = custom;
        await db.SaveChangesAsync(cancellationToken);
        return Results.Json(new { success = true, message = "Custom HTML updated" });
    }

    private static async Task<IReadOnlyList<Dictionary<string, object?>>> EnrichedVaultsAsync(
        MdbrainDbContext db,
        IObjectStore store,
        string tenantId,
        CancellationToken cancellationToken)
    {
        var vaults = await db.Vaults.AsNoTracking()
            .Where(vault => vault.TenantId == tenantId)
            .OrderBy(vault => vault.CreatedAt)
            .ToListAsync(cancellationToken);
        var output = new List<Dictionary<string, object?>>(vaults.Count);
        foreach (var vault in vaults)
        {
            var noteRows = await db.Notes.AsNoTracking()
                .Where(note => note.VaultId == vault.Id && note.DeletedAt == null)
                .OrderBy(note => note.Path)
                .Take(50)
                .Select(note => new { note.ClientId, note.Path, note.Mtime })
                .ToListAsync(cancellationToken);
            var notes = noteRows.Select(note => new Dictionary<string, object?>
            {
                ["client_id"] = note.ClientId,
                ["path"] = note.Path,
                ["mtime"] = note.Mtime
            }).ToArray();
            var storageSize = await db.Assets.AsNoTracking()
                .Where(asset => asset.VaultId == vault.Id && asset.DeletedAt == null)
                .SumAsync(asset => (long?)asset.SizeBytes, cancellationToken) ?? 0;
            var publishStatus = string.IsNullOrWhiteSpace(vault.LastPublishStatus) ? "never" : vault.LastPublishStatus;
            var item = new Dictionary<string, object?>
            {
                ["id"] = vault.Id,
                ["name"] = vault.Name,
                ["domain"] = vault.Domain ?? string.Empty,
                ["sync_key"] = vault.SyncKey,
                ["masked_key"] = ConsoleHelpers.MaskKey(vault.SyncKey),
                ["root_note_id"] = vault.RootNoteId ?? string.Empty,
                ["custom_head_html"] = vault.CustomHeadHtml ?? string.Empty,
                ["last_publish_at"] = vault.LastPublishAt,
                ["last_publish_status"] = publishStatus,
                ["last_publish_error_message"] = vault.LastPublishErrorMessage ?? string.Empty,
                ["publish_ok"] = publishStatus == "ok",
                ["publish_error"] = publishStatus == "error",
                ["publish_never"] = publishStatus != "ok" && publishStatus != "error",
                ["notes"] = notes,
                ["storage_size"] = ConsoleHelpers.FormatStorageSize(storageSize)
            };
            if (!string.IsNullOrWhiteSpace(vault.LogoObjectKey))
            {
                item["logo_url"] = ConsoleCommonEndpoints.ConsoleAssetUrl(vault.Id, vault.LogoObjectKey);
            }
            output.Add(item);
        }

        return output;
    }
}
