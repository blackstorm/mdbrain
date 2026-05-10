using Mdbrain.Data;
using Mdbrain.Data.Entities;
using Mdbrain.Rendering;
using Mdbrain.Security;
using Microsoft.EntityFrameworkCore;

namespace Mdbrain.Features.Console;

public static class ConsoleAuthEndpoints
{
    public static void MapConsoleAuthEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/console/login", LoginPage);
        app.MapGet("/console/init", InitPage);
        app.MapPost("/console/init", InitConsole);
        app.MapPost("/console/login", Login);
        app.MapPost("/console/logout", Logout).AddEndpointFilter(ConsoleEndpointFilters.RequireConsoleAuthAsync);
        app.MapPut("/console/user/password", ChangePassword).AddEndpointFilter(ConsoleEndpointFilters.RequireConsoleAuthAsync);
    }

    private static async Task<IResult> LoginPage(HttpContext context, TemplateRenderer renderer, CancellationToken cancellationToken)
    {
        var html = await renderer.RenderAsync("templates/console/login.html", new Dictionary<string, object?>
        {
            ["csrf-token"] = ConsoleMiddleware.CurrentSession(context).CsrfToken ?? string.Empty
        }, cancellationToken);
        return Results.Content(html, "text/html; charset=utf-8");
    }

    private static async Task<IResult> InitPage(HttpContext context, TemplateRenderer renderer, CancellationToken cancellationToken)
    {
        var html = await renderer.RenderAsync("templates/console/init.html", new Dictionary<string, object?>
        {
            ["csrf-token"] = ConsoleMiddleware.CurrentSession(context).CsrfToken ?? string.Empty
        }, cancellationToken);
        return Results.Content(html, "text/html; charset=utf-8");
    }

    private static async Task<IResult> InitConsole(
        HttpRequest request,
        MdbrainDbContext db,
        IPasswordHasher hasher,
        CancellationToken cancellationToken)
    {
        if (await db.Users.AnyAsync(cancellationToken))
        {
            return Results.Json(new { success = false, error = "System already initialized" }, statusCode: StatusCodes.Status403Forbidden);
        }

        var form = await request.ReadFormAsync(cancellationToken);
        var username = (string?)form["username"].FirstOrDefault() ?? string.Empty;
        var password = (string?)form["password"].FirstOrDefault() ?? string.Empty;
        var tenantName = ConsoleHelpers.FirstNonEmpty(form["tenant-name"].FirstOrDefault(), form["tenantName"].FirstOrDefault()).Trim();

        if (string.IsNullOrWhiteSpace(username) || string.IsNullOrWhiteSpace(password) || string.IsNullOrWhiteSpace(tenantName))
        {
            return Results.Json(new { success = false, error = "Missing required fields" });
        }

        if (await db.Users.AnyAsync(user => user.Username == username.Trim(), cancellationToken))
        {
            return Results.Json(new { success = false, error = "Username already exists" });
        }

        var tenantId = Guid.NewGuid().ToString();
        var userId = Guid.NewGuid().ToString();
        db.Tenants.Add(new Tenant { Id = tenantId, Name = tenantName });
        db.Users.Add(new User
        {
            Id = userId,
            TenantId = tenantId,
            Username = username.Trim(),
            PasswordHash = hasher.Hash(password)
        });
        await db.SaveChangesAsync(cancellationToken);

        return Results.Json(new Dictionary<string, object?>
        {
            ["success"] = true,
            ["tenant-id"] = tenantId,
            ["user-id"] = userId
        });
    }

    private static async Task<IResult> Login(
        HttpContext context,
        HttpRequest request,
        MdbrainDbContext db,
        IPasswordHasher hasher,
        SessionManager sessions,
        CancellationToken cancellationToken)
    {
        var form = await request.ReadFormAsync(cancellationToken);
        var username = ((string?)form["username"].FirstOrDefault() ?? string.Empty).Trim();
        var password = (string?)form["password"].FirstOrDefault() ?? string.Empty;
        var user = await db.Users.AsNoTracking().FirstOrDefaultAsync(item => item.Username == username, cancellationToken);
        if (user is null || !hasher.Verify(password, user.PasswordHash))
        {
            return Results.Json(new { success = false, error = "Invalid username or password" });
        }

        var session = new ConsoleSession
        {
            UserId = user.Id,
            TenantId = user.TenantId,
            CsrfToken = ConsoleMiddleware.CurrentSession(context).CsrfToken ?? Guid.NewGuid().ToString()
        };
        ConsoleMiddleware.SetSessionItems(context, session);
        sessions.Save(context, session);

        return Results.Json(new
        {
            success = true,
            user = new Dictionary<string, object?>
            {
                ["id"] = user.Id,
                ["username"] = user.Username,
                ["tenant-id"] = user.TenantId
            }
        });
    }

    private static IResult Logout(HttpContext context, SessionManager sessions)
    {
        sessions.Clear(context);
        ConsoleMiddleware.SetSessionItems(context, new ConsoleSession());
        return Results.Redirect("/console/login");
    }

    private static async Task<IResult> ChangePassword(
        HttpContext context,
        HttpRequest request,
        MdbrainDbContext db,
        IPasswordHasher hasher,
        CancellationToken cancellationToken)
    {
        var userId = ConsoleMiddleware.CurrentSession(context).UserId?.Trim() ?? string.Empty;
        if (userId.Length == 0)
        {
            return Results.Json(new { success = false, error = "Unauthorized" }, statusCode: StatusCodes.Status401Unauthorized);
        }

        var form = await request.ReadFormAsync(cancellationToken);
        var current = ConsoleHelpers.FirstNonEmpty(form["current-password"].FirstOrDefault(), form["currentPassword"].FirstOrDefault());
        var next = ConsoleHelpers.FirstNonEmpty(form["new-password"].FirstOrDefault(), form["newPassword"].FirstOrDefault());
        var confirm = ConsoleHelpers.FirstNonEmpty(form["confirm-password"].FirstOrDefault(), form["confirmPassword"].FirstOrDefault());

        if (string.IsNullOrWhiteSpace(current) || string.IsNullOrWhiteSpace(next) || string.IsNullOrWhiteSpace(confirm))
            return Results.Json(new { success = false, error = "Missing required fields" }, statusCode: StatusCodes.Status400BadRequest);
        if (next != confirm)
            return Results.Json(new { success = false, error = "New password confirmation does not match" }, statusCode: StatusCodes.Status400BadRequest);
        if (next.Length < 8)
            return Results.Json(new { success = false, error = "New password must be at least 8 characters" }, statusCode: StatusCodes.Status400BadRequest);

        var user = await db.Users.FirstOrDefaultAsync(item => item.Id == userId, cancellationToken);
        if (user is null)
            return Results.Json(new { success = false, error = "User not found" }, statusCode: StatusCodes.Status404NotFound);
        if (!hasher.Verify(current, user.PasswordHash))
            return Results.Json(new { success = false, error = "Current password is incorrect" }, statusCode: StatusCodes.Status400BadRequest);

        user.PasswordHash = hasher.Hash(next);
        await db.SaveChangesAsync(cancellationToken);
        return Results.Json(new { success = true, message = "Password updated" });
    }

}
