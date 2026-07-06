export interface SyncSnapshotEntry {
  id: string;
  hash: string;
}

export interface SyncChangesRequest {
  notes: SyncSnapshotEntry[];
  assets: SyncSnapshotEntry[];
}

export interface SyncChangesResponse {
  success: boolean;
  need_upsert?: {
    notes: SyncSnapshotEntry[];
    assets: SyncSnapshotEntry[];
  };
  deleted_on_server?: {
    notes: SyncSnapshotEntry[];
    assets: SyncSnapshotEntry[];
  };
  error?: string;
}

export interface SyncNoteRequest {
  path: string;
  content: string;
  hash: string;
  metadata?: Record<string, unknown>;
  assets: Array<{ id: string; hash: string }>;
  linked_notes: Array<{ id: string; hash: string }>;
}

export interface SyncAssetRequest {
  path: string;
  contentType: string;
  size: number;
  hash: string;
  content: string;
}

export interface VaultInfoResponse {
  success: boolean;
  vault?: {
    id: string;
    name: string;
    domain?: string;
    createdAt?: string;
  };
  error?: string;
}
