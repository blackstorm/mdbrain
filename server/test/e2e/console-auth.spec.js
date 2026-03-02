const { test, expect } = require('@playwright/test');

const appPort = process.env.E2E_APP_PORT || '18080';

const auth = {
  tenant: 'E2E Organization',
  username: 'e2e-admin@example.com',
  password: 'mdbrain-e2e-pass-123'
};

function uniqueId() {
  return `${Date.now()}-${Math.floor(Math.random() * 10000)}`;
}

function appUrl(domain, path = '/') {
  return `http://${domain}:${appPort}${path}`;
}

function appServerUrl(path = '/') {
  return `http://127.0.0.1:${appPort}${path}`;
}

async function ensureInitialized(page) {
  await page.goto('/console/init');

  if (/\/console\/init$/.test(page.url())) {
    await page.locator('input[name="tenant-name"]').fill(auth.tenant);
    await page.locator('input[name="username"]').fill(auth.username);
    await page.locator('input[name="password"]').fill(auth.password);
    await page.locator('form[hx-post="/console/init"] button[type="submit"]').click();
    await page.waitForURL('**/console/login');
  }
}

async function login(page, username = auth.username, password = auth.password) {
  await page.goto('/console/login');
  await page.locator('input[name="username"]').fill(username);
  await page.locator('input[name="password"]').fill(password);
  await page.locator('form[hx-post="/console/login"] button[type="submit"]').click();
  await page.waitForURL('**/console');
  await expect(page.getByRole('heading', { name: 'Publishing' })).toBeVisible();
}

function vaultCard(page, siteName) {
  return page.locator('.vault-card', { hasText: siteName });
}

async function createSite(page, label, domain = null) {
  const id = uniqueId();
  const siteName = `${label} ${id}`;
  const siteDomain = domain || `e2e-${id}.example.com`;

  await page.getByRole('button', { name: 'New Site' }).click();
  await page.locator('#create-name').fill(siteName);
  await page.locator('#create-domain').fill(siteDomain);
  await page.locator('#modal-create button[type="submit"]').click();

  const card = vaultCard(page, siteName);
  await expect(card).toBeVisible();
  await expect(card).toContainText(siteDomain);

  return { siteName, siteDomain };
}

async function getSyncKey(page, siteName) {
  const card = vaultCard(page, siteName);
  await expect(card).toBeVisible();
  return card.locator('.sync-key-value').getAttribute('data-key');
}

async function syncNote(request, syncKey, noteId, notePath, content) {
  const syncResponse = await request.post(`/obsidian/sync/notes/${noteId}`, {
    headers: {
      Authorization: `Bearer ${syncKey}`
    },
    data: {
      path: notePath,
      content,
      hash: `hash-${uniqueId()}`,
      assets: [],
      linked_notes: []
    }
  });

  expect(syncResponse.ok()).toBeTruthy();
  const syncBody = await syncResponse.json();
  expect(['stored', 'skipped']).toContain(syncBody.status);
  return syncBody;
}

async function deleteSite(page, siteName) {
  const card = vaultCard(page, siteName);
  if (await card.count() === 0) return;

  await card.locator('.action-menu-btn').click();
  page.once('dialog', (dialog) => dialog.accept());
  await card.getByRole('button', { name: 'Delete' }).click();
  await expect(vaultCard(page, siteName)).toHaveCount(0);
}

test.describe.serial('console auth and management flows', () => {
  test('redirects unauthenticated console to init page', async ({ page }) => {
    await page.goto('/console');
    await expect(page).toHaveURL(/\/console\/init$/);
    await expect(page.getByRole('heading', { name: 'Welcome to Mdbrain' })).toBeVisible();
  });

  test('bootstraps first admin and logs in', async ({ page }) => {
    await ensureInitialized(page);

    await page.goto('/console/login');
    await page.locator('input[name="username"]').fill(auth.username);
    await page.locator('input[name="password"]').fill(auth.password);
    await page.locator('form[hx-post="/console/login"] button[type="submit"]').click();

    await expect(page.locator('#message-container')).toContainText('Login successful, redirecting...');
    await page.waitForURL('**/console');
    await expect(page.getByRole('heading', { name: 'Publishing' })).toBeVisible();
  });

  test('shows login error for wrong password', async ({ page }) => {
    await ensureInitialized(page);

    await page.goto('/console/login');
    await page.locator('input[name="username"]').fill(auth.username);
    await page.locator('input[name="password"]').fill('wrong-password');
    await page.locator('form[hx-post="/console/login"] button[type="submit"]').click();

    await expect(page.locator('#message-container')).toContainText('Invalid username or password');
    await expect(page).toHaveURL(/\/console\/login$/);
  });

  test('logout redirects to login and blocks console', async ({ page }) => {
    await ensureInitialized(page);
    await login(page);

    await page.getByRole('button', { name: 'Account' }).click();
    await page.getByRole('button', { name: 'Logout' }).click();
    await page.waitForURL('**/console/login');

    await page.goto('/console');
    await expect(page).toHaveURL(/\/console\/login$/);
  });

  test('creates, edits, and deletes a site', async ({ page }) => {
    const id = uniqueId();
    const siteName = `E2E Site ${id}`;
    const siteDomain = `e2e-${id}.example.com`;
    const updatedName = `E2E Site Updated ${id}`;
    const updatedDomain = `e2e-updated-${id}.example.com`;

    await ensureInitialized(page);
    await login(page);

    await page.getByRole('button', { name: 'New Site' }).click();
    await page.locator('#create-name').fill(siteName);
    await page.locator('#create-domain').fill(siteDomain);
    await page.locator('#modal-create button[type="submit"]').click();

    let card = vaultCard(page, siteName);
    await expect(card).toBeVisible();
    await expect(card).toContainText(siteDomain);

    await card.locator('.action-menu-btn').click();
    await card.getByRole('button', { name: 'Edit' }).click();
    await page.locator('#edit-name').fill(updatedName);
    await page.locator('#edit-domain').fill(updatedDomain);
    await page.locator('#modal-edit button[type="submit"]').click();

    card = vaultCard(page, updatedName);
    await expect(card).toBeVisible();
    await expect(card).toContainText(updatedDomain);

    await card.locator('.action-menu-btn').click();
    page.once('dialog', (dialog) => dialog.accept());
    await card.getByRole('button', { name: 'Delete' }).click();

    await expect(vaultCard(page, updatedName)).toHaveCount(0);
    await expect(page.getByText('No sites yet')).toBeVisible();
  });

  test('rejects duplicate domain create attempt', async ({ page }) => {
    await ensureInitialized(page);
    await login(page);

    const { siteName, siteDomain } = await createSite(page, 'Duplicate Domain Site');
    const duplicateName = `Duplicate Domain Retry ${uniqueId()}`;

    await page.getByRole('button', { name: 'New Site' }).click();
    await page.locator('#create-name').fill(duplicateName);
    await page.locator('#create-domain').fill(siteDomain);
    await page.locator('#modal-create button[type="submit"]').click();

    await expect(page.locator('.vault-card .vault-domain', { hasText: siteDomain })).toHaveCount(1);
    await expect(vaultCard(page, duplicateName)).toHaveCount(0);

    await deleteSite(page, siteName);
  });

  test('shows warning when root note has no synced notes', async ({ page }) => {
    await ensureInitialized(page);
    await login(page);

    const { siteName } = await createSite(page, 'No Note Warning Site');
    const card = vaultCard(page, siteName);

    await expect(card).toContainText('No notes synced yet. Please use the Obsidian plugin to sync files before setting a homepage.');

    await deleteSite(page, siteName);
  });

  test('saves and reloads custom head HTML', async ({ page }) => {
    const snippet = `<style data-e2e="${uniqueId()}">body{--e2e:1;}</style>`;

    await ensureInitialized(page);
    await login(page);

    const { siteName } = await createSite(page, 'Custom HTML Site');

    let card = vaultCard(page, siteName);
    await card.locator('.action-menu-btn').click();
    await card.getByRole('button', { name: 'Custom HTML' }).click();

    await page.locator('#custom-html-textarea').fill(snippet);
    await page.locator('#modal-custom-html button[type="submit"]').click();
    await expect(page.locator('#notification-container')).toContainText('Custom HTML saved');

    card = vaultCard(page, siteName);
    await expect(card).toBeVisible();
    await card.locator('.action-menu-btn').click();
    await card.getByRole('button', { name: 'Custom HTML' }).click();
    await expect(page.locator('#custom-html-textarea')).toHaveValue(snippet);

    await page.locator('#modal-custom-html button:has-text("Cancel")').click();
    await deleteSite(page, siteName);
  });

  test('renews publish key and updates key value', async ({ page }) => {
    await ensureInitialized(page);
    await login(page);

    const { siteName } = await createSite(page, 'Renew Key Site');
    const oldKey = await getSyncKey(page, siteName);

    const card = vaultCard(page, siteName);
    await card.locator('.action-menu-btn').click();
    page.once('dialog', (dialog) => dialog.accept());
    await card.getByRole('button', { name: 'Renew publish key' }).click();

    await expect(page.locator('#notification-container')).toContainText('Publish key renewed');

    await expect.poll(async () => getSyncKey(page, siteName)).not.toBe(oldKey);
    await deleteSite(page, siteName);
  });

  test('renders app home and stacked note paths', async ({ page, request }) => {
    const appDomain = `app-${uniqueId()}.localhost`;
    const noteAId = `note-a-${uniqueId()}`;
    const noteBId = `note-b-${uniqueId()}`;
    const noteAPath = `Alpha-${uniqueId()}.md`;
    const noteBPath = `Beta-${uniqueId()}.md`;

    await ensureInitialized(page);
    await login(page);

    const { siteName } = await createSite(page, 'App Navigation Site', appDomain);
    const syncKey = await getSyncKey(page, siteName);

    await syncNote(request, syncKey, noteAId, noteAPath, '# Alpha Note\n\nAlpha body');
    await syncNote(request, syncKey, noteBId, noteBPath, '# Beta Note\n\nBeta body');

    await page.goto(appUrl(appDomain, '/'));
    await expect(page.getByRole('heading', { name: siteName })).toBeVisible();
    await expect(page.getByRole('link', { name: new RegExp(noteAPath) })).toBeVisible();
    await expect(page.getByRole('link', { name: new RegExp(noteBPath) })).toBeVisible();

    await page.goto(appUrl(appDomain, `/${noteAId}+${noteBId}`));
    await expect(page.locator(`#note-${noteAId}`)).toContainText('Alpha body');
    await expect(page.locator(`#note-${noteBId}`)).toContainText('Beta body');

    await page.goto('/console');
    await deleteSite(page, siteName);
  });

  test('returns htmx push and replace headers on app requests', async ({ page, request }) => {
    const appDomain = `app-hx-${uniqueId()}.localhost`;
    const noteAId = `note-a-${uniqueId()}`;
    const noteBId = `note-b-${uniqueId()}`;

    await ensureInitialized(page);
    await login(page);

    const { siteName } = await createSite(page, 'App HTMX Header Site', appDomain);
    const syncKey = await getSyncKey(page, siteName);

    await syncNote(request, syncKey, noteAId, `A-${uniqueId()}.md`, '# A');
    await syncNote(request, syncKey, noteBId, `B-${uniqueId()}.md`, '# B');

    const hostHeader = `${appDomain}:${appPort}`;

    const htmxResponse = await request.get(appServerUrl(`/${noteBId}`), {
      headers: {
        host: hostHeader,
        'hx-request': 'true',
        'x-from-note-id': noteAId,
        'hx-current-url': appUrl(appDomain, `/${noteAId}`)
      }
    });

    expect(htmxResponse.ok()).toBeTruthy();
    const htmxHeaders = htmxResponse.headers();
    expect(htmxHeaders['hx-push-url']).toBe(`/${noteAId}+${noteBId}`);
    expect(htmxHeaders['content-type']).toContain('text/html');

    const correctedResponse = await request.get(appServerUrl(`/${noteAId}+missing-note`), {
      headers: {
        host: hostHeader
      }
    });
    expect(correctedResponse.ok()).toBeTruthy();
    const correctedHeaders = correctedResponse.headers();
    expect(correctedHeaders['hx-replace-url']).toBe(`/${noteAId}`);

    await page.goto('/console');
    await deleteSite(page, siteName);
  });

  test('sets root note after syncing a note', async ({ page, request }) => {
    const noteId = `note-${uniqueId()}`;
    const notePath = `Home-${uniqueId()}.md`;

    await ensureInitialized(page);
    await login(page);

    const { siteName } = await createSite(page, 'Root Note Site');
    const syncKey = await getSyncKey(page, siteName);

    await syncNote(request, syncKey, noteId, notePath, '# Home');

    await page.goto('/console');

    const card = vaultCard(page, siteName);
    await expect(card.locator('.note-selector')).toBeVisible();
    await card.locator('.note-selector-trigger').click();
    await card.locator('.note-item', { hasText: notePath }).click();

    await expect(card.locator('.note-selector-value')).toContainText(notePath);

    await page.reload();
    await expect(vaultCard(page, siteName).locator('.note-selector-value')).toContainText(notePath);

    await deleteSite(page, siteName);
  });

  test('changes password and enforces new credential', async ({ page }) => {
    const newPassword = `mdbrain-e2e-new-pass-${uniqueId()}`;

    await ensureInitialized(page);
    await login(page);

    await page.getByRole('button', { name: 'Account' }).click();
    await page.getByRole('button', { name: 'Change password' }).click();

    await page.locator('#current-password').fill('wrong-current-password');
    await page.locator('#new-password').fill(newPassword);
    await page.locator('#confirm-password').fill(newPassword);
    await page.locator('#modal-change-password button[type="submit"]').click();
    await expect(page.locator('#notification-container')).toContainText('Current password is incorrect');

    await page.locator('#current-password').fill(auth.password);
    await page.locator('#new-password').fill(newPassword);
    await page.locator('#confirm-password').fill(newPassword);
    await page.locator('#modal-change-password button[type="submit"]').click();
    await expect(page.locator('#notification-container')).toContainText('Password updated');

    auth.password = newPassword;

    await page.getByRole('button', { name: 'Account' }).click();
    await page.getByRole('button', { name: 'Logout' }).click();
    await page.waitForURL('**/console/login');

    await page.locator('input[name="username"]').fill(auth.username);
    await page.locator('input[name="password"]').fill('mdbrain-e2e-pass-123');
    await page.locator('form[hx-post="/console/login"] button[type="submit"]').click();
    await expect(page.locator('#message-container')).toContainText('Invalid username or password');

    await page.locator('input[name="password"]').fill(auth.password);
    await page.locator('form[hx-post="/console/login"] button[type="submit"]').click();
    await page.waitForURL('**/console');
    await expect(page.getByRole('heading', { name: 'Publishing' })).toBeVisible();
  });
});
