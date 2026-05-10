using System.Security.Cryptography;
using System.Text.RegularExpressions;

namespace Mdbrain.Config;

public sealed partial class AppConfig
{
    public required string ProjectRoot { get; init; }
    public required string DataPath { get; init; }
    public required string EnvironmentName { get; init; }
    public required string AppHost { get; init; }
    public required int AppPort { get; init; }
    public required string ConsoleHost { get; init; }
    public required int ConsolePort { get; init; }
    public required string StorageType { get; init; }
    public required string LocalStoragePath { get; init; }
    public string? S3Endpoint { get; init; }
    public string? S3AccessKey { get; init; }
    public string? S3SecretKey { get; init; }
    public required string S3Region { get; init; }
    public required string S3Bucket { get; init; }
    public string? S3PublicUrl { get; init; }
    public required string SessionSecret { get; init; }
    public required string HealthToken { get; init; }
    public required bool OnDemandTlsEnabled { get; init; }

    public bool Production => EnvironmentName == "production";

    public string DatabasePath => Path.Combine(DataPath, "mdbrain.db");

    public string PublicRoot => Path.Combine(ProjectRoot, "server", "resources", "publics");

    public string TemplateRoot => Path.Combine(ProjectRoot, "server", "resources");

    public static AppConfig Load(string projectRoot)
    {
        var dataPath = EnvOr("DATA_PATH", "data");
        var hostFallback = EnvOr("HOST", "0.0.0.0");

        var sessionSecret = FirstNonEmpty(
            Environment.GetEnvironmentVariable("SESSION_SECRET"),
            LoadOrCreateSessionSecret(dataPath));

        var config = new AppConfig
        {
            ProjectRoot = projectRoot,
            DataPath = dataPath,
            EnvironmentName = EnvOr("ENVIRONMENT", "development"),
            AppHost = EnvOr("APP_HOST", hostFallback),
            AppPort = EnvIntOr("APP_PORT", 8080),
            ConsoleHost = EnvOr("CONSOLE_HOST", hostFallback),
            ConsolePort = EnvIntOr("CONSOLE_PORT", 9090),
            StorageType = EnvOr("STORAGE_TYPE", "local").ToLowerInvariant(),
            LocalStoragePath = EnvOr("LOCAL_STORAGE_PATH", Path.Combine(dataPath, "storage")),
            S3Endpoint = Environment.GetEnvironmentVariable("S3_ENDPOINT"),
            S3AccessKey = Environment.GetEnvironmentVariable("S3_ACCESS_KEY"),
            S3SecretKey = Environment.GetEnvironmentVariable("S3_SECRET_KEY"),
            S3Region = EnvOr("S3_REGION", "us-east-1"),
            S3Bucket = EnvOr("S3_BUCKET", "mdbrain"),
            S3PublicUrl = Environment.GetEnvironmentVariable("S3_PUBLIC_URL"),
            SessionSecret = sessionSecret,
            HealthToken = LoadOrCreateHealthToken(dataPath),
            OnDemandTlsEnabled = Environment.GetEnvironmentVariable("CADDY_ON_DEMAND_TLS_ENABLED") == "true"
        };

        config.Validate();
        return config;
    }

    public void Validate()
    {
        if (StorageType == "local")
        {
            return;
        }

        if (StorageType != "s3")
        {
            throw new InvalidOperationException($"unknown STORAGE_TYPE: {StorageType}. supported: local, s3");
        }

        var missing = new List<string>();
        if (string.IsNullOrWhiteSpace(S3Endpoint)) missing.Add("missing S3_ENDPOINT: required when STORAGE_TYPE=s3");
        if (string.IsNullOrWhiteSpace(S3AccessKey)) missing.Add("missing S3_ACCESS_KEY: required when STORAGE_TYPE=s3");
        if (string.IsNullOrWhiteSpace(S3SecretKey)) missing.Add("missing S3_SECRET_KEY: required when STORAGE_TYPE=s3");
        if (string.IsNullOrWhiteSpace(S3PublicUrl)) missing.Add("missing S3_PUBLIC_URL: required when STORAGE_TYPE=s3");

        if (missing.Count > 0)
        {
            throw new InvalidOperationException(string.Join("; ", missing));
        }
    }

    public static string EnvOr(string key, string fallback)
    {
        var value = Environment.GetEnvironmentVariable(key);
        return string.IsNullOrWhiteSpace(value) ? fallback : value;
    }

    public static int EnvIntOr(string key, int fallback)
    {
        return int.TryParse(Environment.GetEnvironmentVariable(key), out var value) ? value : fallback;
    }

    public static string FirstNonEmpty(params string?[] values)
    {
        foreach (var value in values)
        {
            if (!string.IsNullOrWhiteSpace(value))
            {
                return value;
            }
        }

        return string.Empty;
    }

    private static string LoadOrCreateSessionSecret(string dataPath)
    {
        var path = Path.Combine(dataPath, ".secrets.edn");
        if (File.Exists(path))
        {
            var existing = ParseEdnSessionSecret(File.ReadAllText(path));
            if (!string.IsNullOrWhiteSpace(existing))
            {
                return existing;
            }
        }

        var secret = GenerateRandomHex(16);
        Directory.CreateDirectory(Path.GetDirectoryName(path)!);
        File.WriteAllText(path, $"{{:session-secret \"{secret}\"}}\n");
        return secret;
    }

    private static string LoadOrCreateHealthToken(string dataPath)
    {
        var path = Path.Combine(dataPath, ".health-token");
        if (File.Exists(path))
        {
            var existing = File.ReadAllText(path).Trim();
            if (existing.Length > 0)
            {
                return existing;
            }
        }

        var token = GenerateRandomHex(32);
        Directory.CreateDirectory(Path.GetDirectoryName(path)!);
        File.WriteAllText(path, token);
        return token;
    }

    public static string ParseEdnSessionSecret(string content)
    {
        var match = SessionSecretPattern().Match(content);
        return match.Success ? match.Groups[1].Value : string.Empty;
    }

    private static string GenerateRandomHex(int bytes)
    {
        Span<byte> buffer = stackalloc byte[bytes];
        RandomNumberGenerator.Fill(buffer);
        return Convert.ToHexString(buffer).ToLowerInvariant();
    }

    [GeneratedRegex(":session-secret\\s+\"([^\"]+)\"")]
    private static partial Regex SessionSecretPattern();
}
