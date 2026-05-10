namespace Mdbrain.Tests;

internal static class AssertEx
{
    public static void True(bool condition, string message)
    {
        if (!condition)
        {
            throw new TestFailureException(message);
        }
    }

    public static void Equal<T>(T expected, T actual, string? message = null)
    {
        if (!EqualityComparer<T>.Default.Equals(expected, actual))
        {
            throw new TestFailureException(message ?? $"Expected {expected}, got {actual}");
        }
    }

    public static void Contains(string expectedSubstring, string actual, string? message = null)
    {
        if (!actual.Contains(expectedSubstring, StringComparison.Ordinal))
        {
            throw new TestFailureException(message ?? $"Expected {actual} to contain {expectedSubstring}");
        }
    }

    public static void DoesNotContain(string expectedSubstring, string actual, string? message = null)
    {
        if (actual.Contains(expectedSubstring, StringComparison.Ordinal))
        {
            throw new TestFailureException(message ?? $"Expected {actual} not to contain {expectedSubstring}");
        }
    }

    public static void NotEqual<T>(T unexpected, T actual, string? message = null)
    {
        if (EqualityComparer<T>.Default.Equals(unexpected, actual))
        {
            throw new TestFailureException(message ?? $"Did not expect {unexpected}");
        }
    }
}

internal sealed class TestFailureException(string message) : Exception(message);
