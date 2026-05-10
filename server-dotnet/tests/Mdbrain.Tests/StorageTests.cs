using System.Text;
using Mdbrain.Domain;
using Mdbrain.Storage;

namespace Mdbrain.Tests;

internal static class StorageTests
{
    public static void Register(TestRunner runner)
    {
        runner.Add(nameof(ObjectKeysNormalizeTraversalAndGenerateStableKeys), ObjectKeysNormalizeTraversalAndGenerateStableKeys);
        runner.Add(nameof(LocalObjectStoreScopesObjectsAndGuessesContentType), LocalObjectStoreScopesObjectsAndGuessesContentType);
        runner.Add(nameof(LocalObjectStoreDeleteMissingIsNoopAndDeletesVaultPrefix), LocalObjectStoreDeleteMissingIsNoopAndDeletesVaultPrefix);
        runner.Add(nameof(S3EndpointParsingAndPublicUrlsMatchContract), S3EndpointParsingAndPublicUrlsMatchContract);
    }

    private static Task ObjectKeysNormalizeTraversalAndGenerateStableKeys()
    {
        AssertEx.Equal("etc/passwd", ObjectKeys.NormalizePath("../../etc/passwd"));
        AssertEx.Equal("images/logo.png", ObjectKeys.NormalizePath("\\images\\logo.png"));
        AssertEx.Equal(string.Empty, ObjectKeys.NormalizePath("."));
        AssertEx.Equal("vault123/", ObjectKeys.VaultPrefix("vault-123"));
        AssertEx.Equal("assets/asset-1.png", ObjectKeys.AssetObjectKey("asset-1", "PNG"));
        AssertEx.Equal("assets/asset-1", ObjectKeys.AssetObjectKey("asset-1", ""));
        AssertEx.Equal("site/logo/abc123.png", ObjectKeys.LogoObjectKey("abc123", "png"));
        AssertEx.Equal("site/logo/abc123.favicon.png", ObjectKeys.FaviconObjectKey("site/logo/abc123.png"));
        return Task.CompletedTask;
    }

    private static async Task LocalObjectStoreScopesObjectsAndGuessesContentType()
    {
        var store = LocalObjectStore.Create(ConfigTests.TestPath("local-store-scope"));
        const string vaultId = "vault-123";

        await store.PutObjectAsync(vaultId, "assets/image.png", "png"u8.ToArray(), "ignored/header", CancellationToken.None);
        await using var stored = await store.GetObjectAsync(vaultId, "assets/image.png", CancellationToken.None);

        AssertEx.True(stored is not null, "expected stored object");
        AssertEx.Equal("image/png", stored!.ContentType);
        using var reader = new StreamReader(stored.Body, Encoding.UTF8);
        AssertEx.Equal("png", await reader.ReadToEndAsync());
        AssertEx.Equal("/storage/assets/image.png", store.PublicAssetUrl(vaultId, "assets/image.png"));
    }

    private static async Task LocalObjectStoreDeleteMissingIsNoopAndDeletesVaultPrefix()
    {
        var basePath = ConfigTests.TestPath("local-store-delete");
        var store = LocalObjectStore.Create(basePath);
        const string vaultId = "vault-abc";

        await store.DeleteObjectAsync(vaultId, "missing.txt", CancellationToken.None);
        await store.PutObjectAsync(vaultId, "docs/a.txt", "hello"u8.ToArray(), "text/plain", CancellationToken.None);
        AssertEx.True(await store.HeadObjectAsync(vaultId, "docs/a.txt", CancellationToken.None) is not null, "expected object before vault delete");

        await store.DeleteVaultObjectsAsync(vaultId, CancellationToken.None);

        AssertEx.True(await store.HeadObjectAsync(vaultId, "docs/a.txt", CancellationToken.None) is null, "expected object removed after vault delete");
        AssertEx.True(!Directory.Exists(Path.Combine(basePath, ObjectKeys.VaultPrefix(vaultId))), "expected vault directory removed");
    }

    private static Task S3EndpointParsingAndPublicUrlsMatchContract()
    {
        var defaulted = S3ObjectStore.ParseEndpoint("s3.example.com");
        AssertEx.Equal(new S3Endpoint("http", "s3.example.com", 9000), defaulted);

        var https = S3ObjectStore.ParseEndpoint("https://s3.example.com");
        AssertEx.Equal(new S3Endpoint("https", "s3.example.com", 443), https);

        var explicitPort = S3ObjectStore.ParseEndpoint("http://127.0.0.1:9001");
        AssertEx.Equal(new S3Endpoint("http", "127.0.0.1", 9001), explicitPort);
        return Task.CompletedTask;
    }
}
