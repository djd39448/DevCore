// views.jsx — Tasks, Memory, Agents, Gates, Settings

// ─── Tasks ──────────────────────────────────────────────────────────
const TASKS = [
  { id: 'T-200', d: 0, title: 'Phase 3 · port atlas to iOS',                       agent: 'conductor', status: 'active',  st: 'is-active' },
  { id: 'T-201', d: 1, title: 'Behavior spec from web source',                     agent: 'analyst',   status: 'done',    st: 'is-ok' },
  { id: 'T-202', d: 1, title: 'Shared contract — API + data model',                agent: 'architect', status: 'done',    st: 'is-ok' },
  { id: 'T-203', d: 1, title: 'Track plan: backend, data, ios',                    agent: 'conductor', status: 'review',  st: 'is-warn' },
  { id: 'T-204', d: 2, title: 'Wire memory_recall to ollama embeddings',           agent: 'builder/backend', status: 'active', st: 'is-active' },
  { id: 'T-205', d: 2, title: 'Episodic events — schema migrations',               agent: 'builder/backend', status: 'review', st: 'is-warn' },
  { id: 'T-206', d: 2, title: 'Supabase schema for cookbook + sessions',           agent: 'builder/data',    status: 'active', st: 'is-active' },
  { id: 'T-207', d: 2, title: 'SwiftUI shell — tab bar + nav stacks',              agent: 'builder/ios',     status: 'pending', st: '' },
  { id: 'T-208', d: 2, title: 'Recipe detail view — image grid',                   agent: 'builder/ios',     status: 'pending', st: '' },
  { id: 'T-209', d: 1, title: 'Reviewer pass — dc-07 checklist',                   agent: 'reviewer',        status: 'pending', st: '' },
  { id: 'T-210', d: 1, title: 'Verifier — local model wiring test',                agent: 'verifier',        status: 'blocked', st: 'is-blocked' },
  { id: 'T-211', d: 2, title: 'Revisit contract: recall ranking parameters',       agent: 'architect',       status: 'pending', st: '' },
];

// taskStatusClass maps a DevCore task status to the prototype's pill class.
// Unknown statuses fall through to the neutral pill.
const TASK_STATUS_CLASS = {
  active: 'is-active',
  review: 'is-warn',
  done: 'is-ok',
  blocked: 'is-blocked',
  pending: '',
  abandoned: '',
};

function TasksView() {
  // When the API is reachable, render live tasks; otherwise show the
  // prototype's fixed sample so the layout never blanks out.
  const live = window.DevCoreAPI ? window.DevCoreAPI.useTasks() : { data: null };
  const rows = live.data
    ? live.data.map(t => ({
        id: t.id,
        d: 0, // depth from real tasks is flat for now; tree-mode arrives with Phase 3
        title: t.title,
        agent: t.assigned_agent || '—',
        status: t.status,
        st: TASK_STATUS_CLASS[t.status] || '',
        updated: t.updated_at || '',
      }))
    : TASKS.map(t => ({ ...t, updated: '' }));

  const counts = countByStatus(rows);

  return (
    <div className="dc-fade-in">
      <div className="dc-page-h">
        <span className="dc-page-eyebrow">Plan</span>
        <h1 className="dc-page-title">Tasks</h1>
        <div style={{ marginLeft: 'auto', display: 'flex', gap: 8 }}>
          <button className="dc-btn"><span style={{ fontFamily: 'var(--mono)', fontSize: 11 }}>⌘K</span> jump</button>
          <button className="dc-btn is-primary">+ new task</button>
        </div>
      </div>
      <p className="dc-page-sub">
        The Conductor decomposes each cycle into tasks. Status pills mirror the
        episodic store; new agents pick up <em>active</em> rows from the top.
      </p>

      <div className="dc-card">
        <div className="dc-card-h">
          <div className="dc-card-h-title">Tasks{live.data ? ' · live' : ' · placeholder'}</div>
          <span className="dc-mono-faint">
            {counts.active} active · {counts.review} review · {counts.pending} pending · {counts.blocked} blocked
          </span>
        </div>
        <div style={{ padding: '4px 18px 8px' }}>
          <div className="tasks">
            <div className="task-row" style={{ borderTop: 0, color: 'var(--ink-faint)', fontFamily: 'var(--mono)', fontSize: 10, textTransform: 'uppercase', letterSpacing: '0.1em' }}>
              <span/><span>id</span><span>title</span><span>status</span><span>agent</span><span>updated</span>
            </div>
            {rows.map((t, i) => (
              <div className={`task-row ${t.d === 1 ? 'is-child' : t.d === 2 ? 'is-grandchild' : ''}`} key={t.id}>
                <span className="tw">{t.d === 0 ? '▾' : t.d === 1 ? '└' : '·'}</span>
                <span className="id">{t.id}</span>
                <span className="ti">{t.title}</span>
                <span><span className={`dc-pill ${t.st}`}><span className="dot"/>{t.status}</span></span>
                <span className="ag">{t.agent}</span>
                <span className="dc-mono-faint" style={{ textAlign: 'right' }}>
                  {t.updated || ['just now', '2m', '4m', '12m', '1h', '3h'][i % 6]}
                </span>
              </div>
            ))}
            {rows.length === 0 && (
              <div className="task-row" style={{ gridTemplateColumns: '1fr', color: 'var(--ink-muted)', fontStyle: 'italic' }}>
                <span>No tasks yet. The Conductor will populate this list in Phase 3.</span>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

// countByStatus produces the header summary line. Unknown statuses are
// counted under "other" so the totals always tie back to rows.length.
function countByStatus(rows) {
  const out = { active: 0, review: 0, pending: 0, blocked: 0, done: 0, other: 0 };
  for (const r of rows) {
    if (Object.prototype.hasOwnProperty.call(out, r.status)) out[r.status] += 1;
    else out.other += 1;
  }
  return out;
}

// ─── Memory ─────────────────────────────────────────────────────────
const RECALL_RESULTS = [
  { type: 'decision', score: 0.94, title: 'ADR-0008 — reciprocal-rank fusion for recall',
    body: 'Hybrid recall is computed in Go over the events log. Keyword token overlap and brute-force vector distance are fused with reciprocal rank. No SQLite extensions are used — the pure-Go path holds at project scale.', ref: 'decisions/0008-recall-fusion.md', date: '2026-05-22' },
  { type: 'learning', score: 0.81, title: 'Verifier on local profile — proxy wiring',
    body: 'Verifier runs against claude-code-router → Ollama (llama3.1). The proxy must translate tool-use and tool-result blocks faithfully; LiteLLM remains the documented fallback if fidelity issues surface.', ref: 'events#9821', date: '2026-05-20' },
  { type: 'contract', score: 0.76, title: 'memory_recall(query, scope?, limit?)',
    body: 'Hybrid keyword + vector recall across episodic events and (optionally) canonical docs. Scope: events | canonical | both. Limit defaults to 20.', ref: 'contract/contract.md#L42', date: '2026-05-18' },
  { type: 'correction', score: 0.71, title: 'Avoid CGO — modernc.org/sqlite',
    body: 'Earlier wiring drafted with mattn/go-sqlite3. Reversed: portability (P1) and single-binary story require pure-Go. modernc.org/sqlite is the chosen driver.', ref: 'events#7402', date: '2026-05-15' },
  { type: 'note', score: 0.62, title: 'CLAUDE.md stays thin',
    body: 'The thin CLAUDE.md loads the MEMORY.md index and pointers — never the whole corpus. Deep content is retrieved on demand via the memory MCP. This is the gap in Claude Code\u2019s native memory that DevCore closes.', ref: 'CLAUDE.md', date: '2026-05-12' },
];

function MemoryView() {
  const [q, setQ] = React.useState('recall');
  const results = RECALL_RESULTS.filter(r =>
    !q || (r.title + r.body + r.ref).toLowerCase().includes(q.toLowerCase())
  );

  return (
    <div className="dc-fade-in">
      <div className="dc-page-h">
        <span className="dc-page-eyebrow">Memory</span>
        <h1 className="dc-page-title">Recall</h1>
      </div>
      <p className="dc-page-sub">
        Hybrid keyword + vector search across the episodic store and canonical docs.
        Fused with reciprocal-rank. Ranked by score; click a row to open the source.
      </p>

      <div className="dc-search" style={{ height: 40, fontSize: 13, marginBottom: 6 }}>
        <svg width="14" height="14" viewBox="0 0 14 14" fill="none" style={{ color: 'var(--ink-faint)' }}>
          <circle cx="6" cy="6" r="4.25" stroke="currentColor" strokeWidth="1"/>
          <path d="M9.5 9.5l3 3" stroke="currentColor" strokeWidth="1" strokeLinecap="round"/>
        </svg>
        <input value={q} onChange={e => setQ(e.target.value)} placeholder="memory_recall(query, scope?, limit?)"/>
        <span className="dc-mono-faint">{results.length} matches</span>
        <kbd>⌘ K</kbd>
      </div>

      <div style={{ display: 'flex', gap: 8, marginBottom: 14 }}>
        <span className="dc-pill is-active"><span className="dot"/>all</span>
        <span className="dc-pill">events</span>
        <span className="dc-pill">canonical</span>
        <span className="dc-pill">decisions</span>
        <span className="dc-pill">corrections</span>
        <span style={{ marginLeft: 'auto' }} className="dc-mono-faint">scope · both · limit 20</span>
      </div>

      <div className="dc-card">
        <div style={{ padding: '4px 22px 16px' }}>
          {results.map(r => (
            <div className="recall-result" key={r.title}>
              <div className="recall-h">
                <span className="recall-type">{r.type}</span>
                <span className="recall-title">{r.title}</span>
                <span className="recall-meta">{r.ref} · {r.date} · score {r.score.toFixed(2)}</span>
              </div>
              <p className="recall-body" style={{ margin: 0, marginLeft: 88 }}
                 dangerouslySetInnerHTML={{ __html: highlight(r.body, q) }}/>
            </div>
          ))}
          {results.length === 0 && (
            <div style={{ padding: '40px 0', textAlign: 'center', fontFamily: 'var(--serif)', fontStyle: 'italic', color: 'var(--ink-muted)' }}>
              No matches for "{q}". Try a broader query, or scope to canonical.
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function highlight(text, q) {
  if (!q) return text;
  const safe = text.replace(/[&<>]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]));
  const re = new RegExp('(' + q.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + ')', 'gi');
  return safe.replace(re, '<mark>$1</mark>');
}

// ─── Agents ─────────────────────────────────────────────────────────
const AGENT_CARDS = [
  { role: 'plans · routes · gates',     name: 'Conductor', desc: 'Decomposes the goal into a task tree, dispatches work, manages human gates.', model: 'claude-sonnet-4-6',  profile: 'api' },
  { role: 'reads existing systems',     name: 'Analyst',   desc: 'Extracts behavior specs and requirements from the source workload.',         model: 'claude-sonnet-4-6',  profile: 'api' },
  { role: 'system design · contracts',  name: 'Architect', desc: 'Authors the shared contract and ADRs. Owns schema design.',                  model: 'claude-sonnet-4-6',  profile: 'api' },
  { role: 'implementation · per-track', name: 'Builder',   desc: 'One role, three track packs — backend (Go/AWS), data (Supabase), ios (SwiftUI).', model: 'claude-sonnet-4-6', profile: 'api' },
  { role: 'review · standards',         name: 'Reviewer',  desc: 'Enforces the dc-07 checklist on every change. Security review.',             model: 'claude-sonnet-4-6',  profile: 'api' },
  { role: 'builds · tests · verifies',  name: 'Verifier',  desc: 'Local-pinned. Runs builds, tests, device runs; reports pass/fail.',          model: 'llama3.1',           profile: 'local' },
];

function AgentsView() {
  return (
    <div className="dc-fade-in">
      <div className="dc-page-h">
        <span className="dc-page-eyebrow">Roster</span>
        <h1 className="dc-page-title">Agents</h1>
      </div>
      <p className="dc-page-sub">
        Six roles, named by function. Builder is one role with three track packs;
        Verifier is local-pinned. Swap a profile to repoint the brain.
      </p>
      <div className="agents-grid">
        {AGENT_CARDS.map(a => (
          <div className="agent-card" key={a.name}>
            <div className="role">{a.role}</div>
            <div className="name">{a.name}</div>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end' }}>
              <span className={`dc-pill ${a.profile === 'local' ? 'is-verify' : 'is-ok'}`}>
                <span className="dot"/>{a.profile}
              </span>
            </div>
            <div className="desc">{a.desc}</div>
            <div className="model">
              <span style={{ color: 'var(--ink-faint)' }}>model</span>
              <span>{a.model}</span>
              <span className="swap">swap →</span>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

// ─── Gates ──────────────────────────────────────────────────────────
const GATES = [
  { type: 'after',  step: 'track_plan',     title: 'Approve cycle-14 track plan',
    body: 'Conductor proposes three tracks for the iOS port: backend (Go/AWS), data (Supabase schema + RLS), ios (SwiftUI shell + recipe views). Estimated 14 tasks; 2 carry over from cycle 13.',
    refs: 'plan/cycle-14.md · 12 minutes ago', state: 'pending' },
  { type: 'after',  step: 'contract',       title: 'Confirm recall ranking parameters',
    body: 'Architect updated the shared contract to lock RRF parameters (k=60) for memory_recall. Affects every agent\u2019s view of the episodic store. Diff: contract/contract.md +18 −4.',
    refs: 'contract/contract.md · 38 minutes ago', state: 'pending' },
  { type: 'before', step: 'deploy',         title: 'Promote builder/data to apply migrations',
    body: 'Three migrations queued against the dev Supabase project. Reviewer signed off; verifier is green. Awaiting human go-ahead — destructive operation.',
    refs: 'migrations/0004-0006.sql · 1 hour ago', state: 'pending' },
];

function GatesView() {
  return (
    <div className="dc-fade-in">
      <div className="dc-page-h">
        <span className="dc-page-eyebrow">Human gates</span>
        <h1 className="dc-page-title">Approvals</h1>
        <span style={{ marginLeft: 'auto' }} className="dc-pill is-active"><span className="dot"/>3 pending</span>
      </div>
      <p className="dc-page-sub">
        DevCore proposes; you approve at defined milestones. Autonomy is earned
        phase by phase. Anything destructive surfaces here.
      </p>

      {GATES.map(g => (
        <div className="gate-row" key={g.title}>
          <div>
            <div className="gate-eyebrow">{g.type} · {g.step}</div>
            <div className="gate-title">{g.title}</div>
            <div className="gate-body">{g.body}</div>
            <div className="gate-meta">
              <span>↳ {g.refs}</span>
              <span>· awaiting human</span>
            </div>
          </div>
          <div className="gate-actions">
            <button className="dc-btn is-primary" style={{ height: 32 }}>Approve</button>
            <button className="dc-btn" style={{ height: 32 }}>Request changes</button>
            <button className="dc-btn is-ghost" style={{ height: 28, justifyContent: 'center' }}>Open diff</button>
          </div>
        </div>
      ))}
    </div>
  );
}

// ─── Settings (config + doctor) ─────────────────────────────────────
function SettingsView() {
  return (
    <div className="dc-fade-in">
      <div className="dc-page-h">
        <span className="dc-page-eyebrow">Configuration</span>
        <h1 className="dc-page-title">Settings</h1>
      </div>
      <p className="dc-page-sub">
        One file defines a deployment. Repoint the workload, or swap every agent
        from the API profile to local — edit only <code style={{ fontFamily: 'var(--mono)', fontSize: 12.5 }}>devcore.config.yaml</code>.
      </p>

      <div className="dc-card-h" style={{ padding: '0 0 12px', border: 0 }}>
        <div className="dc-card-h-title">System health</div>
        <span className="dc-mono-faint">last check · 12s ago</span>
      </div>

      <div className="dc-card" style={{ padding: '0 22px', marginBottom: 28 }}>
        <Health name="MCP · devcore-memory" desc="local memory server (Go, modernc.org/sqlite)" meta="stdio · pid 41023" status="ok" statusText="connected" />
        <Health name="MCP · pinecone"        desc="coding-standards reference, read-only" meta="stdio · pid 41024" status="ok" statusText="connected" />
        <Health name="Ollama"                desc="embeddings · nomic-embed-text" meta="localhost:11434" status="ok" statusText="reachable" />
        <Health name="claude-code-router"    desc="translation proxy for local profile" meta="localhost:8787" status="warn" statusText="degraded" />
        <Health name="Anthropic API"         desc="api profile · claude-sonnet-4-6" meta="api.anthropic.com" status="ok" statusText="ok" />
      </div>

      <div className="dc-card-h" style={{ padding: '0 0 12px', border: 0 }}>
        <div className="dc-card-h-title">devcore.config.yaml</div>
        <button className="dc-btn">Open in editor →</button>
      </div>

      <pre className="yaml">
{`project:
  `}<span className="k">name</span>:          <span className="v">atlas-port-ios</span>{`           `}<span className="c"># workload #1</span>{`
  `}<span className="k">workload_repo</span>: <span className="s">../atlas-web</span>{`
  `}<span className="k">output_repo</span>:   <span className="s">../atlas-ios</span>{`
  `}<span className="k">workload_spec</span>: <span className="s">.devcore/tasks/atlas-port.md</span>{`

memory:
  `}<span className="k">canonical_dir</span>: <span className="s">.devcore/memory</span>{`
  `}<span className="k">episodic_db</span>:   <span className="s">.devcore/state/episodic.sqlite</span>{`
  `}<span className="k">embeddings</span>:
    <span className="k">provider</span>: <span className="v">ollama</span>{`
    `}<span className="k">model</span>:    <span className="v">nomic-embed-text</span>{`
    `}<span className="k">endpoint</span>: <span className="s">http://localhost:11434</span>{`

models:
  `}<span className="k">proxy</span>:
    <span className="k">type</span>:     <span className="v">claude-code-router</span>{`
    `}<span className="k">endpoint</span>: <span className="s">http://localhost:8787</span>{`
  `}<span className="k">profiles</span>:
    <span className="k">api</span>:   {`{ base_url: null,                  model: `}<span className="v">claude-sonnet-4-6</span>{` }`}{`
    `}<span className="k">local</span>: {`{ base_url: http://localhost:8787, model: `}<span className="v">llama3.1</span>{` }   `}<span className="c"># → studio</span>{`

agents:
  `}<span className="k">conductor</span>: {`{ profile: api,   prompt: prompts/conductor.md }`}{`
  `}<span className="k">analyst</span>:   {`{ profile: api,   prompt: prompts/analyst.md   }`}{`
  `}<span className="k">architect</span>: {`{ profile: api,   prompt: prompts/architect.md }`}{`
  `}<span className="k">builder</span>:   {`{ profile: api,   prompt: prompts/builder.md, tracks: [backend, data, ios] }`}{`
  `}<span className="k">reviewer</span>:  {`{ profile: api,   prompt: prompts/reviewer.md  }`}{`
  `}<span className="k">verifier</span>:  {`{ profile: local, prompt: prompts/verifier.md  }   `}<span className="c"># local-pinned</span>{`
`}
      </pre>
    </div>
  );
}

function Health({ name, desc, meta, status, statusText }) {
  return (
    <div className="health-row">
      <span className={`h-dot ${status === 'warn' ? 'is-warn' : status === 'err' ? 'is-err' : ''}`}/>
      <div>
        <div className="h-name">{name}</div>
        <div className="h-desc">{desc}</div>
      </div>
      <div className="h-meta">{meta}</div>
      <div className={`h-status ${status === 'warn' ? 'is-warn' : status === 'err' ? 'is-err' : ''}`}>{statusText}</div>
    </div>
  );
}

Object.assign(window, { TasksView, MemoryView, AgentsView, GatesView, SettingsView });
