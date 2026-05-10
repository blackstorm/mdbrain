using Mdbrain.Config;

namespace Mdbrain.Tests;

internal static class TestAppConfig
{
    public static AppConfig Create(string? dataPath = null, string? storageType = null)
    {
        dataPath ??= ConfigTests.TestPath($"config-{Guid.NewGuid():N}");
        return new AppConfig
        {
            ProjectRoot = ConfigTests.RepoRoot(),
            DataPath = dataPath,
            EnvironmentName = "test",
            AppHost = "127.0.0.1",
            AppPort = 8080,
            ConsoleHost = "127.0.0.1",
            ConsolePort = 9090,
            StorageType = storageType ?? "local",
            LocalStoragePath = Path.Combine(dataPath, "storage"),
            S3Region = "us-east-1",
            S3Bucket = "mdbrain",
            SessionSecret = "session-secret-for-tests",
            HealthToken = "health-token-for-tests",
            OnDemandTlsEnabled = false
        };
    }
}
