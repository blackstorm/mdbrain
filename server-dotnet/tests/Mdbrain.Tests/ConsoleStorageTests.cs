using System.Text.Json;
using Mdbrain.Data;
using Mdbrain.Data.Entities;
using Mdbrain.Features.Console;
using Mdbrain.Security;
using Mdbrain.Storage;
using Microsoft.AspNetCore.Http;
using Microsoft.Extensions.DependencyInjection;

namespace Mdbrain.Tests;

internal static class ConsoleStorageTests
{
    public static void Register(TestRunner runner)
    {
        runner.Add(nameof(ConsoleAssetUrlMatchesExistingContract), ConsoleAssetUrlMatchesExistingContract);
        runner.Add(nameof(LogoServiceRejectsMissingWrongTenantAndInvalidFiles), LogoServiceRejectsMissingWrongTenantAndInvalidFiles);
        runner.Add(nameof(UploadServeAndDeleteLogo), UploadServeAndDeleteLogo);
        runner.Add(nameof(DeleteLogoWithoutExistingLogoIsSuccessful), DeleteLogoWithoutExistingLogoIsSuccessful);
        runner.Add(nameof(ReadFaviconFallsBackToLogoWhenGeneratedFaviconMissing), ReadFaviconFallsBackToLogoWhenGeneratedFaviconMissing);
    }

    private static Task ConsoleAssetUrlMatchesExistingContract()
    {
        AssertEx.Equal("/console/storage/vault-123/site/logo/abc.png", ConsoleCommonEndpoints.ConsoleAssetUrl("vault-123", "site/logo/abc.png"));
        return Task.CompletedTask;
    }

    private static async Task UploadServeAndDeleteLogo()
    {
        await using var fixture = await ConsoleFixture.CreateAsync();
        var upload = new DefaultHttpContext { RequestServices = fixture.Services };
        ConsoleMiddleware.SetSessionItems(upload, new ConsoleSession { UserId = "user-1", TenantId = fixture.Tenant.Id, CsrfToken = "csrf" });
        var png = File.ReadAllBytes(Path.Combine(ConfigTests.RepoRoot(), "server", "resources", "publics", "console", "images", "logo.png"));
        var formFile = new FormFile(new MemoryStream(png), 0, png.Length, "logo", "logo.png")
        {
            Headers = new HeaderDictionary(),
            ContentType = "image/png"
        };
        var service = new ConsoleLogoService(fixture.Db, fixture.Store);
        var uploadResult = await ExecuteResultAsync(upload, ToHttpResult(await service.UploadLogoAsync(
            fixture.Vault.Id,
            fixture.Tenant.Id,
            formFile,
            CancellationToken.None), upload));
        AssertEx.Equal(StatusCodes.Status200OK, uploadResult.StatusCode);
        AssertEx.Contains("\"logo-url\"", uploadResult.Body);

        var vault = await fixture.Db.Vaults.FindAsync(fixture.Vault.Id);
        AssertEx.True(!string.IsNullOrWhiteSpace(vault!.LogoObjectKey), "expected logo key");

        var logo = await fixture.Store.GetObjectAsync(fixture.Vault.Id, vault.LogoObjectKey!, CancellationToken.None);
        AssertEx.True(logo is not null, "expected stored logo");
        if (logo is not null)
        {
            await logo.DisposeAsync();
        }

        var favicon = await fixture.Store.GetObjectAsync(fixture.Vault.Id, Mdbrain.Domain.ObjectKeys.FaviconObjectKey(vault.LogoObjectKey!), CancellationToken.None);
        AssertEx.True(favicon is not null, "expected stored favicon");
        if (favicon is not null)
        {
            await favicon.DisposeAsync();
        }

        var delete = new DefaultHttpContext { RequestServices = fixture.Services };
        ConsoleMiddleware.SetSessionItems(delete, new ConsoleSession { UserId = "user-1", TenantId = fixture.Tenant.Id, CsrfToken = "csrf" });
        var deleteResult = await ExecuteResultAsync(delete, ToHttpResult(await service.DeleteLogoAsync(
            fixture.Vault.Id,
            fixture.Tenant.Id,
            CancellationToken.None), delete));
        AssertEx.Equal(StatusCodes.Status200OK, deleteResult.StatusCode);

        await fixture.Db.Entry(vault).ReloadAsync();
        AssertEx.True(vault.LogoObjectKey is null, "expected logo key cleared");
    }

    private static async Task LogoServiceRejectsMissingWrongTenantAndInvalidFiles()
    {
        await using var fixture = await ConsoleFixture.CreateAsync();
        var service = new ConsoleLogoService(fixture.Db, fixture.Store);

        var missingVault = await service.UploadLogoAsync("missing", fixture.Tenant.Id, null, CancellationToken.None);
        AssertLogoJson(missingVault, StatusCodes.Status404NotFound, "Vault not found");

        var wrongTenant = await service.UploadLogoAsync(fixture.Vault.Id, "other-tenant", null, CancellationToken.None);
        AssertLogoJson(wrongTenant, StatusCodes.Status403Forbidden, "Permission denied");

        var noFile = await service.UploadLogoAsync(fixture.Vault.Id, fixture.Tenant.Id, null, CancellationToken.None);
        AssertLogoJson(noFile, StatusCodes.Status400BadRequest, "No file uploaded");

        var empty = await service.UploadLogoAsync(fixture.Vault.Id, fixture.Tenant.Id, FormFile([], "empty.png", "image/png"), CancellationToken.None);
        AssertLogoJson(empty, StatusCodes.Status400BadRequest, "File is empty");

        var text = await service.UploadLogoAsync(fixture.Vault.Id, fixture.Tenant.Id, FormFile("hello"u8.ToArray(), "logo.txt", "text/plain"), CancellationToken.None);
        AssertLogoJson(text, StatusCodes.Status400BadRequest, "Invalid file type");
    }

    private static async Task DeleteLogoWithoutExistingLogoIsSuccessful()
    {
        await using var fixture = await ConsoleFixture.CreateAsync();
        var service = new ConsoleLogoService(fixture.Db, fixture.Store);

        var result = await service.DeleteLogoAsync(fixture.Vault.Id, fixture.Tenant.Id, CancellationToken.None);

        AssertEx.Equal(StatusCodes.Status200OK, result.StatusCode);
        AssertEx.Contains("No logo to delete", JsonSerializer.Serialize(result.Body));
    }

    private static async Task ReadFaviconFallsBackToLogoWhenGeneratedFaviconMissing()
    {
        await using var fixture = await ConsoleFixture.CreateAsync();
        var service = new ConsoleLogoService(fixture.Db, fixture.Store);
        var png = File.ReadAllBytes(Path.Combine(ConfigTests.RepoRoot(), "server", "resources", "publics", "console", "images", "logo.png"));
        var upload = await service.UploadLogoAsync(fixture.Vault.Id, fixture.Tenant.Id, FormFile(png, "logo.png", "image/png"), CancellationToken.None);
        AssertEx.Equal(StatusCodes.Status200OK, upload.StatusCode);

        var vault = await fixture.Db.Vaults.FindAsync(fixture.Vault.Id);
        var faviconKey = Mdbrain.Domain.ObjectKeys.FaviconObjectKey(vault!.LogoObjectKey!);
        await fixture.Store.DeleteObjectAsync(vault.Id, faviconKey, CancellationToken.None);

        var result = await service.ReadLogoAsync(fixture.Vault.Id, fixture.Tenant.Id, favicon: true, CancellationToken.None);

        AssertEx.Equal(StatusCodes.Status200OK, result.StatusCode);
        AssertEx.True(result.Bytes is { Length: > 0 }, "expected logo bytes fallback");
        AssertEx.Equal("image/png", result.ContentType);
    }

    private static async Task<(int StatusCode, string Body)> ExecuteResultAsync(DefaultHttpContext context, IResult result)
    {
        await using var body = new MemoryStream();
        context.Response.Body = body;
        await result.ExecuteAsync(context);
        body.Position = 0;
        using var reader = new StreamReader(body);
        return (context.Response.StatusCode, await reader.ReadToEndAsync());
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

    private static FormFile FormFile(byte[] bytes, string fileName, string contentType)
    {
        return new FormFile(new MemoryStream(bytes), 0, bytes.Length, "logo", fileName)
        {
            Headers = new HeaderDictionary(),
            ContentType = contentType
        };
    }

    private static void AssertLogoJson(LogoServiceResult result, int statusCode, string error)
    {
        AssertEx.Equal(statusCode, result.StatusCode);
        var json = JsonSerializer.Serialize(result.Body);
        AssertEx.Contains(@"""success"":false", json);
        AssertEx.Contains(error, json);
    }

    private sealed class ConsoleFixture : IAsyncDisposable
    {
        private readonly TestSqliteDatabase database;

        private ConsoleFixture(TestSqliteDatabase database, MdbrainDbContext db, LocalObjectStore store, ServiceProvider services, Tenant tenant, Vault vault)
        {
            this.database = database;
            Db = db;
            Store = store;
            Services = services;
            Tenant = tenant;
            Vault = vault;
        }

        public MdbrainDbContext Db { get; }
        public LocalObjectStore Store { get; }
        public ServiceProvider Services { get; }
        public Tenant Tenant { get; }
        public Vault Vault { get; }

        public static async Task<ConsoleFixture> CreateAsync()
        {
            var database = await TestSqliteDatabase.CreateAsync();
            var db = database.CreateContext();
            var store = LocalObjectStore.Create(ConfigTests.TestPath($"console-store-{Guid.NewGuid():N}"));
            var tenant = new Tenant { Id = Guid.NewGuid().ToString(), Name = "Acme" };
            var vault = new Vault
            {
                Id = Guid.NewGuid().ToString(),
                TenantId = tenant.Id,
                Name = "Blog",
                Domain = "blog.example.com",
                SyncKey = "sync-key"
            };
            db.Tenants.Add(tenant);
            db.Vaults.Add(vault);
            await db.SaveChangesAsync();
            var services = new ServiceCollection()
                .AddLogging()
                .AddSingleton(db)
                .AddSingleton<IObjectStore>(store)
                .BuildServiceProvider();
            return new ConsoleFixture(database, db, store, services, tenant, vault);
        }

        public async ValueTask DisposeAsync()
        {
            await Services.DisposeAsync();
            await Db.DisposeAsync();
            await database.DisposeAsync();
        }
    }

}
