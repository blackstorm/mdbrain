namespace Mdbrain.Features.Console;

public static class ConsoleHelpers
{
    public static string StringValue(string? value) => value ?? string.Empty;

    public static string FirstNonEmpty(params string?[] values)
    {
        return values.FirstOrDefault(value => !string.IsNullOrWhiteSpace(value)) ?? string.Empty;
    }

    public static string FormatStorageSize(long bytes)
    {
        const long kb = 1024;
        const long mb = kb * 1024;
        const long gb = mb * 1024;
        return bytes switch
        {
            >= gb => $"{bytes / (double)gb:F2} GB",
            >= mb => $"{bytes / (double)mb:F2} MB",
            >= kb => $"{bytes / (double)kb:F2} KB",
            _ => $"{bytes} B"
        };
    }

    public static string MaskKey(string value)
    {
        value = value.Trim();
        return value.Length <= 16 ? value : value[..8] + "******" + value[^8..];
    }
}

