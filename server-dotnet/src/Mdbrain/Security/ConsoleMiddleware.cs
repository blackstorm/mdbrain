using Mdbrain.Data;
using Microsoft.EntityFrameworkCore;

namespace Mdbrain.Security;

public static class ConsoleMiddleware
{
    public static async Task SessionMiddleware(HttpContext context, SessionManager sessions, Func<Task> next)
    {
        var session = sessions.Load(context);
        sessions.EnsureCsrf(session);
        SetSessionItems(context, session);
        await next();
        sessions.Save(context, CurrentSession(context));
    }

    public static async Task CsrfMiddleware(HttpContext context, Func<Task> next)
    {
        if (!context.Request.Path.StartsWithSegments("/console") || !StateChangingMethod(context.Request.Method))
        {
            await next();
            return;
        }

        var session = CurrentSession(context);
        var expected = session.CsrfToken?.Trim();
        var provided = ParseCsrfToken(context);
        if (string.IsNullOrWhiteSpace(expected) || string.IsNullOrWhiteSpace(provided) || expected != provided)
        {
            await Results.Json(new { success = false, error = "CSRF token missing or incorrect" }, statusCode: StatusCodes.Status403Forbidden).ExecuteAsync(context);
            return;
        }

        await next();
    }

    public static async Task InitCheckMiddleware(HttpContext context, MdbrainDbContext db, Func<Task> next)
    {
        var path = context.Request.Path.Value ?? string.Empty;
        if (!path.StartsWith("/console", StringComparison.Ordinal)
            || path is "/console/health" or "/console/domain-check")
        {
            await next();
            return;
        }

        var hasUser = await db.Users.AnyAsync(context.RequestAborted);
        if (hasUser && path == "/console/init")
        {
            context.Response.Redirect("/console/login");
            return;
        }

        if (!hasUser && path != "/console/init")
        {
            context.Response.Redirect("/console/init");
            return;
        }

        await next();
    }

    public static async Task AuthMiddleware(HttpContext context, Func<Task> next)
    {
        if (!string.IsNullOrWhiteSpace(CurrentSession(context).UserId))
        {
            await next();
            return;
        }

        context.Response.Redirect("/console/login");
    }

    public static async Task NoIndexMiddleware(HttpContext context, Func<Task> next)
    {
        await next();
        context.Response.Headers["X-Robots-Tag"] = "noindex, nofollow";
    }

    public static ConsoleSession CurrentSession(HttpContext context)
    {
        return context.Items.TryGetValue("session", out var value) && value is ConsoleSession session
            ? session
            : new ConsoleSession();
    }

    public static void SetSessionItems(HttpContext context, ConsoleSession session)
    {
        context.Items["session"] = session;
        context.Items["session.user_id"] = session.UserId ?? string.Empty;
        context.Items["session.tenant_id"] = session.TenantId ?? string.Empty;
        context.Items["csrf_token"] = session.CsrfToken ?? string.Empty;
    }

    private static bool StateChangingMethod(string method)
    {
        return method is "POST" or "PUT" or "DELETE" or "PATCH";
    }

    private static string ParseCsrfToken(HttpContext context)
    {
        var header = context.Request.Headers["X-CSRF-Token"].FirstOrDefault()?.Trim();
        if (!string.IsNullOrWhiteSpace(header))
        {
            return header;
        }

        return context.Request.HasFormContentType
            ? (context.Request.Form["__anti-forgery-token"].FirstOrDefault()?.Trim() ?? string.Empty)
            : string.Empty;
    }
}

