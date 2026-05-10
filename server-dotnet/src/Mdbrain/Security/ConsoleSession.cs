using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using Mdbrain.Config;

namespace Mdbrain.Security;

public sealed class ConsoleSession
{
    [JsonPropertyName("user_id")]
    public string? UserId { get; set; }

    [JsonPropertyName("tenant_id")]
    public string? TenantId { get; set; }

    [JsonPropertyName("csrf_token")]
    public string? CsrfToken { get; set; }
}

public sealed class SessionManager(AppConfig config)
{
    public const string CookieName = "mdbrain-session";
    private const int SessionMaxAgeSeconds = 7 * 24 * 60 * 60;

    private readonly byte[] signingKey = MD5.HashData(Encoding.UTF8.GetBytes(config.SessionSecret));
    private static readonly JsonSerializerOptions JsonOptions = new(JsonSerializerDefaults.Web);

    public ConsoleSession Load(HttpContext context)
    {
        return context.Request.Cookies.TryGetValue(CookieName, out var raw)
            ? Decode(raw) ?? new ConsoleSession()
            : new ConsoleSession();
    }

    public void Save(HttpContext context, ConsoleSession session)
    {
        context.Response.Cookies.Append(CookieName, Encode(session), CookieOptions(SessionMaxAgeSeconds));
    }

    public void Clear(HttpContext context)
    {
        context.Response.Cookies.Append(CookieName, string.Empty, CookieOptions(-1));
    }

    public void EnsureCsrf(ConsoleSession session)
    {
        if (!string.IsNullOrWhiteSpace(session.CsrfToken))
        {
            return;
        }

        session.CsrfToken = RandomBase64Url(32);
    }

    private string Encode(ConsoleSession session)
    {
        var payload = JsonSerializer.SerializeToUtf8Bytes(session, JsonOptions);
        var payloadPart = Base64UrlEncode(payload);
        var signature = HMACSHA256.HashData(signingKey, payload);
        return payloadPart + "." + Base64UrlEncode(signature);
    }

    private ConsoleSession? Decode(string raw)
    {
        var parts = raw.Split('.', 2);
        if (parts.Length != 2)
        {
            return null;
        }

        try
        {
            var payload = Base64UrlDecode(parts[0]);
            var provided = Base64UrlDecode(parts[1]);
            var expected = HMACSHA256.HashData(signingKey, payload);
            return CryptographicOperations.FixedTimeEquals(provided, expected)
                ? JsonSerializer.Deserialize<ConsoleSession>(payload, JsonOptions)
                : null;
        }
        catch
        {
            return null;
        }
    }

    private CookieOptions CookieOptions(int maxAge)
    {
        return new CookieOptions
        {
            Path = "/",
            HttpOnly = true,
            Secure = config.Production,
            SameSite = SameSiteMode.Lax,
            MaxAge = TimeSpan.FromSeconds(maxAge)
        };
    }

    private static string RandomBase64Url(int bytes)
    {
        var buffer = RandomNumberGenerator.GetBytes(bytes);
        return Base64UrlEncode(buffer);
    }

    private static string Base64UrlEncode(byte[] bytes)
    {
        return Convert.ToBase64String(bytes).TrimEnd('=').Replace('+', '-').Replace('/', '_');
    }

    private static byte[] Base64UrlDecode(string value)
    {
        var padded = value.Replace('-', '+').Replace('_', '/');
        padded += new string('=', (4 - padded.Length % 4) % 4);
        return Convert.FromBase64String(padded);
    }
}

