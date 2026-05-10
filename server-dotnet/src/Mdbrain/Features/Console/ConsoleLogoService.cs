using System.Security.Cryptography;
using Mdbrain.Data;
using Mdbrain.Domain;
using Mdbrain.Storage;
using Microsoft.AspNetCore.Http;
using SixLabors.ImageSharp;
using SixLabors.ImageSharp.Formats.Jpeg;
using SixLabors.ImageSharp.Formats.Png;
using SixLabors.ImageSharp.Processing;

namespace Mdbrain.Features.Console;

public sealed record LogoServiceResult(int StatusCode, object? Body, byte[]? Bytes = null, string? ContentType = null);

public sealed class ConsoleLogoService(MdbrainDbContext db, IObjectStore store)
{
    private const long MaxLogoBytes = 2 * 1024 * 1024;
    private const int MinLogoDimension = 128;
    private const int FaviconSize = 32;

    public async Task<LogoServiceResult> UploadLogoAsync(
        string vaultId,
        string tenantId,
        IFormFile? file,
        CancellationToken cancellationToken)
    {
        var vault = await db.Vaults.FindAsync([vaultId], cancellationToken);
        if (vault is null) return Json(StatusCodes.Status404NotFound, new { success = false, error = "Vault not found" });
        if (vault.TenantId != tenantId) return Json(StatusCodes.Status403Forbidden, new { success = false, error = "Permission denied" });
        if (file is null) return Json(StatusCodes.Status400BadRequest, new { success = false, error = "No file uploaded" });

        if (file.Length == 0 || file.Length > MaxLogoBytes)
        {
            var message = file.Length == 0
                ? "File is empty. Please upload a valid image file."
                : "File too large. Maximum size is 2MB.";
            return Json(StatusCodes.Status400BadRequest, new { success = false, error = message });
        }

        byte[] content;
        await using (var input = file.OpenReadStream())
        {
            using var memory = new MemoryStream();
            await input.CopyToAsync(memory, cancellationToken);
            content = memory.ToArray();
        }

        var contentType = DetectContentType(content, file.ContentType);
        if (contentType is not ("image/png" or "image/jpeg"))
        {
            return Json(StatusCodes.Status400BadRequest, new { success = false, error = "Invalid file type. Allowed: PNG, JPEG. Got: " + contentType });
        }

        Image image;
        try
        {
            image = Image.Load(content);
        }
        catch (UnknownImageFormatException)
        {
            return Json(StatusCodes.Status400BadRequest, new { success = false, error = "Invalid image file" });
        }

        using var _ = image;
        if (Math.Min(image.Width, image.Height) < MinLogoDimension)
        {
            return Json(StatusCodes.Status400BadRequest, new { success = false, error = "Image too small. Minimum size is 128x128." });
        }

        var extension = contentType == "image/png" ? "png" : "jpg";
        var contentHash = Convert.ToHexString(SHA256.HashData(content).AsSpan(0, 16)).ToLowerInvariant();
        var logoKey = ObjectKeys.LogoObjectKey(contentHash, extension);
        byte[] faviconBytes;
        try
        {
            faviconBytes = GenerateFavicon(image, contentType);
        }
        catch
        {
            return Json(StatusCodes.Status500InternalServerError, new { success = false, error = "Failed to generate favicon" });
        }

        var faviconKey = ObjectKeys.FaviconObjectKey(logoKey);
        try
        {
            await store.PutObjectAsync(vault.Id, logoKey, content, contentType, cancellationToken);
        }
        catch
        {
            return Json(StatusCodes.Status500InternalServerError, new { success = false, error = "Failed to store logo" });
        }

        try
        {
            await store.PutObjectAsync(vault.Id, faviconKey, faviconBytes, contentType, cancellationToken);
        }
        catch
        {
            await store.DeleteObjectAsync(vault.Id, logoKey, cancellationToken);
            return Json(StatusCodes.Status500InternalServerError, new { success = false, error = "Failed to store favicon" });
        }

        var oldKey = vault.LogoObjectKey;
        try
        {
            vault.LogoObjectKey = logoKey;
            await db.SaveChangesAsync(cancellationToken);
        }
        catch
        {
            await store.DeleteObjectAsync(vault.Id, logoKey, cancellationToken);
            await store.DeleteObjectAsync(vault.Id, faviconKey, cancellationToken);
            return Json(StatusCodes.Status500InternalServerError, new { success = false, error = "Failed to update vault logo" });
        }

        if (!string.IsNullOrWhiteSpace(oldKey) && oldKey != logoKey)
        {
            await store.DeleteObjectAsync(vault.Id, ObjectKeys.FaviconObjectKey(oldKey), cancellationToken);
            await store.DeleteObjectAsync(vault.Id, oldKey, cancellationToken);
        }

        return Json(StatusCodes.Status200OK, new Dictionary<string, object?>
        {
            ["success"] = true,
            ["message"] = "Logo uploaded successfully",
            ["logo-url"] = ConsoleCommonEndpoints.ConsoleAssetUrl(vault.Id, logoKey)
        });
    }

    public async Task<LogoServiceResult> DeleteLogoAsync(string vaultId, string tenantId, CancellationToken cancellationToken)
    {
        var vault = await db.Vaults.FindAsync([vaultId], cancellationToken);
        if (vault is null) return Json(StatusCodes.Status404NotFound, new { success = false, error = "Vault not found" });
        if (vault.TenantId != tenantId) return Json(StatusCodes.Status403Forbidden, new { success = false, error = "Permission denied" });
        if (string.IsNullOrWhiteSpace(vault.LogoObjectKey))
        {
            return Json(StatusCodes.Status200OK, new { success = true, message = "No logo to delete" });
        }

        await store.DeleteObjectAsync(vault.Id, ObjectKeys.FaviconObjectKey(vault.LogoObjectKey), cancellationToken);
        await store.DeleteObjectAsync(vault.Id, vault.LogoObjectKey, cancellationToken);
        vault.LogoObjectKey = null;
        await db.SaveChangesAsync(cancellationToken);
        return Json(StatusCodes.Status200OK, new { success = true, message = "Logo deleted successfully" });
    }

    public async Task<LogoServiceResult> ReadLogoAsync(string vaultId, string tenantId, bool favicon, CancellationToken cancellationToken)
    {
        var vault = await db.Vaults.FindAsync([vaultId], cancellationToken);
        if (vault is null) return Json(StatusCodes.Status404NotFound, new { success = false, error = "Vault not found" });
        if (vault.TenantId != tenantId) return Json(StatusCodes.Status403Forbidden, new { success = false, error = "Permission denied" });
        if (string.IsNullOrWhiteSpace(vault.LogoObjectKey)) return Json(StatusCodes.Status404NotFound, new { success = false, error = "Logo not found" });

        var key = vault.LogoObjectKey;
        if (favicon)
        {
            var faviconKey = ObjectKeys.FaviconObjectKey(key);
            if (await store.HeadObjectAsync(vault.Id, faviconKey, cancellationToken) is not null)
            {
                key = faviconKey;
            }
        }

        await using var stored = await store.GetObjectAsync(vault.Id, key, cancellationToken);
        if (stored is null) return Json(StatusCodes.Status404NotFound, new { success = false, error = "Logo not found" });

        using var memory = new MemoryStream();
        await stored.Body.CopyToAsync(memory, cancellationToken);
        return new LogoServiceResult(StatusCodes.Status200OK, null, memory.ToArray(), stored.ContentType);
    }

    private static LogoServiceResult Json(int statusCode, object body)
    {
        return new LogoServiceResult(statusCode, body);
    }

    private static byte[] GenerateFavicon(Image source, string contentType)
    {
        using var image = source.Clone(context =>
        {
            var size = Math.Min(source.Width, source.Height);
            var crop = new Rectangle((source.Width - size) / 2, (source.Height - size) / 2, size, size);
            context.Crop(crop).Resize(FaviconSize, FaviconSize);
        });

        using var output = new MemoryStream();
        if (contentType == "image/png")
        {
            image.Save(output, new PngEncoder());
        }
        else
        {
            image.Save(output, new JpegEncoder { Quality = 90 });
        }

        return output.ToArray();
    }

    private static string DetectContentType(byte[] content, string headerContentType)
    {
        if (content.Length >= 8
            && content[0] == 0x89
            && content[1] == 0x50
            && content[2] == 0x4e
            && content[3] == 0x47
            && content[4] == 0x0d
            && content[5] == 0x0a
            && content[6] == 0x1a
            && content[7] == 0x0a)
        {
            return "image/png";
        }

        if (content.Length >= 3 && content[0] == 0xff && content[1] == 0xd8 && content[2] == 0xff)
        {
            return "image/jpeg";
        }

        return string.IsNullOrWhiteSpace(headerContentType) ? "application/octet-stream" : headerContentType;
    }
}
