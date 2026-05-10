using System.Text.Json;
using Mdbrain.Data;
using Mdbrain.Data.Entities;
using Mdbrain.Domain;
using Mdbrain.Storage;
using Microsoft.EntityFrameworkCore;

namespace Mdbrain.Features.Sync;

public sealed record SyncResult(int Status, object Body);

public sealed class SyncService(MdbrainDbContext db, IObjectStore store)
{
    private static readonly JsonSerializerOptions JsonOptions = new(JsonSerializerDefaults.Web);

    public async Task<SyncResult> SyncChangesAsync(Vault vault, SyncChangesRequest request, CancellationToken cancellationToken)
    {
        var serverNotes = await db.Notes.AsNoTracking()
            .Where(note => note.VaultId == vault.Id && note.DeletedAt == null)
            .OrderBy(note => note.Path)
            .ToListAsync(cancellationToken);
        var serverAssets = await db.Assets.AsNoTracking()
            .Where(asset => asset.VaultId == vault.Id && asset.DeletedAt == null)
            .OrderBy(asset => asset.Path)
            .ToListAsync(cancellationToken);

        var clientNotes = (request.Notes ?? []).ToDictionary(item => item.Id, item => item.Hash, StringComparer.Ordinal);
        var clientAssets = (request.Assets ?? []).ToDictionary(item => item.Id, item => item.Hash, StringComparer.Ordinal);
        var serverNoteHashes = serverNotes.ToDictionary(note => note.ClientId, note => note.Hash ?? string.Empty, StringComparer.Ordinal);
        var serverAssetHashes = serverAssets.ToDictionary(asset => asset.ClientId, asset => asset.Md5, StringComparer.Ordinal);

        var notesToDelete = serverNoteHashes
            .Where(item => !clientNotes.ContainsKey(item.Key))
            .Select(item => new HashEntry(item.Key, item.Value))
            .OrderBy(item => item.Id, StringComparer.Ordinal)
            .ToArray();
        var assetsToDelete = serverAssetHashes
            .Where(item => !clientAssets.ContainsKey(item.Key))
            .Select(item => new HashEntry(item.Key, item.Value))
            .OrderBy(item => item.Id, StringComparer.Ordinal)
            .ToArray();

        var notesToUpsert = clientNotes
            .Where(item => serverNoteHashes.GetValueOrDefault(item.Key) != item.Value)
            .Select(item => new HashEntry(item.Key, item.Value))
            .OrderBy(item => item.Id, StringComparer.Ordinal)
            .ToArray();
        var assetsToUpsert = clientAssets
            .Where(item => serverAssetHashes.GetValueOrDefault(item.Key) != item.Value)
            .Select(item => new HashEntry(item.Key, item.Value))
            .OrderBy(item => item.Id, StringComparer.Ordinal)
            .ToArray();

        foreach (var item in notesToDelete)
        {
            await DeleteNoteAsync(vault.Id, item.Id, cancellationToken);
        }

        foreach (var item in assetsToDelete)
        {
            await DeleteAssetAsync(vault.Id, item.Id, cancellationToken);
        }

        return Ok(new
        {
            need_upsert = new { notes = notesToUpsert, assets = assetsToUpsert },
            deleted_on_server = new { notes = notesToDelete, assets = assetsToDelete }
        });
    }

    public async Task<SyncResult> SyncNoteAsync(Vault vault, string noteId, SyncNoteRequest request, CancellationToken cancellationToken)
    {
        var notePath = (request.Path ?? string.Empty).Trim();
        var noteHash = (request.Hash ?? string.Empty).Trim();
        if (string.IsNullOrWhiteSpace(noteId)) return BadRequest("Missing note id");
        if (notePath.Length == 0) return BadRequest("Missing note path");
        if (noteHash.Length == 0) return BadRequest("Missing note hash");
        if (request.Content is null) return BadRequest("Missing note content");
        if (request.Assets is null) return BadRequest("Missing assets");
        if (request.LinkedNotes is null) return BadRequest("Missing linked notes");

        var existing = await db.Notes.AsNoTracking()
            .FirstOrDefaultAsync(note => note.VaultId == vault.Id && note.ClientId == noteId && note.DeletedAt == null, cancellationToken);
        if (existing is not null && (existing.Hash ?? string.Empty) == noteHash && existing.Path == notePath)
        {
            return Ok(new { status = "skipped", noteId });
        }

        var metadataJson = request.Metadata is null ? null : JsonSerializer.Serialize(request.Metadata, JsonOptions);
        await UpsertNoteWithLinksAsync(vault.TenantId, vault.Id, noteId, notePath, request.Content, noteHash, metadataJson, cancellationToken);
        await UpdateNoteAssetRefsAsync(vault.Id, noteId, request.Assets.Select(item => item.Id), cancellationToken);

        var assetIds = request.Assets.Select(item => item.Id).ToArray();
        var existingAssets = await db.Assets.AsNoTracking()
            .Where(asset => asset.VaultId == vault.Id && assetIds.Contains(asset.ClientId) && asset.DeletedAt == null)
            .ToDictionaryAsync(asset => asset.ClientId, asset => asset.Md5, cancellationToken);
        var needUploadAssets = request.Assets
            .Where(entry => existingAssets.GetValueOrDefault(entry.Id) != entry.Hash)
            .ToArray();

        var linkedNoteIds = request.LinkedNotes.Select(item => item.Id).ToArray();
        var existingNotes = await db.Notes.AsNoTracking()
            .Where(note => note.VaultId == vault.Id && linkedNoteIds.Contains(note.ClientId) && note.DeletedAt == null)
            .ToDictionaryAsync(note => note.ClientId, note => note.Hash ?? string.Empty, cancellationToken);
        var needUploadNotes = request.LinkedNotes
            .Where(entry => existingNotes.GetValueOrDefault(entry.Id) != entry.Hash)
            .ToArray();

        return Ok(new
        {
            status = "stored",
            noteId,
            need_upload_assets = needUploadAssets,
            need_upload_notes = needUploadNotes
        });
    }

    public async Task<SyncResult> SyncAssetAsync(Vault vault, string assetId, SyncAssetRequest request, CancellationToken cancellationToken)
    {
        var assetPath = (request.Path ?? string.Empty).Trim();
        var assetHash = (request.Hash ?? string.Empty).Trim();
        var contentType = (request.ContentType ?? string.Empty).Trim();
        if (string.IsNullOrWhiteSpace(assetId)) return BadRequest("Missing asset id");
        if (assetPath.Length == 0) return BadRequest("Missing asset path");
        if (assetHash.Length == 0) return BadRequest("Missing asset hash");
        if (contentType.Length == 0) return BadRequest("Missing asset contentType");

        var existing = await db.Assets.AsNoTracking()
            .FirstOrDefaultAsync(asset => asset.VaultId == vault.Id && asset.ClientId == assetId && asset.DeletedAt == null, cancellationToken);
        if (existing is not null && existing.Md5 == assetHash && existing.Path == assetPath)
        {
            return Ok(new { status = "skipped", assetId });
        }

        if (request.Content is null) return BadRequest("Missing asset content");
        byte[] bytes;
        try
        {
            bytes = Convert.FromBase64String(request.Content);
        }
        catch (FormatException)
        {
            return BadRequest("Missing asset content");
        }

        var objectKey = ObjectKeys.AssetObjectKey(assetId, ObjectKeys.ExtensionFromPath(assetPath));
        await store.PutObjectAsync(vault.Id, objectKey, bytes, contentType, cancellationToken);
        await UpsertAssetAsync(vault.TenantId, vault.Id, assetId, assetPath, objectKey, request.Size ?? bytes.Length, contentType, assetHash, cancellationToken);

        return Ok(new { status = "stored", assetId });
    }

    public async Task RecordPublishResultAsync(Vault vault, SyncResult result, CancellationToken cancellationToken)
    {
        var current = await db.Vaults.FirstAsync(item => item.Id == vault.Id, cancellationToken);
        current.LastPublishAt = DateTimeOffset.UtcNow;
        if (result.Status < 400)
        {
            current.LastPublishStatus = "ok";
            current.LastPublishErrorCode = null;
            current.LastPublishErrorMessage = null;
        }
        else
        {
            current.LastPublishStatus = "error";
            current.LastPublishErrorCode = result.Status >= 500 ? "server_error" : "bad_request";
            current.LastPublishErrorMessage = PublishErrorMessage(result.Body);
        }

        await db.SaveChangesAsync(cancellationToken);
    }

    private async Task UpsertNoteWithLinksAsync(
        string tenantId,
        string vaultId,
        string noteId,
        string path,
        string content,
        string hash,
        string? metadata,
        CancellationToken cancellationToken)
    {
        var now = DateTimeOffset.UtcNow;
        var note = await db.Notes.FirstOrDefaultAsync(item => item.VaultId == vaultId && item.ClientId == noteId, cancellationToken);
        if (note is null)
        {
            db.Notes.Add(new Note
            {
                Id = Guid.NewGuid().ToString(),
                TenantId = tenantId,
                VaultId = vaultId,
                Path = path,
                ClientId = noteId,
                Content = content,
                Metadata = metadata,
                Hash = hash,
                UpdatedAt = now
            });
        }
        else
        {
            note.Path = path;
            note.Content = content;
            note.Metadata = metadata;
            note.Hash = hash;
            note.Mtime = null;
            note.UpdatedAt = now;
        }

        await db.SaveChangesAsync(cancellationToken);
        await RefreshNoteLinksAsync(vaultId, noteId, content, cancellationToken);
    }

    private async Task RefreshNoteLinksAsync(string vaultId, string noteId, string content, CancellationToken cancellationToken)
    {
        var noteRefs = await db.Notes.AsNoTracking()
            .Where(note => note.VaultId == vaultId && note.DeletedAt == null)
            .Select(note => new NoteRef(note.ClientId, note.Path))
            .ToListAsync(cancellationToken);

        var newLinks = LinkResolver.DeduplicateByTarget(
                LinkResolver.ResolveLinks(LinkResolver.ExtractLinks(content), noteRefs)
                    .Where(item => item.TargetClientId.Length > 0))
            .ToDictionary(link => link.TargetClientId, StringComparer.Ordinal);

        var existingLinks = await db.NoteLinks
            .Where(link => link.VaultId == vaultId && link.SourceClientId == noteId)
            .ToListAsync(cancellationToken);
        var existingByTarget = existingLinks.ToDictionary(link => link.TargetClientId, StringComparer.Ordinal);

        foreach (var existing in existingLinks)
        {
            if (!newLinks.TryGetValue(existing.TargetClientId, out var fresh)
                || existing.TargetPath != NullIfWhiteSpace(fresh.TargetPath)
                || existing.LinkType != fresh.LinkType
                || existing.DisplayText != NullIfWhiteSpace(fresh.DisplayText)
                || existing.Original != NullIfWhiteSpace(fresh.Original))
            {
                db.NoteLinks.Remove(existing);
            }
        }

        foreach (var fresh in newLinks.Values)
        {
            if (existingByTarget.TryGetValue(fresh.TargetClientId, out var existing)
                && existing.TargetPath == NullIfWhiteSpace(fresh.TargetPath)
                && existing.LinkType == fresh.LinkType
                && existing.DisplayText == NullIfWhiteSpace(fresh.DisplayText)
                && existing.Original == NullIfWhiteSpace(fresh.Original))
            {
                continue;
            }

            db.NoteLinks.Add(new NoteLink
            {
                Id = Guid.NewGuid().ToString(),
                VaultId = vaultId,
                SourceClientId = noteId,
                TargetClientId = fresh.TargetClientId,
                TargetPath = NullIfWhiteSpace(fresh.TargetPath),
                LinkType = fresh.LinkType,
                DisplayText = NullIfWhiteSpace(fresh.DisplayText),
                Original = NullIfWhiteSpace(fresh.Original)
            });
        }

        await db.SaveChangesAsync(cancellationToken);
    }

    private async Task UpsertAssetAsync(
        string tenantId,
        string vaultId,
        string clientId,
        string path,
        string objectKey,
        long size,
        string contentType,
        string md5,
        CancellationToken cancellationToken)
    {
        var now = DateTimeOffset.UtcNow;
        var asset = await db.Assets.FirstOrDefaultAsync(item => item.VaultId == vaultId && item.ClientId == clientId, cancellationToken);
        if (asset is null)
        {
            db.Assets.Add(new Asset
            {
                Id = Guid.NewGuid().ToString(),
                TenantId = tenantId,
                VaultId = vaultId,
                ClientId = clientId,
                Path = path,
                ObjectKey = objectKey,
                SizeBytes = size,
                ContentType = contentType,
                Md5 = md5,
                UpdatedAt = now
            });
        }
        else
        {
            asset.Path = path;
            asset.ObjectKey = objectKey;
            asset.SizeBytes = size;
            asset.ContentType = contentType;
            asset.Md5 = md5;
            asset.DeletedAt = null;
            asset.UpdatedAt = now;
        }

        await db.SaveChangesAsync(cancellationToken);
    }

    private async Task UpdateNoteAssetRefsAsync(string vaultId, string noteId, IEnumerable<string> assetIds, CancellationToken cancellationToken)
    {
        var orderedNewIds = assetIds.Distinct(StringComparer.Ordinal).ToArray();
        var existingRefs = await db.NoteAssetRefs
            .Where(item => item.VaultId == vaultId && item.NoteClientId == noteId)
            .ToListAsync(cancellationToken);
        var existingIds = existingRefs.Select(item => item.AssetClientId).ToHashSet(StringComparer.Ordinal);

        db.NoteAssetRefs.RemoveRange(existingRefs);
        foreach (var assetId in orderedNewIds)
        {
            db.NoteAssetRefs.Add(new NoteAssetRef
            {
                Id = Guid.NewGuid().ToString(),
                VaultId = vaultId,
                NoteClientId = noteId,
                AssetClientId = assetId
            });
        }

        await db.SaveChangesAsync(cancellationToken);

        foreach (var removedId in existingIds.Except(orderedNewIds, StringComparer.Ordinal))
        {
            if (!await db.NoteAssetRefs.AnyAsync(item => item.VaultId == vaultId && item.AssetClientId == removedId, cancellationToken))
            {
                await DeleteAssetAsync(vaultId, removedId, cancellationToken);
            }
        }
    }

    private async Task DeleteNoteAsync(string vaultId, string noteId, CancellationToken cancellationToken)
    {
        await db.NoteLinks.Where(item => item.VaultId == vaultId && item.SourceClientId == noteId).ExecuteDeleteAsync(cancellationToken);
        await db.NoteAssetRefs.Where(item => item.VaultId == vaultId && item.NoteClientId == noteId).ExecuteDeleteAsync(cancellationToken);
        await db.Notes.Where(item => item.VaultId == vaultId && item.ClientId == noteId).ExecuteDeleteAsync(cancellationToken);
        await db.NoteLinks
            .Where(link => link.VaultId == vaultId && !db.Notes.Any(note => note.VaultId == link.VaultId && note.ClientId == link.TargetClientId))
            .ExecuteDeleteAsync(cancellationToken);
    }

    private async Task DeleteAssetAsync(string vaultId, string assetId, CancellationToken cancellationToken)
    {
        var asset = await db.Assets.FirstOrDefaultAsync(item => item.VaultId == vaultId && item.ClientId == assetId && item.DeletedAt == null, cancellationToken);
        if (asset is null)
        {
            return;
        }

        await store.DeleteObjectAsync(vaultId, asset.ObjectKey, cancellationToken);
        await db.NoteAssetRefs.Where(item => item.VaultId == vaultId && item.AssetClientId == assetId).ExecuteDeleteAsync(cancellationToken);
        db.Assets.Remove(asset);
        await db.SaveChangesAsync(cancellationToken);
    }

    private static SyncResult Ok(object body)
    {
        return new SyncResult(StatusCodes.Status200OK, body);
    }

    private static SyncResult BadRequest(string message)
    {
        return new SyncResult(StatusCodes.Status400BadRequest, new { success = false, error = message });
    }

    private static string? NullIfWhiteSpace(string value)
    {
        return string.IsNullOrWhiteSpace(value) ? null : value;
    }

    private static string PublishErrorMessage(object body)
    {
        var property = body.GetType().GetProperty("error");
        if (property?.GetValue(body) is not string error || string.IsNullOrWhiteSpace(error))
        {
            return "Request failed";
        }

        return error.Length > 400 ? error[..400] : error;
    }
}

