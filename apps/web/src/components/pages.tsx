/** @jsxImportSource ../../../../packages/ui/src */
import { Fragment } from "../../../../packages/ui/src/index";
import { Alert, EmptyState, FormField, Icon } from "./common";

interface TenantViewModel {
  id: string;
  name: string;
}

interface VaultViewModel {
  id: string;
  name: string;
  domain: string;
  syncKey: string;
  maskedKey: string;
  lastPublishStatus: string;
  lastPublishAt?: string | null;
  lastPublishErrorMessage?: string | null;
  rootNoteId?: string | null;
  notes: Array<{ clientId: string; path: string }>;
  storageSize: string;
  logoUrl?: string | null;
  customHeadHtml: string;
  createdAt: string;
  hasCustomHtml: boolean;
}

interface PageProps {
  csrfToken: string;
}

interface ConsolePageProps extends PageProps {
  tenant: TenantViewModel;
  vaults: VaultViewModel[];
}

function Document({
  title,
  csrfToken,
  bodyClass,
  bodyAttributes = {},
  children,
}: {
  title: string;
  csrfToken: string;
  bodyClass?: string;
  bodyAttributes?: Record<string, string>;
  children: unknown;
}) {
  return (
    <html lang="en">
      <head>
        <meta charSet="UTF-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1.0" />
        <meta name="robots" content="noindex, nofollow" />
        <meta name="csrf-token" content={csrfToken} />
        <title>{title}</title>
        <link rel="icon" type="image/png" href="/publics/shared/images/favicon.png" />
        <link href="/publics/console/css/console.css" rel="stylesheet" />
        <script src="/publics/shared/htmx.min.js"></script>
        <script src="/publics/console/js/console.js"></script>
      </head>
      <body class={bodyClass} {...bodyAttributes}>
        {children}
      </body>
    </html>
  );
}

function Header({ tenant }: { tenant: TenantViewModel }) {
  return (
    <header class="app-header">
      <div class="header-content">
        <div class="logo-area">
          <div class="logo-icon">
            <img
              src="/publics/console/images/logo.png"
              alt="Mdbrain Logo"
              class="w-8 h-8 object-contain"
            />
          </div>
          <div class="logo-text">
            <h1>{tenant.name}</h1>
            <p>Mdbrain</p>
          </div>
        </div>
        <div class="header-actions">
          <div class="vault-actions">
            <button type="button" class="btn" onclick="toggleActionMenu(this)">
              <Icon name="user" className="icon-sm" />
              <span>Account</span>
              <Icon name="chevron-down" className="icon-sm" />
            </button>
            <div class="action-menu">
              <button
                type="button"
                onclick="openChangePasswordModal(); toggleActionMenu(this.closest('.vault-actions').querySelector('button'))"
              >
                <Icon name="key" className="icon-xs" />
                <span>Change password</span>
              </button>
              <button
                type="button"
                hx-post="/console/logout"
                hx-swap="none"
                onclick="toggleActionMenu(this.closest('.vault-actions').querySelector('button'))"
                {...{ "hx-on::after-request": "window.location.href='/console/login'" }}
              >
                <Icon name="log-out" className="icon-xs" />
                <span>Logout</span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </header>
  );
}

function Footer() {
  return (
    <footer class="app-footer">
      <p>
        Powered by{" "}
        <strong>
          <a href="https://mdbrain.com" class="hover:underline" target="_blank">
            Mdbrain
          </a>
        </strong>
      </p>
    </footer>
  );
}

function LoginCard({ csrfToken }: PageProps) {
  return (
    <div class="login-container">
      <div class="login-header">
        <div class="logo-area justify-center">
          <div class="logo-icon">
            <img
              src="/publics/console/images/logo.png"
              alt="Mdbrain Logo"
              class="w-10 h-10 object-contain"
            />
          </div>
        </div>
        <p class="text-sm text-[var(--text-secondary)] mt-2 mb-0">Login to console dashboard</p>
      </div>
      <div class="login-body">
        <div id="message-container"></div>
        <form
          hx-post="/console/login"
          hx-target="#message-container"
          hx-swap="innerHTML"
          hx-disabled-elt="button"
          hx-indicator=".spinner"
        >
          <input type="hidden" name="__anti-forgery-token" value={csrfToken} />
          <FormField
            id="username"
            name="username"
            type="email"
            label="Email"
            placeholder="console@example.com"
            autocomplete="username"
            autofocus={true}
            required={true}
          />
          <FormField
            id="password"
            name="password"
            type="password"
            label="Password"
            placeholder="••••••••"
            autocomplete="current-password"
            required={true}
          />
          <button type="submit" class="btn-primary">
            <span class="flex items-center justify-center gap-2">
              <span class="spinner"></span>
              <span>Login</span>
            </span>
          </button>
        </form>
      </div>
    </div>
  );
}

function InitCard({ csrfToken }: PageProps) {
  return (
    <div class="init-container">
      <div class="init-header">
        <div class="welcome-icon">
          <img
            src="/publics/console/images/logo.png"
            alt="Mdbrain Logo"
            class="w-10 h-10 object-contain"
          />
        </div>
        <h1 class="text-[1.375rem] font-semibold text-[var(--text-primary)] m-0 mb-2 text-center">
          Welcome to Mdbrain
        </h1>
        <p class="text-sm text-[var(--text-secondary)] m-0 text-center">
          Create a console account to get started
        </p>
      </div>
      <div class="init-body">
        <div id="message-container"></div>
        <form
          hx-post="/console/init"
          hx-target="#message-container"
          hx-swap="innerHTML"
          hx-disabled-elt="button"
          hx-indicator=".spinner"
        >
          <input type="hidden" name="__anti-forgery-token" value={csrfToken} />
          <FormField
            id="tenant-name"
            name="tenant-name"
            label="Organization Name"
            placeholder="My Organization"
            hint="Your company or personal workspace name"
            autofocus={true}
            required={true}
          />
          <FormField
            id="username"
            name="username"
            type="email"
            label="Console Email"
            placeholder="console@example.com"
            autocomplete="username"
            required={true}
          />
          <FormField
            id="password"
            name="password"
            type="password"
            label="Password"
            placeholder="••••••••"
            autocomplete="new-password"
            minlength={8}
            hint="At least 8 characters, use a strong password"
            required={true}
          />
          <button type="submit" class="btn-primary">
            <span class="spinner"></span>
            <Icon name="rocket" className="icon-md" />
            <span>Create Account</span>
          </button>
        </form>
      </div>
    </div>
  );
}

function CreateModal({ csrfToken }: PageProps) {
  return (
    <dialog id="modal-create" class="modal-dialog">
      <div class="modal-content">
        <div class="modal-header">
          <h3>New Site</h3>
          <button type="button" class="btn-close" onclick="closeCreateModal()">
            <Icon name="x" className="icon-lg" />
          </button>
        </div>
        <form
          hx-post="/console/vaults"
          hx-target="#create-result"
          hx-swap="innerHTML"
          hx-disabled-elt="button[type=submit]"
          hx-indicator=".create-spinner"
          {...{
            "hx-on::after-request":
              "if(event.detail.successful) { closeCreateModal(); this.reset(); htmx.trigger('body', 'refreshList'); showNotification('Site created', 'success'); }",
          }}
        >
          <input type="hidden" name="__anti-forgery-token" value={csrfToken} />
          <div class="modal-body">
            <div id="create-result"></div>
            <FormField
              id="create-name"
              name="name"
              label="Site Name"
              placeholder="My Digital Garden"
              required={true}
            />
            <FormField
              id="create-domain"
              name="domain"
              label="Domain"
              placeholder="notes.example.com"
              hint="Your custom domain (without https://)"
              required={true}
            />
          </div>
          <div class="modal-footer">
            <button type="button" class="btn" onclick="closeCreateModal()">
              Cancel
            </button>
            <button type="submit" class="btn btn-primary">
              <span class="spinner create-spinner"></span>
              <span>Create Site</span>
            </button>
          </div>
        </form>
      </div>
    </dialog>
  );
}

function EditModal({ csrfToken }: PageProps) {
  return (
    <dialog id="modal-edit" class="modal-dialog">
      <div class="modal-content">
        <div class="modal-header">
          <h3>Edit Site</h3>
          <button type="button" class="btn-close" onclick="closeEditModal()">
            <Icon name="x" className="icon-lg" />
          </button>
        </div>
        <form
          id="edit-form"
          hx-put=""
          hx-target="#edit-result"
          hx-swap="innerHTML"
          hx-disabled-elt="button[type=submit]"
          hx-indicator=".edit-spinner"
          {...{
            "hx-on::after-request":
              "if(event.detail.successful) { closeEditModal(); htmx.trigger('body', 'refreshList'); showNotification('Site updated', 'success'); }",
          }}
        >
          <input type="hidden" name="__anti-forgery-token" value={csrfToken} />
          <input type="hidden" name="id" id="edit-vault-id" />
          <div class="modal-body">
            <div id="edit-result"></div>
            <FormField id="edit-name" name="name" label="Site Name" required={true} />
            <FormField id="edit-domain" name="domain" label="Domain" required={true} />
          </div>
          <div class="modal-footer">
            <button type="button" class="btn" onclick="closeEditModal()">
              Cancel
            </button>
            <button type="submit" class="btn btn-primary">
              <span class="spinner edit-spinner"></span>
              <span>Save Changes</span>
            </button>
          </div>
        </form>
      </div>
    </dialog>
  );
}

function ChangePasswordModal({ csrfToken }: PageProps) {
  return (
    <dialog id="modal-change-password" class="modal-dialog">
      <div class="modal-content">
        <div class="modal-header">
          <h3>Change Password</h3>
          <button type="button" class="btn-close" onclick="closeChangePasswordModal()">
            <Icon name="x" className="icon-lg" />
          </button>
        </div>
        <form
          id="change-password-form"
          hx-put="/console/user/password"
          hx-target="#change-password-result"
          hx-swap="innerHTML"
          hx-disabled-elt="button[type=submit]"
          hx-indicator=".password-spinner"
          {...{
            "hx-on::after-request":
              "if(event.detail.successful) { closeChangePasswordModal(); this.reset(); showNotification('Password updated', 'success'); }",
          }}
        >
          <input type="hidden" name="__anti-forgery-token" value={csrfToken} />
          <div class="modal-body">
            <div id="change-password-result"></div>
            <FormField
              id="current-password"
              name="current-password"
              type="password"
              label="Current Password"
              autocomplete="current-password"
              required={true}
            />
            <FormField
              id="new-password"
              name="new-password"
              type="password"
              label="New Password"
              autocomplete="new-password"
              minlength={8}
              required={true}
            />
            <FormField
              id="confirm-password"
              name="confirm-password"
              type="password"
              label="Confirm New Password"
              autocomplete="new-password"
              minlength={8}
              required={true}
            />
          </div>
          <div class="modal-footer">
            <button type="button" class="btn" onclick="closeChangePasswordModal()">
              Cancel
            </button>
            <button type="submit" class="btn btn-primary">
              <span class="spinner password-spinner"></span>
              <span>Update Password</span>
            </button>
          </div>
        </form>
      </div>
    </dialog>
  );
}

function CustomHtmlModal({ csrfToken }: PageProps) {
  return (
    <dialog id="modal-custom-html" class="modal-dialog modal-dialog-wide">
      <div class="modal-content">
        <div class="modal-header">
          <h3>Custom HTML</h3>
          <button type="button" class="btn-close" onclick="closeCustomHtmlModal()">
            <Icon name="x" className="icon-lg" />
          </button>
        </div>
        <form
          id="custom-html-form"
          hx-put=""
          hx-swap="none"
          hx-disabled-elt="button[type=submit]"
          {...{ "hx-on::after-request": "onSaveCustomHtml(event)" }}
        >
          <div class="modal-body">
            <p class="form-hint" style="margin-top: 0; margin-bottom: 12px;">
              Add custom {"<style>"} or {"<script>"} tags to inject into your site's {"<head>"}.
              Useful for analytics or custom styling.
            </p>
            <input type="hidden" name="__anti-forgery-token" value={csrfToken} />
            <input type="hidden" id="custom-html-vault-id" />
            <textarea
              id="custom-html-textarea"
              name="customHeadHtml"
              class="form-input"
              rows="10"
              maxlength="65536"
              style="font-family: var(--font-mono); font-size: 0.875rem; resize: vertical;"
              placeholder={"<style>\n  body { background: #f5f5f5; }\n</style>\n\n<script>\n  console.log('Hello');\n</script>"}
            ></textarea>
          </div>
          <div class="modal-footer">
            <button type="button" class="btn" onclick="closeCustomHtmlModal()">
              Cancel
            </button>
            <button type="submit" class="btn btn-primary">
              <span>Save</span>
            </button>
          </div>
        </form>
      </div>
    </dialog>
  );
}

export function RootNoteSelector({
  vaultId,
  rootNoteId,
  notes,
}: {
  vaultId: string;
  rootNoteId?: string | null;
  notes: Array<{ clientId: string; path: string }>;
}) {
  const selectedNote = notes.find((note) => note.clientId === rootNoteId);

  return notes.length > 0 ? (
    <div class="note-selector" id={`note-selector-${vaultId}`}>
      <div class="note-selector-trigger" onclick={`toggleNoteSelector(${JSON.stringify(vaultId)})`}>
        <span class="note-selector-value">{selectedNote?.path ?? "Select note"}</span>
        <Icon name="chevron-down" className="icon-xs note-selector-arrow" />
      </div>
      <div class="note-selector-dropdown">
        <input
          type="text"
          class="note-search-input"
          placeholder="Search notes..."
          oninput={`filterNotes(${JSON.stringify(vaultId)}, this.value)`}
        />
        <div class="note-list">
          {notes.map((note) => (
            <div
              class={`note-item${note.clientId === rootNoteId ? " selected" : ""}`}
              data-client-id={note.clientId}
              data-path={note.path.toLowerCase()}
              data-display={note.path}
              hx-put={`/console/vaults/${vaultId}/root-note`}
              hx-vals={`{"rootNoteId":"${note.clientId}"}`}
              hx-swap="none"
              {...{
                "hx-on::after-request": `handleNoteSelect(${JSON.stringify(vaultId)}, this, event)`,
              }}
            >
              <span class="note-path">{note.path}</span>
              {note.clientId === rootNoteId ? <Icon name="check" className="icon-xs note-check" /> : null}
            </div>
          ))}
        </div>
      </div>
    </div>
  ) : (
    <div class="warning-box">
      <Icon name="triangle-alert" className="icon-xs shrink-0" />
      <span>No notes synced yet. Please use the Obsidian plugin to sync files before setting a homepage.</span>
    </div>
  );
}

function VaultCard({ vault }: { vault: VaultViewModel }) {
  const editArgs = `${JSON.stringify(vault.id)}, ${JSON.stringify(vault.name)}, ${JSON.stringify(vault.domain)}`;
  const siteUrl = `http://${vault.domain}`;

  return (
    <div class="vault-card">
      <div class="vault-header">
        <div class="vault-info">
          <h3>{vault.name}</h3>
          <a href={siteUrl} target="_blank" class="vault-domain">
            <Icon name="globe" className="icon-xs" />
            <span>{vault.domain}</span>
            <Icon name="external-link" className="icon-xs" />
          </a>
        </div>
        <div class="vault-actions">
          <button type="button" class="action-menu-btn" onclick="toggleActionMenu(this)">
            <Icon name="ellipsis-vertical" className="icon-md" />
          </button>
          <div class="action-menu">
            <button
              type="button"
              onclick={`window.open(${JSON.stringify(siteUrl)}, '_blank'); toggleActionMenu(this.closest('.vault-actions').querySelector('.action-menu-btn'))`}
            >
              <Icon name="external-link" className="icon-xs" />
              <span>Open site</span>
            </button>
            <button
              type="button"
              onclick={`openEditModal(${editArgs}); toggleActionMenu(this.closest('.vault-actions').querySelector('.action-menu-btn'))`}
            >
              <Icon name="pencil" className="icon-xs" />
              <span>Edit</span>
            </button>
            <button
              type="button"
              onclick={`openCustomHtmlModal(${JSON.stringify(vault.id)}); toggleActionMenu(this.closest('.vault-actions').querySelector('.action-menu-btn'))`}
            >
              <Icon name="code" className="icon-xs" />
              <span>Custom HTML</span>
            </button>
            <button
              type="button"
              class="warning"
              hx-post={`/console/vaults/${vault.id}/renew-sync-key`}
              hx-confirm={`Are you sure you want to renew the publish key for '${vault.name}'? The old key will no longer work for publishing.`}
              hx-swap="none"
              {...{
                "hx-on::after-request":
                  "if(event.detail.successful) { showNotification('Publish key renewed', 'success'); htmx.trigger('body', 'refreshList'); } else { showNotification('Failed to renew publish key', 'error'); } toggleActionMenu(this.closest('.vault-actions').querySelector('.action-menu-btn'))",
              }}
            >
              <Icon name="refresh-cw" className="icon-xs" />
              <span>Renew publish key</span>
            </button>
            <button
              type="button"
              class="danger"
              hx-delete={`/console/vaults/${vault.id}`}
              hx-confirm={`Are you sure you want to delete '${vault.name}'? This cannot be undone.`}
              hx-swap="none"
              {...{
                "hx-on::after-request":
                  "if(event.detail.successful) { showNotification('Site deleted', 'success'); htmx.trigger('body', 'refreshList'); } else { showNotification('Delete failed', 'error'); }",
              }}
            >
              <Icon name="trash-2" className="icon-xs" />
              <span>Delete</span>
            </button>
          </div>
          <span hidden id={`custom-html-data-${vault.id}`}>
            {vault.customHeadHtml}
          </span>
        </div>
      </div>
      <div class="vault-section">
        <span class="section-label">Publish Status</span>
        <div>
          {vault.lastPublishStatus === "ok" ? (
            <div class="flex items-center gap-2 text-sm text-[var(--success)]">
              <Icon name="circle-check" className="icon-xs" />
              <span class="font-medium">OK</span>
            </div>
          ) : vault.lastPublishStatus === "error" ? (
            <div class="flex items-center gap-2 text-sm text-[var(--error)]">
              <Icon name="circle-alert" className="icon-xs" />
              <span class="font-medium">Error</span>
            </div>
          ) : (
            <div class="flex items-center gap-2 text-sm text-[var(--text-tertiary)]">
              <Icon name="clock" className="icon-xs" />
              <span>Never</span>
            </div>
          )}
          <div class="text-xs text-[var(--text-tertiary)] mt-2">
            Last publish:{" "}
            {vault.lastPublishAt ? (
              <span data-utc-datetime={vault.lastPublishAt}>{vault.lastPublishAt}</span>
            ) : (
              "Never"
            )}
          </div>
          {vault.lastPublishErrorMessage ? (
            <div class="text-xs text-[var(--error)] mt-2">{vault.lastPublishErrorMessage}</div>
          ) : null}
        </div>
      </div>
      <div class="vault-section">
        <span class="section-label">Site Logo</span>
        <div class="logo-upload-box" id={`logo-box-${vault.id}`}>
          {vault.logoUrl ? (
            <div class="logo-preview-container">
              <img
                src={vault.logoUrl}
                alt="Site logo"
                class="logo-preview-img"
                id={`logo-preview-${vault.id}`}
              />
              <div class="logo-actions">
                <form
                  class="contents"
                  hx-post={`/console/vaults/${vault.id}/logo`}
                  hx-encoding="multipart/form-data"
                  hx-trigger="change from:input[type=file]"
                  hx-swap="none"
                  {...{ "hx-on::after-request": "handleLogoRequest(event)" }}
                >
                  <label class="icon-btn" title="Change logo">
                    <Icon name="upload" className="icon-xs" />
                    <input type="file" name="logo" accept="image/png,image/jpeg" class="hidden" />
                  </label>
                </form>
                <button
                  type="button"
                  class="icon-btn"
                  title="Delete logo"
                  hx-delete={`/console/vaults/${vault.id}/logo`}
                  hx-confirm="Are you sure you want to delete this logo?"
                  hx-swap="none"
                  {...{ "hx-on::after-request": "handleLogoRequest(event)" }}
                >
                  <Icon name="trash-2" className="icon-xs" />
                </button>
              </div>
            </div>
          ) : (
            <form
              class="contents"
              hx-post={`/console/vaults/${vault.id}/logo`}
              hx-encoding="multipart/form-data"
              hx-trigger="change from:input[type=file]"
              hx-swap="none"
              {...{ "hx-on::after-request": "handleLogoRequest(event)" }}
            >
              <label class="logo-upload-area" id={`logo-upload-area-${vault.id}`}>
                <Icon name="image-plus" className="icon-lg text-(--text-tertiary)" />
                <span class="text-xs text-(--text-tertiary)">Click to upload logo</span>
                <span class="text-xs text-(--text-tertiary)">PNG, JPEG (max 2MB)</span>
                <input type="file" name="logo" accept="image/png,image/jpeg" class="hidden" />
              </label>
            </form>
          )}
        </div>
      </div>
      <div class="vault-section">
        <span class="section-label">Publish Key</span>
        <div class="sync-key-box">
          <div class="sync-key-header">
            <span class="text-xs text-(--text-tertiary)">For Obsidian plugin publishing</span>
            <div class="sync-key-actions">
              <button
                type="button"
                class="icon-btn"
                onclick={`toggleSyncKey(this, ${JSON.stringify(vault.id)})`}
                title="Show/Hide"
              >
                <Icon name="eye" className="icon-xs" />
              </button>
              <button
                type="button"
                class="icon-btn"
                onclick={`copyToClipboard(${JSON.stringify(vault.syncKey)})`}
                title="Copy"
              >
                <Icon name="copy" className="icon-xs" />
              </button>
            </div>
          </div>
          <div class="sync-key-value" id={`key-${vault.id}`} data-key={vault.syncKey}>
            {vault.maskedKey}
          </div>
        </div>
      </div>
      <div class="vault-section">
        <span class="section-label">Home Page</span>
        <RootNoteSelector vaultId={vault.id} rootNoteId={vault.rootNoteId} notes={vault.notes} />
      </div>
      <div class="vault-section">
        <span class="section-label">Storage</span>
        <div class="storage-info">
          <Icon name="hard-drive" className="icon-xs text-[var(--text-tertiary)]" />
          <span class="text-sm">{vault.storageSize}</span>
        </div>
      </div>
      <div class="vault-section">
        <span class="section-label">Created</span>
        <div class="storage-info">
          <Icon name="calendar" className="icon-xs text-[var(--text-tertiary)]" />
          <span class="text-sm" data-utc-datetime={vault.createdAt}>
            {vault.createdAt}
          </span>
        </div>
      </div>
    </div>
  );
}

export function VaultList({ vaults }: { vaults: VaultViewModel[] }) {
  return vaults.length > 0 ? (
    <Fragment>
      {vaults.map((vault) => (
        <VaultCard vault={vault} />
      ))}
    </Fragment>
  ) : (
    <EmptyState
      icon="inbox"
      title="No sites yet"
      message="Create your first site to start publishing your Obsidian notes to the web"
      ctaText="Create first site"
      ctaAction="openCreateModal()"
      ctaIcon="plus"
      gridFull={true}
    />
  );
}

export function LoginPage({ csrfToken }: PageProps) {
  return (
    <Document title="Login - Mdbrain" csrfToken={csrfToken} bodyClass="auth-page">
      <LoginCard csrfToken={csrfToken} />
    </Document>
  );
}

export function InitPage({ csrfToken }: PageProps) {
  return (
    <Document title="Welcome - Mdbrain" csrfToken={csrfToken} bodyClass="auth-page">
      <InitCard csrfToken={csrfToken} />
    </Document>
  );
}

export function ConsolePage({ csrfToken, tenant, vaults }: ConsolePageProps) {
  return (
    <Document
      title="Console Dashboard - Mdbrain"
      csrfToken={csrfToken}
      bodyAttributes={{
        "hx-headers": `{"X-CSRF-Token":"${csrfToken}"}`,
      }}
    >
      <div id="notification-container" class="fixed top-4 right-4 z-[9999]"></div>
      <Header tenant={tenant} />
      <main class="main-content">
        <div class="page-header">
          <div>
            <h2>Publishing</h2>
            <p>Manage your published vaults, domains, and publish keys.</p>
          </div>
          <button onclick="openCreateModal()" class="btn btn-primary">
            <Icon name="plus" className="icon-sm" />
            <span>New Site</span>
          </button>
        </div>
        <div
          id="vault-list"
          hx-get="/console/vaults"
          hx-trigger="refreshList from:body"
          hx-swap="innerHTML"
          class="vault-grid"
        >
          <VaultList vaults={vaults} />
        </div>
      </main>
      <Footer />
      <CreateModal csrfToken={csrfToken} />
      <EditModal csrfToken={csrfToken} />
      <CustomHtmlModal csrfToken={csrfToken} />
      <ChangePasswordModal csrfToken={csrfToken} />
    </Document>
  );
}

export function ErrorFragment({ message }: { message: string }) {
  return <Alert message={message} kind="error" />;
}
