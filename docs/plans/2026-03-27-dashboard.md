# Dashboard UI Implementation Plan

> **For agentic workers:** Use magic-powers:subagent-driven-development to implement this plan.

**Goal:** Web UI tại `/dashboard` để monitor workers, tasks, costs — không cần curl.
**Architecture:** Static HTML/JS served bởi Go gateway, poll REST API mỗi 3s. Không cần build step, không cần framework.
**Tech Stack:** Vanilla HTML/CSS/JS, fetch API, Go stdlib.

---

## Files

**Create:**
- `core/internal/gateway/dashboard.go` — embed + serve dashboard HTML
- `core/internal/gateway/static/dashboard.html` — single-file UI
**Modify:**
- `core/internal/gateway/server.go` — register `/dashboard` route

---

### Task 1: Dashboard HTML shell

**Files:**
- Create: `core/internal/gateway/static/dashboard.html`

- [ ] Read `core/internal/gateway/server.go` to understand route registration pattern
- [ ] Create `core/internal/gateway/static/dashboard.html` — single file, inline CSS+JS:

```html
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>MagiC Dashboard</title>
<style>
:root {
  --bg: #0a0e17; --card: #111827; --border: #1e2d3d;
  --text: #e2e8f0; --dim: #64748b; --accent: #2dd4bf;
  --green: #22c55e; --yellow: #fbbf24; --red: #ef4444;
  --font: 'JetBrains Mono', monospace;
}
* { margin:0; padding:0; box-sizing:border-box; }
body { background:var(--bg); color:var(--text); font-family:var(--font); font-size:13px; }
header { padding:16px 24px; border-bottom:1px solid var(--border); display:flex; align-items:center; gap:12px; }
header h1 { font-size:16px; color:var(--accent); }
.status-dot { width:8px; height:8px; border-radius:50%; background:var(--green); }
.status-dot.offline { background:var(--red); }
main { padding:24px; display:grid; gap:24px; }
.stats { display:grid; grid-template-columns:repeat(4,1fr); gap:16px; }
.stat { background:var(--card); border:1px solid var(--border); border-radius:8px; padding:16px; }
.stat .label { color:var(--dim); font-size:11px; text-transform:uppercase; letter-spacing:.05em; }
.stat .value { font-size:28px; font-weight:700; color:var(--accent); margin-top:4px; }
.section { background:var(--card); border:1px solid var(--border); border-radius:8px; overflow:hidden; }
.section-header { padding:12px 16px; border-bottom:1px solid var(--border); display:flex; justify-content:space-between; align-items:center; }
.section-header h2 { font-size:13px; }
.section-header .refresh { color:var(--dim); font-size:11px; }
table { width:100%; border-collapse:collapse; }
th { padding:8px 16px; text-align:left; color:var(--dim); font-size:11px; text-transform:uppercase; border-bottom:1px solid var(--border); }
td { padding:10px 16px; border-bottom:1px solid var(--border); }
tr:last-child td { border-bottom:none; }
tr:hover td { background:rgba(255,255,255,.02); }
.badge { display:inline-block; padding:2px 8px; border-radius:4px; font-size:11px; }
.badge.online  { background:rgba(34,197,94,.15); color:var(--green); }
.badge.offline { background:rgba(239,68,68,.15); color:var(--red); }
.badge.pending   { background:rgba(251,191,36,.15); color:var(--yellow); }
.badge.running   { background:rgba(45,212,191,.15); color:var(--accent); }
.badge.completed { background:rgba(34,197,94,.15); color:var(--green); }
.badge.failed    { background:rgba(239,68,68,.15); color:var(--red); }
.empty { padding:24px; text-align:center; color:var(--dim); }
</style>
</head>
<body>
<header>
  <div class="status-dot" id="dot"></div>
  <h1>MagiC Dashboard</h1>
  <span style="color:var(--dim);margin-left:auto;font-size:11px" id="last-updated"></span>
</header>
<main>
  <div class="stats">
    <div class="stat"><div class="label">Workers</div><div class="value" id="stat-workers">—</div></div>
    <div class="stat"><div class="label">Tasks Total</div><div class="value" id="stat-tasks">—</div></div>
    <div class="stat"><div class="label">Running</div><div class="value" id="stat-running">—</div></div>
    <div class="stat"><div class="label">Total Cost</div><div class="value" id="stat-cost">—</div></div>
  </div>

  <div class="section">
    <div class="section-header"><h2>Workers</h2><span class="refresh" id="refresh-workers"></span></div>
    <table>
      <thead><tr><th>Name</th><th>ID</th><th>Capabilities</th><th>Load</th><th>Status</th></tr></thead>
      <tbody id="workers-body"><tr><td colspan="5" class="empty">Loading...</td></tr></tbody>
    </table>
  </div>

  <div class="section">
    <div class="section-header"><h2>Recent Tasks</h2><span class="refresh" id="refresh-tasks"></span></div>
    <table>
      <thead><tr><th>ID</th><th>Type</th><th>Worker</th><th>Status</th><th>Cost</th><th>Created</th></tr></thead>
      <tbody id="tasks-body"><tr><td colspan="6" class="empty">Loading...</td></tr></tbody>
    </table>
  </div>
</main>

<script>
const API = '';
let apiKey = new URLSearchParams(location.search).get('key') || '';

async function get(path) {
  const headers = apiKey ? { Authorization: 'Bearer ' + apiKey } : {};
  const r = await fetch(API + path, { headers });
  if (!r.ok) throw new Error(r.status);
  return r.json();
}

function badge(status) {
  return `<span class="badge ${status}">${status}</span>`;
}

function reltime(iso) {
  const s = Math.floor((Date.now() - new Date(iso)) / 1000);
  if (s < 60) return s + 's ago';
  if (s < 3600) return Math.floor(s/60) + 'm ago';
  return Math.floor(s/3600) + 'h ago';
}

async function refresh() {
  try {
    const [workers, tasks, costs] = await Promise.all([
      get('/api/v1/workers?limit=50'),
      get('/api/v1/tasks?limit=20'),
      get('/api/v1/costs').catch(() => ({ total_cost: 0 })),
    ]);

    document.getElementById('dot').className = 'status-dot';
    document.getElementById('last-updated').textContent = 'Updated ' + new Date().toLocaleTimeString();

    // Stats
    const wList = workers.workers || workers || [];
    const tList = tasks.tasks || tasks || [];
    document.getElementById('stat-workers').textContent = wList.length;
    document.getElementById('stat-tasks').textContent = tList.length;
    document.getElementById('stat-running').textContent = tList.filter(t => t.status === 'running').length;
    document.getElementById('stat-cost').textContent = '$' + ((costs.total_cost || 0).toFixed(4));

    // Workers table
    const wb = document.getElementById('workers-body');
    if (!wList.length) {
      wb.innerHTML = '<tr><td colspan="5" class="empty">No workers registered</td></tr>';
    } else {
      wb.innerHTML = wList.map(w => `
        <tr>
          <td>${w.name}</td>
          <td style="color:var(--dim);font-size:11px">${(w.id||'').slice(0,8)}…</td>
          <td style="color:var(--dim)">${(w.capabilities||[]).map(c=>c.name).join(', ')}</td>
          <td>${w.current_tasks||0}/${w.limits?.max_concurrent_tasks||'?'}</td>
          <td>${badge(w.status||'online')}</td>
        </tr>`).join('');
    }

    // Tasks table
    const tb = document.getElementById('tasks-body');
    if (!tList.length) {
      tb.innerHTML = '<tr><td colspan="6" class="empty">No tasks yet</td></tr>';
    } else {
      tb.innerHTML = tList.slice(0,20).map(t => `
        <tr>
          <td style="font-size:11px;color:var(--dim)">${(t.id||'').slice(0,8)}…</td>
          <td>${t.type||t.task_type||'—'}</td>
          <td style="color:var(--dim);font-size:11px">${(t.worker_id||'—').slice(0,8)}</td>
          <td>${badge(t.status||'unknown')}</td>
          <td>$${((t.cost||0).toFixed(4))}</td>
          <td style="color:var(--dim)">${t.created_at ? reltime(t.created_at) : '—'}</td>
        </tr>`).join('');
    }
  } catch(e) {
    document.getElementById('dot').className = 'status-dot offline';
  }
}

refresh();
setInterval(refresh, 3000);
</script>
</body>
</html>
```

- [ ] Commit: `feat(dashboard): HTML shell`

---

### Task 2: Serve dashboard from Go

**Files:**
- Create: `core/internal/gateway/dashboard.go`
- Modify: `core/internal/gateway/server.go`

- [ ] Read `core/internal/gateway/server.go` to find where routes are registered
- [ ] Create `core/internal/gateway/dashboard.go`:
  ```go
  package gateway

  import (
      _ "embed"
      "net/http"
  )

  //go:embed static/dashboard.html
  var dashboardHTML []byte

  func dashboardHandler(w http.ResponseWriter, r *http.Request) {
      w.Header().Set("Content-Type", "text/html; charset=utf-8")
      w.Write(dashboardHTML)
  }
  ```
- [ ] Add route in `server.go` — find where `/health` is registered and add next to it:
  ```go
  mux.HandleFunc("/dashboard", dashboardHandler)
  ```
- [ ] Build to verify: `cd core && go build ./cmd/magic`
- [ ] Manual test: `./bin/magic serve` → open `http://localhost:8080/dashboard`
- [ ] Commit: `feat(dashboard): serve dashboard UI at /dashboard`

---

### Task 3: Add to README

**Files:**
- Modify: `README.md`

- [ ] Add Dashboard to API Reference table:
  ```
  | `GET` | `/dashboard` | Web UI — monitor workers, tasks, costs |
  ```
- [ ] Add Dashboard to Roadmap (mark as done): `- [x] Dashboard — Web UI for monitoring`
- [ ] Commit: `docs: add dashboard to README`
