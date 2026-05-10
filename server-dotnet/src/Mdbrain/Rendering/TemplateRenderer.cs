using System.Net;
using System.Text;
using System.Text.RegularExpressions;
using Mdbrain.Config;

namespace Mdbrain.Rendering;

public sealed partial class TemplateRenderer(AppConfig config)
{
    private static readonly IReadOnlyDictionary<string, string> LegacyLucideAliases = new Dictionary<string, string>(StringComparer.Ordinal)
    {
        ["check-circle"] = "circle-check-big",
        ["alert-circle"] = "circle-alert",
        ["alert-triangle"] = "triangle-alert",
        ["more-vertical"] = "ellipsis-vertical"
    };

    public async Task<string> RenderAsync(string templatePath, IReadOnlyDictionary<string, object?> model, CancellationToken cancellationToken)
    {
        var fullPath = Path.Combine(config.TemplateRoot, templatePath);
        var source = await File.ReadAllTextAsync(fullPath, cancellationToken);
        source = await ApplyExtendsAsync(templatePath, source, model, cancellationToken);
        return await RenderSourceAsync(source, model, cancellationToken);
    }

    private async Task<string> ApplyExtendsAsync(
        string templatePath,
        string source,
        IReadOnlyDictionary<string, object?> model,
        CancellationToken cancellationToken)
    {
        var extends = ExtendsPattern().Match(source);
        if (!extends.Success)
        {
            return source;
        }

        var parentPath = extends.Groups[1].Value;
        var parentFullPath = Path.Combine(config.TemplateRoot, parentPath);
        var parent = await File.ReadAllTextAsync(parentFullPath, cancellationToken);
        var blocks = BlockPattern().Matches(source)
            .ToDictionary(match => match.Groups[1].Value, match => match.Groups[2].Value, StringComparer.Ordinal);
        return BlockPattern().Replace(parent, match => blocks.GetValueOrDefault(match.Groups[1].Value, match.Groups[2].Value));
    }

    private async Task<string> RenderSourceAsync(string source, IReadOnlyDictionary<string, object?> model, CancellationToken cancellationToken)
    {
        source = await RenderWithBlocksAsync(source, model, cancellationToken);
        source = await RenderForBlocksAsync(source, model, cancellationToken);
        source = RenderIfBlocks(source, model);
        source = await RenderIncludesAsync(source, model, cancellationToken);
        source = ElifPattern().Replace(source, string.Empty);
        source = ElsePattern().Replace(source, string.Empty);
        source = EndIfPattern().Replace(source, string.Empty);
        source = EndForPattern().Replace(source, string.Empty);
        source = EndWithPattern().Replace(source, string.Empty);
        source = EndBlockPattern().Replace(source, string.Empty);
        source = CommentPattern().Replace(source, string.Empty);
        source = VariablePattern().Replace(source, match => RenderVariable(match.Groups[1].Value, model));
        return source;
    }

    private async Task<string> RenderIncludesAsync(string source, IReadOnlyDictionary<string, object?> model, CancellationToken cancellationToken)
    {
        while (true)
        {
            var match = IncludePattern().Match(source);
            if (!match.Success) return source;
            var rendered = await RenderAsync(match.Groups[1].Value, model, cancellationToken);
            source = source[..match.Index] + rendered + source[(match.Index + match.Length)..];
        }
    }

    private async Task<string> RenderWithBlocksAsync(string source, IReadOnlyDictionary<string, object?> model, CancellationToken cancellationToken)
    {
        while (true)
        {
            var match = WithPattern().Match(source);
            if (!match.Success) return source;
            var scoped = new Dictionary<string, object?>(model, StringComparer.Ordinal);
            foreach (var assignment in SplitFields(match.Groups[1].Value))
            {
                var parts = assignment.Split('=', 2);
                if (parts.Length == 2)
                {
                    scoped[parts[0].Trim()] = ResolveExpression(parts[1], model);
                }
            }
            var rendered = await RenderSourceAsync(match.Groups[2].Value, scoped, cancellationToken);
            source = source[..match.Index] + rendered + source[(match.Index + match.Length)..];
        }
    }

    private async Task<string> RenderForBlocksAsync(string source, IReadOnlyDictionary<string, object?> model, CancellationToken cancellationToken)
    {
        while (true)
        {
            var tags = ForTagPattern().Matches(source);
            if (tags.Count == 0) return source;

            var stack = new Stack<Match>();
            var replaced = false;
            foreach (Match tag in tags)
            {
                if (tag.Groups["end"].Success)
                {
                    if (stack.Count == 0)
                    {
                        continue;
                    }

                    var start = stack.Pop();
                    var rendered = await RenderForBlockAsync(
                        start.Groups["name"].Value,
                        start.Groups["expr"].Value,
                        source[(start.Index + start.Length)..tag.Index],
                        model,
                        cancellationToken);
                    source = source[..start.Index] + rendered + source[(tag.Index + tag.Length)..];
                    replaced = true;
                    break;
                }

                stack.Push(tag);
            }

            if (!replaced)
            {
                return source;
            }
        }
    }

    private async Task<string> RenderForBlockAsync(
        string name,
        string expression,
        string body,
        IReadOnlyDictionary<string, object?> model,
        CancellationToken cancellationToken)
    {
        name = name.Trim();
        var items = ResolveExpression(expression, model) as IEnumerable<object?> ?? [];
        var builder = new StringBuilder();
        foreach (var item in items)
        {
            var scoped = new Dictionary<string, object?>(model, StringComparer.Ordinal) { [name] = item };
            builder.Append(await RenderSourceAsync(body, scoped, cancellationToken));
        }

        return builder.ToString();
    }

    private string RenderIfBlocks(string source, IReadOnlyDictionary<string, object?> model)
    {
        while (true)
        {
            var tags = IfTagPattern().Matches(source);
            if (tags.Count == 0) return source;

            var stack = new Stack<Match>();
            var replaced = false;
            foreach (Match tag in tags)
            {
                var keyword = tag.Groups["keyword"].Value;
                if (keyword == "if")
                {
                    stack.Push(tag);
                    continue;
                }

                if (keyword != "endif" || stack.Count == 0)
                {
                    continue;
                }

                var start = stack.Pop();
                var bodyStart = start.Index + start.Length;
                var body = source[bodyStart..tag.Index];
                var rendered = RenderIfChain(start.Groups["condition"].Value, body, model);
                source = source[..start.Index] + rendered + source[(tag.Index + tag.Length)..];
                replaced = true;
                break;
            }

            if (!replaced)
            {
                return source;
            }
        }
    }

    private string RenderIfChain(string firstCondition, string body, IReadOnlyDictionary<string, object?> model)
    {
        var branchMatches = BranchPattern().Matches(body);
        var branches = new List<(string? Condition, string Content)>();
        var cursor = 0;
        var condition = firstCondition;
        foreach (Match branch in branchMatches)
        {
            branches.Add((condition, body[cursor..branch.Index]));
            condition = branch.Groups[1].Success ? branch.Groups[1].Value : null;
            cursor = branch.Index + branch.Length;
        }

        branches.Add((condition, body[cursor..]));
        foreach (var branch in branches)
        {
            if (branch.Condition is null || Truthy(ResolveCondition(branch.Condition, model)))
            {
                return branch.Content;
            }
        }

        return string.Empty;
    }

    private string RenderVariable(string raw, IReadOnlyDictionary<string, object?> model)
    {
        var parts = raw.Split('|');
        var safe = parts[0].Trim().StartsWith("lucide_icon(", StringComparison.Ordinal)
            || parts.Skip(1).Any(filter => filter.Trim().StartsWith("safe", StringComparison.Ordinal));
        var value = ResolveFilteredExpression(raw, model);

        var output = value?.ToString() ?? string.Empty;
        return safe ? output : WebUtility.HtmlEncode(output);
    }

    private object? ResolveCondition(string raw, IReadOnlyDictionary<string, object?> model)
    {
        raw = raw.Trim();
        foreach (var op in new[] { "!=", "==", ">=", "<=", ">", "<", "=" })
        {
            var parts = raw.Split(op, 2, StringSplitOptions.TrimEntries);
            if (parts.Length != 2) continue;
            var left = ResolveFilteredExpression(parts[0], model);
            var right = ResolveFilteredExpression(parts[1], model);
            var compare = string.CompareOrdinal(left?.ToString(), right?.ToString());
            return op switch
            {
                "!=" => compare != 0,
                "==" or "=" => compare == 0,
                ">" => compare > 0,
                "<" => compare < 0,
                ">=" => compare >= 0,
                "<=" => compare <= 0,
                _ => false
            };
        }

        return ResolveFilteredExpression(raw, model);
    }

    private object? ResolveFilteredExpression(string raw, IReadOnlyDictionary<string, object?> model)
    {
        var parts = raw.Split('|');
        var value = ResolveExpression(parts[0], model);
        foreach (var filterRaw in parts.Skip(1))
        {
            var filter = filterRaw.Trim();
            if (filter.StartsWith("default:", StringComparison.Ordinal))
            {
                if (value is null || value is string text && text.Length == 0)
                {
                    value = Unquote(filter["default:".Length..]);
                }
            }
            else if (filter == "length")
            {
                value = Length(value);
            }
            else if (filter == "lower")
            {
                value = value?.ToString()?.ToLowerInvariant() ?? string.Empty;
            }
            else if (filter == "not-empty")
            {
                value = Length(value) > 0;
            }
        }

        return value;
    }

    private object? ResolveExpression(string raw, IReadOnlyDictionary<string, object?> model)
    {
        raw = raw.Trim();
        if (raw.Length == 0) return string.Empty;
        if (raw is "true" or "false") return raw == "true";
        if ((raw[0] == '"' && raw[^1] == '"') || (raw[0] == '\'' && raw[^1] == '\'')) return Unquote(raw);
        if (int.TryParse(raw, out var number)) return number;
        if (raw.StartsWith("lucide_icon(", StringComparison.Ordinal)) return RenderLucideIcon(raw, model);

        object? current = model;
        foreach (var part in raw.Split('.'))
        {
            if (current is null) return null;
            current = ResolvePart(current, part);
        }
        return current;
    }

    private static object? ResolvePart(object value, string part)
    {
        if (value is IReadOnlyDictionary<string, object?> dict)
        {
            return dict.TryGetValue(part, out var direct) ? direct :
                dict.TryGetValue(part.Replace("-", "_", StringComparison.Ordinal), out var snake) ? snake :
                dict.TryGetValue(part.Replace("_", "-", StringComparison.Ordinal), out var kebab) ? kebab : null;
        }
        if (value is IDictionary<string, object?> mutable)
        {
            return mutable.TryGetValue(part, out var direct) ? direct : null;
        }
        if (int.TryParse(part, out var index) && value is System.Collections.IEnumerable enumerable)
        {
            return enumerable.Cast<object?>().ElementAtOrDefault(index);
        }

        var propertyName = string.Concat(part.Split('-', '_').Select(segment => segment.Length == 0 ? segment : char.ToUpperInvariant(segment[0]) + segment[1..]));
        return value.GetType().GetProperty(propertyName)?.GetValue(value);
    }

    private string RenderLucideIcon(string raw, IReadOnlyDictionary<string, object?> model)
    {
        var inner = raw["lucide_icon(".Length..^1];
        var arguments = SplitArguments(inner).ToArray();
        if (arguments.Length == 0)
        {
            return string.Empty;
        }

        var icon = ResolveExpression(arguments[0], model)?.ToString()?.Trim() ?? string.Empty;
        var className = arguments.Length > 1 ? ResolveExpression(arguments[1], model)?.ToString()?.Trim() ?? string.Empty : string.Empty;
        var ariaLabel = arguments.Length > 2 ? ResolveExpression(arguments[2], model)?.ToString()?.Trim() ?? string.Empty : string.Empty;
        if (!IconNamePattern().IsMatch(icon))
        {
            return string.Empty;
        }

        var loaded = LoadIcon(icon);
        if (loaded is null)
        {
            return string.Empty;
        }

        var (name, svg) = loaded.Value;
        var normalizedClass = NormalizeClass($"lucide lucide-{name} {className}");
        var attrs = normalizedClass.Length == 0 ? string.Empty : " class=\"" + EscapeAttr(normalizedClass) + "\"";
        attrs += " data-lucide=\"" + EscapeAttr(name) + "\"";
        attrs += string.IsNullOrWhiteSpace(ariaLabel)
            ? " aria-hidden=\"true\" focusable=\"false\""
            : " role=\"img\" aria-label=\"" + EscapeAttr(ariaLabel) + "\"";
        return SvgOpenTagPattern().Replace(svg, "<svg" + attrs, 1);
    }

    private (string Name, string Svg)? LoadIcon(string iconName)
    {
        var name = iconName;
        var path = Path.Combine(config.TemplateRoot, "templates", "lucide", name + ".svg");
        if (!File.Exists(path) && LegacyLucideAliases.TryGetValue(iconName, out var alias))
        {
            name = alias;
            path = Path.Combine(config.TemplateRoot, "templates", "lucide", name + ".svg");
        }

        return File.Exists(path) ? (name, File.ReadAllText(path)) : null;
    }

    private static bool Truthy(object? value)
    {
        return value switch
        {
            null => false,
            bool b => b,
            string s => s.Length > 0,
            int i => i != 0,
            long l => l != 0,
            System.Collections.ICollection c => c.Count > 0,
            System.Collections.IEnumerable e => e.Cast<object?>().Any(),
            _ => true
        };
    }

    private static int Length(object? value)
    {
        return value switch
        {
            null => 0,
            string s => s.Length,
            System.Collections.ICollection c => c.Count,
            System.Collections.IEnumerable e => e.Cast<object?>().Count(),
            _ => 0
        };
    }

    private static string Unquote(string raw)
    {
        raw = raw.Trim();
        return raw.Length >= 2 && (raw[0] == '"' && raw[^1] == '"' || raw[0] == '\'' && raw[^1] == '\'')
            ? raw[1..^1]
            : raw;
    }

    private static IEnumerable<string> SplitFields(string raw)
    {
        return SplitArguments(raw, spaceSeparated: true);
    }

    private static IEnumerable<string> SplitArguments(string raw, bool spaceSeparated = false)
    {
        var output = new List<string>();
        var builder = new StringBuilder();
        char? quote = null;
        foreach (var ch in raw)
        {
            if (quote is not null)
            {
                builder.Append(ch);
                if (ch == quote)
                {
                    quote = null;
                }
                continue;
            }

            if (ch is '"' or '\'')
            {
                quote = ch;
                builder.Append(ch);
                continue;
            }

            if ((!spaceSeparated && ch == ',') || (spaceSeparated && char.IsWhiteSpace(ch)))
            {
                if (builder.Length > 0)
                {
                    output.Add(builder.ToString().Trim());
                    builder.Clear();
                }
                continue;
            }

            builder.Append(ch);
        }

        if (builder.Length > 0)
        {
            output.Add(builder.ToString().Trim());
        }

        return output;
    }

    private static string NormalizeClass(string raw)
    {
        var seen = new HashSet<string>(StringComparer.Ordinal);
        return string.Join(' ', raw.Split(' ', StringSplitOptions.RemoveEmptyEntries)
            .Where(part => seen.Add(part)));
    }

    private static string EscapeAttr(string value)
    {
        return value
            .Replace("&", "&amp;", StringComparison.Ordinal)
            .Replace("\"", "&quot;", StringComparison.Ordinal)
            .Replace("<", "&lt;", StringComparison.Ordinal)
            .Replace(">", "&gt;", StringComparison.Ordinal)
            .Replace("'", "&#x27;", StringComparison.Ordinal);
    }

    [GeneratedRegex(@"\{#.*?#\}", RegexOptions.Singleline)]
    private static partial Regex CommentPattern();
    [GeneratedRegex(@"\{%\s*extends\s+""([^""]+)""\s*%\}")]
    private static partial Regex ExtendsPattern();
    [GeneratedRegex(@"\{%\s*block\s+([^\s]+)\s*%\}(.*?)\{%\s*endblock\s*%\}", RegexOptions.Singleline)]
    private static partial Regex BlockPattern();
    [GeneratedRegex(@"\{%\s*include\s+""([^""]+)""\s*%\}")]
    private static partial Regex IncludePattern();
    [GeneratedRegex(@"\{%\s*with\s+(.*?)\s*%\}(.*?)\{%\s*endwith\s*%\}", RegexOptions.Singleline)]
    private static partial Regex WithPattern();
    [GeneratedRegex(@"\{%\s*(?:(?<end>endfor)|for\s+(?<name>[^\s]+)\s+in\s+(?<expr>.*?))\s*%\}", RegexOptions.Singleline)]
    private static partial Regex ForTagPattern();
    [GeneratedRegex(@"\{%\s*(?<keyword>if|elif|else|endif)(?:\s+(?<condition>.*?))?\s*%\}", RegexOptions.Singleline)]
    private static partial Regex IfTagPattern();
    [GeneratedRegex(@"\{%\s*(?:elif\s+(.*?)|else)\s*%\}")]
    private static partial Regex BranchPattern();
    [GeneratedRegex(@"\{%\s*elif\s+.*?\s*%\}", RegexOptions.Singleline)]
    private static partial Regex ElifPattern();
    [GeneratedRegex(@"\{%\s*else\s*%\}")]
    private static partial Regex ElsePattern();
    [GeneratedRegex(@"\{%\s*endif\s*%\}")]
    private static partial Regex EndIfPattern();
    [GeneratedRegex(@"\{%\s*endfor\s*%\}")]
    private static partial Regex EndForPattern();
    [GeneratedRegex(@"\{%\s*endwith\s*%\}")]
    private static partial Regex EndWithPattern();
    [GeneratedRegex(@"\{%\s*endblock\s*%\}")]
    private static partial Regex EndBlockPattern();
    [GeneratedRegex(@"\{\{\s*(.*?)\s*\}\}", RegexOptions.Singleline)]
    private static partial Regex VariablePattern();
    [GeneratedRegex(@"^[a-z0-9-]+$")]
    private static partial Regex IconNamePattern();
    [GeneratedRegex(@"(?i)<svg\b")]
    private static partial Regex SvgOpenTagPattern();
}
