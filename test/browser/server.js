const { spawn, spawnSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const root = process.env.ABM_BROWSER_HOME;
const dataRoot = process.env.ABM_BROWSER_DATA;
fs.mkdirSync(root, { recursive: true });
fs.mkdirSync(dataRoot, { recursive: true });
const executable = path.join(root, process.platform === 'win32' ? 'abm-browser-test.exe' : 'abm-browser-test');
const built = spawnSync('go', ['build', '-o', executable, './cmd/abm'], { cwd: path.resolve(__dirname, '..', '..'), stdio: 'inherit' });
if (built.status !== 0) process.exit(built.status || 1);

const child = spawn(executable, ['gui', '--no-open', '--port', '18766'], {
  cwd: path.resolve(__dirname, '..', '..'),
  env: { ...process.env, ABM_HOME: root },
  stdio: 'inherit',
});
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => child.kill(signal));
process.on('exit', () => { if (!child.killed) child.kill(); });
child.on('exit', code => process.exit(code ?? 0));
