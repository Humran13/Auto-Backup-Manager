// Auto-Backup-Manager GUI frontend. Plain vanilla JS, no build step, no
// framework, no CDN -- fetches JSON from /api/* (the exact same internal Go
// core the CLI calls) and renders it. Works fully offline.
'use strict';

let CSRF = null;
const asArray = value => Array.isArray(value) ? value : [];
let pendingJobDraft = null;
let pendingRestoreIncludes = [];

async function api(path, opts) {
  opts = opts || {};
  const headers = Object.assign({}, opts.headers || {});
  if (opts.body) headers['Content-Type'] = 'application/json';
  if (opts.method && opts.method !== 'GET') headers['X-CSRF-Token'] = CSRF;
  const res = await fetch(path, Object.assign({}, opts, { headers }));
  let data = null;
  try { data = await res.json(); } catch (e) { /* empty body is fine */ }
  if (!res.ok) {
    const msg = (data && data.error) ? data.error : ('request failed (' + res.status + ')');
    throw new Error(msg);
  }
  return data;
}

function h(html) {
  const t = document.createElement('template');
  t.innerHTML = html.trim();
  return t.content.firstChild;
}

function esc(s) {
  if (s === undefined || s === null) return '';
  return String(s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

function badge(text, kind) {
  return `<span class="badge badge-${kind}">${esc(text)}</span>`;
}

function maturityBadge(m) {
  const kind = { stable: 'ok', supported: 'ok', experimental: 'warn', unavailable: 'muted' }[m] || 'muted';
  return badge(m, kind);
}

function setActiveNav(route) {
  document.querySelectorAll('#nav a').forEach(a => {
    a.classList.toggle('active', a.dataset.route === route);
  });
}

function showModal(innerHtml) {
  const root = document.getElementById('modal-root');
  root.innerHTML = `<div class="modal-backdrop" id="modal-backdrop"><div class="modal">${innerHtml}</div></div>`;
  document.getElementById('modal-backdrop').addEventListener('click', e => {
    if (e.target.id === 'modal-backdrop') closeModal();
  });
}
function closeModal() { document.getElementById('modal-root').innerHTML = ''; }

function alertBox(msg, kind) {
  return `<div class="alert alert-${kind || 'err'}">${esc(msg)}</div>`;
}

// Server-side path picker. Every entry comes from /api/files on the ABM
// host, so managing a VPS never opens the administrator laptop's filesystem.
function showServerBrowser(options) {
  options = options || {};
  const selected = new Set(asArray(options.selected));
  let current = options.start || '';
  showModal(`
    <h2>${esc(options.title || 'Choose folders on this server')}</h2>
    <p class="field-hint">This browser shows the filesystem of the machine running Auto-Backup-Manager.</p>
    <div id="fb-alert"></div>
    <div class="path-bar"><button class="secondary" id="fb-up">Up</button><input id="fb-path" type="text" aria-label="Server path"><button id="fb-go">Go</button></div>
    <div id="fb-roots" class="toolbar"></div>
    <div id="fb-entries" class="file-browser"></div>
    <div><b>Selected</b><div id="fb-selected" class="selected-paths"></div></div>
    <div class="modal-actions"><button class="secondary" id="fb-cancel">Cancel</button><button id="fb-done">Use selected</button></div>
  `);
  const renderSelected = () => {
    document.getElementById('fb-selected').innerHTML = selected.size
      ? [...selected].map(p => `<div class="selected-path"><span class="mono">${esc(p)}</span><button class="link danger-text" data-remove-path="${esc(p)}">Remove</button></div>`).join('')
      : '<span class="muted">No folders selected yet.</span>';
    document.querySelectorAll('[data-remove-path]').forEach(b => b.addEventListener('click', () => { selected.delete(b.dataset.removePath); renderSelected(); }));
  };
  const load = async path => {
    const alertEl = document.getElementById('fb-alert');
    try {
      const data = await api('/api/files' + (path ? '?path=' + encodeURIComponent(path) : ''));
      current = data.path;
      document.getElementById('fb-path').value = current;
      document.getElementById('fb-up').disabled = !data.parent;
      document.getElementById('fb-up').dataset.parent = data.parent || '';
      document.getElementById('fb-roots').innerHTML = asArray(data.roots).map(root => `<button class="secondary" data-root="${esc(root)}">${esc(root)}</button>`).join('');
      document.querySelectorAll('[data-root]').forEach(b => b.addEventListener('click', () => load(b.dataset.root)));
      document.getElementById('fb-entries').innerHTML = `
        <div class="file-entry current"><label><input type="checkbox" id="fb-current" ${selected.has(current) ? 'checked' : ''}> Select this folder: <span class="mono">${esc(current)}</span></label></div>
        ${asArray(data.entries).map(entry => `<div class="file-entry ${entry.isDir ? 'folder' : 'file'}">
          ${entry.isDir ? `<button class="link" data-open-path="${esc(entry.path)}">📁 ${esc(entry.name)}</button>
            <label><input type="checkbox" data-select-path="${esc(entry.path)}" ${selected.has(entry.path) ? 'checked' : ''}> select</label>` : `<span>📄 ${esc(entry.name)}</span>`}
        </div>`).join('')}`;
      document.getElementById('fb-current').addEventListener('change', e => { if (e.target.checked) selected.add(current); else selected.delete(current); renderSelected(); });
      document.querySelectorAll('[data-open-path]').forEach(b => b.addEventListener('click', () => load(b.dataset.openPath)));
      document.querySelectorAll('[data-select-path]').forEach(b => b.addEventListener('change', e => { if (e.target.checked) selected.add(e.target.dataset.selectPath); else selected.delete(e.target.dataset.selectPath); renderSelected(); }));
      alertEl.innerHTML = '';
    } catch (e) { alertEl.innerHTML = alertBox(e.message); }
  };
  document.getElementById('fb-up').addEventListener('click', e => { if (e.currentTarget.dataset.parent) load(e.currentTarget.dataset.parent); });
  document.getElementById('fb-go').addEventListener('click', () => load(document.getElementById('fb-path').value.trim()));
  document.getElementById('fb-cancel').addEventListener('click', closeModal);
  document.getElementById('fb-done').addEventListener('click', () => {
    if (!selected.size) { document.getElementById('fb-alert').innerHTML = alertBox('Select at least one folder.'); return; }
    const result = options.multiple === false ? [[...selected][selected.size - 1]] : [...selected];
    closeModal();
    options.onDone(result);
  });
  renderSelected(); load(current);
}

// ---------------------------------------------------------------------
// Router
// ---------------------------------------------------------------------

const routes = {
  dashboard: renderDashboard,
  storage: renderStorage,
  jobs: renderJobs,
  snapshots: renderSnapshots,
  restore: renderRestore,
  schedule: renderSchedule,
  databases: renderDatabases,
  activity: renderActivity,
  health: renderHealth,
  settings: renderSettings,
};

async function router() {
  const hash = location.hash.replace(/^#\//, '') || 'dashboard';
  const route = hash.split('?')[0];
  setActiveNav(route);
  const view = document.getElementById('view');

  try {
    const status = await api('/api/status');
    if (!status.hasConfig && route !== 'setup') {
      await runSetupWizard();
      return router();
    }
    const fn = routes[route] || renderDashboard;
    await fn(view, status);
  } catch (e) {
    view.innerHTML = alertBox('Error loading page: ' + e.message);
  }
}

window.addEventListener('hashchange', router);

async function boot() {
  const csrfResp = await fetch('/api/csrf');
  const csrfData = await csrfResp.json();
  CSRF = csrfData.token;
  router();
}
boot();

// ---------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------

async function renderDashboard(view, status) {
  status = status || await api('/api/status');
  const [storage, doctorChecks] = await Promise.all([
    api('/api/storage'),
    api('/api/doctor').catch(() => []),
  ]);

  const jobs = asArray(status && status.jobs);
  const stores = asArray(storage);
  const checks = asArray(doctorChecks);
  const jobCount = jobs.length;
  const lastSuccesses = jobs.filter(j => j.lastSuccess).sort((a, b) => b.lastSuccess.localeCompare(a.lastSuccess));
  const lastFailed = jobs.filter(j => j.lastError);
  const errCount = checks.filter(c => c.status === 'error').length;

  view.innerHTML = `
    <h1>Dashboard</h1>
    <div class="toolbar">
      <a class="btn" href="#/jobs">Add Backup Job</a>
      <a class="btn secondary" href="#/storage">Add Storage</a>
      <a class="btn secondary" href="#/restore">Restore</a>
      <a class="btn secondary" href="#/activity">View Logs</a>
    </div>
    <div class="grid">
      <div class="card stat"><div class="label">Device</div><div class="value">${esc(status.deviceName)}</div></div>
      <div class="card stat"><div class="label">Protected projects</div><div class="value">${jobCount}</div></div>
      <div class="card stat"><div class="label">Storage destinations</div><div class="value">${stores.length}</div></div>
      <div class="card stat"><div class="label">System health</div><div class="value">${errCount === 0 ? badge('Healthy', 'ok') : badge(errCount + ' issue(s)', 'err')}</div></div>
    </div>
    <div class="card">
      <h2>Jobs</h2>
      ${jobCount === 0 ? '<p class="muted">Nothing is protected yet. <a href="#/jobs">Add a backup</a>.</p>' : renderJobsTable(jobs)}
    </div>
    <div class="card">
      <h2>Recent issues</h2>
      ${lastFailed.length === 0 ? '<p class="muted">No recent failures.</p>' :
        '<ul>' + lastFailed.map(j => `<li><b>${esc(j.name)}</b>: ${esc(j.lastError)}</li>`).join('') + '</ul>'}
    </div>
  `;

  document.querySelectorAll('[data-run-job]').forEach(btn => {
    btn.addEventListener('click', () => runJobNow(btn.dataset.runJob));
  });
}

function renderJobsTable(jobs) {
  jobs = asArray(jobs);
  return `<table><thead><tr><th>Name</th><th>Status</th><th>Last success</th><th>Destinations</th><th></th></tr></thead><tbody>
    ${jobs.map(j => `<tr>
      <td>${esc(j.name)}</td>
      <td>${j.enabled ? (j.lastError ? badge('Failed', 'err') : (j.degraded ? badge('Degraded', 'warn') : badge('OK', 'ok'))) : badge('Disabled', 'muted')}</td>
      <td>${esc(j.lastSuccess || 'never')}</td>
      <td>${esc((j.destinations || []).join(', '))}</td>
      <td><button data-run-job="${esc(j.name)}">Back Up Now</button></td>
    </tr>`).join('')}
  </tbody></table>`;
}

async function runJobNow(name) {
  try {
    const { runId } = await api(`/api/jobs/${encodeURIComponent(name)}/run`, { method: 'POST', body: '{}' });
    showModal(`<h2>Running backup: ${esc(name)}</h2><div id="run-progress">Starting...</div>`);
    await pollRun(runId, document.getElementById('run-progress'));
  } catch (e) {
    alert('Failed to start backup: ' + e.message);
  }
}

async function pollRun(runId, el) {
  const start = Date.now();
  while (true) {
    const rs = await api('/api/runs/' + runId);
    const elapsed = Math.round((Date.now() - start) / 1000);
    el.innerHTML = `<p>${esc(rs.stage)} &mdash; ${elapsed}s elapsed</p>` +
      (rs.error ? alertBox(rs.error) : '') +
      (rs.done && !rs.error ? alertBox('Completed successfully.', 'ok') : '');
    if (rs.done) return rs;
    await new Promise(r => setTimeout(r, 1000));
  }
}

// ---------------------------------------------------------------------
// Storage
// ---------------------------------------------------------------------

async function renderStorage(view) {
  let [storage, providers] = await Promise.all([api('/api/storage'), api('/api/providers')]);
  storage = asArray(storage); providers = asArray(providers);
  const families = ['cloud-drive', 'object-storage', 'sftp', 'local', 'generic-rclone'];
  const familyLabel = { 'cloud-drive': 'Cloud Drive', 'object-storage': 'Object Storage', 'sftp': 'SFTP', 'local': 'Local / External Drive', 'generic-rclone': 'Other' };

  view.innerHTML = `
    <h1>Storage</h1>
    <div class="card">
      <h2>Configured destinations</h2>
      ${storage.length === 0 ? '<p class="muted">None yet.</p>' : `<table><thead><tr><th>Name</th><th>Provider</th><th>Maturity</th><th></th></tr></thead><tbody>
        ${storage.map(s => `<tr>
          <td>${esc(s.name)}</td><td>${esc(s.displayName)}</td><td>${maturityBadge(s.maturity)}</td>
          <td>
            <button class="secondary" data-test-storage="${esc(s.name)}">Test</button>
            ${s.backend === 'rclone' ? `<button class="secondary" data-reconnect-storage="${esc(s.name)}">Reconnect</button>` : ''}
            <button class="danger" data-remove-storage="${esc(s.name)}">Remove</button>
          </td>
        </tr>`).join('')}
      </tbody></table>`}
    </div>
    ${families.map(fam => `
      <div class="card">
        <h2>${familyLabel[fam]}</h2>
        <div class="provider-grid">
          ${providers.filter(p => p.family === fam).map(p => `
            <div class="provider-card ${p.unsupported ? 'unavailable' : ''}">
              <div class="name">${esc(p.displayName)}</div>
              ${maturityBadge(p.unsupported ? 'unavailable' : p.maturity)}
              <div class="note">${p.unsupported ? esc(p.unsupportedReason) : (p.experimental ? 'Experimental: see limitations before relying on it.' : '')}</div>
              <button ${p.unsupported ? 'disabled' : ''} data-connect-provider="${esc(p.id)}">${p.unsupported ? 'Unavailable' : 'Configure / Connect'}</button>
            </div>
          `).join('')}
        </div>
      </div>
    `).join('')}
  `;

  document.querySelectorAll('[data-connect-provider]').forEach(btn => {
    const p = providers.find(x => x.id === btn.dataset.connectProvider);
    btn.addEventListener('click', () => showAddStorageModal(p));
  });
  document.querySelectorAll('[data-test-storage]').forEach(btn => {
    btn.addEventListener('click', async () => {
      btn.disabled = true; btn.textContent = 'Testing...';
      try {
        const r = await api('/api/storage/test', { method: 'POST', body: JSON.stringify({ name: btn.dataset.testStorage }) });
        alert(r.ok ? 'Capability test passed.' : 'Capability test FAILED: ' + r.error);
      } catch (e) { alert('Test failed: ' + e.message); }
      btn.disabled = false; btn.textContent = 'Test';
    });
  });
  document.querySelectorAll('[data-reconnect-storage]').forEach(btn => {
    btn.addEventListener('click', async () => {
      try {
        await api('/api/storage/reconnect', { method: 'POST', body: JSON.stringify({ name: btn.dataset.reconnectStorage }) });
        alert('Reconnected.');
      } catch (e) { alert('Reconnect failed: ' + e.message); }
    });
  });
  document.querySelectorAll('[data-remove-storage]').forEach(btn => {
    btn.addEventListener('click', async () => {
      if (!confirm(`Remove storage "${btn.dataset.removeStorage}" from configuration? This never deletes remote data.`)) return;
      try {
        await api('/api/storage/' + encodeURIComponent(btn.dataset.removeStorage), { method: 'DELETE' });
        renderStorage(view);
      } catch (e) { alert('Remove failed: ' + e.message); }
    });
  });
}

function fieldInputs(fields, prefix) {
  return fields.map(f => `
    <div class="form-row">
      <label>${esc(f.label)}${f.required ? ' *' : ''}</label>
      <input type="${f.secret ? 'password' : 'text'}" data-field="${prefix}:${esc(f.key)}" placeholder="${esc(f.placeholder || f.default || '')}">
    </div>
  `).join('');
}

function showAddStorageModal(p) {
  if (p.id === 'local') {
    showLocalStorageModal(p);
    return;
  }
  if (p.auth === 'oauth') {
    showOAuthStorageModal(p);
    return;
  }

  showModal(`
    <h2>Configure ${esc(p.displayName)}</h2>
    ${p.experimental ? alertBox('This provider is EXPERIMENTAL. Review its limitations before relying on it.', 'warn') : ''}
    ${p.requiresOwnOAuthApp ? alertBox('Enter your own OAuth Client ID and Client Secret. Authorization continues here in the GUI.', 'warn') : ''}
    ${p.backend === 'rclone' ? alertBox('Connection and authorization are completed graphically. Saved tokens are never displayed.') : ''}
    <div id="modal-alert"></div>
    <div class="form-row"><label>Destination name (in Auto-Backup-Manager)</label><input id="st-name" placeholder="e.g. backblaze-primary"></div>
    ${fieldInputs(p.requiredFields, 'req')}
    ${fieldInputs(p.optionalFields, 'opt')}
    <div class="modal-actions">
      <button class="secondary" onclick="closeModal()">Cancel</button>
      <button id="st-save">Test &amp; Save</button>
    </div>
  `);
  document.getElementById('st-save').addEventListener('click', async () => {
    const name = document.getElementById('st-name').value.trim();
    const options = {}; const secrets = {};
    document.querySelectorAll('[data-field]').forEach(inp => {
      const [, key] = inp.dataset.field.split(':');
      const field = [...p.requiredFields, ...p.optionalFields].find(f => f.key === key);
      if (!inp.value) return;
      if (field && field.secret) secrets[key] = inp.value; else options[key] = inp.value;
    });
    await submitStorage(p, name, options, secrets, 'modal-alert');
  });
}

function showOAuthStorageModal(p) {
  showModal(`
    <h2>Connect ${esc(p.displayName)}</h2>
    <p>Sign in with the provider in your browser. Auto-Backup-Manager stores the resulting credential securely and never displays refresh tokens.</p>
    <div id="modal-alert"></div>
    <label>Destination name</label><input id="oauth-storage-name" placeholder="e.g. company-drive">
    ${p.requiresOwnOAuthApp ? `<label>OAuth Client ID</label><input id="oauth-client-id" autocomplete="off"><label>OAuth Client Secret</label><input type="password" id="oauth-client-secret" autocomplete="new-password"><p class="field-hint">Google requires credentials from your own Desktop OAuth application.</p>` : ''}
    <div class="modal-actions"><button class="secondary" onclick="closeModal()">Cancel</button><button id="oauth-connect">Connect ${esc(p.displayName)}</button></div>
  `);
  document.getElementById('oauth-connect').addEventListener('click', async () => {
    const alertEl = document.getElementById('modal-alert');
    const storageName = document.getElementById('oauth-storage-name').value.trim();
    const clientId = document.getElementById('oauth-client-id')?.value.trim() || '';
    const clientSecret = document.getElementById('oauth-client-secret')?.value || '';
    try {
      const started = await api('/api/storage/oauth/start', {method: 'POST', body: JSON.stringify({provider: p.id, storageName, clientId, clientSecret})});
      document.getElementById('oauth-connect').disabled = true;
      while (true) {
        const progress = await api('/api/runs/' + started.runId);
        const authURL = progress.result && progress.result.authorizationUrl;
        alertEl.innerHTML = `<p>${esc(progress.stage)}</p>${authURL ? `<p><a class="btn" href="${esc(authURL)}" target="_blank" rel="noopener">Open authorization page</a></p>` : ''}${progress.error ? alertBox(progress.error) : ''}`;
        if (progress.done) {
          if (progress.error) return;
          const remote = progress.result.remote;
          await submitStorage(p, storageName, {remote}, {}, 'modal-alert');
          return;
        }
        await new Promise(r => setTimeout(r, 500));
      }
    } catch (e) { alertEl.innerHTML = alertBox(e.message); }
  });
}

function showLocalStorageModal(p, initialName, initialPath) {
    showModal(`
      <h2>Add Local / External Disk</h2>
      <div id="modal-alert"></div>
      <div class="form-row"><label>Name</label><input id="st-name" placeholder="e.g. usb-drive" value="${esc(initialName || '')}"></div>
      <div class="form-row"><label>Destination folder on this server</label><div class="path-bar"><input id="st-path" value="${esc(initialPath || '')}" placeholder="Choose a folder"><button class="secondary" id="st-browse">Browse</button></div></div>
      <div class="modal-actions">
        <button class="secondary" onclick="closeModal()">Cancel</button>
        <button id="st-save">Test &amp; Save</button>
      </div>
    `);
    document.getElementById('st-save').addEventListener('click', async () => {
      const name = document.getElementById('st-name').value.trim();
      const path = document.getElementById('st-path').value.trim();
      await submitStorage(p, name, { path }, {}, 'modal-alert');
    });
    document.getElementById('st-browse').addEventListener('click', () => {
      const name = document.getElementById('st-name').value.trim();
      const path = document.getElementById('st-path').value.trim();
      showServerBrowser({ title: 'Choose backup destination folder', multiple: false, selected: path ? [path] : [], start: path, onDone: values => showLocalStorageModal(p, name, values[0]) });
    });
}

async function submitStorage(p, name, options, secrets, alertId) {
  const alertEl = document.getElementById(alertId);
  if (!name) { alertEl.innerHTML = alertBox('Name is required.'); return; }
  alertEl.innerHTML = '<p>Running capability test (init/backup/restore round-trip)...</p>';
  try {
    await api('/api/storage', { method: 'POST', body: JSON.stringify({ name, provider: p.id, options, secrets }) });
    closeModal();
    if (sessionStorage.getItem('abm-return-to-job')) {
      sessionStorage.removeItem('abm-return-to-job');
      location.hash = '#/jobs';
    } else {
      renderStorage(document.getElementById('view'));
    }
  } catch (e) {
    alertEl.innerHTML = alertBox(e.message);
  }
}

// ---------------------------------------------------------------------
// Backup Jobs
// ---------------------------------------------------------------------

async function renderJobs(view) {
  let [jobs, storage] = await Promise.all([api('/api/jobs'), api('/api/storage')]);
  jobs = asArray(jobs); storage = asArray(storage);
  view.innerHTML = `
    <h1>Backup Jobs</h1>
    <div class="toolbar"><button id="add-job">Create Job</button></div>
    <div class="card">
      ${jobs.length === 0 ? '<p class="muted">No jobs yet.</p>' : `<table><thead><tr><th>Name</th><th>Sources</th><th>Destinations</th><th>Status</th><th></th></tr></thead><tbody>
        ${jobs.map(j => `<tr>
          <td>${esc(j.name)}</td>
          <td class="mono">${j.sources.map(esc).join('<br>')}</td>
          <td>${esc(j.destinations.join(', '))}</td>
          <td>${j.enabled ? badge('Enabled', 'ok') : badge('Disabled', 'muted')}</td>
          <td>
            <button data-run="${esc(j.name)}">Run Now</button>
            <button class="secondary" data-toggle="${esc(j.name)}" data-enabled="${j.enabled}">${j.enabled ? 'Disable' : 'Enable'}</button>
            <button class="danger" data-delete="${esc(j.name)}">Delete</button>
          </td>
        </tr>`).join('')}
      </tbody></table>`}
    </div>
  `;
  document.getElementById('add-job').addEventListener('click', () => {
    if (!storage.length) {
      pendingJobDraft = pendingJobDraft || {};
      sessionStorage.setItem('abm-return-to-job', '1');
      view.innerHTML = `<h1>Add Backup</h1><div class="card empty-state"><h2>No backup destination is connected yet</h2><p>Connect storage first. Your backup setup will resume automatically.</p><a class="btn" href="#/storage">Connect Storage</a></div>`;
      return;
    }
    showJobModal(storage, pendingJobDraft || {});
  });
  document.querySelectorAll('[data-run]').forEach(btn => btn.addEventListener('click', () => runJobNow(btn.dataset.run)));
  document.querySelectorAll('[data-toggle]').forEach(btn => btn.addEventListener('click', async () => {
    await api(`/api/jobs/${encodeURIComponent(btn.dataset.toggle)}/enable`, { method: 'POST', body: JSON.stringify({ enabled: btn.dataset.enabled !== 'true' }) });
    renderJobs(view);
  }));
  document.querySelectorAll('[data-delete]').forEach(btn => btn.addEventListener('click', async () => {
    if (!confirm(`Delete job "${btn.dataset.delete}"? This never deletes the backup repository itself.`)) return;
    await api('/api/jobs/' + encodeURIComponent(btn.dataset.delete), { method: 'DELETE' });
    renderJobs(view);
  }));
  if (storage.length && pendingJobDraft && !document.getElementById('modal-root').innerHTML) showJobModal(storage, pendingJobDraft);
}

function captureJobDraft(existing) {
  if (!document.getElementById('j-name')) return existing || {};
  return {
    name: document.getElementById('j-name').value.trim(),
    sources: asArray((existing || {}).sources),
    destinations: [...document.querySelectorAll('.j-dest:checked')].map(c => c.value),
    policy: document.getElementById('j-policy').value,
    keepWithinHourly: document.getElementById('j-retention').value.trim(),
    databaseEnabled: document.getElementById('j-db-enabled').checked,
    database: {
      kind: document.getElementById('j-db-kind').value,
      name: document.getElementById('j-db-name').value.trim(),
      host: document.getElementById('j-db-host').value.trim(),
      port: Number(document.getElementById('j-db-port').value || 0),
      path: document.getElementById('j-db-path').value.trim(),
      username: document.getElementById('j-db-user').value.trim(),
      password: document.getElementById('j-db-password').value,
    },
  };
}

function showJobModal(storage, draft) {
  draft = draft || {};
  draft.sources = asArray(draft.sources);
  draft.destinations = asArray(draft.destinations);
  draft.database = draft.database || {};
  showModal(`
    <h2>Create Backup</h2>
    <div id="modal-alert"></div>
    <div class="form-row"><label>What are you protecting?</label><input id="j-name" value="${esc(draft.name || '')}" placeholder="e.g. Customer Portal"></div>
    <div class="form-row"><label>Folders and application data on this server</label>
      <div id="j-source-list" class="selected-paths">${draft.sources.length ? draft.sources.map(p => `<div class="selected-path"><span class="mono">${esc(p)}</span></div>`).join('') : '<span class="muted">No folders selected.</span>'}</div>
      <button class="secondary" id="j-browse-sources">Browse server &amp; select folders</button>
      <button class="secondary" id="j-detect-docker" ${draft.sources.length ? '' : 'disabled'}>Detect Docker / Compose data</button>
      <details><summary>Advanced: enter an absolute path</summary><div class="path-bar"><input id="j-manual-source" placeholder="/opt/my-app"><button class="secondary" id="j-add-manual">Add</button></div></details>
    </div>
    <div class="form-row"><label>Destinations</label>
      ${storage.map((s, i) => `<div class="checkbox-row"><input type="checkbox" value="${esc(s.name)}" class="j-dest" ${draft.destinations.includes(s.name) || (!draft.destinations.length && i === 0) ? 'checked' : ''}> ${esc(s.name)}</div>`).join('')}
    </div>
    <div class="form-row"><label>Destination policy</label>
      <select id="j-policy"><option value="primary-required" ${(draft.policy || 'primary-required') === 'primary-required' ? 'selected' : ''}>Primary required (default)</option><option value="all-required" ${draft.policy === 'all-required' ? 'selected' : ''}>All required</option></select>
    </div>
    <div class="form-row"><label>Recovery-point history</label><select id="j-retention"><option value="240h">10 days of hourly versions</option><option value="720h">30 days of hourly versions</option><option value="2160h">90 days of hourly versions</option></select></div>
    <div class="checkbox-row"><input type="checkbox" id="j-db-enabled" ${draft.databaseEnabled ? 'checked' : ''}> Include a consistent database backup</div>
    <div id="j-db-fields" class="subcard">
      <label>Database type</label><select id="j-db-kind"><option value="mysql">MySQL / MariaDB / Percona</option><option value="postgresql">PostgreSQL</option><option value="sqlite">SQLite</option></select>
      <label>Database name</label><input id="j-db-name" value="${esc(draft.database.name || '')}">
      <div class="db-network"><label>Host</label><input id="j-db-host" value="${esc(draft.database.host || 'localhost')}"><label>Port</label><input type="number" id="j-db-port" value="${esc(draft.database.port || '')}"><label>Username</label><input id="j-db-user" value="${esc(draft.database.username || '')}"><label>Password</label><input type="password" id="j-db-password" value="${esc(draft.database.password || '')}" autocomplete="new-password"></div>
      <div class="db-sqlite"><label>SQLite database file on this server</label><input id="j-db-path" value="${esc(draft.database.path || '')}"></div>
      <button class="secondary" id="j-db-test">Test connection</button><span id="j-db-result"></span>
    </div>
    <div class="modal-actions">
      <button class="secondary" onclick="closeModal()">Cancel</button>
      <button id="j-save">Create</button>
    </div>
  `);
  document.getElementById('j-retention').value = draft.keepWithinHourly || '240h';
  document.getElementById('j-db-kind').value = draft.database.kind || 'mysql';
  const toggleDB = () => {
    const enabled = document.getElementById('j-db-enabled').checked;
    const sqlite = document.getElementById('j-db-kind').value === 'sqlite';
    document.getElementById('j-db-fields').style.display = enabled ? 'block' : 'none';
    document.querySelector('.db-network').style.display = sqlite ? 'none' : 'block';
    document.querySelector('.db-sqlite').style.display = sqlite ? 'block' : 'none';
  };
  document.getElementById('j-db-enabled').addEventListener('change', toggleDB);
  document.getElementById('j-db-kind').addEventListener('change', toggleDB); toggleDB();
  document.getElementById('j-browse-sources').addEventListener('click', () => {
    pendingJobDraft = captureJobDraft(draft);
    showServerBrowser({ title: 'Choose what to protect', multiple: true, selected: pendingJobDraft.sources, onDone: values => { pendingJobDraft.sources = values; showJobModal(storage, pendingJobDraft); } });
  });
  document.getElementById('j-detect-docker').addEventListener('click', async () => {
    const current = captureJobDraft(draft);
    try {
      const result = await api('/api/docker/inspect?path=' + encodeURIComponent(current.sources[0]));
      const safe = asArray(result.suggestions).filter(s => s.path).map(s => s.path);
      const warnings = asArray(result.suggestions).filter(s => !s.path).map(s => s.message);
      pendingJobDraft = current; pendingJobDraft.sources = [...new Set([...current.sources, ...safe])];
      showJobModal(storage, pendingJobDraft);
      if (warnings.length) document.getElementById('modal-alert').innerHTML = alertBox(warnings.join(' '), 'warn');
    } catch (e) { document.getElementById('modal-alert').innerHTML = alertBox(e.message); }
  });
  document.getElementById('j-add-manual').addEventListener('click', () => {
    const value = document.getElementById('j-manual-source').value.trim();
    if (!value) return;
    pendingJobDraft = captureJobDraft(draft); pendingJobDraft.sources = [...new Set([...pendingJobDraft.sources, value])]; showJobModal(storage, pendingJobDraft);
  });
  document.getElementById('j-db-test').addEventListener('click', async () => {
    const state = captureJobDraft(draft); const result = document.getElementById('j-db-result'); result.textContent = ' Testing…';
    try { await api('/api/database/test', {method: 'POST', body: JSON.stringify(state.database)}); result.innerHTML = ' ' + badge('Connected', 'ok'); }
    catch (e) { result.innerHTML = ' ' + alertBox(e.message); }
  });
  document.getElementById('j-save').addEventListener('click', async () => {
    const alertEl = document.getElementById('modal-alert');
    const state = captureJobDraft(draft);
    const databases = state.databaseEnabled ? [state.database] : [];
    try {
      await api('/api/jobs', { method: 'POST', body: JSON.stringify({ name: state.name, sources: state.sources, destinations: state.destinations, policy: state.policy, keepWithinHourly: state.keepWithinHourly, databases }) });
      pendingJobDraft = null;
      closeModal();
      renderJobs(document.getElementById('view'));
    } catch (e) { alertEl.innerHTML = alertBox(e.message); }
  });
}

// ---------------------------------------------------------------------
// Snapshots
// ---------------------------------------------------------------------

async function renderSnapshots(view) {
  const jobs = asArray(await api('/api/jobs'));
  view.innerHTML = `
    <h1>Recovery Points</h1>
    <div class="card">
      <label>Job</label>
      <select id="snap-job">${jobs.map(j => `<option value="${esc(j.name)}">${esc(j.name)}</option>`).join('')}</select>
    </div>
    <div id="snap-list"></div>
  `;
  const sel = document.getElementById('snap-job');
  const load = async () => {
    const list = document.getElementById('snap-list');
    if (!sel.value) { list.innerHTML = ''; return; }
    list.innerHTML = '<p class="muted">Loading...</p>';
    try {
      const snaps = asArray(await api('/api/snapshots?job=' + encodeURIComponent(sel.value)));
      list.innerHTML = `<div class="card"><table><thead><tr><th>Date/time</th><th>ID</th><th>Host</th><th>Paths</th><th></th></tr></thead><tbody>
        ${snaps.map(s => `<tr>
          <td>${esc(s.time)} ${s.isLatest ? badge('latest', 'ok') : ''}</td>
          <td class="mono">${esc(s.shortId)}</td>
          <td>${esc(s.hostname)}</td>
          <td class="mono">${s.paths.map(esc).join('<br>')}</td>
          <td><button class="secondary" data-browse-recovery="${esc(s.id)}">Browse</button> <a class="btn secondary" href="#/restore?job=${encodeURIComponent(sel.value)}&snapshot=${encodeURIComponent(s.id)}">Restore</a></td>
        </tr>`).join('')}
      </tbody></table></div>`;
      document.querySelectorAll('[data-browse-recovery]').forEach(btn => btn.addEventListener('click', () => showRecoveryPointBrowser(sel.value, btn.dataset.browseRecovery)));
    } catch (e) { list.innerHTML = alertBox(e.message); }
  };
  sel.addEventListener('change', load);
  if (jobs.length > 0) load();
}

async function showRecoveryPointBrowser(jobName, snapshotId) {
  showModal('<h2>Browse recovery point</h2><p class="muted">Loading protected files…</p>');
  try {
    const data = await api('/api/recovery-point/contents?job=' + encodeURIComponent(jobName) + '&snapshot=' + encodeURIComponent(snapshotId));
    const files = asArray(data.files);
    showModal(`
      <h2>Browse recovery point</h2>
      <div class="subcard"><b>Recovery manifest</b><p>${esc(data.manifest.organization)} / ${esc(data.manifest.device)} / ${esc(data.manifest.job)}</p>
        <p class="field-hint">Originally protected from:</p><ul>${asArray(data.manifest.sources).map(p => `<li class="mono">${esc(p)}</li>`).join('')}</ul>
        ${asArray(data.manifest.databases).length ? `<p>Databases: ${data.manifest.databases.map(d => esc(d.kind + ': ' + d.name)).join(', ')}</p>` : ''}
      </div>
      <div class="file-browser recovery-tree">${files.map(f => `<label class="file-entry"><input type="checkbox" data-recovery-path="${esc(f.path)}"> ${f.type === 'dir' ? '📁' : '📄'} <span class="mono">${esc(f.path)}</span></label>`).join('') || '<p class="muted">This recovery point is empty.</p>'}</div>
      <div class="modal-actions"><button class="secondary" onclick="closeModal()">Close</button><button id="recovery-restore-all">Restore everything</button><button id="recovery-restore-selected">Restore selected</button></div>
    `);
    const goRestore = selected => { pendingRestoreIncludes = selected; closeModal(); location.hash = '#/restore?job=' + encodeURIComponent(jobName) + '&snapshot=' + encodeURIComponent(snapshotId); };
    document.getElementById('recovery-restore-all').addEventListener('click', () => goRestore([]));
    document.getElementById('recovery-restore-selected').addEventListener('click', () => {
      const chosen = [...document.querySelectorAll('[data-recovery-path]:checked')].map(x => x.dataset.recoveryPath);
      if (!chosen.length) { alert('Select at least one file or folder.'); return; }
      goRestore(chosen);
    });
  } catch (e) { showModal(`<h2>Browse recovery point</h2>${alertBox(e.message)}<div class="modal-actions"><button onclick="closeModal()">Close</button></div>`); }
}

// ---------------------------------------------------------------------
// Restore
// ---------------------------------------------------------------------

async function renderRestore(view) {
  const jobs = asArray(await api('/api/jobs'));
  const params = new URLSearchParams(location.hash.split('?')[1] || '');
  view.innerHTML = `
    <h1>Restore</h1>
    <div class="card">
      <label>Job</label>
      <select id="r-job">${jobs.map(j => `<option value="${esc(j.name)}" ${j.name === params.get('job') ? 'selected' : ''}>${esc(j.name)}</option>`).join('')}</select>
      <label>Backup version</label>
      <select id="r-snapshot"><option value="latest">latest</option></select>
      <label>Target directory (leave blank for a safe auto-generated location)</label>
      <input id="r-target" placeholder="auto">
      <div id="r-selection">${pendingRestoreIncludes.length ? alertBox(pendingRestoreIncludes.length + ' selected item(s) will be restored.', 'ok') : '<p class="field-hint">The whole project will be restored.</p>'}</div>
      <div class="checkbox-row"><input type="checkbox" id="r-inplace"> Restore in place (overwrites original data -- requires confirmation)</div>
      <div class="modal-actions" style="justify-content:flex-start">
        <button id="r-start" disabled>Start Restore</button>
      </div>
      <div id="r-progress"></div>
    </div>
  `;
  const jobSel = document.getElementById('r-job');
  const snapSel = document.getElementById('r-snapshot');
  const startButton = document.getElementById('r-start');
  const loadSnaps = async () => {
    startButton.disabled = true;
    if (!jobSel.value) return;
    try {
      const snaps = asArray(await api('/api/snapshots?job=' + encodeURIComponent(jobSel.value)));
      snapSel.innerHTML = '<option value="latest">latest</option>' +
        snaps.map(s => `<option value="${esc(s.id)}" ${s.id === params.get('snapshot') ? 'selected' : ''}>${esc(s.shortId)} - ${esc(s.time)}</option>`).join('');
      startButton.disabled = false;
    } catch (e) { /* no destination configured yet */ }
  };
  jobSel.addEventListener('change', loadSnaps);
  document.getElementById('r-inplace').addEventListener('change', event => {
    const target = document.getElementById('r-target');
    if (event.target.checked) target.value = '';
    target.disabled = event.target.checked;
  });
  if (jobs.length > 0) loadSnaps();

  document.getElementById('r-start').addEventListener('click', async () => {
    const inPlace = document.getElementById('r-inplace').checked;
    const progress = document.getElementById('r-progress');
    if (inPlace) {
      const job = jobs.find(j => j.name === jobSel.value);
      const paths = asArray(job && job.sources).join('\n');
      if (!confirm('This advanced restore can overwrite the original locations below:\n\n' + paths + '\n\nContinue?')) return;
      if (prompt('Type RESTORE ORIGINAL to confirm:') !== 'RESTORE ORIGINAL') return;
    }
    try {
      const { runId, target } = await api('/api/restore', {
        method: 'POST',
        body: JSON.stringify({
          job: jobSel.value, snapshotId: snapSel.value,
          target: document.getElementById('r-target').value.trim(),
          include: pendingRestoreIncludes, inPlace, confirm: inPlace,
        }),
      });
      progress.innerHTML = `<p>Restoring to <span class="mono">${esc(target)}</span>...</p><div id="restore-run-status"></div>`;
      await pollRun(runId, document.getElementById('restore-run-status'));
      pendingRestoreIncludes = [];
    } catch (e) { progress.innerHTML = alertBox(e.message); }
  });
}

// ---------------------------------------------------------------------
// Schedule
// ---------------------------------------------------------------------

async function renderSchedule(view) {
  const sched = await api('/api/schedule');
  view.innerHTML = `
    <h1>Schedule</h1>
    <div class="card">
      <h2>Default: every hour, 10 days of hourly history retained</h2>
      <p class="field-hint">Linux: systemd timer. Windows: Task Scheduler (runs as SYSTEM).</p>
      <div id="sched-alert"></div>
      <pre class="mono" style="white-space:pre-wrap">${esc(sched.status || sched.error || 'not installed yet')}</pre>
      <button id="sched-install">Install / Refresh Hourly Schedule</button>
    </div>
  `;
  document.getElementById('sched-install').addEventListener('click', async () => {
    const alertEl = document.getElementById('sched-alert');
    try {
      await api('/api/schedule', { method: 'POST', body: '{}' });
      alertEl.innerHTML = alertBox('Schedule installed.', 'ok');
      renderSchedule(view);
    } catch (e) { alertEl.innerHTML = alertBox(e.message); }
  });
}

// ---------------------------------------------------------------------
// Databases
// ---------------------------------------------------------------------

async function renderDatabases(view) {
  let [jobs, doctorChecks] = await Promise.all([api('/api/jobs'), api('/api/doctor')]);
  jobs = asArray(jobs); doctorChecks = asArray(doctorChecks);
  const dbTools = doctorChecks.filter(c => c.name.startsWith('db-tool:'));
  view.innerHTML = `
    <h1>Databases</h1>
    <div class="card">
      <h2>Dump tool availability</h2>
      <table><thead><tr><th>Tool</th><th>Status</th><th>Detail</th></tr></thead><tbody>
        ${dbTools.map(c => `<tr><td>${esc(c.name.replace('db-tool:', ''))}</td><td>${c.status === 'healthy' ? badge('OK', 'ok') : badge('Missing', 'err')}</td><td>${esc(c.detail)}</td></tr>`).join('')}
      </tbody></table>
      <p class="field-hint">MySQL/Percona/MariaDB, PostgreSQL, and SQLite are supported via safe logical dumps -- never a raw copy of a live datadir.</p>
    </div>
    <div class="card">
      <h2>Jobs with database hooks</h2>
      ${jobs.filter(j => j.databases && j.databases.length).length === 0 ? '<p class="muted">No backup includes a database yet. Choose “Include a consistent database backup” when adding a backup.</p>' :
        jobs.filter(j => j.databases && j.databases.length).map(j => `<p><b>${esc(j.name)}</b>: ${j.databases.map(d => esc(d.kind) + ' (' + esc(d.name) + ')').join(', ')}</p>`).join('')}
    </div>
  `;
}

// ---------------------------------------------------------------------
// Activity
// ---------------------------------------------------------------------

async function renderActivity(view) {
  const entries = asArray(await api('/api/activity'));
  view.innerHTML = `
    <h1>Activity / Logs</h1>
    <div class="card">
      ${entries.length === 0 ? '<p class="muted">No activity recorded yet.</p>' :
        entries.map(e => e.raw
          ? `<div class="log-line">${esc(e.raw)}</div>`
          : `<div class="log-line">[${esc(e.time)}] ${esc(e.level)} ${esc(e.job ? '(' + e.job + ') ' : '')}${esc(e.message)}</div>`
        ).join('')}
    </div>
  `;
}

// ---------------------------------------------------------------------
// System Health
// ---------------------------------------------------------------------

async function renderHealth(view) {
  const checks = asArray(await api('/api/doctor'));
  view.innerHTML = `
    <h1>System Health</h1>
    <div class="toolbar"><button id="recheck">Run checks again</button></div>
    <div class="card">
      <table><thead><tr><th>Check</th><th>Status</th><th>Detail</th></tr></thead><tbody>
        ${checks.map(c => `<tr><td>${esc(c.name)}</td><td>${c.status === 'healthy' ? badge('Healthy', 'ok') : badge('Error', 'err')}</td><td>${esc(c.detail)}</td></tr>`).join('')}
      </tbody></table>
    </div>
  `;
  document.getElementById('recheck').addEventListener('click', () => renderHealth(view));
}

// ---------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------

async function renderSettings(view) {
  const s = await api('/api/settings');
  view.innerHTML = `
    <h1>Settings</h1>
    <div class="card">
      <div id="settings-alert"></div>
      <label>Device name</label><input id="s-device" value="${esc(s.deviceName)}">
      <label>Organization</label><input id="s-org" value="${esc(s.organization)}">
      <label>Log level</label>
      <select id="s-loglevel">
        ${['debug', 'info', 'warn', 'error'].map(l => `<option value="${l}" ${l === s.logLevel ? 'selected' : ''}>${l}</option>`).join('')}
      </select>
      <p class="field-hint">Default retention: ${esc(s.defaultRetention || '240h (10 days hourly)')}. Repository passwords, OAuth tokens, and cloud secret keys are never shown here -- they live in the OS-native secret store.</p>
      <div class="modal-actions" style="justify-content:flex-start">
        <button id="s-save">Save</button>
      </div>
    </div>
  `;
  document.getElementById('s-save').addEventListener('click', async () => {
    const alertEl = document.getElementById('settings-alert');
    try {
      await api('/api/settings', {
        method: 'POST',
        body: JSON.stringify({
          deviceName: document.getElementById('s-device').value.trim(),
          organization: document.getElementById('s-org').value.trim(),
          logLevel: document.getElementById('s-loglevel').value,
        }),
      });
      alertEl.innerHTML = alertBox('Saved.', 'ok');
    } catch (e) { alertEl.innerHTML = alertBox(e.message); }
  });
}

// ---------------------------------------------------------------------
// First-run setup wizard
// ---------------------------------------------------------------------

async function runSetupWizard() {
  return new Promise(resolve => {
    const view = document.getElementById('view');
    let step = 0;
    const state = { deviceName: '', organization: '', jobName: '', sources: [], storageName: 'local-backup', storagePath: '', retention: '240h' };

    function render() {
      view.innerHTML = `
        <h1>Welcome to Auto-Backup-Manager</h1>
        <div class="wizard-progress">${[0, 1, 2, 3, 4, 5].map(i => `<div class="dot ${i <= step ? 'done' : ''}"></div>`).join('')}</div>
        <div class="card">
          <div class="wizard-step active" id="wiz-step">${stepHtml()}</div>
          <div class="modal-actions" style="justify-content:flex-start">
            ${step > 0 ? '<button class="secondary" id="wiz-back">Back</button>' : ''}
            <button id="wiz-next">${step === 5 ? 'Open Dashboard' : (step === 4 ? 'Test & Run First Backup' : 'Next')}</button>
          </div>
          <div id="wiz-alert"></div>
        </div>
      `;
      if (step > 0) document.getElementById('wiz-back').addEventListener('click', () => { step--; render(); });
      document.getElementById('wiz-next').addEventListener('click', onNext);
    }

    function stepHtml() {
      if (step === 0) {
        return `<h2>Protect the files and applications that matter</h2><p>This guided setup will connect storage, choose one or more folders on this server, create an encrypted versioned backup, and verify the first recovery point.</p><p>No Restic, rclone, repository syntax, or snapshot IDs are required.</p>`;
      }
      if (step === 1) {
        return `<label>Organization / company</label><input id="wiz-org" placeholder="e.g. Motion Ventures Ltd" value="${esc(state.organization)}">
          <label>Device / server name</label><input id="wiz-device" placeholder="e.g. MVL Web Server 01" value="${esc(state.deviceName)}">`;
      }
      if (step === 2) {
        return `<label>Backup name</label><input id="wiz-job" placeholder="e.g. Motion Ventures Website" value="${esc(state.jobName)}">
          <label>What should be protected?</label><div class="selected-paths">${state.sources.length ? state.sources.map(p => `<div class="selected-path mono">${esc(p)}</div>`).join('') : '<span class="muted">No folders selected.</span>'}</div>
          <button class="secondary" id="wiz-browse">Browse this server and select folders</button>`;
      }
      if (step === 3) {
        return `<h2>Choose a backup destination</h2><p>This first-run path configures local or externally mounted storage. Cloud accounts can also be connected graphically from Storage.</p>
          <label>Destination name</label><input id="wiz-storage-name" value="${esc(state.storageName)}">
          <label>Destination folder on this server</label><div class="path-bar"><input id="wiz-storage-path" value="${esc(state.storagePath)}" placeholder="Choose a folder"><button class="secondary" id="wiz-dest-browse">Browse</button></div>`;
      }
      if (step === 4) {
        return `<h2>Review</h2><dl><dt>Organization</dt><dd>${esc(state.organization)}</dd><dt>Device</dt><dd>${esc(state.deviceName)}</dd><dt>Backup</dt><dd>${esc(state.jobName)}</dd><dt>Sources</dt><dd>${state.sources.map(esc).join('<br>')}</dd><dt>Destination</dt><dd>${esc(state.storagePath)}</dd></dl>
          <label>Recovery-point history</label><select id="wiz-retention"><option value="240h">10 days of hourly versions</option><option value="720h">30 days of hourly versions</option></select>`;
      }
      return `<h2>Protection verified</h2><div id="wiz-result">${alertBox('The destination passed its read/write test and the first recovery point was created.', 'ok')}</div><p>Your dashboard is ready.</p>`;
    }

    async function onNext() {
      const alertEl = document.getElementById('wiz-alert');
      if (step === 1) {
        state.deviceName = document.getElementById('wiz-device').value.trim();
        state.organization = document.getElementById('wiz-org').value.trim();
        if (!state.deviceName || !state.organization) { alertEl.innerHTML = alertBox('Organization and device name are required.'); return; }
        try { await api('/api/setup', { method: 'POST', body: JSON.stringify({deviceName: state.deviceName, organization: state.organization}) }); }
        catch (e) { alertEl.innerHTML = alertBox(e.message); return; }
      }
      if (step === 2) {
        state.jobName = document.getElementById('wiz-job').value.trim();
        if (!state.jobName || !state.sources.length) { alertEl.innerHTML = alertBox('Give the backup a name and select at least one folder.'); return; }
      }
      if (step === 3) {
        state.storageName = document.getElementById('wiz-storage-name').value.trim();
        state.storagePath = document.getElementById('wiz-storage-path').value.trim();
        if (!state.storageName || !state.storagePath) { alertEl.innerHTML = alertBox('Choose and name a destination folder.'); return; }
      }
      if (step === 4) {
        state.retention = document.getElementById('wiz-retention').value;
        try {
          alertEl.innerHTML = '<p>Testing destination with a real encrypted write/read/delete round trip…</p>';
          await api('/api/storage', {method: 'POST', body: JSON.stringify({name: state.storageName, provider: 'local', options: {path: state.storagePath}, secrets: {}})});
          await api('/api/jobs', {method: 'POST', body: JSON.stringify({name: state.jobName, sources: state.sources, destinations: [state.storageName], policy: 'primary-required', keepWithinHourly: state.retention})});
          const run = await api('/api/jobs/' + encodeURIComponent(state.jobName) + '/run', {method: 'POST', body: '{}'});
          while (true) {
            const status = await api('/api/runs/' + run.runId);
            alertEl.innerHTML = `<p>${esc(status.stage)}…</p>`;
            if (status.done) { if (status.error) throw new Error(status.error); break; }
            await new Promise(r => setTimeout(r, 500));
          }
          const points = asArray(await api('/api/snapshots?job=' + encodeURIComponent(state.jobName)));
          if (!points.length) throw new Error('The backup finished but no recovery point could be verified.');
        } catch (e) { alertEl.innerHTML = alertBox(e.message); return; }
      }
      if (step === 5) { resolve(); return; }
      step++;
      render();
    }

    render();
    const originalRender = render;
    render = function() {
      originalRender();
      if (step === 2) document.getElementById('wiz-browse').addEventListener('click', () => {
        state.jobName = document.getElementById('wiz-job').value.trim();
        showServerBrowser({title: 'Choose what to protect', multiple: true, selected: state.sources, onDone: values => { state.sources = values; render(); }});
      });
      if (step === 3) document.getElementById('wiz-dest-browse').addEventListener('click', () => {
        state.storageName = document.getElementById('wiz-storage-name').value.trim();
        state.storagePath = document.getElementById('wiz-storage-path').value.trim();
        showServerBrowser({title: 'Choose destination folder', multiple: false, selected: state.storagePath ? [state.storagePath] : [], start: state.storagePath, onDone: values => { state.storagePath = values[0]; render(); }});
      });
    };
    render();
  });
}
