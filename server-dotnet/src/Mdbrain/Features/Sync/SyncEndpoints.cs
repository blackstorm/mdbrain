using Mdbrain.Data;
using Microsoft.EntityFrameworkCore;

namespace Mdbrain.Features.Sync;

public static class SyncEndpoints
{
    public static void MapSyncEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/obsidian");
        group.MapMethods("/sync/changes", ["OPTIONS"], Options);
        group.MapPost("/sync/changes", SyncChanges);
        group.MapMethods("/sync/notes/{id}", ["OPTIONS"], Options);
        group.MapPost("/sync/notes/{id}", SyncNote);
        group.MapMethods("/sync/assets/{id}", ["OPTIONS"], Options);
        group.MapPost("/sync/assets/{id}", SyncAsset);
        group.MapGet("/vault/info", VaultInfo);
    }

    private static IResult Options()
    {
        return Results.Ok();
    }

    private static async Task<IResult> SyncChanges(
        HttpRequest request,
        SyncChangesRequest body,
        MdbrainDbContext db,
        SyncService service,
        CancellationToken cancellationToken)
    {
        var vaultResult = await RequireSyncVaultAsync(request, db, cancellationToken);
        if (vaultResult.Result is not null) return vaultResult.Result;

        var result = await service.SyncChangesAsync(vaultResult.Vault!, body, cancellationToken);
        await service.RecordPublishResultAsync(vaultResult.Vault!, result, cancellationToken);
        return Results.Json(result.Body, statusCode: result.Status);
    }

    private static async Task<IResult> SyncNote(
        string id,
        HttpRequest request,
        SyncNoteRequest body,
        MdbrainDbContext db,
        SyncService service,
        CancellationToken cancellationToken)
    {
        var vaultResult = await RequireSyncVaultAsync(request, db, cancellationToken);
        if (vaultResult.Result is not null) return vaultResult.Result;

        var result = await service.SyncNoteAsync(vaultResult.Vault!, id, body, cancellationToken);
        await service.RecordPublishResultAsync(vaultResult.Vault!, result, cancellationToken);
        return Results.Json(result.Body, statusCode: result.Status);
    }

    private static async Task<IResult> SyncAsset(
        string id,
        HttpRequest request,
        SyncAssetRequest body,
        MdbrainDbContext db,
        SyncService service,
        CancellationToken cancellationToken)
    {
        var vaultResult = await RequireSyncVaultAsync(request, db, cancellationToken);
        if (vaultResult.Result is not null) return vaultResult.Result;

        var result = await service.SyncAssetAsync(vaultResult.Vault!, id, body, cancellationToken);
        await service.RecordPublishResultAsync(vaultResult.Vault!, result, cancellationToken);
        return Results.Json(result.Body, statusCode: result.Status);
    }

    private static async Task<IResult> VaultInfo(HttpRequest request, MdbrainDbContext db, CancellationToken cancellationToken)
    {
        var vaultResult = await RequireSyncVaultAsync(request, db, cancellationToken);
        if (vaultResult.Result is not null) return vaultResult.Result;
        var vault = vaultResult.Vault!;
        return Results.Json(new
        {
            vault = new
            {
                id = vault.Id,
                name = vault.Name,
                domain = vault.Domain ?? string.Empty,
                createdAt = vault.CreatedAt
            }
        });
    }

    private static async Task<(Data.Entities.Vault? Vault, IResult? Result)> RequireSyncVaultAsync(
        HttpRequest request,
        MdbrainDbContext db,
        CancellationToken cancellationToken)
    {
        var authHeader = request.Headers.Authorization.FirstOrDefault();
        if (authHeader is null || !authHeader.StartsWith("Bearer ", StringComparison.Ordinal))
        {
            return (null, Results.Json(new { success = false, error = "Missing authorization header" }, statusCode: StatusCodes.Status401Unauthorized));
        }

        var syncKey = authHeader["Bearer ".Length..].Trim();
        var vault = await db.Vaults.FirstOrDefaultAsync(item => item.SyncKey == syncKey, cancellationToken);
        return vault is null
            ? (null, Results.Json(new { success = false, error = "Invalid publish key" }, statusCode: StatusCodes.Status401Unauthorized))
            : (vault, null);
    }
}

