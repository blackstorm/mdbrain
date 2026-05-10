using System.Net;
using System.Text;
using System.Text.RegularExpressions;
using Markdig;
using Markdig.Syntax.Inlines;
using Markdig.Syntax;
using Mdbrain.Data;
using Mdbrain.Storage;
using Microsoft.EntityFrameworkCore;

namespace Mdbrain.Features.App;

public sealed partial class MarkdownRenderer
{
    private readonly MarkdownPipeline pipeline = new MarkdownPipelineBuilder()
        .UseAdvancedExtensions()
        .UseAutoIdentifiers()
        .UseSoftlineBreakAsHardlineBreak()
        .UseYamlFrontMatter()
        .Build();

    public async Task<string> RenderMarkdownAsync(
        MdbrainDbContext db,
        IObjectStore store,
        string vaultId,
        string content,
        IReadOnlyList<StoredLink> links,
        CancellationToken cancellationToken)
    {
        if (string.IsNullOrWhiteSpace(content))
        {
            return string.Empty;
        }

        var (contentWithMath, formulas) = ExtractMath(content);
        var contentWithAssets = await RewriteAssetLinksAsync(db, store, contentWithMath, vaultId, cancellationToken);
        var contentWithLinks = await ReplaceObsidianLinksAsync(db, store, contentWithAssets, vaultId, links, cancellationToken);
        return RestoreMath(MdToHtml(contentWithLinks), formulas);
    }

    public string MdToHtml(string content)
    {
        var trimmed = StripYamlFrontMatter(content).Trim();
        return trimmed.Length == 0 ? string.Empty : Markdown.ToHtml(trimmed, pipeline).Trim();
    }

    public string ExtractTitle(string content)
    {
        var clean = StripYamlFrontMatter(content).Trim();
        if (clean.Length == 0)
        {
            return string.Empty;
        }

        var document = Markdown.Parse(clean, pipeline);
        var heading = document.Descendants<HeadingBlock>().FirstOrDefault(item => item.Level == 1);
        if (heading?.Inline is null)
        {
            return string.Empty;
        }

        return WebUtility.HtmlDecode(PlainText(heading.Inline)).Trim();
    }

    public string ExtractDescription(string content, int maxLength)
    {
        var body = StripYamlFrontMatter(content);
        if (string.IsNullOrWhiteSpace(body))
        {
            return string.Empty;
        }

        foreach (var rawLine in body.Split('\n'))
        {
            var line = rawLine.Trim();
            if (line.Length == 0 || line.StartsWith("#", StringComparison.Ordinal) || ImageLinePattern().IsMatch(line))
            {
                continue;
            }

            return line.EnumerateRunes().Count() > maxLength
                ? new string(line.EnumerateRunes().Take(Math.Max(maxLength, 0)).SelectMany(item => item.ToString()).ToArray())
                : line;
        }

        return string.Empty;
    }

    internal async Task<string> ReplaceObsidianLinksAsync(
        MdbrainDbContext db,
        IObjectStore store,
        string content,
        string vaultId,
        IReadOnlyList<StoredLink> links,
        CancellationToken cancellationToken)
    {
        var (masked, segments) = MaskCode(content);
        var linkIndex = links.ToDictionary(link => link.Original, StringComparer.Ordinal);
        var matches = ObsidianLinkPattern().Matches(masked);
        if (matches.Count == 0)
        {
            return UnmaskCode(masked, segments);
        }

        var builder = new StringBuilder();
        var position = 0;
        foreach (Match match in matches)
        {
            builder.Append(masked, position, match.Index - position);
            var parsed = ParseObsidianLink(match.Value);
            var normalized = NormalizeAssetPath(parsed.Path);
            if (parsed.Embed && AssetEmbed(normalized))
            {
                builder.Append(await RenderAssetEmbedAsync(db, store, vaultId, normalized, parsed.Display, cancellationToken));
            }
            else if (!parsed.Embed && AssetEmbed(normalized))
            {
                builder.Append(await RenderAssetLinkAsync(db, store, vaultId, normalized, parsed.Display, cancellationToken));
            }
            else if (linkIndex.TryGetValue(match.Value, out var link) && !string.IsNullOrWhiteSpace(link.TargetClientId))
            {
                if (link.LinkType == "embed")
                {
                    builder.Append($"""<img src="/{link.TargetClientId}" alt="{Html(link.DisplayText)}" class="obsidian-embed">""");
                }
                else
                {
                    var href = "/" + link.TargetClientId;
                    if (!string.IsNullOrWhiteSpace(parsed.Anchor))
                    {
                        href += "#" + parsed.Anchor;
                    }

                    builder.Append($"""<a href="{href}" class="internal-link" data-note-id="{link.TargetClientId}">{Html(link.DisplayText)}</a>""");
                }
            }
            else
            {
                builder.Append($"""<span class="internal-link broken" title="Note not found: {Html(parsed.Path)}">{Html(parsed.Display)}</span>""");
            }

            position = match.Index + match.Length;
        }

        builder.Append(masked, position, masked.Length - position);
        return UnmaskCode(builder.ToString(), segments);
    }

    private async Task<string> RewriteAssetLinksAsync(
        MdbrainDbContext db,
        IObjectStore store,
        string content,
        string vaultId,
        CancellationToken cancellationToken)
    {
        var (masked, segments) = MaskCode(content);
        var destinations = ExtractReferenceDefinitions(masked);
        var result = await ReplaceMatchesAsync(InlineImagePattern(), masked, async match =>
        {
            var (dest, rest) = SplitLinkDestination(match.Groups[2].Value);
            var normalized = NormalizeAssetPath(dest);
            return AssetEmbed(normalized)
                ? $"![{match.Groups[1].Value}]({await AssetUrlForAsync(db, store, vaultId, normalized, cancellationToken)}{rest})"
                : match.Value;
        });

        result = await ReplaceMatchesAsync(ReferenceImagePattern(), result, async match =>
        {
            var label = string.IsNullOrEmpty(match.Groups[2].Value) ? match.Groups[1].Value : match.Groups[2].Value;
            var destination = destinations.GetValueOrDefault(NormalizeReferenceLabel(label), string.Empty);
            return AssetEmbed(destination)
                ? $"![{match.Groups[1].Value}]({await AssetUrlForAsync(db, store, vaultId, destination, cancellationToken)})"
                : match.Value;
        });

        result = await ReplaceMatchesAsync(ReferenceDefinitionPattern(), result, async match =>
        {
            var (dest, tail) = SplitLinkDestination(match.Groups[2].Value);
            var normalized = NormalizeAssetPath(dest);
            return AssetEmbed(normalized)
                ? $"[{match.Groups[1].Value}]: {await AssetUrlForAsync(db, store, vaultId, normalized, cancellationToken)}{tail}"
                : match.Value;
        });

        result = await ReplaceMatchesAsync(HtmlMediaTagPattern(), result, async match =>
        {
            var tag = match.Value;
            var src = HtmlSrcPattern().Match(tag);
            if (!src.Success)
            {
                return tag;
            }

            var raw = FirstNonEmpty(src.Groups[1].Value, src.Groups[2].Value, src.Groups[3].Value);
            var normalized = NormalizeAssetPath(raw);
            return AssetEmbed(normalized)
                ? HtmlSrcPattern().Replace(tag, "src=\"" + await AssetUrlForAsync(db, store, vaultId, normalized, cancellationToken) + "\"", 1)
                : tag;
        });

        return UnmaskCode(result, segments);
    }

    private async Task<string> RenderAssetEmbedAsync(
        MdbrainDbContext db,
        IObjectStore store,
        string vaultId,
        string path,
        string display,
        CancellationToken cancellationToken)
    {
        var escaped = Html(display);
        var assetUrl = await AssetUrlForAsync(db, store, vaultId, path, cancellationToken);
        var lower = path.ToLowerInvariant();
        if (ImageAssetPattern().IsMatch(lower))
        {
            return $"""<img src="{assetUrl}" alt="{escaped}" class="asset-embed">""";
        }

        if (lower.EndsWith(".pdf", StringComparison.Ordinal))
        {
            return $"""<a href="{assetUrl}" class="asset-link pdf-link">{escaped}</a>""";
        }

        if (AudioAssetPattern().IsMatch(lower))
        {
            return $"""<audio src="{assetUrl}" controls class="asset-embed">{escaped}</audio>""";
        }

        if (VideoAssetPattern().IsMatch(lower))
        {
            return $"""<video src="{assetUrl}" controls class="asset-embed">{escaped}</video>""";
        }

        return $"""<a href="{assetUrl}" class="asset-link">{escaped}</a>""";
    }

    private async Task<string> RenderAssetLinkAsync(
        MdbrainDbContext db,
        IObjectStore store,
        string vaultId,
        string path,
        string display,
        CancellationToken cancellationToken)
    {
        return $"""<a href="{await AssetUrlForAsync(db, store, vaultId, path, cancellationToken)}" class="asset-link">{Html(display)}</a>""";
    }

    private async Task<string> AssetUrlForAsync(
        MdbrainDbContext db,
        IObjectStore store,
        string vaultId,
        string path,
        CancellationToken cancellationToken)
    {
        var asset = await db.Assets.AsNoTracking()
            .Where(item => item.VaultId == vaultId && item.DeletedAt == null)
            .Where(item => item.Path == path || item.Path.EndsWith("/" + path))
            .OrderBy(item => item.Path == path ? 0 : 1)
            .FirstOrDefaultAsync(cancellationToken);
        if (asset is not null)
        {
            var publicUrl = store.PublicAssetUrl(vaultId, asset.ObjectKey);
            return string.IsNullOrWhiteSpace(publicUrl) ? "/storage/" + asset.ObjectKey : publicUrl;
        }

        var escaped = Uri.EscapeDataString(path.TrimStart('/')).Replace("%2F", "/", StringComparison.OrdinalIgnoreCase);
        return "/storage/" + escaped;
    }

    internal static ParsedObsidianLink ParseObsidianLink(string link)
    {
        var embed = link.StartsWith('!');
        var inner = link.TrimStart('!').TrimPrefix("[[").TrimSuffix("]]");
        var pathPart = inner;
        var display = inner;
        var pipe = inner.IndexOf('|', StringComparison.Ordinal);
        if (pipe >= 0)
        {
            pathPart = inner[..pipe];
            display = inner[(pipe + 1)..];
        }

        var path = pathPart;
        var anchor = string.Empty;
        var hash = pathPart.IndexOf('#', StringComparison.Ordinal);
        if (hash >= 0)
        {
            path = pathPart[..hash];
            anchor = pathPart[(hash + 1)..];
        }

        return new ParsedObsidianLink(
            embed ? "embed" : "link",
            embed,
            path.Trim(),
            display.Trim(),
            anchor.Trim());
    }

    private static (string Content, IReadOnlyList<MathFormula> Formulas) ExtractMath(string content)
    {
        var formulas = new List<MathFormula>();
        content = MathBlockPattern().Replace(content, match =>
        {
            var index = formulas.Count;
            formulas.Add(new MathFormula("block", match.Groups[1].Value.Trim()));
            return $"MATHBLOCK{index}MATHBLOCK";
        });

        var output = new StringBuilder();
        for (var i = 0; i < content.Length;)
        {
            if (content[i] != '$' || i + 1 < content.Length && content[i + 1] == '$')
            {
                output.Append(content[i++]);
                continue;
            }

            var end = i + 1;
            while (end < content.Length && content[end] != '$' && content[end] != '\n')
            {
                end++;
            }

            if (end < content.Length && content[end] == '$')
            {
                var index = formulas.Count;
                formulas.Add(new MathFormula("inline", content[(i + 1)..end].Trim()));
                output.Append($"MATHINLINE{index}MATHINLINE");
                i = end + 1;
                continue;
            }

            output.Append(content[i++]);
        }

        return (output.ToString(), formulas);
    }

    private static string RestoreMath(string html, IReadOnlyList<MathFormula> formulas)
    {
        var result = html;
        for (var index = 0; index < formulas.Count; index++)
        {
            var formula = formulas[index];
            var placeholder = formula.Type == "block" ? $"MATHBLOCK{index}MATHBLOCK" : $"MATHINLINE{index}MATHINLINE";
            var replacement = formula.Type == "block"
                ? $"""<div class="math-block">{formula.Formula}</div>"""
                : $"""<span class="math-inline">{formula.Formula}</span>""";
            result = result.Replace(placeholder, replacement, StringComparison.Ordinal);
        }

        return result;
    }

    private static string StripYamlFrontMatter(string content)
    {
        if (!content.StartsWith("---", StringComparison.Ordinal))
        {
            return content;
        }

        var lines = content.Split('\n');
        for (var index = 1; index < lines.Length; index++)
        {
            if (lines[index].StartsWith("---", StringComparison.Ordinal))
            {
                return string.Join('\n', lines.Skip(index + 1));
            }
        }

        return content;
    }

    private static string NormalizeAssetPath(string path)
    {
        path = path.Trim().Trim('"', '\'', '<', '>').TrimPrefix("./").Replace('\\', '/');
        try
        {
            path = Uri.UnescapeDataString(path);
        }
        catch (UriFormatException)
        {
        }

        foreach (var separator in new[] { '?', '#' })
        {
            var index = path.IndexOf(separator);
            if (index >= 0)
            {
                path = path[..index];
            }
        }

        return path;
    }

    private static bool AssetEmbed(string path)
    {
        var lower = path.ToLowerInvariant();
        return !lower.EndsWith(".md", StringComparison.Ordinal)
            && (ImageAssetPattern().IsMatch(lower)
                || lower.EndsWith(".pdf", StringComparison.Ordinal)
                || AudioAssetPattern().IsMatch(lower)
                || VideoAssetPattern().IsMatch(lower));
    }

    private static (string Content, IReadOnlyList<string> Segments) MaskCode(string content)
    {
        var segments = new List<string>();
        string Replacer(Match match)
        {
            var index = segments.Count;
            segments.Add(match.Value);
            return $"MB_CODE_SEGMENT_{index}_MB";
        }

        content = CodeBlockPattern().Replace(content, Replacer);
        content = InlineCodePattern().Replace(content, Replacer);
        return (content, segments);
    }

    private static string UnmaskCode(string content, IReadOnlyList<string> segments)
    {
        for (var index = 0; index < segments.Count; index++)
        {
            content = content.Replace($"MB_CODE_SEGMENT_{index}_MB", segments[index], StringComparison.Ordinal);
        }

        return content;
    }

    private static (string Destination, string Tail) SplitLinkDestination(string raw)
    {
        var trimmed = raw.Trim();
        if (trimmed.StartsWith('<'))
        {
            var end = trimmed.IndexOf('>', StringComparison.Ordinal);
            return end > 0 ? (trimmed[1..end], trimmed[(end + 1)..]) : (string.Empty, string.Empty);
        }

        var space = trimmed.IndexOf(' ', StringComparison.Ordinal);
        return space < 0 ? (trimmed, string.Empty) : (trimmed[..space], trimmed[space..]);
    }

    private static Dictionary<string, string> ExtractReferenceDefinitions(string content)
    {
        return ReferenceDefinitionPattern().Matches(content)
            .ToDictionary(
                match => NormalizeReferenceLabel(match.Groups[1].Value),
                match => NormalizeAssetPath(SplitLinkDestination(match.Groups[2].Value).Destination),
                StringComparer.Ordinal);
    }

    private static string NormalizeReferenceLabel(string label)
    {
        return string.Join(' ', label.Trim().ToLowerInvariant().Split(' ', StringSplitOptions.RemoveEmptyEntries));
    }

    private static async Task<string> ReplaceMatchesAsync(Regex regex, string input, Func<Match, Task<string>> replacement)
    {
        var matches = regex.Matches(input);
        if (matches.Count == 0)
        {
            return input;
        }

        var output = new StringBuilder();
        var position = 0;
        foreach (Match match in matches)
        {
            output.Append(input, position, match.Index - position);
            output.Append(await replacement(match));
            position = match.Index + match.Length;
        }

        output.Append(input, position, input.Length - position);
        return output.ToString();
    }

    private static string FirstNonEmpty(params string[] values)
    {
        return values.FirstOrDefault(value => !string.IsNullOrWhiteSpace(value)) ?? string.Empty;
    }

    private static string Html(string value)
    {
        return WebUtility.HtmlEncode(value);
    }

    private static string PlainText(Inline? inline)
    {
        if (inline is null)
        {
            return string.Empty;
        }

        var output = new StringBuilder();
        for (var current = inline; current is not null; current = current.NextSibling)
        {
            switch (current)
            {
                case LiteralInline literal:
                    output.Append(literal.Content.ToString());
                    break;
                case CodeInline code:
                    output.Append(code.Content);
                    break;
                case LineBreakInline:
                    output.Append(' ');
                    break;
                case ContainerInline container:
                    output.Append(PlainText(container.FirstChild));
                    break;
            }
        }

        return output.ToString();
    }

    [GeneratedRegex(@"\$\$([^\$]+?)\$\$", RegexOptions.Singleline)]
    private static partial Regex MathBlockPattern();
    [GeneratedRegex("(?s)(?:```|~~~).*?(?:```|~~~)")]
    private static partial Regex CodeBlockPattern();
    [GeneratedRegex("`[^`\n]*`")]
    private static partial Regex InlineCodePattern();
    [GeneratedRegex(@"(!?)\[\[([^\]]+)\]\]")]
    private static partial Regex ObsidianLinkPattern();
    [GeneratedRegex(@"!\[([^\]]*)\]\(([^)]+)\)")]
    private static partial Regex InlineImagePattern();
    [GeneratedRegex(@"!\[([^\]]*)\]\[([^\]]*)\]")]
    private static partial Regex ReferenceImagePattern();
    [GeneratedRegex(@"(?m)^\s*\[([^\]]+)\]:\s+(.+)$")]
    private static partial Regex ReferenceDefinitionPattern();
    [GeneratedRegex(@"<(img|audio|video|source)\b[^>]*>", RegexOptions.IgnoreCase)]
    private static partial Regex HtmlMediaTagPattern();
    [GeneratedRegex("""src\s*=\s*(?:"([^"]+)"|'([^']+)'|([^\s>]+))""", RegexOptions.IgnoreCase)]
    private static partial Regex HtmlSrcPattern();
    [GeneratedRegex(@".*\.(png|jpg|jpeg|gif|webp|svg|bmp|ico)$", RegexOptions.IgnoreCase)]
    private static partial Regex ImageAssetPattern();
    [GeneratedRegex(@".*\.(mp3|ogg|wav)$", RegexOptions.IgnoreCase)]
    private static partial Regex AudioAssetPattern();
    [GeneratedRegex(@".*\.(mp4|webm)$", RegexOptions.IgnoreCase)]
    private static partial Regex VideoAssetPattern();
    [GeneratedRegex(@"^(!\[.*\]\(.*\)|!\[.*\]\[.*\]|!\[\[.*\]\])$")]
    private static partial Regex ImageLinePattern();
}

public sealed record StoredLink(string Original, string TargetClientId, string TargetPath, string DisplayText, string LinkType);

internal sealed record ParsedObsidianLink(string Type, bool Embed, string Path, string Display, string Anchor);

internal sealed record MathFormula(string Type, string Formula);

file static class StringExtensions
{
    public static string TrimPrefix(this string value, string prefix)
    {
        return value.StartsWith(prefix, StringComparison.Ordinal) ? value[prefix.Length..] : value;
    }

    public static string TrimSuffix(this string value, string suffix)
    {
        return value.EndsWith(suffix, StringComparison.Ordinal) ? value[..^suffix.Length] : value;
    }
}
