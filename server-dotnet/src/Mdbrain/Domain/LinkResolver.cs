using System.Text.RegularExpressions;

namespace Mdbrain.Domain;

public sealed record ObsidianLink(string Original, string Path, string Display, string Anchor, string LinkType);

public sealed record NoteRef(string ClientId, string Path);

public sealed record ResolvedLink(
    string TargetClientId,
    string TargetPath,
    string LinkType,
    string DisplayText,
    string Original);

public static partial class LinkResolver
{
    public static IReadOnlyList<ObsidianLink> ExtractLinks(string content)
    {
        if (string.IsNullOrWhiteSpace(content))
        {
            return [];
        }

        return LinkPattern().Matches(content)
            .Select(match =>
            {
                var linkType = match.Groups[1].Value == "!" ? "embed" : "link";
                var inner = match.Groups[2].Value;
                var pathAndDisplay = inner.Split('|', 2);
                var pathAndAnchor = pathAndDisplay[0].Trim();
                var display = pathAndDisplay.Length == 2 ? pathAndDisplay[1].Trim() : pathAndAnchor;
                var anchorParts = pathAndAnchor.Split('#', 2);
                return new ObsidianLink(
                    match.Value,
                    anchorParts[0].Trim(),
                    display,
                    anchorParts.Length == 2 ? anchorParts[1].Trim() : string.Empty,
                    linkType);
            })
            .ToArray();
    }

    public static IReadOnlyList<ResolvedLink> ResolveLinks(IEnumerable<ObsidianLink> links, IEnumerable<NoteRef> notes)
    {
        var index = BuildNoteIndex(notes);
        return links.Select(link =>
        {
            var target = FindNote(link.Path, index);
            return new ResolvedLink(target.ClientId, link.Path, link.LinkType, link.Display, link.Original);
        }).ToArray();
    }

    public static IReadOnlyList<ResolvedLink> DeduplicateByTarget(IEnumerable<ResolvedLink> links)
    {
        var seen = new HashSet<string>(StringComparer.Ordinal);
        var output = new List<ResolvedLink>();

        foreach (var link in links)
        {
            var key = link.TargetClientId.Length > 0 ? link.TargetClientId : link.TargetPath;
            if (seen.Add(key))
            {
                output.Add(link);
            }
        }

        return output;
    }

    private static NoteRef FindNote(string path, NoteIndex index)
    {
        var normalized = NormalizePath(path);
        return index.ByFullPath.TryGetValue(normalized, out var full)
            ? full
            : index.ByFilename.GetValueOrDefault(Filename(normalized), new NoteRef(string.Empty, string.Empty));
    }

    private static NoteIndex BuildNoteIndex(IEnumerable<NoteRef> notes)
    {
        var byFullPath = new Dictionary<string, NoteRef>(StringComparer.Ordinal);
        var byFilename = new Dictionary<string, NoteRef>(StringComparer.Ordinal);

        foreach (var note in notes)
        {
            var normalized = NormalizePath(note.Path);
            byFullPath[normalized] = note;
            byFilename[Filename(normalized)] = note;
        }

        return new NoteIndex(byFullPath, byFilename);
    }

    private static string NormalizePath(string path)
    {
        return path.Trim().TrimSuffix(".md").ToLowerInvariant();
    }

    private static string Filename(string path)
    {
        var parts = path.Split('/');
        return parts[^1];
    }

    private sealed record NoteIndex(
        Dictionary<string, NoteRef> ByFullPath,
        Dictionary<string, NoteRef> ByFilename);

    [GeneratedRegex(@"(!?)\[\[([^\]]+)\]\]")]
    private static partial Regex LinkPattern();
}

internal static class StringExtensions
{
    public static string TrimSuffix(this string value, string suffix)
    {
        return value.EndsWith(suffix, StringComparison.OrdinalIgnoreCase)
            ? value[..^suffix.Length]
            : value;
    }
}

