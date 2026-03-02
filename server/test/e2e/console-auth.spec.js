const { test, expect } = require('@playwright/test');

test('redirects unauthenticated console to init page', async ({ page }) => {
  await page.goto('/console');
  await expect(page).toHaveURL(/\/console\/init$/);
  await expect(page.getByRole('heading', { name: 'Welcome to Mdbrain' })).toBeVisible();
});

test('bootstraps first admin and logs in', async ({ page }) => {
  const id = Date.now();
  const username = `e2e-${id}@example.com`;
  const password = 'mdbrain-e2e-pass-123';

  await page.goto('/console/init');
  await page.locator('input[name="tenant-name"]').fill('E2E Organization');
  await page.locator('input[name="username"]').fill(username);
  await page.locator('input[name="password"]').fill(password);
  await page.locator('button[type="submit"]').click();

  await expect(page.locator('#message-container')).toContainText('Account created successfully, redirecting...');
  await page.waitForURL('**/console/login');

  await page.locator('input[name="username"]').fill(username);
  await page.locator('input[name="password"]').fill(password);
  await page.locator('button[type="submit"]').click();

  await expect(page.locator('#message-container')).toContainText('Login successful, redirecting...');
  await page.waitForURL('**/console');
  await expect(page.getByRole('heading', { name: 'Publishing' })).toBeVisible();
});
