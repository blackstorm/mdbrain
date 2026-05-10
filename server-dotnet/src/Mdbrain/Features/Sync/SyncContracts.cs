using System.Text.Json.Serialization;

namespace Mdbrain.Features.Sync;

public sealed record HashEntry(
    [property: JsonPropertyName("id")] string Id,
    [property: JsonPropertyName("hash")] string Hash);

public sealed record SyncChangesRequest(
    [property: JsonPropertyName("notes")] IReadOnlyList<HashEntry>? Notes,
    [property: JsonPropertyName("assets")] IReadOnlyList<HashEntry>? Assets);

public sealed record SyncNoteRequest(
    [property: JsonPropertyName("path")] string? Path,
    [property: JsonPropertyName("content")] string? Content,
    [property: JsonPropertyName("hash")] string? Hash,
    [property: JsonPropertyName("metadata")] Dictionary<string, object?>? Metadata,
    [property: JsonPropertyName("assets")] IReadOnlyList<HashEntry>? Assets,
    [property: JsonPropertyName("linked_notes")] IReadOnlyList<HashEntry>? LinkedNotes);

public sealed record SyncAssetRequest(
    [property: JsonPropertyName("path")] string? Path,
    [property: JsonPropertyName("contentType")] string? ContentType,
    [property: JsonPropertyName("size")] long? Size,
    [property: JsonPropertyName("hash")] string? Hash,
    [property: JsonPropertyName("content")] string? Content);

