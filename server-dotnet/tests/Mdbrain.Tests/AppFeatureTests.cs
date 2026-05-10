using Mdbrain.Data;
using Mdbrain.Data.Entities;
using Mdbrain.Features.App;
using Mdbrain.Rendering;
using Mdbrain.Storage;

namespace Mdbrain.Tests;

internal static class AppFeatureTests
{
    public static void Register(TestRunner runner)
    {
        runner.Add(nameof(ParseHostDomainMatchesGoRules), ParseHostDomainMatchesGoRules);
        runner.Add(nameof(MarkdownRendersObsidianLinksAssetsAndMath), MarkdownRendersObsidianLinksAssetsAndMath);
        runner.Add(nameof(BuildPushUrlKeepsStackSemantics), BuildPushUrlKeepsStackSemantics);
        runner.Add(nameof(TemplateRendererSupportsConsoleSyntax), TemplateRendererSupportsConsoleSyntax);
    }

    private static Task ParseHostDomainMatchesGoRules()
    {
        AssertEx.Equal(new ParsedHost("notes.example.com", string.Empty), HostParser.ParseHostDomain("notes.example.com"));
        AssertEx.Equal(new ParsedHost("notes.example.com", string.Empty), HostParser.ParseHostDomain("notes.example.com:8080"));
        AssertEx.Equal(new ParsedHost("::1", string.Empty), HostParser.ParseHostDomain("[::1]:8080"));
        AssertEx.Equal("missing", HostParser.ParseHostDomain("").Error);
        AssertEx.Equal("invalid", HostParser.ParseHostDomain("bad host").Error);
        AssertEx.Equal("invalid", HostParser.ParseHostDomain("host:bad").Error);
        AssertEx.Equal("invalid", HostParser.ParseHostDomain("host:70000").Error);
        return Task.CompletedTask;
    }

    private static async Task MarkdownRendersObsidianLinksAssetsAndMath()
    {
        await using var fixture = await AppFixture.CreateAsync();
        fixture.Db.Assets.Add(new Asset
        {
            Id = Guid.NewGuid().ToString(),
            TenantId = fixture.Tenant.Id,
            VaultId = fixture.Vault.Id,
            ClientId = "asset-1",
            Path = "assets/logo.png",
            ObjectKey = "assets/asset-1.png",
            SizeBytes = 10,
            ContentType = "image/png",
            Md5 = "md5-a"
        });
        await fixture.Db.SaveChangesAsync();
        await fixture.Store.PutObjectAsync(fixture.Vault.Id, "assets/asset-1.png", "png"u8.ToArray(), "image/png", CancellationToken.None);

        var renderer = new MarkdownRenderer();
        var html = await renderer.RenderMarkdownAsync(
            fixture.Db,
            fixture.Store,
            fixture.Vault.Id,
            """
            ---
            title: hidden
            ---
            # Title
            See [[Target Note|Target]] and ![[assets/logo.png]].
            Inline $x+y$.
            """,
            [new StoredLink("[[Target Note|Target]]", "note-target", "Target Note.md", "Target", "link")],
            CancellationToken.None);

        AssertEx.Contains("""<a href="/note-target" class="internal-link" data-note-id="note-target">Target</a>""", html);
        AssertEx.Contains("""<img src="/storage/assets/asset-1.png" alt="assets/logo.png" class="asset-embed">""", html);
        AssertEx.Contains("""<span class="math-inline">x+y</span>""", html);
        AssertEx.True(!html.Contains("title: hidden", StringComparison.Ordinal), "front matter should be stripped");
    }

    private static Task BuildPushUrlKeepsStackSemantics()
    {
        AssertEx.Equal("/root+target", AppEndpoints.BuildPushUrl("/", "", "target", "root"));
        AssertEx.Equal("/a+b+target", AppEndpoints.BuildPushUrl("/a+b+c?x=1", "b", "target", "root"));
        AssertEx.Equal("/a+b+c+target", AppEndpoints.BuildPushUrl("https://notes.example.com/a+b+c", "", "target", "root"));
        return Task.CompletedTask;
    }

    private static async Task TemplateRendererSupportsConsoleSyntax()
    {
        await using var fixture = await AppFixture.CreateAsync();
        var renderer = new TemplateRenderer(new Config.AppConfig
        {
            ProjectRoot = ConfigTests.RepoRoot(),
            DataPath = ConfigTests.TestPath("template-renderer"),
            EnvironmentName = "test",
            AppHost = "127.0.0.1",
            AppPort = 8080,
            ConsoleHost = "127.0.0.1",
            ConsolePort = 9090,
            StorageType = "local",
            LocalStoragePath = ConfigTests.TestPath("template-renderer-storage"),
            S3Region = "us-east-1",
            S3Bucket = "mdbrain",
            SessionSecret = "secret",
            HealthToken = "token",
            OnDemandTlsEnabled = false
        });

        var html = await renderer.RenderAsync("templates/console/root-note-selector.html", new Dictionary<string, object?>
        {
            ["vault_id"] = fixture.Vault.Id,
            ["selected_note_path"] = "A Note.md",
            ["notes"] = new[]
            {
                new Dictionary<string, object?>
                {
                    ["vault_id"] = fixture.Vault.Id,
                    ["client_id"] = "note-a",
                    ["path"] = "A Note.md",
                    ["selected"] = true
                }
            }
        }, CancellationToken.None);

        AssertEx.Contains("data-path=\"a note.md\"", html);
        AssertEx.Contains("data-lucide=\"chevron-down\"", html);
        AssertEx.Contains("selected", html);
    }

    private sealed class AppFixture : IAsyncDisposable
    {
        private readonly TestSqliteDatabase database;

        private AppFixture(TestSqliteDatabase database, MdbrainDbContext db, LocalObjectStore store, Tenant tenant, Vault vault)
        {
            this.database = database;
            Db = db;
            Store = store;
            Tenant = tenant;
            Vault = vault;
        }

        public MdbrainDbContext Db { get; }
        public LocalObjectStore Store { get; }
        public Tenant Tenant { get; }
        public Vault Vault { get; }

        public static async Task<AppFixture> CreateAsync()
        {
            var database = await TestSqliteDatabase.CreateAsync();
            var db = database.CreateContext();
            var tenant = new Tenant { Id = Guid.NewGuid().ToString(), Name = "Test Org" };
            var vault = new Vault
            {
                Id = Guid.NewGuid().ToString(),
                TenantId = tenant.Id,
                Name = "Blog",
                Domain = "notes.example.com",
                SyncKey = "sync-key-app"
            };
            db.Tenants.Add(tenant);
            db.Vaults.Add(vault);
            await db.SaveChangesAsync();

            return new AppFixture(
                database,
                db,
                LocalObjectStore.Create(ConfigTests.TestPath($"app-store-{Guid.NewGuid():N}")),
                tenant,
                vault);
        }

        public async ValueTask DisposeAsync()
        {
            await Db.DisposeAsync();
            await database.DisposeAsync();
        }
    }
}
