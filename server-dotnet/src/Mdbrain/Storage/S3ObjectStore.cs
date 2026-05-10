using System.Net;
using Amazon;
using Amazon.Runtime;
using Amazon.S3;
using Amazon.S3.Model;
using Mdbrain.Config;
using Mdbrain.Domain;

namespace Mdbrain.Storage;

public sealed class S3ObjectStore : IObjectStore
{
    private readonly IAmazonS3 client;
    private readonly string bucket;
    private readonly string publicUrl;

    private S3ObjectStore(IAmazonS3 client, string bucket, string publicUrl)
    {
        this.client = client;
        this.bucket = bucket;
        this.publicUrl = publicUrl.TrimEnd('/');
    }

    public static async Task<IObjectStore> CreateAsync(AppConfig config, CancellationToken cancellationToken)
    {
        var endpoint = ParseEndpoint(config.S3Endpoint ?? string.Empty);
        var clientConfig = new AmazonS3Config
        {
            RegionEndpoint = RegionEndpoint.GetBySystemName(config.S3Region),
            ServiceURL = $"{endpoint.Protocol}://{endpoint.Hostname}:{endpoint.Port}",
            ForcePathStyle = true
        };
        var credentials = new BasicAWSCredentials(config.S3AccessKey, config.S3SecretKey);
        var store = new S3ObjectStore(
            new AmazonS3Client(credentials, clientConfig),
            config.S3Bucket.Trim(),
            config.S3PublicUrl?.Trim() ?? string.Empty);
        await store.EnsureBucketAsync(cancellationToken);
        return store;
    }

    internal static S3Endpoint ParseEndpoint(string endpoint)
    {
        var raw = endpoint.Trim();
        if (raw.Length == 0)
        {
            throw new InvalidOperationException("S3_ENDPOINT is required for S3 storage");
        }

        var withScheme = raw.StartsWith("http://", StringComparison.OrdinalIgnoreCase)
            || raw.StartsWith("https://", StringComparison.OrdinalIgnoreCase)
                ? raw
                : "http://" + raw;
        if (!Uri.TryCreate(withScheme, UriKind.Absolute, out var uri) || string.IsNullOrWhiteSpace(uri.Host))
        {
            throw new InvalidOperationException($"invalid S3 endpoint: {endpoint}");
        }

        var protocol = string.IsNullOrWhiteSpace(uri.Scheme) ? "http" : uri.Scheme.ToLowerInvariant();
        var port = uri.IsDefaultPort ? protocol == "https" ? 443 : 9000 : uri.Port;
        return new S3Endpoint(protocol, uri.Host, port);
    }

    public async Task PutObjectAsync(string vaultId, string objectKey, byte[] content, string contentType, CancellationToken cancellationToken)
    {
        var request = new PutObjectRequest
        {
            BucketName = bucket,
            Key = FullKey(vaultId, objectKey),
            InputStream = new MemoryStream(content),
            ContentType = contentType
        };
        await client.PutObjectAsync(request, cancellationToken);
    }

    public async Task<StoredObject?> GetObjectAsync(string vaultId, string objectKey, CancellationToken cancellationToken)
    {
        try
        {
            var response = await client.GetObjectAsync(bucket, FullKey(vaultId, objectKey), cancellationToken);
            return new StoredObject(
                response.ResponseStream,
                response.ContentLength,
                response.Headers.ContentType ?? "application/octet-stream",
                response.LastModified == DateTime.MinValue ? null : response.LastModified);
        }
        catch (AmazonS3Exception ex) when (IsNotFound(ex))
        {
            return null;
        }
    }

    public async Task DeleteObjectAsync(string vaultId, string objectKey, CancellationToken cancellationToken)
    {
        try
        {
            await client.DeleteObjectAsync(bucket, FullKey(vaultId, objectKey), cancellationToken);
        }
        catch (AmazonS3Exception ex) when (IsNotFound(ex))
        {
        }
    }

    public async Task<ObjectMetadata?> HeadObjectAsync(string vaultId, string objectKey, CancellationToken cancellationToken)
    {
        try
        {
            var response = await client.GetObjectMetadataAsync(bucket, FullKey(vaultId, objectKey), cancellationToken);
            return new ObjectMetadata(
                response.ContentLength,
                response.Headers.ContentType ?? "application/octet-stream",
                response.LastModified == DateTime.MinValue ? null : response.LastModified);
        }
        catch (AmazonS3Exception ex) when (IsNotFound(ex))
        {
            return null;
        }
    }

    public async Task DeleteVaultObjectsAsync(string vaultId, CancellationToken cancellationToken)
    {
        string? token = null;
        var prefix = ObjectKeys.VaultPrefix(vaultId);
        do
        {
            var response = await client.ListObjectsV2Async(new ListObjectsV2Request
            {
                BucketName = bucket,
                Prefix = prefix,
                ContinuationToken = token
            }, cancellationToken);

            foreach (var item in response.S3Objects.Where(item => !string.IsNullOrWhiteSpace(item.Key)))
            {
                await client.DeleteObjectAsync(bucket, item.Key, cancellationToken);
            }

            token = response.IsTruncated == true ? response.NextContinuationToken : null;
        } while (!string.IsNullOrWhiteSpace(token));
    }

    public string PublicAssetUrl(string vaultId, string objectKey)
    {
        return publicUrl.Length == 0 ? string.Empty : $"{publicUrl}/{bucket}/{FullKey(vaultId, objectKey)}";
    }

    private async Task EnsureBucketAsync(CancellationToken cancellationToken)
    {
        if (string.IsNullOrWhiteSpace(bucket))
        {
            throw new InvalidOperationException("S3 bucket is required");
        }

        try
        {
            await client.HeadBucketAsync(new HeadBucketRequest { BucketName = bucket }, cancellationToken);
            return;
        }
        catch (AmazonS3Exception ex) when (IsNotFound(ex))
        {
        }
        catch (AmazonS3Exception ex)
        {
            throw new InvalidOperationException($"S3 connection failed: {ex.Message}", ex);
        }

        try
        {
            await client.PutBucketAsync(new PutBucketRequest { BucketName = bucket }, cancellationToken);
        }
        catch (AmazonS3Exception ex) when (ex.ErrorCode == "BucketAlreadyOwnedByYou")
        {
        }
    }

    private string FullKey(string vaultId, string objectKey)
    {
        return ObjectKeys.VaultPrefix(vaultId) + objectKey;
    }

    private static bool IsNotFound(AmazonS3Exception ex)
    {
        return ex.StatusCode == HttpStatusCode.NotFound
            || ex.ErrorCode is "NotFound" or "NoSuchKey" or "NoSuchBucket" or "404";
    }
}

public sealed record S3Endpoint(string Protocol, string Hostname, int Port);
