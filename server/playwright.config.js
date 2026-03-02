const { defineConfig, devices } = require('@playwright/test');
const os = require('node:os');
const path = require('node:path');

const appPort = process.env.E2E_APP_PORT || '18080';
const consolePort = process.env.E2E_CONSOLE_PORT || '19090';
const dataPath = process.env.E2E_DATA_PATH || path.join(os.tmpdir(), `mdbrain-playwright-${Date.now()}-${process.pid}`);

module.exports = defineConfig({
  testDir: './test/e2e',
  timeout: 45_000,
  expect: {
    timeout: 10_000
  },
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  forbidOnly: !!process.env.CI,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  outputDir: 'test-results',
  use: {
    baseURL: `http://127.0.0.1:${consolePort}`,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure'
  },
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome']
      }
    }
  ],
  webServer: {
    command: 'clojure -M -m mdbrain.core',
    cwd: __dirname,
    url: `http://127.0.0.1:${consolePort}/console/init`,
    reuseExistingServer: !process.env.CI,
    timeout: 180_000,
    env: {
      ...process.env,
      APP_PORT: String(appPort),
      CONSOLE_PORT: String(consolePort),
      DATA_PATH: dataPath,
      MDBRAIN_LOG_LEVEL: process.env.MDBRAIN_LOG_LEVEL || 'INFO',
      STORAGE_TYPE: process.env.STORAGE_TYPE || 'local'
    }
  }
});
