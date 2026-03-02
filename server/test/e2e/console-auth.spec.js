const { test, expect } = require('@playwright/test');

const auth = {
  tenant: 'E2E Organization',
  username: 'e2e-admin@example.com',
  password: 'mdbrain-e2e-pass-123'
};

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

  test('creates, edits, and deletes a site', async ({ page }) => {
    const id = Date.now();
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

    const createdCard = page.locator('.vault-card', { hasText: siteName });
    await expect(createdCard).toBeVisible();
    await expect(createdCard).toContainText(siteDomain);

    await createdCard.locator('.action-menu-btn').click();
    await page.getByRole('button', { name: 'Edit' }).click();
    await page.locator('#edit-name').fill(updatedName);
    await page.locator('#edit-domain').fill(updatedDomain);
    await page.locator('#modal-edit button[type="submit"]').click();

    const updatedCard = page.locator('.vault-card', { hasText: updatedName });
    await expect(updatedCard).toBeVisible();
    await expect(updatedCard).toContainText(updatedDomain);

    await updatedCard.locator('.action-menu-btn').click();
    page.once('dialog', (dialog) => dialog.accept());
    await page.getByRole('button', { name: 'Delete' }).click();

    await expect(page.locator('.vault-card', { hasText: updatedName })).toHaveCount(0);
    await expect(page.getByText('No sites yet')).toBeVisible();
  });

  test('changes password and enforces new credential', async ({ page }) => {
    const newPassword = `mdbrain-e2e-new-pass-${Date.now()}`;

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
