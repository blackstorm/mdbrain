package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	entdialect "entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	dbent "mdbrain.dev/ent"
	"mdbrain.dev/ent/asset"
	"mdbrain.dev/ent/note"
	"mdbrain.dev/ent/noteassetref"
	"mdbrain.dev/ent/notelink"
	"mdbrain.dev/ent/tenant"
	"mdbrain.dev/ent/user"
	"mdbrain.dev/ent/vault"
	"mdbrain.dev/internal/domain/model"
)

type Repository struct {
	db     *sql.DB
	client *dbent.Client
}

func New(db *sql.DB) *Repository {
	driver := entsql.OpenDB(entdialect.SQLite, db)
	return &Repository{
		db:     db,
		client: dbent.NewClient(dbent.Driver(driver)),
	}
}

func (r *Repository) CreateTenant(ctx context.Context, id, name string) error {
	return r.client.Tenant.Create().
		SetID(id).
		SetName(name).
		Exec(ctx)
}

func (r *Repository) GetTenant(ctx context.Context, id string) (*model.Tenant, error) {
	entity, err := r.client.Tenant.Query().
		Where(tenant.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, normalizeNotFound(err)
	}
	return tenantModel(entity), nil
}

func (r *Repository) CreateUser(ctx context.Context, id, tenantID, username, passwordHash string) error {
	return r.client.User.Create().
		SetID(id).
		SetTenantID(tenantID).
		SetUsername(username).
		SetPasswordHash(passwordHash).
		Exec(ctx)
}

func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	entity, err := r.client.User.Query().
		Where(user.UsernameEQ(username)).
		Only(ctx)
	if err != nil {
		return nil, normalizeNotFound(err)
	}
	return userModel(entity), nil
}

func (r *Repository) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	entity, err := r.client.User.Query().
		Where(user.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, normalizeNotFound(err)
	}
	return userModel(entity), nil
}

func (r *Repository) UpdateUserPassword(ctx context.Context, userID, passwordHash string) error {
	return r.client.User.Update().
		Where(user.IDEQ(userID)).
		SetPasswordHash(passwordHash).
		Exec(ctx)
}

func (r *Repository) HasAnyUser(ctx context.Context) (bool, error) {
	return r.client.User.Query().Exist(ctx)
}

func (r *Repository) CreateVault(ctx context.Context, id, tenantID, name, domain, syncKey string) error {
	builder := r.client.Vault.Create().
		SetID(id).
		SetTenantID(tenantID).
		SetName(name).
		SetSyncKey(syncKey)
	if strings.TrimSpace(domain) != "" {
		builder.SetDomain(domain)
	}
	return builder.Exec(ctx)
}

func (r *Repository) GetVaultByID(ctx context.Context, id string) (*model.Vault, error) {
	entity, err := r.client.Vault.Query().
		Where(vault.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, normalizeNotFound(err)
	}
	return vaultModel(entity), nil
}

func (r *Repository) GetVaultByDomain(ctx context.Context, domain string) (*model.Vault, error) {
	entity, err := r.client.Vault.Query().
		Where(vault.DomainEQ(domain)).
		Only(ctx)
	if err != nil {
		return nil, normalizeNotFound(err)
	}
	return vaultModel(entity), nil
}

func (r *Repository) GetVaultBySyncKey(ctx context.Context, syncKey string) (*model.Vault, error) {
	entity, err := r.client.Vault.Query().
		Where(vault.SyncKeyEQ(syncKey)).
		Only(ctx)
	if err != nil {
		return nil, normalizeNotFound(err)
	}
	return vaultModel(entity), nil
}

func (r *Repository) ListVaultsByTenant(ctx context.Context, tenantID string) ([]model.Vault, error) {
	entities, err := r.client.Vault.Query().
		Where(vault.TenantIDEQ(tenantID)).
		Order(vault.ByCreatedAt()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return vaultModels(entities), nil
}

func (r *Repository) DeleteVault(ctx context.Context, id string) error {
	_, err := r.client.Vault.Delete().
		Where(vault.IDEQ(id)).
		Exec(ctx)
	return err
}

func (r *Repository) UpdateVaultRootNote(ctx context.Context, vaultID string, rootNoteID *string) error {
	updater := r.client.Vault.Update().
		Where(vault.IDEQ(vaultID))
	if rootNoteID == nil {
		updater.ClearRootNoteID()
	} else {
		updater.SetRootNoteID(*rootNoteID)
	}
	return updater.Exec(ctx)
}

func (r *Repository) UpdateVault(ctx context.Context, vaultID, name, domain string) error {
	return r.client.Vault.Update().
		Where(vault.IDEQ(vaultID)).
		SetName(name).
		SetDomain(domain).
		Exec(ctx)
}

func (r *Repository) UpdateVaultSyncKey(ctx context.Context, vaultID, syncKey string) error {
	return r.client.Vault.Update().
		Where(vault.IDEQ(vaultID)).
		SetSyncKey(syncKey).
		Exec(ctx)
}

func (r *Repository) RecordVaultPublishSuccess(ctx context.Context, vaultID string) error {
	now := time.Now().UTC()
	return r.client.Vault.Update().
		Where(vault.IDEQ(vaultID)).
		SetLastPublishStatus("ok").
		SetLastPublishAt(now).
		ClearLastPublishErrorCode().
		ClearLastPublishErrorMessage().
		Exec(ctx)
}

func (r *Repository) RecordVaultPublishError(ctx context.Context, vaultID, errorCode, errorMessage string) error {
	now := time.Now().UTC()
	return r.client.Vault.Update().
		Where(vault.IDEQ(vaultID)).
		SetLastPublishStatus("error").
		SetLastPublishAt(now).
		SetLastPublishErrorCode(errorCode).
		SetLastPublishErrorMessage(errorMessage).
		Exec(ctx)
}

func (r *Repository) UpdateVaultLogo(ctx context.Context, vaultID string, logoObjectKey *string) error {
	updater := r.client.Vault.Update().
		Where(vault.IDEQ(vaultID))
	if logoObjectKey == nil {
		updater.ClearLogoObjectKey()
	} else {
		updater.SetLogoObjectKey(*logoObjectKey)
	}
	return updater.Exec(ctx)
}

func (r *Repository) UpdateVaultCustomHeadHTML(ctx context.Context, vaultID string, html *string) error {
	updater := r.client.Vault.Update().
		Where(vault.IDEQ(vaultID))
	if html == nil {
		updater.ClearCustomHeadHTML()
	} else {
		updater.SetCustomHeadHTML(*html)
	}
	return updater.Exec(ctx)
}

func (r *Repository) UpsertNote(ctx context.Context, id, tenantID, vaultID, path, clientID string, content, metadata, hash *string, mtime *time.Time) error {
	mtimeValue := formatTimeStringPtr(mtime)
	now := time.Now().UTC()

	return r.client.Note.Create().
		SetID(id).
		SetTenantID(tenantID).
		SetVaultID(vaultID).
		SetPath(path).
		SetClientID(clientID).
		SetNillableContent(content).
		SetNillableMetadata(metadata).
		SetNillableHash(hash).
		SetNillableMtime(mtimeValue).
		OnConflictColumns(note.FieldVaultID, note.FieldClientID).
		Update(func(upsert *dbent.NoteUpsert) {
			upsert.UpdatePath()
			if content == nil {
				upsert.ClearContent()
			} else {
				upsert.UpdateContent()
			}
			if metadata == nil {
				upsert.ClearMetadata()
			} else {
				upsert.UpdateMetadata()
			}
			if hash == nil {
				upsert.ClearHash()
			} else {
				upsert.UpdateHash()
			}
			if mtimeValue == nil {
				upsert.ClearMtime()
			} else {
				upsert.UpdateMtime()
			}
			upsert.SetUpdatedAt(now)
		}).
		Exec(ctx)
}

func (r *Repository) DeleteNoteByClientID(ctx context.Context, vaultID, clientID string) error {
	_, err := r.client.Note.Delete().
		Where(note.VaultIDEQ(vaultID), note.ClientIDEQ(clientID)).
		Exec(ctx)
	return err
}

func (r *Repository) GetNoteByClientID(ctx context.Context, vaultID, clientID string) (*model.Note, error) {
	entity, err := r.client.Note.Query().
		Where(note.VaultIDEQ(vaultID), note.ClientIDEQ(clientID), note.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		return nil, normalizeNotFound(err)
	}
	return noteModel(entity), nil
}

func (r *Repository) ListNotesByVault(ctx context.Context, vaultID string) ([]model.Note, error) {
	entities, err := r.client.Note.Query().
		Where(note.VaultIDEQ(vaultID), note.DeletedAtIsNil()).
		Order(note.ByPath()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return noteModels(entities), nil
}

func (r *Repository) GetNotesForLinkResolution(ctx context.Context, vaultID string) ([]model.Note, error) {
	entities, err := r.client.Note.Query().
		Where(note.VaultIDEQ(vaultID), note.DeletedAtIsNil()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return noteModels(entities), nil
}

func (r *Repository) SearchNotesByVault(ctx context.Context, vaultID, query string) ([]model.Note, error) {
	builder := r.client.Note.Query().
		Where(note.VaultIDEQ(vaultID), note.DeletedAtIsNil()).
		Order(note.ByPath()).
		Limit(50)
	if strings.TrimSpace(query) != "" {
		builder.Where(note.Or(note.PathContains(query), note.ContentContains(query)))
	}
	entities, err := builder.All(ctx)
	if err != nil {
		return nil, err
	}
	return noteModels(entities), nil
}

func (r *Repository) DeleteNoteLinksBySource(ctx context.Context, vaultID, sourceClientID string) error {
	_, err := r.client.NoteLink.Delete().
		Where(notelink.VaultIDEQ(vaultID), notelink.SourceClientIDEQ(sourceClientID)).
		Exec(ctx)
	return err
}

func (r *Repository) DeleteNoteLinkByTarget(ctx context.Context, vaultID, sourceClientID, targetClientID string) error {
	_, err := r.client.NoteLink.Delete().
		Where(
			notelink.VaultIDEQ(vaultID),
			notelink.SourceClientIDEQ(sourceClientID),
			notelink.TargetClientIDEQ(targetClientID),
		).
		Exec(ctx)
	return err
}

func (r *Repository) InsertNoteLink(ctx context.Context, vaultID, sourceClientID, targetClientID, targetPath, linkType, displayText, original string) error {
	return r.client.NoteLink.Create().
		SetID(uuid.NewString()).
		SetVaultID(vaultID).
		SetSourceClientID(sourceClientID).
		SetTargetClientID(targetClientID).
		SetNillableTargetPath(nullableInput(targetPath)).
		SetLinkType(linkType).
		SetNillableDisplayText(nullableInput(displayText)).
		SetNillableOriginal(nullableInput(original)).
		Exec(ctx)
}

func (r *Repository) GetNoteLinks(ctx context.Context, vaultID, sourceClientID string) ([]model.NoteLink, error) {
	entities, err := r.client.NoteLink.Query().
		Where(notelink.VaultIDEQ(vaultID), notelink.SourceClientIDEQ(sourceClientID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return noteLinkModels(entities), nil
}

func (r *Repository) GetBacklinksWithNotes(ctx context.Context, vaultID, targetClientID string) ([]model.Backlink, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT n.id, n.tenant_id, n.vault_id, n.path, n.client_id, n.content, n.metadata, n.hash, n.mtime, n.updated_at,
		       nl.display_text, nl.link_type
		FROM note_links nl
		JOIN notes n ON n.vault_id = nl.vault_id AND n.client_id = nl.source_client_id
		WHERE nl.vault_id = ? AND nl.target_client_id = ?
		ORDER BY n.path
	`, vaultID, targetClientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Backlink
	for rows.Next() {
		item, err := scanBacklink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteOrphanLinks(ctx context.Context, vaultID string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM note_links
		WHERE vault_id = ?
		  AND NOT EXISTS (
			SELECT 1
			FROM notes
			WHERE notes.vault_id = note_links.vault_id
			  AND notes.client_id = note_links.target_client_id
		  )
	`, vaultID)
	return err
}

func (r *Repository) UpsertAsset(ctx context.Context, id, tenantID, vaultID, clientID, path, objectKey string, sizeBytes int64, contentType, md5 string) error {
	now := time.Now().UTC()
	return r.client.Asset.Create().
		SetID(id).
		SetTenantID(tenantID).
		SetVaultID(vaultID).
		SetClientID(clientID).
		SetPath(path).
		SetObjectKey(objectKey).
		SetSizeBytes(sizeBytes).
		SetContentType(contentType).
		SetMd5(md5).
		OnConflictColumns(asset.FieldVaultID, asset.FieldClientID).
		Update(func(upsert *dbent.AssetUpsert) {
			upsert.UpdatePath()
			upsert.UpdateSizeBytes()
			upsert.UpdateContentType()
			upsert.UpdateMd5()
			upsert.ClearDeletedAt()
			upsert.SetUpdatedAt(now)
		}).
		Exec(ctx)
}

func (r *Repository) GetAssetByClientID(ctx context.Context, vaultID, clientID string) (*model.Asset, error) {
	entity, err := r.client.Asset.Query().
		Where(asset.VaultIDEQ(vaultID), asset.ClientIDEQ(clientID), asset.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		return nil, normalizeNotFound(err)
	}
	return assetModel(entity), nil
}

func (r *Repository) DeleteAssetByClientID(ctx context.Context, vaultID, clientID string) error {
	_, err := r.client.Asset.Delete().
		Where(asset.VaultIDEQ(vaultID), asset.ClientIDEQ(clientID)).
		Exec(ctx)
	return err
}

func (r *Repository) GetAssetByPath(ctx context.Context, vaultID, path string) (*model.Asset, error) {
	entity, err := r.client.Asset.Query().
		Where(asset.VaultIDEQ(vaultID), asset.PathEQ(path), asset.DeletedAtIsNil()).
		First(ctx)
	if err != nil {
		return nil, normalizeNotFound(err)
	}
	return assetModel(entity), nil
}

func (r *Repository) GetAssetByFilename(ctx context.Context, vaultID, suffix string) (*model.Asset, error) {
	entity, err := r.client.Asset.Query().
		Where(asset.VaultIDEQ(vaultID), asset.PathHasSuffix(suffix), asset.DeletedAtIsNil()).
		First(ctx)
	if err != nil {
		return nil, normalizeNotFound(err)
	}
	return assetModel(entity), nil
}

func (r *Repository) FindAsset(ctx context.Context, vaultID, path string) (*model.Asset, error) {
	assetItem, err := r.GetAssetByPath(ctx, vaultID, path)
	if err == nil || !errors.Is(err, sql.ErrNoRows) {
		return assetItem, err
	}
	return r.GetAssetByFilename(ctx, vaultID, "/"+path)
}

func (r *Repository) ListAssetsByVault(ctx context.Context, vaultID string) ([]model.Asset, error) {
	entities, err := r.client.Asset.Query().
		Where(asset.VaultIDEQ(vaultID), asset.DeletedAtIsNil()).
		Order(asset.ByPath()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return assetModels(entities), nil
}

func (r *Repository) GetVaultStorageSize(ctx context.Context, vaultID string) (int64, error) {
	entities, err := r.client.Asset.Query().
		Where(asset.VaultIDEQ(vaultID), asset.DeletedAtIsNil()).
		All(ctx)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, entity := range entities {
		total += entity.SizeBytes
	}
	return total, nil
}

func (r *Repository) DeleteNoteAssetRefsByNote(ctx context.Context, vaultID, noteClientID string) error {
	_, err := r.client.NoteAssetRef.Delete().
		Where(noteassetref.VaultIDEQ(vaultID), noteassetref.NoteClientIDEQ(noteClientID)).
		Exec(ctx)
	return err
}

func (r *Repository) DeleteNoteAssetRefsByAsset(ctx context.Context, vaultID, assetClientID string) error {
	_, err := r.client.NoteAssetRef.Delete().
		Where(noteassetref.VaultIDEQ(vaultID), noteassetref.AssetClientIDEQ(assetClientID)).
		Exec(ctx)
	return err
}

func (r *Repository) GetAssetRefsByNote(ctx context.Context, vaultID, noteClientID string) ([]model.NoteAssetRef, error) {
	entities, err := r.client.NoteAssetRef.Query().
		Where(noteassetref.VaultIDEQ(vaultID), noteassetref.NoteClientIDEQ(noteClientID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return noteAssetRefModels(entities), nil
}

func (r *Repository) CountAssetRefs(ctx context.Context, vaultID, assetClientID string) (int, error) {
	return r.client.NoteAssetRef.Query().
		Where(noteassetref.VaultIDEQ(vaultID), noteassetref.AssetClientIDEQ(assetClientID)).
		Count(ctx)
}

func (r *Repository) UpdateNoteAssetRefs(ctx context.Context, vaultID, noteClientID string, assetClientIDs []string) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.NoteAssetRef.Delete().
		Where(noteassetref.VaultIDEQ(vaultID), noteassetref.NoteClientIDEQ(noteClientID)).
		Exec(ctx); err != nil {
		return err
	}

	for _, assetClientID := range assetClientIDs {
		if err := tx.NoteAssetRef.Create().
			SetID(uuid.NewString()).
			SetVaultID(vaultID).
			SetNoteClientID(noteClientID).
			SetAssetClientID(assetClientID).
			OnConflictColumns(noteassetref.FieldVaultID, noteassetref.FieldNoteClientID, noteassetref.FieldAssetClientID).
			DoNothing().
			Exec(ctx); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func tenantModel(entity *dbent.Tenant) *model.Tenant {
	return &model.Tenant{
		ID:        entity.ID,
		Name:      entity.Name,
		CreatedAt: entity.CreatedAt,
	}
}

func userModel(entity *dbent.User) *model.User {
	return &model.User{
		ID:           entity.ID,
		TenantID:     entity.TenantID,
		Username:     entity.Username,
		PasswordHash: entity.PasswordHash,
		Role:         entity.Role,
		CreatedAt:    entity.CreatedAt,
	}
}

func vaultModel(entity *dbent.Vault) *model.Vault {
	return &model.Vault{
		ID:                      entity.ID,
		TenantID:                entity.TenantID,
		Name:                    entity.Name,
		Domain:                  cloneString(entity.Domain),
		SyncKey:                 entity.SyncKey,
		ClientType:              entity.ClientType,
		RootNoteID:              cloneString(entity.RootNoteID),
		LogoObjectKey:           cloneString(entity.LogoObjectKey),
		CustomHeadHTML:          cloneString(entity.CustomHeadHTML),
		LastPublishAt:           cloneTime(entity.LastPublishAt),
		LastPublishStatus:       entity.LastPublishStatus,
		LastPublishErrorCode:    cloneString(entity.LastPublishErrorCode),
		LastPublishErrorMessage: cloneString(entity.LastPublishErrorMessage),
		CreatedAt:               entity.CreatedAt,
	}
}

func vaultModels(entities []*dbent.Vault) []model.Vault {
	out := make([]model.Vault, 0, len(entities))
	for _, entity := range entities {
		out = append(out, *vaultModel(entity))
	}
	return out
}

func noteModel(entity *dbent.Note) *model.Note {
	return &model.Note{
		ID:        entity.ID,
		TenantID:  entity.TenantID,
		VaultID:   entity.VaultID,
		Path:      entity.Path,
		ClientID:  entity.ClientID,
		Content:   cloneString(entity.Content),
		Metadata:  cloneString(entity.Metadata),
		Hash:      cloneString(entity.Hash),
		MTime:     parseTimePtr(entity.Mtime),
		DeletedAt: cloneInt64(entity.DeletedAt),
		CreatedAt: entity.CreatedAt,
		UpdatedAt: entity.UpdatedAt,
	}
}

func noteModels(entities []*dbent.Note) []model.Note {
	out := make([]model.Note, 0, len(entities))
	for _, entity := range entities {
		out = append(out, *noteModel(entity))
	}
	return out
}

func noteLinkModel(entity *dbent.NoteLink) model.NoteLink {
	return model.NoteLink{
		ID:             entity.ID,
		VaultID:        entity.VaultID,
		SourceClientID: entity.SourceClientID,
		TargetClientID: entity.TargetClientID,
		TargetPath:     cloneString(entity.TargetPath),
		LinkType:       entity.LinkType,
		DisplayText:    cloneString(entity.DisplayText),
		Original:       cloneString(entity.Original),
		CreatedAt:      entity.CreatedAt,
		UpdatedAt:      entity.UpdatedAt,
	}
}

func noteLinkModels(entities []*dbent.NoteLink) []model.NoteLink {
	out := make([]model.NoteLink, 0, len(entities))
	for _, entity := range entities {
		out = append(out, noteLinkModel(entity))
	}
	return out
}

func assetModel(entity *dbent.Asset) *model.Asset {
	return &model.Asset{
		ID:          entity.ID,
		TenantID:    entity.TenantID,
		VaultID:     entity.VaultID,
		ClientID:    entity.ClientID,
		Path:        entity.Path,
		ObjectKey:   entity.ObjectKey,
		SizeBytes:   entity.SizeBytes,
		ContentType: entity.ContentType,
		MD5:         entity.Md5,
		DeletedAt:   cloneInt64(entity.DeletedAt),
		CreatedAt:   entity.CreatedAt,
		UpdatedAt:   entity.UpdatedAt,
	}
}

func assetModels(entities []*dbent.Asset) []model.Asset {
	out := make([]model.Asset, 0, len(entities))
	for _, entity := range entities {
		out = append(out, *assetModel(entity))
	}
	return out
}

func noteAssetRefModel(entity *dbent.NoteAssetRef) model.NoteAssetRef {
	return model.NoteAssetRef{
		ID:            entity.ID,
		VaultID:       entity.VaultID,
		NoteClientID:  entity.NoteClientID,
		AssetClientID: entity.AssetClientID,
		CreatedAt:     entity.CreatedAt,
	}
}

func noteAssetRefModels(entities []*dbent.NoteAssetRef) []model.NoteAssetRef {
	out := make([]model.NoteAssetRef, 0, len(entities))
	for _, entity := range entities {
		out = append(out, noteAssetRefModel(entity))
	}
	return out
}

func normalizeNotFound(err error) error {
	if dbent.IsNotFound(err) {
		return sql.ErrNoRows
	}
	return err
}

func cloneString(v *string) *string {
	if v == nil {
		return nil
	}
	value := *v
	return &value
}

func cloneInt64(v *int64) *int64 {
	if v == nil {
		return nil
	}
	value := *v
	return &value
}

func cloneTime(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	value := *v
	return &value
}

func nullableInput(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return &v
}

func parseTimePtr(value *string) *time.Time {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	parsed := parseTime(*value)
	return &parsed
}

func parseTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05.999999999-07:00",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func formatTimeStringPtr(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339Nano)
	return &formatted
}

func scanBacklink(rows *sql.Rows) (*model.Backlink, error) {
	var item model.Backlink
	var content, metadata, hash, mtime, updatedAt, displayText, linkType sql.NullString
	if err := rows.Scan(
		&item.ID, &item.TenantID, &item.VaultID, &item.Path, &item.ClientID,
		&content, &metadata, &hash, &mtime, &updatedAt, &displayText, &linkType,
	); err != nil {
		return nil, err
	}
	item.Content = nullableString(content)
	item.Metadata = nullableString(metadata)
	item.Hash = nullableString(hash)
	item.MTime = nullableTime(mtime)
	item.UpdatedAt = parseTime(updatedAt.String)
	item.LinkDisplayText = nullableString(displayText)
	item.LinkType = nullableString(linkType)
	return &item, nil
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	v := value.String
	return &v
}

func nullableTime(value sql.NullString) *time.Time {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	parsed := parseTime(value.String)
	return &parsed
}

func IsUniqueConstraint(err error, table, column string) bool {
	if err == nil {
		return false
	}
	if dbent.IsConstraintError(err) {
		return strings.Contains(err.Error(), fmt.Sprintf("%s.%s", table, column))
	}
	return strings.Contains(err.Error(), fmt.Sprintf("UNIQUE constraint failed: %s.%s", table, column))
}

func sortBacklinks(items []model.Backlink) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].Path < items[j].Path
	})
}
