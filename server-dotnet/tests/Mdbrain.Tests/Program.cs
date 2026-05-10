namespace Mdbrain.Tests;

internal static class Program
{
    public static async Task<int> Main()
    {
        var runner = new TestRunner();
        ConfigTests.Register(runner);
        DatabaseTests.Register(runner);
        SecurityTests.Register(runner);
        StorageTests.Register(runner);
        InternalResultTests.Register(runner);
        SyncTests.Register(runner);
        MarkdownTests.Register(runner);
        AppFeatureTests.Register(runner);
        ConsoleStorageTests.Register(runner);
        return await runner.RunAsync();
    }
}
