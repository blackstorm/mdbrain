using Microsoft.AspNetCore.Http.HttpResults;

namespace Mdbrain.Features.Internal;

public static class InternalResults
{
    public static IResult Robots()
    {
        return Results.Text("User-agent: *\nDisallow: /\n", "text/plain; charset=utf-8");
    }

    public static IResult Health(string? headerToken, string? queryToken, string healthToken)
    {
        var token = string.IsNullOrWhiteSpace(headerToken) ? queryToken : headerToken;
        return token == healthToken
            ? Results.Text("ok", "text/plain; charset=utf-8")
            : Results.Text("unauthorized", "text/plain; charset=utf-8", statusCode: StatusCodes.Status401Unauthorized);
    }

    public static IResult DomainCheck(bool exists)
    {
        return exists
            ? Results.Text("ok", "text/plain; charset=utf-8")
            : Results.Text("not found", "text/plain; charset=utf-8", statusCode: StatusCodes.Status404NotFound);
    }
}

