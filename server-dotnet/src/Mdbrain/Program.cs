using Mdbrain.Config;
using Mdbrain.Data;
using Mdbrain.Features.App;
using Mdbrain.Features.Console;
using Mdbrain.Features.Internal;
using Mdbrain.Features.Sync;
using Mdbrain.Rendering;
using Mdbrain.Security;
using Mdbrain.Storage;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.FileProviders;

var builder = WebApplication.CreateBuilder(args);
var projectRoot = FindProjectRoot(AppContext.BaseDirectory);
var config = AppConfig.Load(projectRoot);
builder.WebHost.UseUrls(
    $"http://{config.AppHost}:{config.AppPort}",
    $"http://{config.ConsoleHost}:{config.ConsolePort}");
var objectStore = await ObjectStoreFactory.CreateAsync(config, CancellationToken.None);

builder.Services.AddSingleton(config);
builder.Services.AddSingleton<SessionManager>();
builder.Services.AddSingleton<IPasswordHasher, Pbkdf2PasswordHasher>();
builder.Services.AddSingleton(objectStore);
builder.Services.AddSingleton<TemplateRenderer>();
builder.Services.AddSingleton<MarkdownRenderer>();
builder.Services.AddDbContext<MdbrainDbContext>(options =>
{
    options.UseSqlite(SqliteDatabase.FileConnectionString(config));
});
builder.Services.AddScoped<SyncService>();

var app = builder.Build();

await using (var scope = app.Services.CreateAsyncScope())
{
    var db = scope.ServiceProvider.GetRequiredService<MdbrainDbContext>();
    await SqliteDatabase.InitializeFileDatabaseAsync(db, app.Lifetime.ApplicationStopping);
}

app.Use((context, next) => AppEndpointGuards.PortBoundaryMiddleware(context, config, next));
app.UseStaticFiles();
app.UseStaticFiles(new StaticFileOptions
{
    FileProvider = new PhysicalFileProvider(config.PublicRoot),
    RequestPath = "/publics"
});
app.Use(async (context, next) =>
{
    if (context.Request.Path.StartsWithSegments("/obsidian"))
    {
        context.Response.Headers.AccessControlAllowOrigin = "*";
        context.Response.Headers.AccessControlAllowMethods = "GET, POST, PUT, DELETE, OPTIONS";
        context.Response.Headers.AccessControlAllowHeaders = "Content-Type, Authorization";
        if (HttpMethods.IsOptions(context.Request.Method))
        {
            context.Response.StatusCode = StatusCodes.Status200OK;
            return;
        }
    }

    await next();
});
app.Use((context, next) => ConsoleMiddleware.SessionMiddleware(context, app.Services.GetRequiredService<SessionManager>(), next));
app.Use((context, next) => ConsoleMiddleware.CsrfMiddleware(context, next));
app.Use((context, next) =>
{
    var db = context.RequestServices.GetRequiredService<MdbrainDbContext>();
    return ConsoleMiddleware.InitCheckMiddleware(context, db, next);
});
app.Use((context, next) => ConsoleMiddleware.NoIndexMiddleware(context, next));
app.MapInternalEndpoints();
app.MapSyncEndpoints();
app.MapConsoleAuthEndpoints();
app.MapConsoleVaultEndpoints();
app.MapConsoleCommonEndpoints();
app.MapConsoleLogoEndpoints();
app.MapAppEndpoints();

await app.RunAsync();

static string FindProjectRoot(string start)
{
    var current = new DirectoryInfo(start);
    while (current is not null)
    {
        if (File.Exists(Path.Combine(current.FullName, "AGENTS.md"))
            && Directory.Exists(Path.Combine(current.FullName, "server"))
            && Directory.Exists(Path.Combine(current.FullName, "server-dotnet")))
        {
            return current.FullName;
        }

        current = current.Parent;
    }

    return Directory.GetCurrentDirectory();
}

public partial class Program;
