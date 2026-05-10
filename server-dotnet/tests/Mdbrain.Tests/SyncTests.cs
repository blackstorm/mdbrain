using System.Text;
using System.Text.Json;
using Mdbrain.Data;
using Mdbrain.Data.Entities;
using Mdbrain.Features.Sync;
using Mdbrain.Storage;
using Microsoft.AspNetCore.Http;

namespace Mdbrain.Tests;

internal static class SyncTests
{
    private static readonly JsonSerializerOptions JsonOptions = new(JsonSerializerDefaults.Web);

    public static void Register(TestRunner runner)
    {
        runner.Add(nameof(SyncChangesReturnsUpsertsAndDeletes), SyncChangesReturnsUpsertsAndDeletes);
        runner.Add(nameof(SyncNoteAndAssetStoresData), SyncNoteAndAssetStoresData);
        runner.Add(nameof(SyncNoteValidationReturnsContractErrors), SyncNoteValidationReturnsContractErrors);
        runner.Add(nameof(SyncAssetValidationReturnsContractErrors), SyncAssetValidationReturnsContractErrors);
        runner.Add(nameof(SyncNoteAndAssetSkipUnchangedContent), SyncNoteAndAssetSkipUnchangedContent);
        runner.Add(nameof(SyncChangesDeletesServerRowsLinksRefsAndObjects), SyncChangesDeletesServerRowsLinksRefsAndObjects);
        runner.Add(nameof(PublishResultRecordsOkStatus), PublishResultRecordsOkStatus);
        runner.Add(nameof(SyncValidationRecordsPublishError), SyncValidationRecordsPublishError);
    }

    private static async Task SyncChangesReturnsUpsertsAndDeletes()
    {
        await using var fixture = await SyncFixture.CreateAsync();
        var tenantId = fixture.Vault.TenantId;
        fixture.Db.Notes.AddRange(
            new Note { Id = Guid.NewGuid().ToString(), TenantId = tenantId, VaultId = fixture.Vault.Id, Path = "a.md", ClientId = "note-1", Content = "A", Metadata = "{}", Hash = "hash-a" },
            new Note { Id = Guid.NewGuid().ToString(), TenantId = tenantId, VaultId = fixture.Vault.Id, Path = "b.md", ClientId = "note-2", Content = "B", Metadata = "{}", Hash = "hash-b" });
        fixture.Db.Assets.Add(new Asset
        {
            Id = Guid.NewGuid().ToString(),
            TenantId = tenantId,
            VaultId = fixture.Vault.Id,
            ClientId = "asset-1",
            Path = "img/a.png",
            ObjectKey = "assets/a.png",
            SizeBytes = 10,
            ContentType = "image/png",
            Md5 = "md5-a"
        });
        await fixture.Db.SaveChangesAsync();

        var result = await fixture.Service.SyncChangesAsync(fixture.Vault, new SyncChangesRequest(
            [new HashEntry("note-1", "hash-a"), new HashEntry("note-2", "hash-b-new")],
            [new HashEntry("asset-1", "md5-a"), new HashEntry("asset-2", "md5-b")]), CancellationToken.None);

        AssertEx.Equal(StatusCodes.Status200OK, result.Status);
        var json = JsonSerializer.Serialize(result.Body, JsonOptions);
        AssertEx.Contains(@"""id"":""note-2""", json);
        AssertEx.Contains(@"""id"":""asset-2""", json);
    }

    private static async Task SyncNoteAndAssetStoresData()
    {
        await using var fixture = await SyncFixture.CreateAsync();
        var tenantId = fixture.Vault.TenantId;
        fixture.Db.Assets.Add(new Asset
        {
            Id = Guid.NewGuid().ToString(),
            TenantId = tenantId,
            VaultId = fixture.Vault.Id,
            ClientId = "asset-1",
            Path = "img/a.png",
            ObjectKey = "assets/a.png",
            SizeBytes = 10,
            ContentType = "image/png",
            Md5 = "md5-a"
        });
        fixture.Db.Notes.Add(new Note
        {
            Id = Guid.NewGuid().ToString(),
            TenantId = tenantId,
            VaultId = fixture.Vault.Id,
            Path = "Note B.md",
            ClientId = "note-b",
            Content = "B",
            Metadata = "{}",
            Hash = "hash-b"
        });
        await fixture.Db.SaveChangesAsync();

        var noteResult = await fixture.Service.SyncNoteAsync(fixture.Vault, "note-a", new SyncNoteRequest(
            "Note A.md",
            "Links [[Note B]]",
            "hash-a",
            null,
            [new HashEntry("asset-1", "md5-a")],
            [new HashEntry("note-b", "hash-b")]), CancellationToken.None);
        AssertEx.Equal(StatusCodes.Status200OK, noteResult.Status);
        AssertEx.True(fixture.Db.NoteAssetRefs.Any(item => item.NoteClientId == "note-a" && item.AssetClientId == "asset-1"), "expected asset ref");
        AssertEx.True(fixture.Db.NoteLinks.Any(item => item.SourceClientId == "note-a" && item.TargetClientId == "note-b"), "expected note link");

        var content = Convert.ToBase64String(Encoding.UTF8.GetBytes("pngdata"));
        var assetResult = await fixture.Service.SyncAssetAsync(fixture.Vault, "asset-c", new SyncAssetRequest(
            "img/c.png",
            "image/png",
            null,
            "md5-c",
            content), CancellationToken.None);
        AssertEx.Equal(StatusCodes.Status200OK, assetResult.Status);
        AssertEx.True(fixture.Db.Assets.Any(item => item.ClientId == "asset-c" && item.Md5 == "md5-c"), "expected asset row");
    }

    private static async Task SyncValidationRecordsPublishError()
    {
        await using var fixture = await SyncFixture.CreateAsync();

        var result = await fixture.Service.SyncNoteAsync(fixture.Vault, "note-a", new SyncNoteRequest(
            "Note A.md",
            null,
            "hash-a",
            null,
            [],
            []), CancellationToken.None);
        await fixture.Service.RecordPublishResultAsync(fixture.Vault, result, CancellationToken.None);

        var vault = await fixture.Db.Vaults.FindAsync(fixture.Vault.Id);
        AssertEx.Equal("error", vault!.LastPublishStatus);
        AssertEx.Equal("bad_request", vault.LastPublishErrorCode);
        AssertEx.Equal("Missing note content", vault.LastPublishErrorMessage);
    }

    private static async Task SyncNoteValidationReturnsContractErrors()
    {
        await using var fixture = await SyncFixture.CreateAsync();

        var missingPath = await fixture.Service.SyncNoteAsync(fixture.Vault, "note-a", new SyncNoteRequest(
            "",
            "content",
            "hash-a",
            null,
            [],
            []), CancellationToken.None);
        AssertBadRequest(missingPath, "Missing note path");

        var missingHash = await fixture.Service.SyncNoteAsync(fixture.Vault, "note-a", new SyncNoteRequest(
            "Note A.md",
            "content",
            "",
            null,
            [],
            []), CancellationToken.None);
        AssertBadRequest(missingHash, "Missing note hash");

        var missingAssets = await fixture.Service.SyncNoteAsync(fixture.Vault, "note-a", new SyncNoteRequest(
            "Note A.md",
            "content",
            "hash-a",
            null,
            null,
            []), CancellationToken.None);
        AssertBadRequest(missingAssets, "Missing assets");

        var missingLinkedNotes = await fixture.Service.SyncNoteAsync(fixture.Vault, "note-a", new SyncNoteRequest(
            "Note A.md",
            "content",
            "hash-a",
            null,
            [],
            null), CancellationToken.None);
        AssertBadRequest(missingLinkedNotes, "Missing linked notes");
    }

    private static async Task SyncAssetValidationReturnsContractErrors()
    {
        await using var fixture = await SyncFixture.CreateAsync();

        var missingPath = await fixture.Service.SyncAssetAsync(fixture.Vault, "asset-a", new SyncAssetRequest(
            "",
            "image/png",
            null,
            "md5-a",
            "cG5n"), CancellationToken.None);
        AssertBadRequest(missingPath, "Missing asset path");

        var missingContentType = await fixture.Service.SyncAssetAsync(fixture.Vault, "asset-a", new SyncAssetRequest(
            "img/a.png",
            "",
            null,
            "md5-a",
            "cG5n"), CancellationToken.None);
        AssertBadRequest(missingContentType, "Missing asset contentType");

        var invalidBase64 = await fixture.Service.SyncAssetAsync(fixture.Vault, "asset-a", new SyncAssetRequest(
            "img/a.png",
            "image/png",
            null,
            "md5-a",
            "not-base64"), CancellationToken.None);
        AssertBadRequest(invalidBase64, "Missing asset content");
    }

    private static async Task SyncNoteAndAssetSkipUnchangedContent()
    {
        await using var fixture = await SyncFixture.CreateAsync();
        var tenantId = fixture.Vault.TenantId;
        fixture.Db.Notes.Add(new Note
        {
            Id = Guid.NewGuid().ToString(),
            TenantId = tenantId,
            VaultId = fixture.Vault.Id,
            Path = "Note A.md",
            ClientId = "note-a",
            Content = "existing",
            Metadata = "{}",
            Hash = "hash-a"
        });
        fixture.Db.Assets.Add(new Asset
        {
            Id = Guid.NewGuid().ToString(),
            TenantId = tenantId,
            VaultId = fixture.Vault.Id,
            ClientId = "asset-a",
            Path = "img/a.png",
            ObjectKey = "assets/asset-a.png",
            SizeBytes = 10,
            ContentType = "image/png",
            Md5 = "md5-a"
        });
        await fixture.Db.SaveChangesAsync();

        var noteResult = await fixture.Service.SyncNoteAsync(fixture.Vault, "note-a", new SyncNoteRequest(
            "Note A.md",
            "new content should be ignored",
            "hash-a",
            null,
            [],
            []), CancellationToken.None);
        AssertEx.Equal(StatusCodes.Status200OK, noteResult.Status);
        AssertJsonContains(noteResult, @"""status"":""skipped""");
        AssertJsonContains(noteResult, @"""noteId"":""note-a""");

        var assetResult = await fixture.Service.SyncAssetAsync(fixture.Vault, "asset-a", new SyncAssetRequest(
            "img/a.png",
            "image/png",
            10,
            "md5-a",
            null), CancellationToken.None);
        AssertEx.Equal(StatusCodes.Status200OK, assetResult.Status);
        AssertJsonContains(assetResult, @"""status"":""skipped""");
        AssertJsonContains(assetResult, @"""assetId"":""asset-a""");
    }

    private static async Task SyncChangesDeletesServerRowsLinksRefsAndObjects()
    {
        await using var fixture = await SyncFixture.CreateAsync();
        var tenantId = fixture.Vault.TenantId;
        fixture.Db.Notes.AddRange(
            new Note { Id = Guid.NewGuid().ToString(), TenantId = tenantId, VaultId = fixture.Vault.Id, Path = "A.md", ClientId = "note-a", Content = "A", Metadata = "{}", Hash = "hash-a" },
            new Note { Id = Guid.NewGuid().ToString(), TenantId = tenantId, VaultId = fixture.Vault.Id, Path = "B.md", ClientId = "note-b", Content = "B", Metadata = "{}", Hash = "hash-b" });
        fixture.Db.NoteLinks.AddRange(
            new NoteLink { Id = Guid.NewGuid().ToString(), VaultId = fixture.Vault.Id, SourceClientId = "note-a", TargetClientId = "note-b", LinkType = "link" },
            new NoteLink { Id = Guid.NewGuid().ToString(), VaultId = fixture.Vault.Id, SourceClientId = "note-b", TargetClientId = "note-a", LinkType = "link" });
        fixture.Db.Assets.Add(new Asset
        {
            Id = Guid.NewGuid().ToString(),
            TenantId = tenantId,
            VaultId = fixture.Vault.Id,
            ClientId = "asset-a",
            Path = "img/a.png",
            ObjectKey = "assets/asset-a.png",
            SizeBytes = 3,
            ContentType = "image/png",
            Md5 = "md5-a"
        });
        fixture.Db.NoteAssetRefs.Add(new NoteAssetRef
        {
            Id = Guid.NewGuid().ToString(),
            VaultId = fixture.Vault.Id,
            NoteClientId = "note-a",
            AssetClientId = "asset-a"
        });
        await fixture.Db.SaveChangesAsync();
        await fixture.Store.PutObjectAsync(fixture.Vault.Id, "assets/asset-a.png", "png"u8.ToArray(), "image/png", CancellationToken.None);

        var result = await fixture.Service.SyncChangesAsync(fixture.Vault, new SyncChangesRequest(
            [new HashEntry("note-a", "hash-a")],
            []), CancellationToken.None);

        AssertEx.Equal(StatusCodes.Status200OK, result.Status);
        AssertEx.True(!fixture.Db.Notes.Any(item => item.ClientId == "note-b"), "expected missing client note to be deleted");
        AssertEx.True(!fixture.Db.NoteLinks.Any(item => item.TargetClientId == "note-b" || item.SourceClientId == "note-b"), "expected deleted note links removed");
        AssertEx.True(!fixture.Db.Assets.Any(item => item.ClientId == "asset-a"), "expected missing client asset to be deleted");
        AssertEx.True(!fixture.Db.NoteAssetRefs.Any(item => item.AssetClientId == "asset-a"), "expected asset refs removed");
        AssertEx.True(await fixture.Store.HeadObjectAsync(fixture.Vault.Id, "assets/asset-a.png", CancellationToken.None) is null, "expected asset object deleted");
    }

    private static async Task PublishResultRecordsOkStatus()
    {
        await using var fixture = await SyncFixture.CreateAsync();

        await fixture.Service.RecordPublishResultAsync(fixture.Vault, new SyncResult(StatusCodes.Status200OK, new { success = true }), CancellationToken.None);

        var vault = await fixture.Db.Vaults.FindAsync(fixture.Vault.Id);
        AssertEx.Equal("ok", vault!.LastPublishStatus);
        AssertEx.Equal(null, vault.LastPublishErrorCode);
        AssertEx.Equal(null, vault.LastPublishErrorMessage);
        AssertEx.True(vault.LastPublishAt is not null, "expected publish timestamp");
    }

    private static void AssertBadRequest(SyncResult result, string error)
    {
        AssertEx.Equal(StatusCodes.Status400BadRequest, result.Status);
        AssertJsonContains(result, @"""success"":false");
        AssertJsonContains(result, $@"""error"":""{error}""");
    }

    private static void AssertJsonContains(SyncResult result, string expected)
    {
        AssertEx.Contains(expected, JsonSerializer.Serialize(result.Body, JsonOptions));
    }

    private sealed class SyncFixture : IAsyncDisposable
    {
        private readonly TestSqliteDatabase database;

        private SyncFixture(TestSqliteDatabase database, MdbrainDbContext db, SyncService service, LocalObjectStore store, Vault vault)
        {
            this.database = database;
            Db = db;
            Service = service;
            Store = store;
            Vault = vault;
        }

        public MdbrainDbContext Db { get; }
        public SyncService Service { get; }
        public LocalObjectStore Store { get; }
        public Vault Vault { get; }

        public static async Task<SyncFixture> CreateAsync()
        {
            var database = await TestSqliteDatabase.CreateAsync();
            var db = database.CreateContext();
            var store = LocalObjectStore.Create(ConfigTests.TestPath($"sync-store-{Guid.NewGuid():N}"));
            var tenant = new Tenant { Id = Guid.NewGuid().ToString(), Name = "Test Org" };
            var vault = new Vault
            {
                Id = Guid.NewGuid().ToString(),
                TenantId = tenant.Id,
                Name = "Blog",
                Domain = "sync.example.com",
                SyncKey = "sync-key-1"
            };
            db.Tenants.Add(tenant);
            db.Vaults.Add(vault);
            await db.SaveChangesAsync();
            return new SyncFixture(database, db, new SyncService(db, store), store, vault);
        }

        public async ValueTask DisposeAsync()
        {
            await Db.DisposeAsync();
            await database.DisposeAsync();
        }
    }
}
