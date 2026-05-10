using Mdbrain.Features.Internal;
using Microsoft.AspNetCore.Http;
using Microsoft.Extensions.DependencyInjection;

namespace Mdbrain.Tests;

internal static class InternalResultTests
{
    public static void Register(TestRunner runner)
    {
        runner.Add(nameof(HealthAcceptsHeaderAndQueryToken), HealthAcceptsHeaderAndQueryToken);
        runner.Add(nameof(RobotsReturnsDisallowAll), RobotsReturnsDisallowAll);
        runner.Add(nameof(DomainCheckMatchesExistingVault), DomainCheckMatchesExistingVault);
    }

    private static async Task HealthAcceptsHeaderAndQueryToken()
    {
        var missing = await ExecuteAsync(InternalResults.Health(null, null, "health-token"));
        AssertEx.Equal(StatusCodes.Status401Unauthorized, missing.StatusCode);
        AssertEx.Equal("unauthorized", missing.Body.Trim());

        var query = await ExecuteAsync(InternalResults.Health(null, "health-token", "health-token"));
        AssertEx.Equal(StatusCodes.Status200OK, query.StatusCode);
        AssertEx.Equal("ok", query.Body.Trim());

        var header = await ExecuteAsync(InternalResults.Health("health-token", null, "health-token"));
        AssertEx.Equal(StatusCodes.Status200OK, header.StatusCode);
        AssertEx.Equal("ok", header.Body.Trim());
    }

    private static async Task RobotsReturnsDisallowAll()
    {
        var result = await ExecuteAsync(InternalResults.Robots());

        AssertEx.Equal(StatusCodes.Status200OK, result.StatusCode);
        AssertEx.Equal("text/plain; charset=utf-8", result.ContentType);
        AssertEx.Equal("User-agent: *\nDisallow: /\n", result.Body);
    }

    private static async Task DomainCheckMatchesExistingVault()
    {
        var found = await ExecuteAsync(InternalResults.DomainCheck(exists: true));
        AssertEx.Equal(StatusCodes.Status200OK, found.StatusCode);
        AssertEx.Equal("ok", found.Body.Trim());

        var missing = await ExecuteAsync(InternalResults.DomainCheck(exists: false));
        AssertEx.Equal(StatusCodes.Status404NotFound, missing.StatusCode);
        AssertEx.Equal("not found", missing.Body.Trim());
    }

    private static async Task<(int StatusCode, string? ContentType, string Body)> ExecuteAsync(IResult result)
    {
        var context = new DefaultHttpContext
        {
            RequestServices = new ServiceCollection().AddLogging().BuildServiceProvider()
        };
        await using var body = new MemoryStream();
        context.Response.Body = body;

        await result.ExecuteAsync(context);
        body.Position = 0;
        using var reader = new StreamReader(body);
        return (context.Response.StatusCode, context.Response.ContentType, await reader.ReadToEndAsync());
    }
}
