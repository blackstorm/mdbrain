using Mdbrain.Config;
using Mdbrain.Data;
using Microsoft.EntityFrameworkCore;

namespace Mdbrain.Features.Internal;

public static class InternalEndpoints
{
    public static void MapInternalEndpoints(this WebApplication app)
    {
        app.MapGet("/robots.txt", InternalResults.Robots);

        app.MapGet("/console/health", (HttpRequest request, AppConfig config) =>
        {
            var token = request.Headers["X-Health-Token"].FirstOrDefault();
            return InternalResults.Health(token, request.Query["token"].FirstOrDefault(), config.HealthToken);
        });

        if (app.Services.GetRequiredService<AppConfig>().OnDemandTlsEnabled)
        {
            app.MapGet("/console/domain-check", async (string? domain, MdbrainDbContext db, CancellationToken cancellationToken) =>
            {
                var exists = !string.IsNullOrWhiteSpace(domain)
                    && await db.Vaults.AsNoTracking().AnyAsync(vault => vault.Domain == domain, cancellationToken);

                return InternalResults.DomainCheck(exists);
            });
        }
    }
}
