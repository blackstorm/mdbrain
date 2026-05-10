using Mdbrain.Data;
using Mdbrain.Storage;

namespace Mdbrain.Features.Console;

public static class ConsoleCommonEndpoints
{
    public static void MapConsoleCommonEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/console/storage/{id}/{**path}", ServeConsoleAsset)
            .AddEndpointFilter(ConsoleEndpointFilters.RequireConsoleAuthAsync);
    }

    public static string ConsoleAssetUrl(string vaultId, string objectKey)
    {
        return $"/console/storage/{vaultId}/{objectKey}";
    }

    private static async Task<IResult> ServeConsoleAsset(
        string id,
        string? path,
        HttpContext context,
        MdbrainDbContext db,
        IObjectStore store,
        CancellationToken cancellationToken)
    {
        var (vault, tenantId) = await ConsoleEndpointFilters.AuthorizedVaultForSessionAsync(context, db, id, cancellationToken);
        if (vault is null)
        {
            return Results.Json(new { success = false, error = "Vault not found" }, statusCode: StatusCodes.Status404NotFound);
        }

        if (vault.TenantId != tenantId)
        {
            return Results.Json(new { success = false, error = "Permission denied" }, statusCode: StatusCodes.Status403Forbidden);
        }

        path = (path ?? string.Empty).Trim();
        if (path.Length == 0)
        {
            return Results.Json(new { success = false, error = "Missing path" }, statusCode: StatusCodes.Status400BadRequest);
        }

        await using var stored = await store.GetObjectAsync(vault.Id, path, cancellationToken);
        if (stored is null)
        {
            return Results.Json(new { success = false, error = "Not found" }, statusCode: StatusCodes.Status404NotFound);
        }

        using var memory = new MemoryStream();
        await stored.Body.CopyToAsync(memory, cancellationToken);
        context.Response.Headers.CacheControl = "public, max-age=31536000, immutable";
        return Results.Bytes(memory.ToArray(), stored.ContentType);
    }
}
