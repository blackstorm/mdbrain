using Mdbrain.Config;
using Mdbrain.Data;
using Mdbrain.Data.Entities;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;

namespace Mdbrain.Tests;

internal static class DatabaseTests
{
    public static void Register(TestRunner runner)
    {
        runner.Add(nameof(InMemorySqliteCreatesSchemaAndKeepsForeignKeys), InMemorySqliteCreatesSchemaAndKeepsForeignKeys);
        runner.Add(nameof(FileSqliteUsesWalAndBusyTimeout), FileSqliteUsesWalAndBusyTimeout);
    }

    private static async Task InMemorySqliteCreatesSchemaAndKeepsForeignKeys()
    {
        await using var database = await TestSqliteDatabase.CreateAsync();
        await using var db = database.CreateContext();

        foreach (var table in new[] { "tenants", "users", "vaults", "notes", "assets", "note_links", "note_asset_refs" })
        {
            var exists = await ScalarLongAsync(db, """
                SELECT COUNT(*)
                FROM sqlite_master
                WHERE type = 'table' AND name = $table
                """, new SqliteParameter("$table", table));
            AssertEx.Equal(1L, exists, $"expected table {table}");
        }

        db.Users.Add(new User
        {
            Id = "user-1",
            TenantId = "missing",
            Username = "admin",
            PasswordHash = "hash"
        });

        try
        {
            await db.SaveChangesAsync();
            throw new TestFailureException("expected foreign key violation");
        }
        catch (DbUpdateException)
        {
            db.ChangeTracker.Clear();
        }
    }

    private static async Task FileSqliteUsesWalAndBusyTimeout()
    {
        var dataPath = ConfigTests.TestPath("file-sqlite");
        var cfg = new AppConfig
        {
            ProjectRoot = ConfigTests.RepoRoot(),
            DataPath = dataPath,
            EnvironmentName = "test",
            AppHost = "127.0.0.1",
            AppPort = 8080,
            ConsoleHost = "127.0.0.1",
            ConsolePort = 9090,
            StorageType = "local",
            LocalStoragePath = Path.Combine(dataPath, "storage"),
            S3Region = "us-east-1",
            S3Bucket = "mdbrain",
            SessionSecret = "secret",
            HealthToken = "token",
            OnDemandTlsEnabled = false
        };

        var options = SqliteDatabase.Options(SqliteDatabase.FileConnectionString(cfg));
        await using var db = new MdbrainDbContext(options);
        await SqliteDatabase.InitializeFileDatabaseAsync(db, CancellationToken.None);

        var journalMode = await ScalarTextAsync(db, "PRAGMA journal_mode;");
        AssertEx.Equal("wal", journalMode.ToLowerInvariant());

        var busyTimeout = await ScalarLongAsync(db, "PRAGMA busy_timeout;");
        AssertEx.Equal((long)SqliteDatabase.BusyTimeoutMilliseconds, busyTimeout);
    }

    private static async Task<long> ScalarLongAsync(DbContext db, string sql, params SqliteParameter[] parameters)
    {
        await using var command = db.Database.GetDbConnection().CreateCommand();
        command.CommandText = sql;
        foreach (var parameter in parameters)
        {
            command.Parameters.Add(parameter);
        }

        if (command.Connection!.State != System.Data.ConnectionState.Open)
        {
            await command.Connection.OpenAsync();
        }

        var value = await command.ExecuteScalarAsync();
        return Convert.ToInt64(value);
    }

    private static async Task<string> ScalarTextAsync(DbContext db, string sql)
    {
        await using var command = db.Database.GetDbConnection().CreateCommand();
        command.CommandText = sql;
        if (command.Connection!.State != System.Data.ConnectionState.Open)
        {
            await command.Connection.OpenAsync();
        }

        return Convert.ToString(await command.ExecuteScalarAsync()) ?? string.Empty;
    }
}

