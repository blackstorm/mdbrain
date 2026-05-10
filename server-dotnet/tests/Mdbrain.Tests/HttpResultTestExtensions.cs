using Microsoft.AspNetCore.Http;
using Microsoft.Extensions.DependencyInjection;

namespace Mdbrain.Tests;

internal static class HttpResultTestExtensions
{
    public static async Task<ExecutedResult> ExecuteAsync(
        this IResult result,
        DefaultHttpContext? context = null)
    {
        context ??= new DefaultHttpContext
        {
            RequestServices = new ServiceCollection().AddLogging().BuildServiceProvider()
        };

        await using var body = new MemoryStream();
        context.Response.Body = body;
        await result.ExecuteAsync(context);
        body.Position = 0;
        using var reader = new StreamReader(body);
        return new ExecutedResult(
            context.Response.StatusCode,
            context.Response.ContentType,
            await reader.ReadToEndAsync(),
            context);
    }
}

internal sealed record ExecutedResult(
    int StatusCode,
    string? ContentType,
    string Body,
    DefaultHttpContext Context);
