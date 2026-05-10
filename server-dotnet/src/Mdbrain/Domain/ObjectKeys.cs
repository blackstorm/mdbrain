using System.Text;

namespace Mdbrain.Domain;

public static class ObjectKeys
{
    public static string VaultPrefix(string vaultId)
    {
        return vaultId.Replace("-", string.Empty, StringComparison.Ordinal) + "/";
    }

    public static string NormalizePath(string value)
    {
        if (value.Length == 0)
        {
            return string.Empty;
        }

        value = value.Replace('\\', '/').TrimStart('/');
        value = Path.GetRelativePath(".", value).Replace('\\', '/');
        while (value == ".." || value.StartsWith("../", StringComparison.Ordinal))
        {
            value = value[2..].TrimStart('/');
        }

        return value == "." ? string.Empty : value;
    }

    public static string ExtensionFromPath(string path)
    {
        var extension = Path.GetExtension(Path.GetFileName(path)).TrimStart('.').ToLowerInvariant();
        if (extension.Length == 0)
        {
            return string.Empty;
        }

        var builder = new StringBuilder();
        foreach (var ch in extension)
        {
            if ((ch is >= 'a' and <= 'z') || (ch is >= '0' and <= '9') || ch is '-' or '_' or '+')
            {
                builder.Append(ch);
            }

            if (builder.Length >= 32)
            {
                break;
            }
        }

        return builder.ToString();
    }

    public static string AssetObjectKey(string clientId, string extension)
    {
        var ext = ExtensionFromPath("." + extension);
        return ext.Length == 0 ? $"assets/{clientId}" : $"assets/{clientId}.{ext}";
    }

    public static string LogoObjectKey(string contentHash, string extension)
    {
        return $"site/logo/{contentHash}.{ExtensionFromPath("." + extension)}";
    }

    public static string FaviconObjectKey(string logoObjectKey)
    {
        var ext = ExtensionFromPath(logoObjectKey);
        if (ext.Length == 0)
        {
            return string.Empty;
        }

        var withoutExtension = logoObjectKey[..^Path.GetExtension(logoObjectKey).Length];
        return $"{withoutExtension}.favicon.{ext}";
    }
}

