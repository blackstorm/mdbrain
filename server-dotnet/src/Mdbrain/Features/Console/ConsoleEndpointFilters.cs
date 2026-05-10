using Mdbrain.Data;
using Mdbrain.Data.Entities;
using Mdbrain.Security;
using Microsoft.EntityFrameworkCore;

namespace Mdbrain.Features.Console;

public static class ConsoleEndpointFilters
{
    public static async ValueTask<object?> RequireConsoleAuthAsync(EndpointFilterInvocationContext context, EndpointFilterDelegate next)
    {
        var session = ConsoleMiddleware.CurrentSession(context.HttpContext);
        return string.IsNullOrWhiteSpace(session.UserId)
            ? Results.Redirect("/console/login")
            : await next(context);
    }

    public static async Task<(Vault? Vault, string TenantId)> AuthorizedVaultForSessionAsync(
        HttpContext context,
        MdbrainDbContext db,
        string id,
        CancellationToken cancellationToken)
    {
        var tenantId = ConsoleMiddleware.CurrentSession(context).TenantId?.Trim() ?? string.Empty;
        var vault = await db.Vaults.FirstOrDefaultAsync(item => item.Id == id, cancellationToken);
        return (vault, tenantId);
    }
}

