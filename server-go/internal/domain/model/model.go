package model

import "time"

type Tenant struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

type User struct {
	ID           string
	TenantID     string
	Username     string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
}

type Vault struct {
	ID                      string
	TenantID                string
	Name                    string
	Domain                  *string
	SyncKey                 string
	ClientType              string
	RootNoteID              *string
	LogoObjectKey           *string
	CustomHeadHTML          *string
	LastPublishAt           *time.Time
	LastPublishStatus       string
	LastPublishErrorCode    *string
	LastPublishErrorMessage *string
	CreatedAt               time.Time
}

type Note struct {
	ID        string
	TenantID  string
	VaultID   string
	Path      string
	ClientID  string
	Content   *string
	Metadata  *string
	Hash      *string
	MTime     *time.Time
	DeletedAt *int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

type NoteLink struct {
	ID             string
	VaultID        string
	SourceClientID string
	TargetClientID string
	TargetPath     *string
	LinkType       string
	DisplayText    *string
	Original       *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Asset struct {
	ID          string
	TenantID    string
	VaultID     string
	ClientID    string
	Path        string
	ObjectKey   string
	SizeBytes   int64
	ContentType string
	MD5         string
	DeletedAt   *int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type NoteAssetRef struct {
	ID            string
	VaultID       string
	NoteClientID  string
	AssetClientID string
	CreatedAt     time.Time
}

type Backlink struct {
	ID              string
	TenantID        string
	VaultID         string
	Path            string
	ClientID        string
	Content         *string
	Metadata        *string
	Hash            *string
	MTime           *time.Time
	UpdatedAt       time.Time
	LinkDisplayText *string
	LinkType        *string
	Title           string
	Description     string
}

type NoteHashEntry struct {
	ID   string `json:"id"`
	Hash string `json:"hash"`
}

type AssetHashEntry struct {
	ID   string `json:"id"`
	Hash string `json:"hash"`
}
