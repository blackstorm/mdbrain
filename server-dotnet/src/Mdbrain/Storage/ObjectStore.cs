namespace Mdbrain.Storage;

public sealed record StoredObject(Stream Body, long ContentLength, string ContentType, DateTimeOffset? LastModified) : IAsyncDisposable
{
    public ValueTask DisposeAsync()
    {
        return Body.DisposeAsync();
    }
}

public sealed record ObjectMetadata(long ContentLength, string ContentType, DateTimeOffset? LastModified);

public interface IObjectStore
{
    Task PutObjectAsync(string vaultId, string objectKey, byte[] content, string contentType, CancellationToken cancellationToken);
    Task<StoredObject?> GetObjectAsync(string vaultId, string objectKey, CancellationToken cancellationToken);
    Task DeleteObjectAsync(string vaultId, string objectKey, CancellationToken cancellationToken);
    Task<ObjectMetadata?> HeadObjectAsync(string vaultId, string objectKey, CancellationToken cancellationToken);
    Task DeleteVaultObjectsAsync(string vaultId, CancellationToken cancellationToken);
    string PublicAssetUrl(string vaultId, string objectKey);
}

