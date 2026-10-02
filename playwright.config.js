const path = require('path');
const os = require('os');
const { defineConfig } = require('@playwright/test');

const runID = String(process.pid);
const testRoot = path.join(os.tmpdir(), `abm-browser-home-${runID}`);
const dataRoot = path.join(os.tmpdir(), `abm-browser-data-${runID}`);
process.env.ABM_BROWSER_HOME = testRoot;
process.env.ABM_BROWSER_DATA = dataRoot;

module.exports = defineConfig({
  testDir: './test/browser',
  timeout: 180000,
  workers: 1,
  use: { baseURL: 'http://127.0.0.1:18766', headless: true, trace: 'retain-on-failure' },
  webServer: {
    command: 'node test/browser/server.js',
    url: 'http://127.0.0.1:18766/api/csrf',
    reuseExistingServer: false,
    timeout: 120000,
    env: { ...process.env, ABM_HOME: testRoot, ABM_BROWSER_HOME: testRoot, ABM_BROWSER_DATA: dataRoot },
  },
  reporter: [['list']],
});
