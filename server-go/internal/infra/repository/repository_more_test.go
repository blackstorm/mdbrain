package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNoteListingSearchAndWikilinks(t *testing.T) {
	ctx := context.Background()
	repo := setupRepository(t)
	tenantID, vaultID := seedTenantVault(t, repo, "docs.example.com")

	now := time.Now().UTC()
	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "z-last.md", "note-z", strPtr("plain"), strPtr("{}"), strPtr("h-z"), &now); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "a-first.md", "note-a", strPtr("contains [[Note Z]] and alpha"), strPtr("{}"), strPtr("h-a"), &now); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "b-middle.md", "note-b", strPtr("beta body"), strPtr("{}"), strPtr("h-b"), &now); err != nil {
		t.Fatal(err)
	}

	listed, err := repo.ListNotesByVault(ctx, vaultID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 3 || listed[0].Path != "a-first.md" || listed[1].Path != "b-middle.md" || listed[2].Path != "z-last.md" {
		t.Fatalf("unexpected note order: %#v", listed)
	}

	searched, err := repo.SearchNotesByVault(ctx, vaultID, "beta")
	if err != nil {
		t.Fatal(err)
	}
	if len(searched) != 1 || searched[0].ClientID != "note-b" {
		t.Fatalf("unexpected search results: %#v", searched)
	}

	forLinkResolution, err := repo.GetNotesForLinkResolution(ctx, vaultID)
	if err != nil {
		t.Fatal(err)
	}
	if len(forLinkResolution) != 3 {
		t.Fatalf("unexpected link-resolution notes count: %d", len(forLinkResolution))
	}
}

func TestNoteLinksBacklinksAndOrphanCleanup(t *testing.T) {
	ctx := context.Background()
	repo := setupRepository(t)
	tenantID, vaultID := seedTenantVault(t, repo, "links.example.com")

	now := time.Now().UTC()
	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "source.md", "source", strPtr("S"), strPtr("{}"), strPtr("h-s"), &now); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "target.md", "target", strPtr("T"), strPtr("{}"), strPtr("h-t"), &now); err != nil {
		t.Fatal(err)
	}

	if err := repo.InsertNoteLink(ctx, vaultID, "source", "target", "target.md", "link", "Target", "[[target]]"); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertNoteLink(ctx, vaultID, "source", "ghost", "ghost.md", "link", "Ghost", "[[ghost]]"); err != nil {
		t.Fatal(err)
	}

	links, err := repo.GetNoteLinks(ctx, vaultID, "source")
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 {
		t.Fatalf("unexpected links: %#v", links)
	}

	backlinks, err := repo.GetBacklinksWithNotes(ctx, vaultID, "target")
	if err != nil {
		t.Fatal(err)
	}
	if len(backlinks) != 1 || backlinks[0].ClientID != "source" {
		t.Fatalf("unexpected backlinks: %#v", backlinks)
	}

	if err := repo.DeleteNoteLinkByTarget(ctx, vaultID, "source", "target"); err != nil {
		t.Fatal(err)
	}
	links, err = repo.GetNoteLinks(ctx, vaultID, "source")
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].TargetClientID != "ghost" {
		t.Fatalf("unexpected links after delete by target: %#v", links)
	}

	if err := repo.DeleteOrphanLinks(ctx, vaultID); err != nil {
		t.Fatal(err)
	}
	links, err = repo.GetNoteLinks(ctx, vaultID, "source")
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Fatalf("expected orphan links deleted, got %#v", links)
	}

	if err := repo.InsertNoteLink(ctx, vaultID, "source", "target", "target.md", "link", "Target", "[[target]]"); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteNoteLinksBySource(ctx, vaultID, "source"); err != nil {
		t.Fatal(err)
	}
	links, err = repo.GetNoteLinks(ctx, vaultID, "source")
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Fatalf("expected no links after delete by source, got %#v", links)
	}
}

func TestAssetsRefsAndFindFallback(t *testing.T) {
	ctx := context.Background()
	repo := setupRepository(t)
	tenantID, vaultID := seedTenantVault(t, repo, "assets.example.com")

	now := time.Now().UTC()
	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "n.md", "note-1", strPtr("N"), strPtr("{}"), strPtr("h-n"), &now); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertAsset(ctx, uuid.NewString(), tenantID, vaultID, "asset-1", "images/a.png", "assets/asset-1.png", 10, "image/png", "md5-1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertAsset(ctx, uuid.NewString(), tenantID, vaultID, "asset-2", "docs/b.pdf", "assets/asset-2.pdf", 20, "application/pdf", "md5-2"); err != nil {
		t.Fatal(err)
	}

	found, err := repo.FindAsset(ctx, vaultID, "images/a.png")
	if err != nil {
		t.Fatal(err)
	}
	if found.ClientID != "asset-1" {
		t.Fatalf("unexpected exact-path asset: %#v", found)
	}

	found, err = repo.FindAsset(ctx, vaultID, "b.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if found.ClientID != "asset-2" {
		t.Fatalf("unexpected filename fallback asset: %#v", found)
	}

	size, err := repo.GetVaultStorageSize(ctx, vaultID)
	if err != nil {
		t.Fatal(err)
	}
	if size != 30 {
		t.Fatalf("unexpected vault storage size: %d", size)
	}

	if err := repo.UpdateNoteAssetRefs(ctx, vaultID, "note-1", []string{"asset-1"}); err != nil {
		t.Fatal(err)
	}
	count, err := repo.CountAssetRefs(ctx, vaultID, "asset-1")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("unexpected ref count after dedupe upsert: %d", count)
	}

	if err := repo.UpdateNoteAssetRefs(ctx, vaultID, "note-1", []string{"asset-1", "asset-2"}); err != nil {
		t.Fatal(err)
	}
	refs, err := repo.GetAssetRefsByNote(ctx, vaultID, "note-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("unexpected refs after replace: %#v", refs)
	}

	if err := repo.DeleteNoteAssetRefsByAsset(ctx, vaultID, "asset-1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteNoteAssetRefsByAsset(ctx, vaultID, "asset-2"); err != nil {
		t.Fatal(err)
	}
	refs, err = repo.GetAssetRefsByNote(ctx, vaultID, "note-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.Fatalf("unexpected refs after deletions: %#v", refs)
	}
}

func TestVaultMetadataAndCascadeDelete(t *testing.T) {
	ctx := context.Background()
	repo := setupRepository(t)
	tenantID, vaultID := seedTenantVault(t, repo, "cascade.example.com")

	root := "root-note"
	customHead := "<meta name=\"x\" content=\"1\">"
	logoKey := "site/logo/abc123.png"
	if err := repo.UpdateVaultRootNote(ctx, vaultID, &root); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateVaultCustomHeadHTML(ctx, vaultID, &customHead); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateVaultLogo(ctx, vaultID, &logoKey); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateVaultSyncKey(ctx, vaultID, "sync-new"); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordVaultPublishError(ctx, vaultID, "bad_request", "broken payload"); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordVaultPublishSuccess(ctx, vaultID); err != nil {
		t.Fatal(err)
	}

	vault, err := repo.GetVaultBySyncKey(ctx, "sync-new")
	if err != nil {
		t.Fatal(err)
	}
	if vault.RootNoteID == nil || *vault.RootNoteID != root || vault.CustomHeadHTML == nil || *vault.CustomHeadHTML != customHead || vault.LogoObjectKey == nil || *vault.LogoObjectKey != logoKey {
		t.Fatalf("unexpected vault metadata after updates: %#v", vault)
	}
	if vault.LastPublishStatus != "ok" || vault.LastPublishErrorCode != nil || vault.LastPublishErrorMessage != nil {
		t.Fatalf("unexpected publish status after success: %#v", vault)
	}

	now := time.Now().UTC()
	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "n.md", "note-1", strPtr("N"), strPtr("{}"), strPtr("h"), &now); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertAsset(ctx, uuid.NewString(), tenantID, vaultID, "asset-1", "img.png", "assets/asset-1.png", 5, "image/png", "md5-a1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateNoteAssetRefs(ctx, vaultID, "note-1", []string{"asset-1"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertNoteLink(ctx, vaultID, "note-1", "note-1", "n.md", "link", "Self", "[[n]]"); err != nil {
		t.Fatal(err)
	}

	if err := repo.DeleteVault(ctx, vaultID); err != nil {
		t.Fatal(err)
	}

	_, err = repo.GetVaultByID(ctx, vaultID)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected missing vault, got %v", err)
	}

	notes, err := repo.ListNotesByVault(ctx, vaultID)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 0 {
		t.Fatalf("expected notes to be cascade-deleted, got %#v", notes)
	}
	assets, err := repo.ListAssetsByVault(ctx, vaultID)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 0 {
		t.Fatalf("expected assets to be cascade-deleted, got %#v", assets)
	}
	links, err := repo.GetNoteLinks(ctx, vaultID, "note-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Fatalf("expected links to be cascade-deleted, got %#v", links)
	}
	refs, err := repo.GetAssetRefsByNote(ctx, vaultID, "note-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected refs to be cascade-deleted, got %#v", refs)
	}
}

func TestNotFoundNormalizationAndUniqueConstraint(t *testing.T) {
	ctx := context.Background()
	repo := setupRepository(t)

	_, err := repo.GetTenant(ctx, "missing-tenant")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected missing tenant sql.ErrNoRows, got %v", err)
	}
	_, err = repo.GetUserByUsername(ctx, "missing-user")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected missing user sql.ErrNoRows, got %v", err)
	}

	tenantID, _ := seedTenantVault(t, repo, "unique.example.com")
	err = repo.CreateVault(ctx, uuid.NewString(), tenantID, "Dup", "unique.example.com", "sync-dup")
	if err == nil {
		t.Fatal("expected unique-constraint error for duplicate domain")
	}
	if !IsUniqueConstraint(err, "vaults", "domain") {
		t.Fatalf("expected domain unique-constraint detection, got %v", err)
	}

	_, err = repo.GetNoteByClientID(ctx, "missing-vault", "missing-note")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected missing note sql.ErrNoRows, got %v", err)
	}
	_, err = repo.GetAssetByClientID(ctx, "missing-vault", "missing-asset")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected missing asset sql.ErrNoRows, got %v", err)
	}
	_, err = repo.FindAsset(ctx, "missing-vault", "missing.png")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected missing asset search sql.ErrNoRows, got %v", err)
	}
}

func seedTenantVault(t *testing.T, repo *Repository, domain string) (tenantID string, vaultID string) {
	t.Helper()

	tenantID = uuid.NewString()
	vaultID = uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenantID, "Test Org"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(context.Background(), vaultID, tenantID, "Blog", domain, "sync-"+uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	return tenantID, vaultID
}
