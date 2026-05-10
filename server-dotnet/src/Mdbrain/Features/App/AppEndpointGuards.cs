using Mdbrain.Config;

namespace Mdbrain.Features.App;

public static class AppEndpointGuards
{
    public static async Task PortBoundaryMiddleware(HttpContext context, AppConfig config, Func<Task> next)
    {
        var port = RequestPort(context);
        if (port == config.ConsolePort && !IsConsolePortPath(context.Request.Path))
        {
            await Results.NotFound("Not found").ExecuteAsync(context);
            return;
        }

        if (IsConsoleOnlyPath(context.Request.Path) && port == config.AppPort)
        {
            await Results.NotFound("Not found").ExecuteAsync(context);
            return;
        }

        await next();
    }

    public static bool IsConsoleOnlyPath(PathString path)
    {
        return path == "/robots.txt"
            || path.StartsWithSegments("/console")
            || path.StartsWithSegments("/obsidian")
            || path.StartsWithSegments("/publics/console");
    }

    public static bool IsAppStaticPath(PathString path)
    {
        return path.StartsWithSegments("/publics/app")
            || path.StartsWithSegments("/publics/shared");
    }

    private static bool IsAppOnlyPath(PathString path)
    {
        return path == "/favicon.ico"
            || path.StartsWithSegments("/storage")
            || path.StartsWithSegments("/publics/app");
    }

    public static int? RequestPort(HttpContext context)
    {
        return context.Connection.LocalPort > 0
            ? context.Connection.LocalPort
            : context.Request.Host.Port;
    }

    private static bool IsConsolePortPath(PathString path)
    {
        return path == "/"
            || path == "/robots.txt"
            || path.StartsWithSegments("/console")
            || path.StartsWithSegments("/obsidian")
            || path.StartsWithSegments("/publics/console")
            || path.StartsWithSegments("/publics/shared");
    }
}
