namespace Mdbrain.Features.App;

public sealed record ParsedHost(string Domain, string Error);

public static class HostParser
{
    public static ParsedHost ParseHostDomain(string host)
    {
        host = host.Trim();
        if (host.Length == 0)
        {
            return new ParsedHost(string.Empty, "missing");
        }

        if (host.Contains(' ') || host.Contains('/'))
        {
            return new ParsedHost(string.Empty, "invalid");
        }

        if (host.StartsWith("[", StringComparison.Ordinal))
        {
            var close = host.IndexOf(']', StringComparison.Ordinal);
            if (close < 0)
            {
                return new ParsedHost(string.Empty, "invalid");
            }

            var domain = host[1..close];
            var rest = host[(close + 1)..];
            if (domain.Length == 0)
            {
                return new ParsedHost(string.Empty, "invalid");
            }

            if (rest.Length == 0)
            {
                return new ParsedHost(domain, string.Empty);
            }

            return rest.StartsWith(":", StringComparison.Ordinal) && ValidPort(rest[1..])
                ? new ParsedHost(domain, string.Empty)
                : new ParsedHost(string.Empty, "invalid");
        }

        var parts = host.Split(':');
        if (parts.Length > 2)
        {
            return new ParsedHost(string.Empty, "invalid");
        }

        var plainDomain = parts[0];
        if (plainDomain.Length == 0)
        {
            return new ParsedHost(string.Empty, "invalid");
        }

        foreach (var ch in plainDomain)
        {
            if (!char.IsAsciiLetterOrDigit(ch) && ch is not '.' and not '-' and not '_')
            {
                return new ParsedHost(string.Empty, "invalid");
            }
        }

        if (parts.Length == 2 && !ValidPort(parts[1]))
        {
            return new ParsedHost(string.Empty, "invalid");
        }

        return new ParsedHost(plainDomain, string.Empty);
    }

    private static bool ValidPort(string raw)
    {
        return int.TryParse(raw, out var port) && port is >= 1 and <= 65535;
    }
}
