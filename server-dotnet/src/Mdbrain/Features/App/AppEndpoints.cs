using Mdbrain.Config;
using Mdbrain.Data;
using Mdbrain.Data.Entities;
using Mdbrain.Domain;
using Mdbrain.Rendering;
using Mdbrain.Storage;
using Microsoft.EntityFrameworkCore;

namespace Mdbrain.Features.App;

public static class AppEndpoints
{
    public static void MapAppEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/favicon.ico", ServeFavicon);
        app.MapGet("/storage/{**path}", ServeAsset);
        app.MapGet("/{**path}", GetNote);
    }

    private static async Task<IResult> GetNote(
        HttpContext context,
        AppConfig config,
        string? path,
        MdbrainDbContext db,
        IObjectStore store,
        TemplateRenderer renderer,
        MarkdownRenderer markdown,
        CancellationToken cancellationToken)
    {
        if (AppEndpointGuards.RequestPort(context) == config.ConsolePort)
        {
            return Results.NotFound("Not found");
        }

        if (AppEndpointGuards.IsConsoleOnlyPath(context.Request.Path) || AppEndpointGuards.IsAppStaticPath(context.Request.Path))
        {
            return Results.NotFound("Not found");
        }

        var vaultResult = await RequireVaultAsync(context, db, cancellationToken);
        if (vaultResult.Result is not null)
        {
            return vaultResult.Result;
        }

        var vault = vaultResult.Vault!;
        var vaultData = PublicVault(vault, store);
        var isHtmx = !string.IsNullOrWhiteSpace(context.Request.Headers["HX-Request"].FirstOrDefault());
        var pathClientIds = ParsePathIds(path ?? string.Empty);
        if (pathClientIds.Count == 0)
        {
            if (!string.IsNullOrWhiteSpace(vault.RootNoteId))
            {
                var rootNote = await db.Notes.AsNoTracking()
                    .FirstOrDefaultAsync(item => item.VaultId == vault.Id && item.ClientId == vault.RootNoteId && item.DeletedAt == null, cancellationToken);
                if (rootNote is not null)
                {
                    var renderData = await PrepareNoteDataAsync(db, store, markdown, rootNote, vault.Id, cancellationToken);
                    var description = markdown.ExtractDescription(rootNote.Content ?? string.Empty, 160);
                    if (isHtmx)
                    {
                        return Html(await renderer.RenderAsync("templates/app/note.html", renderData, cancellationToken));
                    }

                    return Html(await renderer.RenderAsync("templates/app/note-page.html", new Dictionary<string, object?>
                    {
                        ["notes"] = new[] { renderData },
                        ["vault"] = vaultData,
                        ["description"] = description
                    }, cancellationToken));
                }
            }

            var notes = await db.Notes.AsNoTracking()
                .Where(item => item.VaultId == vault.Id && item.DeletedAt == null)
                .OrderBy(item => item.Path)
                .ToListAsync(cancellationToken);
            return Html(await renderer.RenderAsync("templates/app/home.html", new Dictionary<string, object?>
            {
                ["vault"] = vaultData,
                ["notes"] = NoteListData(notes)
            }, cancellationToken));
        }

        var validNotes = new List<Note>();
        var validIds = new List<string>();
        foreach (var clientId in pathClientIds)
        {
            var note = await db.Notes.AsNoTracking()
                .FirstOrDefaultAsync(item => item.VaultId == vault.Id && item.ClientId == clientId && item.DeletedAt == null, cancellationToken);
            if (note is null)
            {
                continue;
            }

            validNotes.Add(note);
            validIds.Add(note.ClientId);
        }

        if (validNotes.Count == 0)
        {
            return Results.NotFound("Note not found");
        }

        var needsCorrection = validIds.Count != pathClientIds.Count;
        var correctedPath = "/" + string.Join('+', validIds);
        if (isHtmx)
        {
            var lastNote = validNotes[^1];
            var renderData = await PrepareNoteDataAsync(db, store, markdown, lastNote, vault.Id, cancellationToken);
            var pushUrl = needsCorrection
                ? correctedPath
                : BuildPushUrl(
                    context.Request.Headers["HX-Current-Url"].FirstOrDefault() ?? string.Empty,
                    context.Request.Headers["X-From-Note-Id"].FirstOrDefault() ?? string.Empty,
                    lastNote.ClientId,
                    vault.RootNoteId ?? string.Empty);
            context.Response.Headers["HX-Push-Url"] = pushUrl;
            return Html(await renderer.RenderAsync("templates/app/note.html", renderData, cancellationToken));
        }

        var notesData = new List<Dictionary<string, object?>>();
        foreach (var note in validNotes)
        {
            notesData.Add(await PrepareNoteDataAsync(db, store, markdown, note, vault.Id, cancellationToken));
        }

        var page = await renderer.RenderAsync("templates/app/note-page.html", new Dictionary<string, object?>
        {
            ["notes"] = notesData,
            ["vault"] = vaultData,
            ["description"] = markdown.ExtractDescription(validNotes[0].Content ?? string.Empty, 160)
        }, cancellationToken);
        if (needsCorrection)
        {
            context.Response.Headers["HX-Replace-Url"] = correctedPath;
        }

        return Html(page);
    }

    private static async Task<IResult> ServeAsset(
        HttpContext context,
        string? path,
        AppConfig config,
        MdbrainDbContext db,
        IObjectStore store,
        CancellationToken cancellationToken)
    {
        var objectKey = (path ?? string.Empty).Trim('/');
        if (objectKey.Length == 0)
        {
            return Results.BadRequest("Bad request");
        }

        if (config.StorageType == "s3")
        {
            return Results.NotFound("Not found");
        }

        var vaultResult = await RequireVaultAsync(context, db, cancellationToken);
        if (vaultResult.Result is not null)
        {
            return vaultResult.Result;
        }

        await using var stored = await store.GetObjectAsync(vaultResult.Vault!.Id, objectKey, cancellationToken);
        if (stored is null)
        {
            return Results.NotFound("Not found");
        }

        var bytes = await ReadAllAsync(stored.Body, cancellationToken);
        context.Response.Headers.CacheControl = "public, max-age=31536000, immutable";
        return Results.Bytes(bytes, stored.ContentType);
    }

    private static async Task<IResult> ServeFavicon(
        HttpContext context,
        MdbrainDbContext db,
        IObjectStore store,
        CancellationToken cancellationToken)
    {
        var vaultResult = await RequireVaultAsync(context, db, cancellationToken);
        if (vaultResult.Result is not null)
        {
            return vaultResult.Result;
        }

        var vault = vaultResult.Vault!;
        if (string.IsNullOrWhiteSpace(vault.LogoObjectKey))
        {
            return Results.NotFound("Not found");
        }

        var logoKey = vault.LogoObjectKey;
        var hash = ExtractLogoHash(logoKey);
        var faviconKey = ObjectKeys.FaviconObjectKey(logoKey);
        var assetKey = logoKey;
        if (!string.IsNullOrWhiteSpace(faviconKey)
            && await store.HeadObjectAsync(vault.Id, faviconKey, cancellationToken) is not null)
        {
            assetKey = faviconKey;
        }

        context.Response.Headers.CacheControl = context.Request.Query.ContainsKey("v")
            ? "public, max-age=31536000, immutable"
            : "public, max-age=300";
        context.Response.Headers.ETag = "\"" + hash + "\"";
        return Results.Redirect(store.PublicAssetUrl(vault.Id, assetKey));
    }

    private static async Task<(Vault? Vault, IResult? Result)> RequireVaultAsync(
        HttpContext context,
        MdbrainDbContext db,
        CancellationToken cancellationToken)
    {
        var parsed = HostParser.ParseHostDomain(context.Request.Host.Value ?? string.Empty);
        if (parsed.Error is "missing" or "invalid")
        {
            return (null, Results.BadRequest("Bad request"));
        }

        var vault = await db.Vaults.AsNoTracking()
            .FirstOrDefaultAsync(item => item.Domain == parsed.Domain, cancellationToken);
        return vault is null ? (null, Results.Text("Forbidden", statusCode: StatusCodes.Status403Forbidden)) : (vault, null);
    }

    private static async Task<Dictionary<string, object?>> PrepareNoteDataAsync(
        MdbrainDbContext db,
        IObjectStore store,
        MarkdownRenderer markdown,
        Note note,
        string vaultId,
        CancellationToken cancellationToken)
    {
        var links = await db.NoteLinks.AsNoTracking()
            .Where(item => item.VaultId == vaultId && item.SourceClientId == note.ClientId)
            .Select(item => new StoredLink(
                item.Original ?? string.Empty,
                item.TargetClientId,
                item.TargetPath ?? string.Empty,
                item.DisplayText ?? string.Empty,
                item.LinkType))
            .ToListAsync(cancellationToken);
        var content = note.Content ?? string.Empty;
        var title = markdown.ExtractTitle(content);
        if (string.IsNullOrWhiteSpace(title))
        {
            title = PathWithoutExtension(note.Path).Replace("/", " / ", StringComparison.Ordinal);
        }

        var backlinks = await db.NoteLinks.AsNoTracking()
            .Where(link => link.VaultId == vaultId && link.TargetClientId == note.ClientId)
            .Join(db.Notes.AsNoTracking().Where(item => item.VaultId == vaultId && item.DeletedAt == null),
                link => link.SourceClientId,
                backlink => backlink.ClientId,
                (_, backlink) => backlink)
            .OrderBy(backlink => backlink.Path)
            .ToListAsync(cancellationToken);

        var backlinkData = backlinks.Select(backlink =>
        {
            var backlinkTitle = markdown.ExtractTitle(backlink.Content ?? string.Empty);
            if (string.IsNullOrWhiteSpace(backlinkTitle))
            {
                backlinkTitle = PathWithoutExtension(backlink.Path);
            }

            return new Dictionary<string, object?>
            {
                ["client_id"] = backlink.ClientId,
                ["title"] = backlinkTitle,
                ["description"] = markdown.ExtractDescription(backlink.Content ?? string.Empty, 100)
            };
        }).ToArray();

        return new Dictionary<string, object?>
        {
            ["note"] = new Dictionary<string, object?>
            {
                ["client_id"] = note.ClientId,
                ["title"] = title,
                ["html_content"] = await markdown.RenderMarkdownAsync(db, store, vaultId, content, links, cancellationToken),
                ["path"] = note.Path,
                ["updated_at"] = note.UpdatedAt
            },
            ["backlinks"] = backlinkData
        };
    }

    private static Dictionary<string, object?> PublicVault(Vault vault, IObjectStore store)
    {
        var data = new Dictionary<string, object?>
        {
            ["id"] = vault.Id,
            ["name"] = vault.Name,
            ["domain"] = vault.Domain ?? string.Empty,
            ["root_note_id"] = vault.RootNoteId ?? string.Empty,
            ["custom_head_html"] = vault.CustomHeadHtml ?? string.Empty
        };
        if (!string.IsNullOrWhiteSpace(vault.LogoObjectKey))
        {
            data["logo_url"] = store.PublicAssetUrl(vault.Id, vault.LogoObjectKey);
            data["favicon_version"] = Uri.EscapeDataString(vault.LogoObjectKey);
            data["logo_hash"] = ExtractLogoHash(vault.LogoObjectKey);
        }

        return data;
    }

    private static IReadOnlyList<string> ParsePathIds(string path)
    {
        var normalized = path.Trim('/');
        return normalized.Length == 0 ? [] : normalized.Split('+');
    }

    public static string BuildPushUrl(string currentUrl, string fromNoteId, string targetNoteId, string rootNoteId)
    {
        var currentPath = "/";
        if (!string.IsNullOrWhiteSpace(currentUrl))
        {
            currentPath = currentUrl;
            if (Uri.TryCreate(currentUrl, UriKind.Absolute, out var uri))
            {
                currentPath = uri.PathAndQuery;
            }

            var queryIndex = currentPath.IndexOf('?', StringComparison.Ordinal);
            if (queryIndex >= 0)
            {
                currentPath = currentPath[..queryIndex];
            }
        }

        if (currentPath == "/")
        {
            return string.IsNullOrWhiteSpace(rootNoteId) ? "/" + targetNoteId : "/" + rootNoteId + "+" + targetNoteId;
        }

        var parts = currentPath.TrimStart('/').Split('+').ToList();
        if (!string.IsNullOrWhiteSpace(fromNoteId))
        {
            var index = parts.IndexOf(fromNoteId);
            if (index >= 0)
            {
                parts = parts.Take(index + 1).ToList();
            }
        }

        parts.Add(targetNoteId);
        return "/" + string.Join('+', parts);
    }

    private static IReadOnlyList<Dictionary<string, object?>> NoteListData(IEnumerable<Note> notes)
    {
        return notes.Select(note => new Dictionary<string, object?>
        {
            ["client_id"] = note.ClientId,
            ["path"] = note.Path,
            ["mtime"] = note.Mtime
        }).ToArray();
    }

    private static string ExtractLogoHash(string logoObjectKey)
    {
        var filename = Path.GetFileName(logoObjectKey);
        var extension = ObjectKeys.ExtensionFromPath(filename);
        return extension.Length == 0 ? filename : filename[..^("." + extension).Length];
    }

    private static string PathWithoutExtension(string path)
    {
        return path.EndsWith(".md", StringComparison.OrdinalIgnoreCase) ? path[..^3] : path;
    }

    private static async Task<byte[]> ReadAllAsync(Stream stream, CancellationToken cancellationToken)
    {
        using var memory = new MemoryStream();
        await stream.CopyToAsync(memory, cancellationToken);
        return memory.ToArray();
    }

    private static IResult Html(string body)
    {
        return Results.Content(body, "text/html; charset=utf-8");
    }
}
