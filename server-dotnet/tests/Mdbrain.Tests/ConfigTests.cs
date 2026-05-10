using Mdbrain.Config;

namespace Mdbrain.Tests;

internal static class ConfigTests
{
    public static void Register(TestRunner runner)
    {
        runner.Add(nameof(LoadDefaultsAndSecrets), LoadDefaultsAndSecrets);
        runner.Add(nameof(LoadReadsExistingSecret), LoadReadsExistingSecret);
        runner.Add(nameof(LoadAppliesRuntimeOverrides), LoadAppliesRuntimeOverrides);
        runner.Add(nameof(LoadRegeneratesEmptyHealthToken), LoadRegeneratesEmptyHealthToken);
        runner.Add(nameof(ValidateRejectsUnknownStorageType), ValidateRejectsUnknownStorageType);
        runner.Add(nameof(ValidateAcceptsCompleteS3Configuration), ValidateAcceptsCompleteS3Configuration);
        runner.Add(nameof(LoadRejectsInvalidS3Configuration), LoadRejectsInvalidS3Configuration);
    }

    private static Task LoadDefaultsAndSecrets()
    {
        using var env = TestEnvironment();
        var dataPath = TestPath("config-defaults");
        env.Set("DATA_PATH", dataPath);

        var cfg = AppConfig.Load(RepoRoot());

        AssertEx.Equal(8080, cfg.AppPort);
        AssertEx.Equal(9090, cfg.ConsolePort);
        AssertEx.Equal("local", cfg.StorageType);
        AssertEx.True(cfg.SessionSecret.Length > 0, "expected generated session secret");
        AssertEx.True(cfg.HealthToken.Length > 0, "expected generated health token");
        AssertEx.True(File.Exists(Path.Combine(dataPath, ".secrets.edn")), "expected secrets file");
        AssertEx.True(File.Exists(Path.Combine(dataPath, ".health-token")), "expected health token file");
        return Task.CompletedTask;
    }

    private static Task LoadReadsExistingSecret()
    {
        using var env = TestEnvironment();
        var dataPath = TestPath("config-existing");
        Directory.CreateDirectory(dataPath);
        File.WriteAllText(Path.Combine(dataPath, ".secrets.edn"), "{:session-secret \"abc123\"}\n");
        File.WriteAllText(Path.Combine(dataPath, ".health-token"), "token-1");
        env.Set("DATA_PATH", dataPath);

        var cfg = AppConfig.Load(RepoRoot());

        AssertEx.Equal("abc123", cfg.SessionSecret);
        AssertEx.Equal("token-1", cfg.HealthToken);
        return Task.CompletedTask;
    }

    private static Task LoadAppliesRuntimeOverrides()
    {
        using var env = TestEnvironment();
        var dataPath = TestPath("config-overrides");
        var storagePath = TestPath("config-overrides-storage");
        env.Set("DATA_PATH", dataPath);
        env.Set("ENVIRONMENT", "production");
        env.Set("HOST", "fallback.example.com");
        env.Set("APP_HOST", "app.example.com");
        env.Set("CONSOLE_HOST", "console.example.com");
        env.Set("APP_PORT", "18080");
        env.Set("CONSOLE_PORT", "19090");
        env.Set("LOCAL_STORAGE_PATH", storagePath);
        env.Set("SESSION_SECRET", "from-env");
        env.Set("CADDY_ON_DEMAND_TLS_ENABLED", "true");

        var cfg = AppConfig.Load(RepoRoot());

        AssertEx.Equal("production", cfg.EnvironmentName);
        AssertEx.True(cfg.Production, "expected production flag");
        AssertEx.Equal("app.example.com", cfg.AppHost);
        AssertEx.Equal("console.example.com", cfg.ConsoleHost);
        AssertEx.Equal(18080, cfg.AppPort);
        AssertEx.Equal(19090, cfg.ConsolePort);
        AssertEx.Equal(storagePath, cfg.LocalStoragePath);
        AssertEx.Equal("from-env", cfg.SessionSecret);
        AssertEx.True(cfg.OnDemandTlsEnabled, "expected on-demand TLS flag");
        return Task.CompletedTask;
    }

    private static Task LoadRegeneratesEmptyHealthToken()
    {
        using var env = TestEnvironment();
        var dataPath = TestPath("config-empty-health-token");
        File.WriteAllText(Path.Combine(dataPath, ".health-token"), " \n");
        env.Set("DATA_PATH", dataPath);

        var cfg = AppConfig.Load(RepoRoot());

        AssertEx.True(cfg.HealthToken.Length > 0, "expected regenerated token");
        AssertEx.Equal(cfg.HealthToken, File.ReadAllText(Path.Combine(dataPath, ".health-token")));
        return Task.CompletedTask;
    }

    private static Task ValidateRejectsUnknownStorageType()
    {
        var cfg = TestAppConfig.Create(storageType: "ftp");

        try
        {
            cfg.Validate();
            throw new TestFailureException("expected unknown storage type to fail");
        }
        catch (InvalidOperationException ex)
        {
            AssertEx.Contains("unknown STORAGE_TYPE", ex.Message);
        }

        return Task.CompletedTask;
    }

    private static Task ValidateAcceptsCompleteS3Configuration()
    {
        var cfg = new AppConfig
        {
            ProjectRoot = RepoRoot(),
            DataPath = TestPath("config-valid-s3"),
            EnvironmentName = "test",
            AppHost = "127.0.0.1",
            AppPort = 8080,
            ConsoleHost = "127.0.0.1",
            ConsolePort = 9090,
            StorageType = "s3",
            LocalStoragePath = TestPath("config-valid-s3-storage"),
            S3Endpoint = "https://s3.example.com",
            S3AccessKey = "access",
            S3SecretKey = "secret",
            S3Region = "us-east-1",
            S3Bucket = "mdbrain",
            S3PublicUrl = "https://cdn.example.com",
            SessionSecret = "secret",
            HealthToken = "token",
            OnDemandTlsEnabled = false
        };

        cfg.Validate();
        return Task.CompletedTask;
    }

    private static Task LoadRejectsInvalidS3Configuration()
    {
        using var env = TestEnvironment();
        env.Set("DATA_PATH", TestPath("config-invalid-s3"));
        env.Set("STORAGE_TYPE", "s3");

        try
        {
            AppConfig.Load(RepoRoot());
            throw new TestFailureException("expected invalid s3 configuration to fail");
        }
        catch (InvalidOperationException ex)
        {
            AssertEx.Contains("missing S3_ENDPOINT", ex.Message);
            AssertEx.Contains("missing S3_ACCESS_KEY", ex.Message);
            AssertEx.Contains("missing S3_SECRET_KEY", ex.Message);
            AssertEx.Contains("missing S3_PUBLIC_URL", ex.Message);
        }

        return Task.CompletedTask;
    }

    private static EnvironmentScope TestEnvironment()
    {
        var env = new EnvironmentScope();
        foreach (var key in new[]
                 {
                     "DATA_PATH",
                     "ENVIRONMENT",
                     "HOST",
                     "APP_HOST",
                     "CONSOLE_HOST",
                     "APP_PORT",
                     "CONSOLE_PORT",
                     "STORAGE_TYPE",
                     "LOCAL_STORAGE_PATH",
                     "S3_ENDPOINT",
                     "S3_ACCESS_KEY",
                     "S3_SECRET_KEY",
                     "S3_REGION",
                     "S3_BUCKET",
                     "S3_PUBLIC_URL",
                     "SESSION_SECRET",
                     "CADDY_ON_DEMAND_TLS_ENABLED"
                 })
        {
            env.Set(key, null);
        }

        return env;
    }

    internal static string RepoRoot()
    {
        var current = new DirectoryInfo(AppContext.BaseDirectory);
        while (current is not null)
        {
            if (File.Exists(Path.Combine(current.FullName, "AGENTS.md"))
                && Directory.Exists(Path.Combine(current.FullName, "server-dotnet")))
            {
                return current.FullName;
            }

            current = current.Parent;
        }

        throw new InvalidOperationException("repo root not found");
    }

    internal static string TestPath(string name)
    {
        var path = Path.Combine(RepoRoot(), "server-dotnet", ".testdata", name);
        if (Directory.Exists(path))
        {
            Directory.Delete(path, recursive: true);
        }

        Directory.CreateDirectory(path);
        return path;
    }
}
