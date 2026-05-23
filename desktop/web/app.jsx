// app.jsx — window chrome + sidebar + view router + tweaks.
//
// Live data is overlaid on top of the prototype's animated fixtures: the
// useLiveRun() hook (live-run.jsx) supplies a cosmetic loop animation, and
// window.DevCoreAPI.useStats() supplies real episodic-store counts when the
// devcore-api subprocess is reachable. Real numbers win when present; mocked
// numbers fill in when the API is offline so the page still looks alive.

const TWEAK_DEFAULTS = /*EDITMODE-BEGIN*/{
  "theme": "light",
  "accent": "#c9601a",
  "density": "regular",
  "sidebar": "full"
}/*EDITMODE-END*/;

// formatCount renders an integer the way the nav badges expect: small numbers
// pass through unchanged; >=1000 collapses to a "1.2k" form so the badge
// stays narrow.
function formatCount(n) {
  if (n < 1000) return String(n);
  return (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k';
}

// Helpers — convert hex to rgba for the soft/line accents
function hexToRgba(hex, a) {
  const m = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(hex);
  if (!m) return `rgba(201,96,26,${a})`;
  const r = parseInt(m[1], 16), g = parseInt(m[2], 16), b = parseInt(m[3], 16);
  return `rgba(${r},${g},${b},${a})`;
}

// ─── Nav items ──────────────────────────────────────────────────────
const NAV = [
  { section: 'Engine' },
  { id: 'chat',    label: 'Chat',      glyph: 'chat',     badge: '●', badgeClass: 'is-pulse' },
  { id: 'live',    label: 'Live run',  glyph: 'pulse',    badge: 'cycle 14', badgeClass: 'is-pulse' },
  { id: 'tasks',   label: 'Tasks',     glyph: 'tree',     badge: '12' },
  { id: 'gates',   label: 'Gates',     glyph: 'gate',     badge: '3', badgeClass: 'is-pulse' },

  { section: 'Memory' },
  { id: 'memory',  label: 'Recall',    glyph: 'search' },
  { id: 'canonical', label: 'Canonical', glyph: 'doc',  badge: '24' },
  { id: 'events',  label: 'Episodic',  glyph: 'rows',   badge: '1.2k' },

  { section: 'System' },
  { id: 'agents',   label: 'Agents',   glyph: 'agents' },
  { id: 'settings', label: 'Settings', glyph: 'gear' },
];

function Glyph({ name }) {
  const stroke = 'currentColor';
  const props = { width: 14, height: 14, viewBox: '0 0 14 14', fill: 'none', stroke, strokeWidth: 1.1, strokeLinecap: 'round', strokeLinejoin: 'round' };
  switch (name) {
    case 'chat': return (
      <svg {...props}><path d="M2 4a1 1 0 0 1 1-1h8a1 1 0 0 1 1 1v5a1 1 0 0 1-1 1H6l-3 2.5V10H3a1 1 0 0 1-1-1z"/></svg>
    );
    case 'pulse': return (
      <svg {...props}><path d="M1 7h2.5l1.5-4 2 8 1.5-4h4.5"/></svg>
    );
    case 'tree': return (
      <svg {...props}><path d="M3 2v10M3 4h4M3 7h4M3 10h4"/></svg>
    );
    case 'gate': return (
      <svg {...props}><rect x="2" y="3" width="10" height="8" rx="1"/><path d="M2 6h10M5 3v8"/></svg>
    );
    case 'search': return (
      <svg {...props}><circle cx="6" cy="6" r="3.8"/><path d="M9 9l3 3"/></svg>
    );
    case 'doc': return (
      <svg {...props}><path d="M3 2h6l2 2v8H3z"/><path d="M9 2v2h2"/><path d="M5 7h4M5 9.5h4"/></svg>
    );
    case 'rows': return (
      <svg {...props}><path d="M2 4h10M2 7h10M2 10h10"/></svg>
    );
    case 'agents': return (
      <svg {...props}><circle cx="4.5" cy="5" r="1.7"/><circle cx="9.5" cy="5" r="1.7"/><path d="M1.5 11c.5-1.5 1.7-2.2 3-2.2M9.5 8.8c1.3 0 2.5.7 3 2.2"/></svg>
    );
    case 'gear': return (
      <svg {...props}><circle cx="7" cy="7" r="2"/><path d="M7 1v2M7 11v2M1 7h2M11 7h2M2.8 2.8l1.4 1.4M9.8 9.8l1.4 1.4M11.2 2.8L9.8 4.2M4.2 9.8L2.8 11.2"/></svg>
    );
    default: return null;
  }
}

// ─── Sidebar ────────────────────────────────────────────────────────
function Sidebar({ view, setView, tokensIn, tokensOut, cost, liveEvents }) {
  // Overlay real episodic counts onto the static nav badges when the API is
  // reachable. The `liveEvents` value is null when the API is offline.
  const eventsBadge = liveEvents !== null ? formatCount(liveEvents) : null;
  return (
    <aside className="dc-sidebar">
      <div className="dc-brand">
        <div className="dc-brand-mark"/>
        <div>
          <div className="dc-brand-name">DevCore</div>
          <div className="dc-brand-version">v0.3 · phase 3</div>
        </div>
      </div>

      {NAV.map((item, i) => {
        if (item.section) return <div className="dc-nav-section" key={'s-' + i}>{item.section}</div>;
        // Replace the "Episodic" badge with the live event count when the
        // API reports one; leave the others on their static prototype values.
        const badge = item.id === 'events' && eventsBadge !== null ? eventsBadge : item.badge;
        return (
          <div className="dc-nav" key={item.id}>
            <div
              className={`dc-nav-item ${view === item.id ? 'is-active' : ''}`}
              onClick={() => setView(item.id)}
            >
              <span className="dc-nav-glyph"><Glyph name={item.glyph}/></span>
              <span className="dc-nav-label">{item.label}</span>
              {badge && (
                <span className={`dc-nav-badge ${item.badgeClass || ''}`}>{badge}</span>
              )}
            </div>
          </div>
        );
      })}

      <div className="dc-sidebar-foot">
        <div className="row"><span>workload</span><span style={{ color: 'var(--ink-2)' }}>atlas-port-ios</span></div>
        <div className="row"><span>cycle</span><span style={{ color: 'var(--ink-2)' }}>14 / iterative</span></div>
        <div className="row"><span>spend</span><span style={{ color: 'var(--ink-2)' }}>${cost.toFixed(2)}</span></div>
        <div className="row"><span>tokens</span><span style={{ color: 'var(--ink-2)' }}>{((tokensIn + tokensOut) / 1000).toFixed(1)}k</span></div>
      </div>
    </aside>
  );
}

// ─── Window + view router ───────────────────────────────────────────
function App() {
  const [t, setTweak] = useTweaks(TWEAK_DEFAULTS);
  const [view, setView] = React.useState('chat');
  const run = useLiveRun();
  // Real episodic counts (null until the API responds; null forever if the
  // API is not reachable). The cosmetic `run.eventsTotal` is used as a
  // fallback so the chrome still has a number when offline.
  const stats = window.DevCoreAPI ? window.DevCoreAPI.useStats() : { data: null };
  const liveEvents = stats.data ? stats.data.events : null;

  React.useEffect(() => {
    const r = document.documentElement;
    r.setAttribute('data-theme', t.theme);
    r.setAttribute('data-density', t.density);
    r.setAttribute('data-sidebar', t.sidebar);
    r.style.setProperty('--accent', t.accent);
    r.style.setProperty('--accent-soft', hexToRgba(t.accent, t.theme === 'dark' ? 0.14 : 0.10));
    r.style.setProperty('--accent-line', hexToRgba(t.accent, 0.32));
  }, [t.theme, t.accent, t.density, t.sidebar]);

  const titles = {
    chat: 'Chat',
    live: 'Live run',
    tasks: 'Tasks',
    gates: 'Approvals',
    memory: 'Recall',
    canonical: 'Canonical memory',
    events: 'Episodic events',
    agents: 'Agents',
    settings: 'Settings',
  };

  return (
    <div className="dc-desktop">
      <div className="dc-window">
        <Sidebar
          view={view}
          setView={setView}
          tokensIn={run.tokensIn}
          tokensOut={run.tokensOut}
          cost={run.cost}
          liveEvents={liveEvents}
        />
        <div className="dc-main">
          <div className="dc-titlebar">
            <div className="dc-traffic">
              <span className="lt lt-r"/><span className="lt lt-y"/><span className="lt lt-g"/>
            </div>
            <div className="dc-titlebar-title">
              <em>DevCore</em> &nbsp;·&nbsp; atlas-port-ios &nbsp;·&nbsp; {titles[view]}
            </div>
            <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 8 }}>
              <span className="dc-mono-faint dc-blink" style={{ color: 'var(--accent)' }}>● </span>
              <span className="dc-mono-faint">cycle 14 · running</span>
            </div>
          </div>

          <div className="dc-view" key={view} style={view === 'chat' ? { padding: 0, display: 'flex', flexDirection: 'column' } : null}>
            {view === 'chat'      && <ChatView activeIndex={run.activeIndex}/>}
            {view === 'live'      && <LiveRun {...run}/>}
            {view === 'tasks'     && <TasksView/>}
            {view === 'gates'     && <GatesView/>}
            {view === 'memory'    && <MemoryView/>}
            {view === 'canonical' && <CanonicalView/>}
            {view === 'events'    && <EventsView/>}
            {view === 'agents'    && <AgentsView/>}
            {view === 'settings'  && <SettingsView/>}
          </div>

          <div className="dc-statusbar">
            <span>
              <span className="sb-dot"/>
              memory · {(liveEvents !== null ? liveEvents : run.eventsTotal).toLocaleString()} events
            </span>
            <span className="sb-sep"/>
            <span>recall · keyword + vector · rrf k=60</span>
            <span className="sb-sep"/>
            <span>verifier · local · ollama llama3.1</span>
            <span className="sb-spacer"/>
            <span>${run.cost.toFixed(2)}</span>
            <span className="sb-sep"/>
            <span>{(run.tokensIn / 1000).toFixed(1)}k in · {(run.tokensOut / 1000).toFixed(1)}k out</span>
            <span className="sb-sep"/>
            <span>{run.cycleSeconds}s</span>
          </div>
        </div>
      </div>

      <TweaksPanel title="Tweaks">
        <TweakSection label="Appearance"/>
        <TweakRadio  label="Theme"   value={t.theme}   options={['light', 'dark']}            onChange={(v) => setTweak('theme', v)}/>
        <TweakColor  label="Accent"  value={t.accent}
                     options={['#c9601a', '#3a6bbd', '#5b8a5f', '#8a4d6f']}
                     onChange={(v) => setTweak('accent', v)}/>
        <TweakRadio  label="Density" value={t.density} options={['regular', 'compact']}        onChange={(v) => setTweak('density', v)}/>
        <TweakSection label="Layout"/>
        <TweakRadio  label="Sidebar" value={t.sidebar} options={['full', 'rail', 'hidden']}    onChange={(v) => setTweak('sidebar', v)}/>
        <TweakSection label="Jump to"/>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 6 }}>
          {['chat', 'live', 'tasks', 'gates', 'memory', 'agents', 'settings'].map(v => (
            <TweakButton key={v} onClick={() => setView(v)}>{titles[v] || v}</TweakButton>
          ))}
        </div>
      </TweaksPanel>
    </div>
  );
}

// ─── Two "thin" extra views for the nav items ───────────────────────
const CANON_DOCS = [
  { dir: 'architecture/', name: 'overview.md',        owner: 'architect', age: '3d', words: 1840 },
  { dir: 'architecture/', name: 'memory-tiers.md',    owner: 'architect', age: '5d', words: 1206 },
  { dir: 'decisions/',    name: '0001-go-engine.md',  owner: 'architect', age: '12d', words: 412 },
  { dir: 'decisions/',    name: '0006-no-cgo.md',     owner: 'architect', age: '7d',  words: 380 },
  { dir: 'decisions/',    name: '0008-recall-fusion.md', owner: 'architect', age: '2d', words: 511 },
  { dir: 'domain/',       name: 'atlas-behaviors.md', owner: 'analyst',   age: '1d', words: 2417 },
  { dir: 'contract/',     name: 'contract.md',        owner: 'architect', age: '38m', words: 1108 },
  { dir: 'conventions/',  name: 'devcore-coding-standards.md', owner: 'reviewer', age: '14d', words: 3104 },
];
function CanonicalView() {
  // Pull the live list of .md files from the API; fall back to the prototype
  // fixture when the API is offline so the view never blanks out.
  const live = window.DevCoreAPI ? window.DevCoreAPI.useCanonical() : { data: null };
  const docs = (live.data && Array.isArray(live.data.docs))
    ? live.data.docs.map(splitDocPath)
    : CANON_DOCS.map(d => ({ dir: d.dir, name: d.name }));

  return (
    <div className="dc-fade-in">
      <div className="dc-page-h">
        <span className="dc-page-eyebrow">Tier 1</span>
        <h1 className="dc-page-title">Canonical memory</h1>
      </div>
      <p className="dc-page-sub">
        Git-versioned files. The source of truth for what DevCore is working on.
        <code style={{ fontFamily: 'var(--mono)', fontSize: 12.5 }}> MEMORY.md </code>
        is the index; every doc carries YAML frontmatter and an owner.
      </p>
      <div className="dc-card">
        <div className="dc-card-h">
          <div className="dc-card-h-title">.devcore/memory/</div>
          <span className="dc-mono-faint">
            {docs.length} {docs.length === 1 ? 'doc' : 'docs'}
            {live.data ? ' · live' : ' · placeholder'}
          </span>
        </div>
        <div style={{ padding: '0 18px 8px' }}>
          <div className="tasks">
            <div className="task-row" style={{ borderTop: 0, color: 'var(--ink-faint)', fontFamily: 'var(--mono)', fontSize: 10, textTransform: 'uppercase', letterSpacing: '0.1em', gridTemplateColumns: '180px 1fr' }}>
              <span>dir</span><span>file</span>
            </div>
            {docs.map((d, i) => (
              <div className="task-row" key={(d.dir + d.name) || i} style={{ gridTemplateColumns: '180px 1fr' }}>
                <span className="dc-mono-muted">{d.dir || '/'}</span>
                <span className="ti" style={{ fontFamily: 'var(--mono)', fontSize: 12 }}>{d.name}</span>
              </div>
            ))}
            {docs.length === 0 && (
              <div className="task-row" style={{ gridTemplateColumns: '1fr', color: 'var(--ink-muted)', fontStyle: 'italic' }}>
                <span>No canonical memory yet.</span>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

// splitDocPath turns "a/b/file.md" into { dir: "a/b/", name: "file.md" }.
// A top-level file becomes { dir: "", name: "file.md" }.
function splitDocPath(path) {
  const slash = path.lastIndexOf('/');
  if (slash < 0) return { dir: '', name: path };
  return { dir: path.slice(0, slash + 1), name: path.slice(slash + 1) };
}

function EventsView() {
  // Live event log when the API is reachable; the seeded prototype rows
  // otherwise. Real events get mapped into the same row shape the original
  // table renders, so the layout below stays unchanged.
  const [filter, setFilter] = React.useState('all');
  const filters = ['all', 'decision', 'action', 'correction', 'learning', 'note'];
  const live = window.DevCoreAPI ? window.DevCoreAPI.useEvents(200) : { data: null };
  const rows = live.data
    ? live.data.map(eventToRow)
    : (window.__seedEvents || (window.__seedEvents = seedEvents()));
  const filtered = filter === 'all' ? rows : rows.filter(r => r.type === filter);
  return (
    <div className="dc-fade-in">
      <div className="dc-page-h">
        <span className="dc-page-eyebrow">Tier 2</span>
        <h1 className="dc-page-title">Episodic events</h1>
      </div>
      <p className="dc-page-sub">
        Append-only behavioral log. Each row carries an agent, a type, a summary,
        refs, and a 768-dim embedding. Consolidation promotes durable learnings
        into the canonical store.
      </p>
      <div style={{ display: 'flex', gap: 8, marginBottom: 14 }}>
        {filters.map(f => (
          <span key={f} className={`dc-pill ${filter === f ? 'is-active' : ''}`} onClick={() => setFilter(f)} style={{ cursor: 'default' }}>
            <span className="dot"/>{f}
          </span>
        ))}
        <span style={{ marginLeft: 'auto' }} className="dc-mono-faint">showing {filtered.length} of {rows.length}</span>
      </div>
      <div className="dc-card">
        <div style={{ padding: '8px 18px' }}>
          <div className="log" style={{ maxHeight: 'unset' }}>
            {filtered.map(r => (
              <div className="log-line" key={r.id} style={{ gridTemplateColumns: '70px 100px 90px 1fr 160px' }}>
                <span className="t">#{r.id}</span>
                <span className="t">{r.time}</span>
                <span className={`a ${r.klass}`}>{r.type}</span>
                <span className="m">{r.agent} <span className="ref">·</span> {r.summary}</span>
                <span className="t" style={{ textAlign: 'right' }}>{r.ref}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

function seedEvents() {
  const types = [
    ['decision',   'is-active'],
    ['action',     ''],
    ['correction', 'is-active'],
    ['learning',   'is-ok'],
    ['note',       ''],
    ['error',      ''],
  ];
  const agents = ['conductor', 'analyst', 'architect', 'builder/backend', 'builder/data', 'builder/ios', 'reviewer', 'verifier'];
  const summaries = [
    'Dispatched T-204 → builder/backend',
    'Editing internal/embed/embed.go',
    'Behavior spec rev 3 drafted',
    'ADR-0008 — reciprocal-rank fusion',
    'go test ./internal/embed/... — 14 pass',
    'Widen error wrap at events.go:84',
    'Local proxy → ollama llama3.1 green',
    'Cycle 14 opened — gate after track_plan',
    'Supabase schema draft, 3 migrations',
    'SwiftUI tab bar — interim mock',
    'Reviewer: dc-07 checklist passes',
    'No SQLite extensions — pure-Go path',
  ];
  const refs = ['internal/embed/embed.go', 'contract/contract.md', 'events#9821', '.devcore/tasks/atlas-port.md', 'migrations/0004.sql', 'CLAUDE.md', ''];
  const out = [];
  let t = new Date();
  for (let i = 0; i < 40; i++) {
    const [type, klass] = types[Math.floor(Math.random() * types.length)];
    out.push({
      id: 12440 - i,
      time: ((t.getHours() % 24) + 24).toString().padStart(2, '0').slice(-2) + ':' + String(t.getMinutes()).padStart(2,'0') + ':' + String(t.getSeconds()).padStart(2,'0'),
      type, klass,
      agent: agents[Math.floor(Math.random() * agents.length)],
      summary: summaries[Math.floor(Math.random() * summaries.length)],
      ref: refs[Math.floor(Math.random() * refs.length)],
    });
    t = new Date(t.getTime() - (30 + Math.random() * 90) * 1000);
  }
  return out;
}

// eventToRow projects a DevCore episodic event (from /api/events) into the
// row shape the events table renders. The CSS pill class is picked from the
// event type so live rows light up the same colors as the prototype's seeds.
const EVENT_TYPE_CLASS = {
  decision: 'is-active',
  correction: 'is-active',
  learning: 'is-ok',
  action: '',
  note: '',
  error: '',
};
function eventToRow(event) {
  return {
    id: event.id,
    time: formatEventTime(event.ts),
    type: event.type,
    klass: EVENT_TYPE_CLASS[event.type] || '',
    agent: event.agent || '—',
    summary: event.summary,
    ref: refsFirst(event.refs),
  };
}

// formatEventTime renders an RFC3339 timestamp as HH:MM:SS, matching the
// prototype's existing column. Unparseable input falls back to the raw value.
function formatEventTime(ts) {
  if (!ts) return '';
  const parsed = new Date(ts);
  if (isNaN(parsed.getTime())) return ts;
  const pad = (n) => String(n).padStart(2, '0');
  return pad(parsed.getHours()) + ':' + pad(parsed.getMinutes()) + ':' + pad(parsed.getSeconds());
}

// refsFirst extracts the first reference from the stored JSON array of refs,
// or returns the raw string when it does not parse as JSON. The events table
// shows at most one ref per row.
function refsFirst(refs) {
  if (!refs) return '';
  try {
    const parsed = JSON.parse(refs);
    if (Array.isArray(parsed) && parsed.length > 0) return String(parsed[0]);
  } catch (_e) {
    // Not JSON — render the raw string as the ref.
  }
  return refs;
}

window.App = App;
window.CanonicalView = CanonicalView;
window.EventsView = EventsView;
