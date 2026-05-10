using Mdbrain.Data;
using Mdbrain.Data.Entities;
using Mdbrain.Features.App;
using Mdbrain.Storage;

namespace Mdbrain.Tests;

internal static class MarkdownTests
{
    public static void Register(TestRunner runner)
    {
        runner.Add(nameof(ExtractTitleAndDescriptionIgnoreFrontMatterAndImages), ExtractTitleAndDescriptionIgnoreFrontMatterAndImages);
        runner.Add(nameof(BrokenInternalLinksRenderAsBrokenSpans), BrokenInternalLinksRenderAsBrokenSpans);
        runner.Add(nameof(CodeBlocksAreNotRewritten), CodeBlocksAreNotRewritten);
        runner.Add(nameof(ReferenceImagesAndHtmlMediaSourcesAreRewritten), ReferenceImagesAndHtmlMediaSourcesAreRewritten);
        runner.Add(nameof(AssetEmbedsRenderByMediaType), AssetEmbedsRenderByMediaType);
    }

    private static Task ExtractTitleAndDescriptionIgnoreFrontMatterAndImages()
    {
        var renderer = new MarkdownRenderer();
        var content = """
            ---
            title: Hidden
            ---
            # Visible Title
            ![cover](cover.png)

            First useful sentence for search results.
            """;

        AssertEx.Equal("Visible Title", renderer.ExtractTitle(content));
        AssertEx.Equal("First useful sentence for search results.", renderer.ExtractDescription(content, 160));
        AssertEx.Equal("abcd", renderer.ExtractDescription("abcdef", 4));
        return Task.CompletedTask;
    }

    private static async Task BrokenInternalLinksRenderAsBrokenSpans()
    {
        await using var fixture = await MarkdownFixture.CreateAsync();
        var renderer = new MarkdownRenderer();

        var html = await renderer.RenderMarkdownAsync(
            fixture.Db,
            fixture.Store,
            fixture.Vault.Id,
            "See [[Missing Note|missing]].",
            [],
            CancellationToken.None);

        AssertEx.Contains("""<span class="internal-link broken" title="Note not found: Missing Note">missing</span>""", html);
    }

    private static async Task CodeBlocksAreNotRewritten()
    {
        await using var fixture = await MarkdownFixture.CreateAsync();
        var renderer = new MarkdownRenderer();

        var html = await renderer.RenderMarkdownAsync(
            fixture.Db,
            fixture.Store,
            fixture.Vault.Id,
            """
            `[[Inline Code]]`

            ```md
            ![[assets/logo.png]]
            [[Code Block]]
            ```
            """,
            [],
            CancellationToken.None);

        AssertEx.Contains("[[Inline Code]]", html);
        AssertEx.Contains("![[assets/logo.png]]", html);
        AssertEx.Contains("[[Code Block]]", html);
        AssertEx.DoesNotContain("internal-link broken", html);
        AssertEx.DoesNotContain("asset-embed", html);
    }

    private static async Task ReferenceImagesAndHtmlMediaSourcesAreRewritten()
    {
        await using var fixture = await MarkdownFixture.CreateAsync();
        await fixture.AddAssetAsync("asset-logo", "assets/logo.png", "assets/asset-logo.png", "image/png");
        var renderer = new MarkdownRenderer();

        var html = await renderer.RenderMarkdownAsync(
            fixture.Db,
            fixture.Store,
            fixture.Vault.Id,
            """
            ![Logo][logo]

            [logo]: assets/logo.png

            <img src="assets/logo.png">
            """,
            [],
            CancellationToken.None);

        AssertEx.Contains("src=\"/storage/assets/asset-logo.png\"", html);
        AssertEx.Contains("""<img src="/storage/assets/asset-logo.png">""", html);
    }

    private static async Task AssetEmbedsRenderByMediaType()
    {
        await using var fixture = await MarkdownFixture.CreateAsync();
        var renderer = new MarkdownRenderer();

        var html = await renderer.RenderMarkdownAsync(
            fixture.Db,
            fixture.Store,
            fixture.Vault.Id,
            """
            ![[docs/file.pdf]]
            ![[audio/theme.mp3]]
            ![[video/demo.mp4]]
            [[docs/file.pdf|PDF]]
            """,
            [],
            CancellationToken.None);

        AssertEx.Contains("""<a href="/storage/docs/file.pdf" class="asset-link pdf-link">docs/file.pdf</a>""", html);
        AssertEx.Contains("""<audio src="/storage/audio/theme.mp3" controls class="asset-embed">audio/theme.mp3</audio>""", html);
        AssertEx.Contains("""<video src="/storage/video/demo.mp4" controls class="asset-embed">video/demo.mp4</video>""", html);
        AssertEx.Contains("""<a href="/storage/docs/file.pdf" class="asset-link">PDF</a>""", html);
    }

    private sealed class MarkdownFixture : IAsyncDisposable
    {
        private readonly TestSqliteDatabase database;

        private MarkdownFixture(TestSqliteDatabase database, MdbrainDbContext db, LocalObjectStore store, Vault vault)
        {
            this.database = database;
            Db = db;
            Store = store;
            Vault = vault;
        }

        public MdbrainDbContext Db { get; }
        public LocalObjectStore Store { get; }
        public Vault Vault { get; }

        public static async Task<MarkdownFixture> CreateAsync()
        {
            var database = await TestSqliteDatabase.CreateAsync();
            var db = database.CreateContext();
            var tenant = new Tenant { Id = Guid.NewGuid().ToString(), Name = "Test Org" };
            var vault = new Vault
            {
                Id = Guid.NewGuid().ToString(),
                TenantId = tenant.Id,
                Name = "Docs",
                Domain = "docs.example.com",
                SyncKey = "sync-key-md"
            };
            db.Tenants.Add(tenant);
            db.Vaults.Add(vault);
            await db.SaveChangesAsync();
            return new MarkdownFixture(
                database,
                db,
                LocalObjectStore.Create(ConfigTests.TestPath($"markdown-store-{Guid.NewGuid():N}")),
                vault);
        }

        public async Task AddAssetAsync(string clientId, string path, string objectKey, string contentType)
        {
            Db.Assets.Add(new Asset
            {
                Id = Guid.NewGuid().ToString(),
                TenantId = Vault.TenantId,
                VaultId = Vault.Id,
                ClientId = clientId,
                Path = path,
                ObjectKey = objectKey,
                SizeBytes = 3,
                ContentType = contentType,
                Md5 = clientId + "-md5"
            });
            await Db.SaveChangesAsync();
            await Store.PutObjectAsync(Vault.Id, objectKey, "obj"u8.ToArray(), contentType, CancellationToken.None);
        }

        public async ValueTask DisposeAsync()
        {
            await Db.DisposeAsync();
            await database.DisposeAsync();
        }
    }
}
