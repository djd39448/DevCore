// live-run.jsx — the hero view: agent loop in motion

const AGENTS = [
  { id: 'conductor', label: 'Conductor', verb: 'planning',  klass: '' },
  { id: 'analyst',   label: 'Analyst',   verb: 'reading',   klass: '' },
  { id: 'architect', label: 'Architect', verb: 'designing', klass: '' },
  { id: 'builder',   label: 'Builder',   verb: 'building',  klass: '' },
  { id: 'reviewer',  label: 'Reviewer',  verb: 'reviewing', klass: '' },
  { id: 'verifier',  label: 'Verifier',  verb: 'verifying', klass: 'is-verify' },
];

// A fixed library of plausible log entries, keyed by agent. The ticker walks
// the loop and picks a couple of lines each beat.
const LOG_LINES = {
  conductor: [
    ['decision', 'Dispatched T-204 → builder/backend'],
    ['action',   'Task tree updated: 6 active, 3 review, 11 done'],
    ['note',     'Cycle 14 opened — gate after track_plan'],
    ['action',   'Routing T-211 to architect for contract revision'],
    ['decision', 'Holding T-219 pending verifier feedback on T-217'],
  ],
  analyst: [
    ['action',   'Reading workload spec atlas-port-ios.md'],
    ['note',     'Extracted 14 behaviors from source web app'],
    ['action',   'Drafting behavior_spec.md (rev 3)'],
  ],
  architect: [
    ['action',   'Updating contract/contract.md → memory_recall signature'],
    ['decision', 'ADR-0008: episodic recall fused with reciprocal-rank'],
    ['note',     'No SQLite extensions — pure-Go path stands'],
  ],
  builder: [
    ['action',   'Editing internal/embed/embed.go +42 −18'],
    ['action',   'go test ./internal/embed/...  — 14 pass, 0 fail'],
    ['action',   'Committing wip: nomic-embed-text adapter'],
    ['action',   'Editing internal/memoryserver/server.go +18 −4'],
    ['correction', 'Re-running gofmt on internal/episodic/events.go'],
  ],
  reviewer: [
    ['action',   'Checking diff against dc-07 pre-commit checklist'],
    ['note',     'No standards regressions; header dc-01 present'],
    ['correction', 'Asked Builder to widen error wrap in events.go:84'],
  ],
  verifier: [
    ['action',   'devcore doctor --test-local — proxy → ollama'],
    ['action',   'go build ./... → ok'],
    ['note',     'All 47 tests green (1.2s)'],
  ],
};

const REF_FILES = [
  'internal/embed/embed.go',
  'internal/episodic/events.go',
  'internal/memoryserver/server.go',
  '.devcore/memory/contract/contract.md',
  'cmd/devcore-memory/main.go',
];

function fmtTime(d) {
  const pad = (n) => String(n).padStart(2, '0');
  return pad(d.getHours()) + ':' + pad(d.getMinutes()) + ':' + pad(d.getSeconds());
}

function LiveRun({ activeIndex, log, cycleSeconds, tokensIn, tokensOut, cost, eventsTotal }) {
  // Compute the lane-rule-active gradient extent: from first station to active
  const stationW = 100 / 6;
  const left = stationW * 0.5;
  const width = Math.max(0, stationW * activeIndex);

  return (
    <div className="dc-fade-in">
      {/* page header */}
      <div className="dc-page-h">
        <span className="dc-page-eyebrow">Workload</span>
        <h1 className="dc-page-title">atlas-port-ios</h1>
        <div style={{ marginLeft: 'auto', display: 'flex', gap: 8 }}>
          <span className="dc-pill is-active"><span className="dot"/>cycle 14</span>
          <span className="dc-pill"><span className="dot"/>phase 3 · tracks</span>
          <span className="dc-pill"><span className="dot"/>gate · after track_plan</span>
        </div>
      </div>
      <p className="dc-page-sub">
        Conductor is leading the loop. Builder/backend has held the floor for
        the last minute — the proxy → Ollama verifier path is green.
      </p>

      {/* The lane */}
      <div className="lane">
        <div className="lane-stations">
          <div className="lane-rule"/>
          <div className="lane-rule-active" style={{ left: `${left}%`, width: `${width}%` }}/>
          {AGENTS.map((a, i) => {
            const klass = i < activeIndex ? 'is-done' : i === activeIndex ? 'is-active' : '';
            return (
              <div key={a.id} className={`station ${klass}`}>
                <div className="station-dot"/>
                <div className="station-label">{a.label}</div>
                <div className="station-sub">{i === activeIndex ? a.verb + '…' : (i < activeIndex ? 'done' : '')}</div>
              </div>
            );
          })}
        </div>
      </div>

      {/* lower grid */}
      <div className="live-grid">
        {/* current task */}
        <div className="dc-card">
          <div className="dc-card-h">
            <div className="dc-card-h-title">Current task</div>
            <span className="dc-pill is-active"><span className="dot"/>active</span>
          </div>
          <div className="dc-card-body">
            <div className="dc-mono-faint" style={{ marginBottom: 6 }}>T-204 · track/backend</div>
            <div style={{ fontFamily: 'var(--serif)', fontSize: 19, fontWeight: 500, lineHeight: 1.25, marginBottom: 4 }}>
              Wire memory_recall to the Ollama embeddings path
            </div>
            <div style={{ fontFamily: 'var(--serif)', fontStyle: 'italic', color: 'var(--ink-muted)', fontSize: 13, marginBottom: 18 }}>
              Fuse keyword + vector ranks with reciprocal-rank fusion. No SQLite extensions.
            </div>

            <div className="progress" style={{ marginBottom: 18 }}>
              <div className="progress-bar"/>
            </div>

            <dl className="task-meta">
              <dt>agent</dt><dd>{AGENTS[activeIndex].id}</dd>
              <dt>elapsed</dt><dd>{Math.floor(cycleSeconds / 60)}m {String(cycleSeconds % 60).padStart(2, '0')}s</dd>
              <dt>tokens</dt><dd>{tokensIn.toLocaleString()} in · {tokensOut.toLocaleString()} out</dd>
              <dt>model</dt><dd>claude-sonnet-4-6 <span style={{ color: 'var(--ink-faint)' }}>· api</span></dd>
              <dt>spec_ref</dt><dd style={{ color: 'var(--ink-muted)' }}>.devcore/tasks/atlas-port.md#L84</dd>
            </dl>
          </div>
        </div>

        {/* event log */}
        <div className="dc-card">
          <div className="dc-card-h">
            <div className="dc-card-h-title">Activity</div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <span className="dc-mono-faint">{eventsTotal.toLocaleString()} events</span>
              <span className="dc-pill is-active"><span className="dot"/>live</span>
            </div>
          </div>
          <div className="dc-card-body" style={{ padding: '12px 18px 16px' }}>
            <div className="log">
              {log.map((line, i) => (
                <div className="log-line dc-fade-in" key={line.key}>
                  <span className="t">{line.time}</span>
                  <span className={`a ${line.agentKlass}`}>{line.agent}</span>
                  <span className="m">
                    {line.message}
                    {line.ref && <span className="ref"> · {line.ref}</span>}
                  </span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>

      {/* readout strip */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 18, marginTop: 24 }}>
        <Readout label="tokens this cycle"  value={(tokensIn + tokensOut).toLocaleString()} unit="t" />
        <Readout label="spend this cycle"   value={'$' + cost.toFixed(2)} unit="usd" />
        <Readout label="episodic events"    value={eventsTotal.toLocaleString()} unit="rows" />
        <Readout label="gate"               value={'after track_plan'} sub="awaiting human" />
      </div>
    </div>
  );
}

function Readout({ label, value, unit, sub }) {
  return (
    <div className="dc-card" style={{ padding: '16px 18px' }}>
      <div className="dc-mono-faint" style={{ textTransform: 'uppercase', letterSpacing: '0.12em', fontSize: 9.5, marginBottom: 6 }}>{label}</div>
      <div className="dc-readout">
        {value}{unit && <span className="unit">{unit}</span>}
      </div>
      {sub && <div style={{ fontFamily: 'var(--serif)', fontStyle: 'italic', fontSize: 12, color: 'var(--ink-muted)', marginTop: 4 }}>{sub}</div>}
    </div>
  );
}

// ─── Ticker hook ────────────────────────────────────────────────────
// Walks the loop on a 2.5-3s beat and pushes 1-2 new lines per beat.
function useLiveRun() {
  const [activeIndex, setActiveIndex] = React.useState(3); // start on builder
  const [log, setLog] = React.useState(() => seedLog());
  const [cycleSeconds, setCycleSeconds] = React.useState(72);
  const [tokensIn, setTokensIn] = React.useState(8421);
  const [tokensOut, setTokensOut] = React.useState(3107);
  const [cost, setCost] = React.useState(0.34);
  const [eventsTotal, setEventsTotal] = React.useState(1204);

  // seconds counter
  React.useEffect(() => {
    const i = setInterval(() => setCycleSeconds(s => s + 1), 1000);
    return () => clearInterval(i);
  }, []);

  // agent walker + log ticker
  React.useEffect(() => {
    let stayCount = 0;
    const tick = () => {
      setActiveIndex(prev => {
        // 60% chance to stay (Builder spends most time), 40% advance
        stayCount += 1;
        if (stayCount < 2 && Math.random() < 0.5) return prev;
        stayCount = 0;
        return (prev + 1) % AGENTS.length;
      });
    };
    const logTick = () => {
      setActiveIndex(curIdx => {
        const agent = AGENTS[curIdx];
        const pool = LOG_LINES[agent.id];
        const pick = pool[Math.floor(Math.random() * pool.length)];
        const ref = Math.random() < 0.4 ? REF_FILES[Math.floor(Math.random() * REF_FILES.length)] : null;
        const newLine = makeLine(agent, pick, ref);
        setLog(l => [newLine, ...l].slice(0, 24));
        setTokensIn(t => t + Math.floor(80 + Math.random() * 380));
        setTokensOut(t => t + Math.floor(40 + Math.random() * 180));
        setCost(c => +(c + 0.003 + Math.random() * 0.008).toFixed(2));
        setEventsTotal(n => n + 1);
        return curIdx;
      });
    };

    const t1 = setInterval(tick, 4200);
    const t2 = setInterval(logTick, 2100);
    return () => { clearInterval(t1); clearInterval(t2); };
  }, []);

  return { activeIndex, log, cycleSeconds, tokensIn, tokensOut, cost, eventsTotal };
}

let __lineKey = 0;
function makeLine(agent, [type, message], ref) {
  __lineKey += 1;
  const klass = type === 'decision' || type === 'correction'
    ? 'is-active'
    : agent.id === 'verifier' ? 'is-verify'
    : agent.id === 'reviewer' && type === 'note' ? 'is-ok'
    : '';
  return {
    key: __lineKey,
    time: fmtTime(new Date()),
    agent: agent.id + (agent.id === 'builder' ? '/backend' : ''),
    agentKlass: klass,
    message,
    ref,
  };
}

function seedLog() {
  const now = new Date();
  const lines = [
    ['conductor',  'decision',   'Dispatched T-204 → builder/backend', null],
    ['builder',    'action',     'Editing internal/embed/embed.go', '+42 −18'],
    ['builder',    'action',     'go test ./internal/embed/... — 14 pass', null],
    ['analyst',    'note',       'Extracted 14 behaviors from source app', null],
    ['architect',  'decision',   'ADR-0008: reciprocal-rank fusion', null],
    ['conductor',  'action',     'Task tree updated · 6 active', null],
    ['verifier',   'note',       'All 47 tests green (1.2s)', null],
    ['reviewer',   'correction', 'Widen error wrap at events.go:84', null],
    ['builder',    'action',     'Editing internal/episodic/events.go', '+8 −2'],
    ['conductor',  'note',       'Cycle 14 opened', null],
  ];
  return lines.map(([aid, type, msg, ref], i) => {
    const agent = AGENTS.find(a => a.id === aid);
    const t = new Date(now.getTime() - (i + 1) * 7000);
    __lineKey += 1;
    return {
      key: __lineKey,
      time: fmtTime(t),
      agent: agent.id + (agent.id === 'builder' ? '/backend' : ''),
      agentKlass:
        type === 'decision' || type === 'correction' ? 'is-active'
        : aid === 'verifier' ? 'is-verify'
        : aid === 'reviewer' && type === 'note' ? 'is-ok'
        : '',
      message: msg,
      ref,
    };
  });
}

Object.assign(window, { LiveRun, useLiveRun });
