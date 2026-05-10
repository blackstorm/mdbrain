using Mdbrain.Data;
using Mdbrain.Security;
using Mdbrain.Storage;

namespace Mdbrain.Features.Console;

public static class ConsoleLogoEndpoints
{
    public static void MapConsoleLogoEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/console/vaults/{id}/logo", ServeVaultLogo)
            .AddEndpointFilter(ConsoleEndpointFilters.RequireConsoleAuthAsync);
        app.MapPost("/console/vaults/{id}/logo", UploadVaultLogo)
            .AddEndpointFilter(ConsoleEndpointFilters.RequireConsoleAuthAsync);
        app.MapDelete("/console/vaults/{id}/logo", DeleteVaultLogo)
            .AddEndpointFilter(ConsoleEndpointFilters.RequireConsoleAuthAsync);
        app.MapGet("/console/vaults/{id}/favicon", ServeVaultFavicon)
            .AddEndpointFilter(ConsoleEndpointFilters.RequireConsoleAuthAsync);
    }

    private static async Task<IResult> UploadVaultLogo(
        string id,
        HttpContext context,
        HttpRequest request,
        MdbrainDbContext db,
        IObjectStore store,
        CancellationToken cancellationToken)
    {
        if (!request.HasFormContentType)
        {
            return Results.Json(new { success = false, error = "No file uploaded" }, statusCode: StatusCodes.Status400BadRequest);
        }

        var form = await request.ReadFormAsync(cancellationToken);
        var service = new ConsoleLogoService(db, store);
        return ToHttpResult(await service.UploadLogoAsync(
            id,
            ConsoleMiddleware.CurrentSession(context).TenantId?.Trim() ?? string.Empty,
            form.Files.GetFile("logo"),
            cancellationToken), context);
    }

    private static async Task<IResult> DeleteVaultLogo(
        string id,
        HttpContext context,
        MdbrainDbContext db,
        IObjectStore store,
        CancellationToken cancellationToken)
    {
        var service = new ConsoleLogoService(db, store);
        return ToHttpResult(await service.DeleteLogoAsync(
            id,
            ConsoleMiddleware.CurrentSession(context).TenantId?.Trim() ?? string.Empty,
            cancellationToken), context);
    }

    private static Task<IResult> ServeVaultLogo(
        string id,
        HttpContext context,
        MdbrainDbContext db,
        IObjectStore store,
        CancellationToken cancellationToken)
    {
        return ServeVaultObject(id, favicon: false, context, db, store, cancellationToken);
    }

    private static Task<IResult> ServeVaultFavicon(
        string id,
        HttpContext context,
        MdbrainDbContext db,
        IObjectStore store,
        CancellationToken cancellationToken)
    {
        return ServeVaultObject(id, favicon: true, context, db, store, cancellationToken);
    }

    private static async Task<IResult> ServeVaultObject(
        string id,
        bool favicon,
        HttpContext context,
        MdbrainDbContext db,
        IObjectStore store,
        CancellationToken cancellationToken)
    {
        var service = new ConsoleLogoService(db, store);
        return ToHttpResult(await service.ReadLogoAsync(
            id,
            ConsoleMiddleware.CurrentSession(context).TenantId?.Trim() ?? string.Empty,
            favicon,
            cancellationToken), context);
    }

    private static IResult ToHttpResult(LogoServiceResult result, HttpContext context)
    {
        if (result.Bytes is not null)
        {
            context.Response.Headers.CacheControl = "public, max-age=31536000, immutable";
            return Results.Bytes(result.Bytes, result.ContentType ?? "application/octet-stream");
        }

        return Results.Json(result.Body, statusCode: result.StatusCode);
    }
}
