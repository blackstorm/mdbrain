using Mdbrain.Domain;
using Microsoft.AspNetCore.StaticFiles;

namespace Mdbrain.Storage;

public sealed class LocalObjectStore(string basePath) : IObjectStore
{
    private static readonly FileExtensionContentTypeProvider ContentTypes = new();

    public static LocalObjectStore Create(string basePath)
    {
        Directory.CreateDirectory(basePath);
        return new LocalObjectStore(basePath);
    }

    public async Task PutObjectAsync(string vaultId, string objectKey, byte[] content, string contentType, CancellationToken cancellationToken)
    {
        var path = FullPath(vaultId, objectKey);
        Directory.CreateDirectory(Path.GetDirectoryName(path)!);
        await File.WriteAllBytesAsync(path, content, cancellationToken);
    }

    public async Task<StoredObject?> GetObjectAsync(string vaultId, string objectKey, CancellationToken cancellationToken)
    {
        var path = FullPath(vaultId, objectKey);
        if (!File.Exists(path))
        {
            return null;
        }

        var file = File.OpenRead(path);
        var stat = new FileInfo(path);
        await Task.CompletedTask;
        return new StoredObject(file, stat.Length, GuessContentType(path), stat.LastWriteTimeUtc);
    }

    public Task DeleteObjectAsync(string vaultId, string objectKey, CancellationToken cancellationToken)
    {
        var path = FullPath(vaultId, objectKey);
        if (File.Exists(path))
        {
            File.Delete(path);
        }

        return Task.CompletedTask;
    }

    public Task<ObjectMetadata?> HeadObjectAsync(string vaultId, string objectKey, CancellationToken cancellationToken)
    {
        var path = FullPath(vaultId, objectKey);
        if (!File.Exists(path))
        {
            return Task.FromResult<ObjectMetadata?>(null);
        }

        var stat = new FileInfo(path);
        return Task.FromResult<ObjectMetadata?>(new ObjectMetadata(stat.Length, GuessContentType(path), stat.LastWriteTimeUtc));
    }

    public Task DeleteVaultObjectsAsync(string vaultId, CancellationToken cancellationToken)
    {
        var path = Path.Combine(basePath, ObjectKeys.VaultPrefix(vaultId));
        if (Directory.Exists(path))
        {
            Directory.Delete(path, recursive: true);
        }

        return Task.CompletedTask;
    }

    public string PublicAssetUrl(string vaultId, string objectKey)
    {
        return "/storage/" + objectKey;
    }

    private string FullPath(string vaultId, string objectKey)
    {
        var normalized = ObjectKeys.NormalizePath(objectKey);
        return Path.Combine(basePath, ObjectKeys.VaultPrefix(vaultId), normalized);
    }

    private static string GuessContentType(string path)
    {
        return ContentTypes.TryGetContentType(path, out var contentType)
            ? contentType
            : "application/octet-stream";
    }
}
