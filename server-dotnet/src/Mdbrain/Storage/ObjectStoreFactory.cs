using Mdbrain.Config;

namespace Mdbrain.Storage;

public static class ObjectStoreFactory
{
    public static Task<IObjectStore> CreateAsync(AppConfig config, CancellationToken cancellationToken)
    {
        return config.StorageType switch
        {
            "local" => Task.FromResult<IObjectStore>(LocalObjectStore.Create(config.LocalStoragePath)),
            "s3" => S3ObjectStore.CreateAsync(config, cancellationToken),
            _ => throw new InvalidOperationException("unsupported storage type")
        };
    }
}
