namespace Mdbrain.Tests;

internal sealed class EnvironmentScope : IDisposable
{
    private readonly Dictionary<string, string?> previous = [];

    public void Set(string key, string? value)
    {
        if (!previous.ContainsKey(key))
        {
            previous[key] = Environment.GetEnvironmentVariable(key);
        }

        Environment.SetEnvironmentVariable(key, value);
    }

    public void Dispose()
    {
        foreach (var item in previous)
        {
            Environment.SetEnvironmentVariable(item.Key, item.Value);
        }
    }
}

