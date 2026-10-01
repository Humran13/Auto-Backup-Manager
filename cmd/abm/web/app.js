// Auto-Backup-Manager GUI frontend. Plain vanilla JS, no build step, no
// framework, no CDN -- fetches JSON from /api/* (the exact same internal Go
// core the CLI calls) and renders it. Works fully offline.
'use strict';

let CSRF = null;

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

  const jobCount = status.jobs.length;
  const lastSuccesses = status.jobs.filter(j => j.lastSuccess).sort((a, b) => b.lastSuccess.localeCompare(a.lastSuccess));
  const lastFailed = status.jobs.filter(j => j.lastError);
  const errCount = doctorChecks.filter(c => c.status === 'error').length;

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
      <div class="card stat"><div class="label">Jobs configured</div><div class="value">${jobCount}</div></div>
      <div class="card stat"><div class="label">Storage destinations</div><div class="value">${storage.length}</div></div>
      <div class="card stat"><div class="label">System health</div><div class="value">${errCount === 0 ? badge('Healthy', 'ok') : badge(errCount + ' issue(s)', 'err')}</div></div>
    </div>
    <div class="card">
      <h2>Jobs</h2>
      ${jobCount === 0 ? '<p class="muted">No jobs configured yet. <a href="#/jobs">Add one</a>.</p>' : renderJobsTable(status.jobs)}
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
  const [storage, providers] = await Promise.all([api('/api/storage'), api('/api/providers')]);
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
    showModal(`
      <h2>Add Local / External Disk</h2>
      <div id="modal-alert"></div>
      <div class="form-row"><label>Name</label><input id="st-name" placeholder="e.g. usb-drive"></div>
      <div class="form-row"><label>Destination folder (full path)</label><input id="st-path" placeholder="e.g. D:\\Backups or /mnt/backup"></div>
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
    return;
  }

  showModal(`
    <h2>Configure ${esc(p.displayName)}</h2>
    ${p.experimental ? alertBox('This provider is EXPERIMENTAL. See docs/providers/ for its limitations before relying on it.', 'warn') : ''}
    ${p.requiresOwnOAuthApp ? alertBox('Google requires your own OAuth Client ID/Secret for this provider -- see docs/providers/GOOGLE-DRIVE.md.', 'warn') : ''}
    ${p.backend === 'rclone' ? alertBox('Create the rclone remote first (rclone config, or Quick Connect below for supported providers), then enter its name here.') : ''}
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

async function submitStorage(p, name, options, secrets, alertId) {
  const alertEl = document.getElementById(alertId);
  if (!name) { alertEl.innerHTML = alertBox('Name is required.'); return; }
  alertEl.innerHTML = '<p>Running capability test (init/backup/restore round-trip)...</p>';
  try {
    await api('/api/storage', { method: 'POST', body: JSON.stringify({ name, provider: p.id, options, secrets }) });
    closeModal();
    renderStorage(document.getElementById('view'));
  } catch (e) {
    alertEl.innerHTML = alertBox(e.message);
  }
}

// ---------------------------------------------------------------------
// Backup Jobs
// ---------------------------------------------------------------------

async function renderJobs(view) {
  const [jobs, storage] = await Promise.all([api('/api/jobs'), api('/api/storage')]);
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
  document.getElementById('add-job').addEventListener('click', () => showJobModal(storage));
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
}

function showJobModal(storage) {
  showModal(`
    <h2>Create Backup Job</h2>
    <div id="modal-alert"></div>
    <div class="form-row"><label>Name</label><input id="j-name"></div>
    <div class="form-row"><label>Source folders (one per line)</label><textarea id="j-sources" rows="3" placeholder="/var/www&#10;/etc"></textarea></div>
    <div class="form-row"><label>Destinations</label>
      ${storage.length === 0 ? '<p class="muted">Add a storage destination first.</p>' :
        storage.map(s => `<div class="checkbox-row"><input type="checkbox" value="${esc(s.name)}" class="j-dest"> ${esc(s.name)}</div>`).join('')}
    </div>
    <div class="form-row"><label>Destination policy</label>
      <select id="j-policy"><option value="primary-required">Primary required (default)</option><option value="all-required">All required</option></select>
    </div>
    <div class="form-row"><label>Retention (keep hourly within)</label><input id="j-retention" value="240h" class="field-hint"></div>
    <div class="field-hint">Default: keep hourly recovery points for 10 days (240h).</div>
    <div class="modal-actions">
      <button class="secondary" onclick="closeModal()">Cancel</button>
      <button id="j-save">Create</button>
    </div>
  `);
  document.getElementById('j-save').addEventListener('click', async () => {
    const alertEl = document.getElementById('modal-alert');
    const name = document.getElementById('j-name').value.trim();
    const sources = document.getElementById('j-sources').value.split('\n').map(s => s.trim()).filter(Boolean);
    const destinations = [...document.querySelectorAll('.j-dest:checked')].map(c => c.value);
    const policy = document.getElementById('j-policy').value;
    const keepWithinHourly = document.getElementById('j-retention').value.trim();
    try {
      await api('/api/jobs', { method: 'POST', body: JSON.stringify({ name, sources, destinations, policy, keepWithinHourly }) });
      closeModal();
      renderJobs(document.getElementById('view'));
    } catch (e) { alertEl.innerHTML = alertBox(e.message); }
  });
}

// ---------------------------------------------------------------------
// Snapshots
// ---------------------------------------------------------------------

async function renderSnapshots(view) {
  const jobs = await api('/api/jobs');
  view.innerHTML = `
    <h1>Snapshots</h1>
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
      const snaps = await api('/api/snapshots?job=' + encodeURIComponent(sel.value));
      list.innerHTML = `<div class="card"><table><thead><tr><th>Date/time</th><th>ID</th><th>Host</th><th>Paths</th><th></th></tr></thead><tbody>
        ${snaps.map(s => `<tr>
          <td>${esc(s.time)} ${s.isLatest ? badge('latest', 'ok') : ''}</td>
          <td class="mono">${esc(s.shortId)}</td>
          <td>${esc(s.hostname)}</td>
          <td class="mono">${s.paths.map(esc).join('<br>')}</td>
          <td><a class="btn secondary" href="#/restore?job=${encodeURIComponent(sel.value)}&snapshot=${encodeURIComponent(s.id)}">Restore</a></td>
        </tr>`).join('')}
      </tbody></table></div>`;
    } catch (e) { list.innerHTML = alertBox(e.message); }
  };
  sel.addEventListener('change', load);
  if (jobs.length > 0) load();
}

// ---------------------------------------------------------------------
// Restore
// ---------------------------------------------------------------------

async function renderRestore(view) {
  const jobs = await api('/api/jobs');
  const params = new URLSearchParams(location.hash.split('?')[1] || '');
  view.innerHTML = `
    <h1>Restore</h1>
    <div class="card">
      <label>Job</label>
      <select id="r-job">${jobs.map(j => `<option value="${esc(j.name)}" ${j.name === params.get('job') ? 'selected' : ''}>${esc(j.name)}</option>`).join('')}</select>
      <label>Snapshot</label>
      <select id="r-snapshot"><option value="latest">latest</option></select>
      <label>Target directory (leave blank for a safe auto-generated location)</label>
      <input id="r-target" placeholder="auto">
      <div class="checkbox-row"><input type="checkbox" id="r-inplace"> Restore in place (overwrites original data -- requires confirmation)</div>
      <div class="modal-actions" style="justify-content:flex-start">
        <button id="r-start">Start Restore</button>
      </div>
      <div id="r-progress"></div>
    </div>
  `;
  const jobSel = document.getElementById('r-job');
  const snapSel = document.getElementById('r-snapshot');
  const loadSnaps = async () => {
    if (!jobSel.value) return;
    try {
      const snaps = await api('/api/snapshots?job=' + encodeURIComponent(jobSel.value));
      snapSel.innerHTML = '<option value="latest">latest</option>' +
        snaps.map(s => `<option value="${esc(s.id)}" ${s.id === params.get('snapshot') ? 'selected' : ''}>${esc(s.shortId)} - ${esc(s.time)}</option>`).join('');
    } catch (e) { /* no destination configured yet */ }
  };
  jobSel.addEventListener('change', loadSnaps);
  if (jobs.length > 0) loadSnaps();

  document.getElementById('r-start').addEventListener('click', async () => {
    const inPlace = document.getElementById('r-inplace').checked;
    const progress = document.getElementById('r-progress');
    if (inPlace && !confirm('This will OVERWRITE the original source data. Continue?')) return;
    try {
      const { runId, target } = await api('/api/restore', {
        method: 'POST',
        body: JSON.stringify({
          job: jobSel.value, snapshotId: snapSel.value,
          target: document.getElementById('r-target').value.trim(),
          inPlace, confirm: inPlace,
        }),
      });
      progress.innerHTML = `<p>Restoring to <span class="mono">${esc(target)}</span>...</p>`;
      await pollRun(runId, progress);
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
  const [jobs, doctorChecks] = await Promise.all([api('/api/jobs'), api('/api/doctor')]);
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
      ${jobs.filter(j => j.databases && j.databases.length).length === 0 ? '<p class="muted">No job has a database hook configured yet. Add one by editing config.yaml\'s job entry (databases:), then store its credential with \'abm job set-db-credentials\'.</p>' :
        jobs.filter(j => j.databases && j.databases.length).map(j => `<p><b>${esc(j.name)}</b>: ${j.databases.map(d => esc(d.kind) + ' (' + esc(d.name) + ')').join(', ')}</p>`).join('')}
    </div>
  `;
}

// ---------------------------------------------------------------------
// Activity
// ---------------------------------------------------------------------

async function renderActivity(view) {
  const entries = await api('/api/activity');
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
  const checks = await api('/api/doctor');
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
    const state = { deviceName: '', organization: '' };

    function render() {
      view.innerHTML = `
        <h1>Welcome to Auto-Backup-Manager</h1>
        <div class="wizard-progress">${[0, 1, 2].map(i => `<div class="dot ${i <= step ? 'done' : ''}"></div>`).join('')}</div>
        <div class="card">
          <div class="wizard-step active" id="wiz-step">${stepHtml()}</div>
          <div class="modal-actions" style="justify-content:flex-start">
            ${step > 0 ? '<button class="secondary" id="wiz-back">Back</button>' : ''}
            <button id="wiz-next">${step === 2 ? 'Finish' : 'Next'}</button>
          </div>
          <div id="wiz-alert"></div>
        </div>
      `;
      if (step > 0) document.getElementById('wiz-back').addEventListener('click', () => { step--; render(); });
      document.getElementById('wiz-next').addEventListener('click', onNext);
    }

    function stepHtml() {
      if (step === 0) {
        return `<p>Let's set up hourly, encrypted backups. This only takes a minute.</p>
          <label>Name this device</label><input id="wiz-device" placeholder="e.g. web-server-1" value="${esc(state.deviceName)}">`;
      }
      if (step === 1) {
        return `<label>Organization (used to scope backup paths)</label><input id="wiz-org" placeholder="e.g. acme-corp" value="${esc(state.organization)}">`;
      }
      return `<p>Ready to create your configuration for <b>${esc(state.deviceName)}</b> (${esc(state.organization)}).</p>
        <p class="field-hint">Next you'll add a storage destination and your first backup job from the Storage and Backup Jobs pages.</p>`;
    }

    async function onNext() {
      const alertEl = document.getElementById('wiz-alert');
      if (step === 0) {
        state.deviceName = document.getElementById('wiz-device').value.trim();
        if (!state.deviceName) { alertEl.innerHTML = alertBox('Device name is required.'); return; }
      }
      if (step === 1) {
        state.organization = document.getElementById('wiz-org').value.trim() || 'default-org';
      }
      if (step === 2) {
        try {
          await api('/api/setup', { method: 'POST', body: JSON.stringify(state) });
          resolve();
          return;
        } catch (e) { alertEl.innerHTML = alertBox(e.message); return; }
      }
      step++;
      render();
    }

    render();
  });
}
