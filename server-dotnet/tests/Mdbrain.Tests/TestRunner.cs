namespace Mdbrain.Tests;

internal sealed class TestRunner
{
    private readonly List<(string Name, Func<Task> Run)> tests = [];

    public void Add(string name, Func<Task> run)
    {
        tests.Add((name, run));
    }

    public async Task<int> RunAsync()
    {
        var failed = 0;
        foreach (var test in tests)
        {
            try
            {
                await test.Run();
                Console.WriteLine($"PASS {test.Name}");
            }
            catch (Exception ex)
            {
                failed++;
                Console.WriteLine($"FAIL {test.Name}");
                Console.WriteLine(ex);
            }
        }

        Console.WriteLine($"{tests.Count - failed}/{tests.Count} passed");
        return failed == 0 ? 0 : 1;
    }
}

