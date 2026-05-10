using System.Net;
using System.Text;
using Mdbrain.Security;
using Microsoft.AspNetCore.Http;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Primitives;

namespace Mdbrain.Tests;

internal static class SecurityTests
{
    public static void Register(TestRunner runner)
    {
        runner.Add(nameof(Pbkdf2HasherVerifiesRoundTripAndRejectsBadInput), Pbkdf2HasherVerifiesRoundTripAndRejectsBadInput);
        runner.Add(nameof(PasswordHasherVerifiesLegacyBuddyHash), PasswordHasherVerifiesLegacyBuddyHash);
        runner.Add(nameof(SessionCookieRoundTripRejectsTampering), SessionCookieRoundTripRejectsTampering);
        runner.Add(nameof(CsrfMiddlewareProtectsConsoleStateChanges), CsrfMiddlewareProtectsConsoleStateChanges);
        runner.Add(nameof(CsrfMiddlewareAllowsSafeAndNonConsoleRequests), CsrfMiddlewareAllowsSafeAndNonConsoleRequests);
    }

    private static Task Pbkdf2HasherVerifiesRoundTripAndRejectsBadInput()
    {
        var hasher = new Pbkdf2PasswordHasher();
        var encoded = hasher.Hash("correct-password");

        AssertEx.True(hasher.Verify("correct-password", encoded), "expected password to verify");
        AssertEx.True(!hasher.Verify("wrong-password", encoded), "expected mismatch to fail");
        AssertEx.True(!hasher.Verify("correct-password", "invalid"), "malformed hashes should fail closed");
        AssertEx.True(!hasher.Verify("correct-password", "pbkdf2-sha256$abc$bad$bad"), "bad pbkdf2 parts should fail closed");
        return Task.CompletedTask;
    }

    private static Task PasswordHasherVerifiesLegacyBuddyHash()
    {
        const string encoded = "bcrypt+sha512$4f69f3674b49036f12333a5db32ea405$12$9abfaaa3ff461b8cd035d41190d8013e268837304492dcd7";
        var hasher = new Pbkdf2PasswordHasher();

        AssertEx.True(hasher.Verify("password123", encoded), "expected Clojure buddy.hashers vector to verify");
        AssertEx.True(!hasher.Verify("wrong-password", encoded), "expected wrong legacy password to fail");
        AssertEx.True(!hasher.Verify("password123", "bcrypt+sha512$bad$12$ff"), "bad legacy hash should fail closed");
        return Task.CompletedTask;
    }

    private static Task SessionCookieRoundTripRejectsTampering()
    {
        var sessions = new SessionManager(TestAppConfig.Create());
        var saveContext = new DefaultHttpContext();
        sessions.Save(saveContext, new ConsoleSession
        {
            UserId = "user-1",
            TenantId = "tenant-1",
            CsrfToken = "csrf-1"
        });

        var cookie = saveContext.Response.Headers.SetCookie.FirstOrDefault()?.Split(';', 2)[0].Split('=', 2)[1];
        AssertEx.True(!string.IsNullOrWhiteSpace(cookie), "expected session cookie");

        var loadContext = new DefaultHttpContext();
        loadContext.Request.Headers.Cookie = $"{SessionManager.CookieName}={cookie}";
        var loaded = sessions.Load(loadContext);
        AssertEx.Equal("user-1", loaded.UserId);
        AssertEx.Equal("tenant-1", loaded.TenantId);
        AssertEx.Equal("csrf-1", loaded.CsrfToken);

        var tamperedContext = new DefaultHttpContext();
        tamperedContext.Request.Headers.Cookie = $"{SessionManager.CookieName}={Tamper(cookie!)}";
        var tampered = sessions.Load(tamperedContext);
        AssertEx.Equal(null, tampered.UserId);
        AssertEx.Equal(null, tampered.TenantId);
        return Task.CompletedTask;
    }

    private static async Task CsrfMiddlewareProtectsConsoleStateChanges()
    {
        var missing = await RunCsrfAsync(HttpMethods.Post, "/console/vaults", null, null, new ConsoleSession { CsrfToken = "token-1" });
        AssertEx.Equal(StatusCodes.Status403Forbidden, missing.StatusCode);
        AssertEx.True(!missing.Called, "missing token should not call next");
        AssertEx.Contains("CSRF token missing or incorrect", missing.Body);

        var wrong = await RunCsrfAsync(HttpMethods.Post, "/console/vaults", "wrong", null, new ConsoleSession { CsrfToken = "token-1" });
        AssertEx.Equal(StatusCodes.Status403Forbidden, wrong.StatusCode);
        AssertEx.True(!wrong.Called, "wrong token should not call next");

        var header = await RunCsrfAsync(HttpMethods.Post, "/console/vaults", "token-1", null, new ConsoleSession { CsrfToken = "token-1" });
        AssertEx.Equal(StatusCodes.Status200OK, header.StatusCode);
        AssertEx.True(header.Called, "matching header token should call next");

        var form = await RunCsrfAsync(HttpMethods.Put, "/console/vaults/vault-1", null, "token-1", new ConsoleSession { CsrfToken = "token-1" });
        AssertEx.Equal(StatusCodes.Status200OK, form.StatusCode);
        AssertEx.True(form.Called, "matching form token should call next");
    }

    private static async Task CsrfMiddlewareAllowsSafeAndNonConsoleRequests()
    {
        var get = await RunCsrfAsync(HttpMethods.Get, "/console/vaults", null, null, new ConsoleSession { CsrfToken = "token-1" });
        AssertEx.Equal(StatusCodes.Status200OK, get.StatusCode);
        AssertEx.True(get.Called, "safe console methods should bypass csrf");

        var sync = await RunCsrfAsync(HttpMethods.Post, "/obsidian/sync/changes", null, null, new ConsoleSession());
        AssertEx.Equal(StatusCodes.Status200OK, sync.StatusCode);
        AssertEx.True(sync.Called, "non-console paths should bypass csrf");
    }

    private static async Task<CsrfRun> RunCsrfAsync(
        string method,
        string path,
        string? headerToken,
        string? formToken,
        ConsoleSession session)
    {
        var context = new DefaultHttpContext
        {
            RequestServices = new ServiceCollection().AddLogging().BuildServiceProvider()
        };
        context.Request.Method = method;
        context.Request.Path = path;
        ConsoleMiddleware.SetSessionItems(context, session);
        if (!string.IsNullOrWhiteSpace(headerToken))
        {
            context.Request.Headers["X-CSRF-Token"] = headerToken;
        }

        if (!string.IsNullOrWhiteSpace(formToken))
        {
            context.Request.ContentType = "application/x-www-form-urlencoded";
            context.Request.Form = new FormCollection(new Dictionary<string, StringValues>
            {
                ["__anti-forgery-token"] = formToken
            });
        }

        var called = false;
        await using var body = new MemoryStream();
        context.Response.Body = body;
        await ConsoleMiddleware.CsrfMiddleware(context, () =>
        {
            called = true;
            context.Response.StatusCode = StatusCodes.Status200OK;
            return Task.CompletedTask;
        });
        body.Position = 0;
        using var reader = new StreamReader(body);
        return new CsrfRun(context.Response.StatusCode, called, await reader.ReadToEndAsync());
    }

    private static string Tamper(string cookie)
    {
        var chars = cookie.ToCharArray();
        chars[^1] = chars[^1] == 'A' ? 'B' : 'A';
        return WebUtility.UrlEncode(new string(chars));
    }

    private sealed record CsrfRun(int StatusCode, bool Called, string Body);
}
