const { test, expect } = require('@playwright/test');
const fs = require('fs');
const path = require('path');
const os = require('os');

const testRoot = process.env.ABM_BROWSER_DATA;
const sourceA = path.join(testRoot, 'source-a');
const sourceB = path.join(testRoot, 'source-b');
const destination = path.join(testRoot, 'destination');
const restoreTarget = path.join(os.tmpdir(), 'abm-browser-restore-' + path.basename(testRoot));

function restoredPath(target, source) {
  const parsed = path.parse(source);
  if (parsed.root && /^[A-Za-z]:\\$/.test(parsed.root)) return path.join(target, parsed.root[0], source.slice(parsed.root.length));
  return path.join(target, source);
}

async function chooseServerFolder(page, value, keepOpen = false) {
  await page.getByLabel('Server path').fill(value);
  await page.getByRole('button', { name: 'Go', exact: true }).click();
  await page.locator('#fb-current').check();
  if (!keepOpen) await page.getByRole('button', { name: 'Use selected' }).click();
}

async function startRestoreAndWait(page) {
  const responsePromise = page.waitForResponse(response => response.url().endsWith('/api/restore') && response.request().method() === 'POST');
  await page.getByRole('button', { name: 'Start Restore' }).click();
  const payload = await (await responsePromise).json();
  await expect(page.locator('#restore-run-status')).toContainText('Completed successfully.', { timeout: 90000 });
  return payload.target;
}

test.beforeAll(() => {
  fs.rmSync(restoreTarget, { recursive: true, force: true });
  for (const dir of [sourceA, sourceB, destination]) fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(path.join(sourceA, 'first.txt'), 'first version');
  fs.writeFileSync(path.join(sourceB, 'second.txt'), 'second source');
});

test('rendered GUI completes setup, backup, recovery browsing, and safe restore', async ({ page }) => {
  const consoleErrors = [];
  page.on('console', message => { if (message.type() === 'error') consoleErrors.push(message.text()); });
  page.on('pageerror', error => consoleErrors.push(error.message));

  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Welcome to Auto-Backup-Manager' })).toBeVisible();
  await page.getByRole('button', { name: 'Next' }).click();
  await page.locator('#wiz-org').fill('Test Company');
  await page.locator('#wiz-device').fill('Test Server');
  await page.getByRole('button', { name: 'Next' }).click();
  await page.locator('#wiz-job').fill('Browser Acceptance App');
  await page.getByRole('button', { name: /Browse this server/ }).click();
  await chooseServerFolder(page, sourceA, true);
  await chooseServerFolder(page, sourceB, false);
  await expect(page.getByText(sourceA, { exact: true })).toBeVisible();
  await expect(page.getByText(sourceB, { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Next' }).click();
  await page.getByRole('button', { name: 'Browse', exact: true }).click();
  await chooseServerFolder(page, destination);
  await page.getByRole('button', { name: 'Next' }).click();
  await expect(page.getByRole('heading', { name: 'Review' })).toBeVisible();
  await page.getByRole('button', { name: 'Test & Run First Backup' }).click();
  await expect(page.getByRole('heading', { name: 'Protection verified' })).toBeVisible({ timeout: 90000 });
  await page.getByRole('button', { name: 'Open Dashboard' }).click();
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
  await expect(page.getByText('Browser Acceptance App', { exact: true })).toBeVisible();

  fs.writeFileSync(path.join(sourceA, 'first.txt'), 'modified version');
  fs.writeFileSync(path.join(sourceA, 'new.txt'), 'new file');
  fs.rmSync(path.join(sourceB, 'second.txt'));
  await page.getByRole('button', { name: 'Back Up Now' }).click();
  await expect(page.getByText('Completed successfully.')).toBeVisible({ timeout: 90000 });
  await page.evaluate(() => closeModal());

  await page.getByRole('link', { name: 'Recovery Points' }).click();
  await expect(page.locator('[data-browse-recovery]')).toHaveCount(2, { timeout: 30000 });
  const rows = page.locator('#snap-list tbody tr');
  const oldestRow = rows.filter({ hasNot: page.locator('.badge-ok') });
  await expect(oldestRow).toHaveCount(1);
  await oldestRow.getByRole('button', { name: 'Browse' }).click();
  await expect(page.getByText('Recovery manifest')).toBeVisible();
  await expect(page.getByText(sourceA, { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Close' }).click();

  await oldestRow.getByRole('link', { name: 'Restore' }).click();
  await expect(page.locator('#r-snapshot')).not.toHaveValue('latest');
  const oldestSnapshot = await page.locator('#r-snapshot').inputValue();
  fs.rmSync(sourceA, { recursive: true, force: true });
  fs.rmSync(sourceB, { recursive: true, force: true });
  const actualRestoreTarget = await startRestoreAndWait(page);
  expect(fs.readFileSync(path.join(restoredPath(actualRestoreTarget, sourceA), 'first.txt'), 'utf8')).toBe('first version');
  expect(fs.readFileSync(path.join(restoredPath(actualRestoreTarget, sourceB), 'second.txt'), 'utf8')).toBe('second source');

  await page.locator('#r-snapshot').selectOption('latest');
  const latestRestoreTarget = await startRestoreAndWait(page);
  expect(fs.readFileSync(path.join(restoredPath(latestRestoreTarget, sourceA), 'first.txt'), 'utf8')).toBe('modified version');
  expect(fs.readFileSync(path.join(restoredPath(latestRestoreTarget, sourceA), 'new.txt'), 'utf8')).toBe('new file');
  expect(fs.existsSync(path.join(restoredPath(latestRestoreTarget, sourceB), 'second.txt'))).toBe(false);

  // Exercise the destructive path only inside this isolated test tree. The
  // double browser confirmation is part of the acceptance contract.
  fs.mkdirSync(sourceA, { recursive: true });
  fs.writeFileSync(path.join(sourceA, 'first.txt'), 'must be overwritten');
  fs.mkdirSync(sourceB, { recursive: true });
  fs.writeFileSync(path.join(sourceB, 'second.txt'), 'must be overwritten');
  await page.locator('#r-snapshot').selectOption(oldestSnapshot);
  await page.locator('#r-inplace').check();
  page.on('dialog', async dialog => {
    await dialog.accept(dialog.type() === 'prompt' ? 'RESTORE ORIGINAL' : undefined);
  });
  const originalTarget = await startRestoreAndWait(page);
  expect(originalTarget).toBe('configured original locations');
  expect(fs.readFileSync(path.join(sourceA, 'first.txt'), 'utf8')).toBe('first version');
  expect(fs.readFileSync(path.join(sourceB, 'second.txt'), 'utf8')).toBe('second source');
  expect(consoleErrors).toEqual([]);
});

test('fresh dashboard tolerates null API lists', async ({ page }) => {
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.route('**/api/status', route => route.fulfill({ json: { hasConfig: true, deviceName: 'Null Test', jobs: null } }));
  await page.route('**/api/storage', route => route.request().method() === 'GET' ? route.fulfill({ json: null }) : route.continue());
  await page.route('**/api/doctor', route => route.fulfill({ json: null }));
  await page.goto('/#/dashboard');
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
  await expect(page.getByText('Nothing is protected yet.')).toBeVisible();
  expect(errors).toEqual([]);
});

test('provider, database, settings, and empty-state workflows are graphical', async ({ page }) => {
  const errors = [];
  let oauthRequest;
  let storageRequest;
  page.on('console', message => { if (message.type() === 'error') errors.push(message.text()); });
  page.on('pageerror', error => errors.push(error.message));

  await page.route('**/api/storage/oauth/start', async route => {
    oauthRequest = route.request().postDataJSON();
    await route.fulfill({ status: 202, json: { runId: 'mock-oauth', remote: 'abm-browser-google' } });
  });
  await page.route('**/api/runs/mock-oauth', route => route.fulfill({ json: { kind: 'oauth', stage: 'Connected', done: true, result: { remote: 'abm-browser-google' } } }));
  await page.route('**/api/storage', async route => {
    if (route.request().method() !== 'POST') return route.continue();
    storageRequest = route.request().postDataJSON();
    await route.fulfill({ json: { ok: true } });
  });

  await page.goto('/#/storage');
  const openProvider = async id => {
    await page.locator(`[data-connect-provider="${id}"]`).click();
  };
  await openProvider('google-drive');
  await expect(page.locator('#oauth-client-id')).toBeVisible();
  await expect(page.locator('#oauth-client-secret')).toHaveAttribute('type', 'password');
  await page.locator('#oauth-storage-name').fill('browser-google');
  await page.locator('#oauth-client-id').fill('mock-client-id');
  await page.locator('#oauth-client-secret').fill('mock-client-secret');
  await page.getByRole('button', { name: 'Connect Google Drive' }).click();
  await expect(page.locator('#modal-root')).toBeEmpty();
  expect(oauthRequest).toMatchObject({ provider: 'google-drive', storageName: 'browser-google', clientId: 'mock-client-id', clientSecret: 'mock-client-secret' });
  expect(storageRequest).toMatchObject({ provider: 'google-drive', name: 'browser-google', options: { remote: 'abm-browser-google' }, secrets: {} });

  await openProvider('onedrive');
  await expect(page.getByRole('button', { name: 'Connect Microsoft OneDrive' })).toBeVisible();
  await page.getByRole('button', { name: 'Cancel' }).click();
  await openProvider('dropbox');
  await expect(page.getByRole('button', { name: 'Connect Dropbox' })).toBeVisible();
  await page.getByRole('button', { name: 'Cancel' }).click();

  await openProvider('mega');
  await expect(page.getByText('MEGA account email')).toBeVisible();
  await expect(page.locator('[data-field="req:password"]')).toHaveAttribute('type', 'password');
  await expect(page.getByText(/rclone remote name/)).toHaveCount(0);
  await page.getByRole('button', { name: 'Cancel' }).click();

  await openProvider('generic-s3');
  await expect(page.getByText('Endpoint')).toBeVisible();
  await expect(page.locator('[data-field="req:secret_key"]')).toHaveAttribute('type', 'password');
  await page.getByRole('button', { name: 'Cancel' }).click();
  await openProvider('sftp');
  await expect(page.getByText('Hostname')).toBeVisible();
  await expect(page.getByText('SSH private key file (preferred)')).toBeVisible();
  await expect(page.locator('[data-field="opt:password"]')).toHaveCount(0);
  await page.getByRole('button', { name: 'Cancel' }).click();

  await page.goto('/#/jobs');
  await page.getByRole('button', { name: 'Create Job' }).click();
  await page.locator('#j-db-enabled').check();
  await expect(page.locator('#j-db-fields')).toBeVisible();
  await page.locator('#j-db-kind').selectOption('sqlite');
  await expect(page.getByText('SQLite database file on this server')).toBeVisible();
  await page.getByRole('button', { name: 'Cancel' }).click();

  await page.goto('/#/settings');
  await expect(page.locator('#s-device')).toHaveValue('Test Server');
  await expect(page.locator('#s-org')).toHaveValue('Test Company');

  await page.route('**/api/storage', route => route.request().method() === 'GET' ? route.fulfill({ json: [] }) : route.continue());
  await page.goto('/#/jobs');
  await page.getByRole('button', { name: 'Create Job' }).click();
  await expect(page.getByRole('heading', { name: 'No backup destination is connected yet' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Connect Storage' })).toBeVisible();
  expect(errors).toEqual([]);
});
