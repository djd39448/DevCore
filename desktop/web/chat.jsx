// chat.jsx — conversation with the Conductor, with inline tool calls,
// dispatches, diffs, and human gates.

const CHAT_AGENT_ORDER = ['conductor', 'analyst', 'architect', 'builder', 'reviewer', 'verifier'];

function fmtChatTime(d) {
  const h = d.getHours(), m = d.getMinutes();
  const am = h < 12;
  const hh = ((h + 11) % 12) + 1;
  return `${hh}:${String(m).padStart(2, '0')} ${am ? 'AM' : 'PM'}`;
}

// ─── Seeded conversation ────────────────────────────────────────────
function seedChat() {
  const t0 = new Date();
  const t = (mins) => fmtChatTime(new Date(t0.getTime() - mins * 60 * 1000));
  return [
    {
      id: 'm-1', kind: 'human',
      time: t(8),
      body: [
        <p key="1">Port the dietary-restrictions filter from the web app, but make it
          native to iOS — prefer the system food picker if possible, otherwise
          a hybrid that still lets users add custom values.</p>,
      ],
    },
    {
      id: 'm-2', kind: 'conductor',
      time: t(8),
      body: [
        <p key="1">Reading the source. I'll route through Analyst first to pull
          the current behavior, then Architect for the iOS contract change,
          then Builder/ios to implement. Let me check what we already know.</p>,
        <ToolCard key="rcl"
          fn="memory_recall"
          args={{ query: "dietary restrictions filter", scope: "both", limit: 8 }}
          status="ok"
          results={[
            { kind: 'event',     ref: 'events#9412',            title: 'Web app uses multi-select w/ free-form input',     score: 0.91 },
            { kind: 'canonical', ref: 'domain/atlas-behaviors.md#L142', title: 'Filter rules — 14 preset + arbitrary',          score: 0.86 },
            { kind: 'decision',  ref: 'decisions/0004-platform.md', title: 'iOS native components preferred where viable',  score: 0.72 },
          ]}
        />,
        <p key="2">Prior context exists. Architect noted the web app supports
          arbitrary values; iOS-native pickers don't, so a hybrid is required.
          I'm proposing this dispatch:</p>,
        <Dispatch key="d1" target="Analyst"   task="T-220"
                  blurb="read web filter behavior in full" status="queued"/>,
        <Dispatch key="d2" target="Architect" task="T-221"
                  blurb="revise contract — hybrid picker pattern" status="active"/>,
        <Dispatch key="d3" target="Builder/ios" task="T-222"
                  blurb="implement HybridDietaryFilterView.swift" status="queued"/>,
      ],
    },
    {
      id: 'm-3', kind: 'system',
      time: t(7),
      body: [<p key="1">Architect requested an approval gate before the contract change.</p>],
    },
    {
      id: 'm-4', kind: 'conductor',
      time: t(7),
      body: [
        <p key="1">The contract change touches the shared filter shape used by
          every track. Worth pausing for your call.</p>,
        <GateCard key="g"
          eyebrow="gate · after contract"
          title="Confirm hybrid picker pattern"
          body="Use a UIPickerView with the 14 preset values, plus a 'Custom…' row that opens an inline text field. Custom values are tagged and stored alongside preset IDs in the same shape."
        />,
      ],
    },
    {
      id: 'm-5', kind: 'human',
      time: t(5),
      body: [<p key="1">Approve — but document the rationale in an ADR.</p>],
    },
    {
      id: 'm-6', kind: 'conductor',
      time: t(5),
      body: [
        <p key="1">Noted. ADR-0009 will be authored as part of T-221.</p>,
        <ToolCard key="log"
          fn="memory_log"
          args={{ type: "decision", agent: "conductor", summary: "Approved hybrid picker; ADR-0009 to follow" }}
          status="ok"
          results={[{ kind: 'event', ref: 'events#12,441', title: 'logged', score: null }]}
        />,
        <p key="2">Architect picked up T-221 and started the ADR. Builder/ios
          already has the contract stub and is scaffolding the view.</p>,
        <DiffCard key="diff"
          path="atlas-ios/Filters/HybridDietaryFilterView.swift"
          adds={32}
          dels={0}
          lines={[
            { n: 1,  mk: '+', t: 'import SwiftUI' },
            { n: 2,  mk: '+', t: '' },
            { n: 3,  mk: '+', t: 'struct HybridDietaryFilterView: View {' },
            { n: 4,  mk: '+', t: '  @State private var selected: Set<DietaryTag> = []' },
            { n: 5,  mk: '+', t: '  @State private var customDraft = ""' },
            { n: 6,  mk: '+', t: '' },
            { n: 7,  mk: '+', t: '  var body: some View {' },
            { n: 8,  mk: '+', t: '    List {' },
            { n: 9,  mk: '+', t: '      Section("Common") {' },
            { n: 10, mk: '+', t: '        ForEach(DietaryTag.presets) { tag in' },
            { n: 11, mk: '+', t: '          PresetRow(tag: tag, selected: $selected)' },
            { n: 12, mk: '+', t: '        }' },
            { n: 13, mk: '+', t: '      }' },
          ]}
        />,
      ],
    },
    {
      id: 'm-7', kind: 'human',
      time: t(3),
      body: [<p key="1">Looks good. Have Reviewer check the tag persistence — I want presets and custom values to round-trip through the contract cleanly.</p>],
    },
    {
      id: 'm-8', kind: 'conductor',
      time: t(3),
      body: [
        <p key="1">Reviewer will run the dc-07 checklist with focus on the
          persistence contract. I've staged a verifier run after that — local
          profile, since it's a thin behavior check.</p>,
        <Dispatch key="d4" target="Reviewer" task="T-222·review"
                  blurb="dc-07 + tag round-trip" status="active"/>,
        <Dispatch key="d5" target="Verifier" task="T-222·verify"
                  blurb="local · llama3.1 via proxy" status="queued"/>,
      ],
    },
  ];
}

// ─── Sub-components ─────────────────────────────────────────────────
function ToolCard({ fn, args, status = 'ok', results = [] }) {
  const [open, setOpen] = React.useState(false);
  const argStr = '(' + Object.entries(args).map(([k, v]) =>
    `${k}: ${typeof v === 'string' ? `"${v}"` : v}`
  ).join(', ') + ')';
  return (
    <div className="tool-card">
      <div className="tool-card-h" onClick={() => setOpen(o => !o)}>
        <span className="arrow">→</span>
        <span className="fn">{fn}</span>
        <span className="args">{argStr}</span>
        <span className={`status ${status === 'pending' ? 'is-pending' : ''}`}>
          {status === 'ok' ? `${results.length} result${results.length === 1 ? '' : 's'}` : status}
        </span>
        <span className="chev">{open ? '▾' : '▸'}</span>
      </div>
      {open && (
        <div className="tool-card-body dc-fade-in">
          {results.map((r, i) => (
            <div className="row" key={i}>
              <span className="k">{r.kind}</span>
              <span className="v">
                {r.title}
                {r.ref && <span style={{ color: 'var(--ink-faint)' }}> · {r.ref}</span>}
                {r.score != null && <span className="score">score {r.score.toFixed(2)}</span>}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function Dispatch({ target, task, blurb, status = 'queued' }) {
  return (
    <div className="dispatch">
      <span className="arrow">→</span>
      <span className="target">{target}</span>
      <span className="task">{task}</span>
      <span style={{ color: 'var(--ink-muted)' }}>·</span>
      <span style={{ color: 'var(--ink-2)', fontFamily: 'var(--serif)', fontStyle: 'italic' }}>{blurb}</span>
      <span className="live">{status === 'active' ? '● live' : status}</span>
    </div>
  );
}

function DiffCard({ path, adds, dels, lines }) {
  return (
    <div className="diff-card">
      <div className="diff-card-h">
        <span className="path">{path}</span>
        <span className="stat">
          <span className="add">+{adds}</span> <span className="del">−{dels}</span>
        </span>
        <div className="actions">
          <button className="dc-btn" style={{ height: 22, padding: '0 8px', fontSize: 11 }}>Open</button>
          <button className="dc-btn is-primary" style={{ height: 22, padding: '0 8px', fontSize: 11 }}>Accept</button>
        </div>
      </div>
      <div className="diff-body">
        {lines.map((l, i) => (
          <div className={`diff-line ${l.mk === '+' ? 'is-add' : l.mk === '-' ? 'is-del' : ''}`} key={i}>
            <span className="ln">{l.n}</span>
            <span className="mk">{l.mk}</span>
            <span className="src">{l.t || ' '}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function GateCard({ eyebrow, title, body }) {
  const [decided, setDecided] = React.useState(null);
  if (decided) {
    return (
      <div className="gate-card-inline" style={{ background: decided === 'approve' ? 'rgba(106,138,95,0.10)' : 'rgba(168,118,37,0.10)', borderColor: decided === 'approve' ? 'rgba(106,138,95,0.32)' : 'rgba(168,118,37,0.32)' }}>
        <div className="eyebrow" style={{ color: decided === 'approve' ? 'var(--ok)' : 'var(--warn)' }}>
          {decided === 'approve' ? 'approved' : 'changes requested'} · just now
        </div>
        <div className="title">{title}</div>
      </div>
    );
  }
  return (
    <div className="gate-card-inline">
      <div className="eyebrow">{eyebrow}</div>
      <div className="title">{title}</div>
      <div className="body">{body}</div>
      <div className="actions">
        <button className="dc-btn is-primary" onClick={() => setDecided('approve')}>Approve</button>
        <button className="dc-btn" onClick={() => setDecided('changes')}>Request changes</button>
        <button className="dc-btn is-ghost">Open diff</button>
      </div>
    </div>
  );
}

function MsgRow({ m }) {
  const isHuman = m.kind === 'human';
  const isCond  = m.kind === 'conductor';
  const isSys   = m.kind === 'system';

  if (isSys) {
    return <div className="sys-note dc-fade-in">{m.body}</div>;
  }

  return (
    <div className={`msg dc-fade-in ${isHuman ? 'is-human' : ''}`}>
      <div className={`msg-av ${isCond ? 'is-conductor' : ''}`}>
        {isHuman ? 'D' : isCond ? '◐' : '·'}
      </div>
      <div>
        <div className="msg-h">
          <span className={`name ${isCond ? 'is-conductor' : ''}`}>
            {isHuman ? 'You' : 'Conductor'}
          </span>
          {isCond && <span>· planning</span>}
          <span className="t">· {m.time}</span>
        </div>
        <div className="msg-body">{m.body}</div>
      </div>
    </div>
  );
}

// ─── Composer ───────────────────────────────────────────────────────
function Composer({ onSend }) {
  const [val, setVal] = React.useState('');
  const ref = React.useRef(null);

  React.useEffect(() => {
    if (!ref.current) return;
    ref.current.style.height = 'auto';
    ref.current.style.height = Math.min(240, ref.current.scrollHeight) + 'px';
  }, [val]);

  const send = () => {
    if (!val.trim()) return;
    onSend(val.trim());
    setVal('');
  };

  const onKey = (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      send();
    }
  };

  return (
    <div className="composer-wrap">
      <div className="composer">
        <div className="composer-input">
          <textarea
            ref={ref}
            value={val}
            onChange={e => setVal(e.target.value)}
            onKeyDown={onKey}
            placeholder="Ask the Conductor, or type / for commands…"
            rows={1}
          />
          <div className="composer-foot">
            <span className="chip"><span className="lbl">model</span>claude-sonnet-4-6</span>
            <span className="chip"><span className="lbl">profile</span>api</span>
            <span className="chip"><span className="lbl">scope</span>workload</span>
            <span className="chip">+ attach context</span>
            <button className="send" onClick={send} disabled={!val.trim()}>
              Send <kbd>↵</kbd>
            </button>
          </div>
        </div>
        <div className="composer-help">
          <span className="slash">/plan</span> decompose a goal
          <span className="sep">·</span>
          <span className="slash">/recall</span> query memory
          <span className="sep">·</span>
          <span className="slash">/dispatch</span> route to an agent
          <span className="sep">·</span>
          <span className="slash">/gate</span> open an approval
          <span style={{ marginLeft: 'auto' }}>shift+↵ for newline</span>
        </div>
      </div>
    </div>
  );
}

// ─── Mini lane (in the strip) ───────────────────────────────────────
function MiniLane({ activeIndex }) {
  const names = ['cond', 'anal', 'arch', 'buil', 'revi', 'veri'];
  return (
    <div className="mini-lane" title={`active: ${names[activeIndex]}`}>
      {names.map((n, i) => (
        <React.Fragment key={i}>
          <div className={`ml-dot ${i < activeIndex ? 'is-done' : i === activeIndex ? 'is-active' : ''}`}/>
          {i < names.length - 1 && <div className={`ml-rule ${i < activeIndex ? 'is-done' : ''}`}/>}
        </React.Fragment>
      ))}
    </div>
  );
}

// ─── Canned Conductor replies for the live composer ─────────────────
const CANNED_REPLIES = [
  {
    body: [
      <p key="1">Got it. Pulling related context from memory first, then I'll
        decompose into tasks and assign tracks.</p>,
      <ToolCard key="t"
        fn="memory_recall"
        args={{ query: "%PROMPT%", scope: "both", limit: 6 }}
        status="ok"
        results={[
          { kind: 'event',     ref: 'events#11,902',     title: 'Prior conversation on adjacent feature', score: 0.74 },
          { kind: 'canonical', ref: 'contract/contract.md', title: 'Shared filter shape — preset + custom', score: 0.68 },
        ]}
      />,
      <p key="2">Two candidate tracks: <em>builder/ios</em> for the view and
        <em> builder/backend</em> for any contract change. Drafting tasks now.</p>,
      <Dispatch key="d" target="Architect" task="T-230"
                blurb="evaluate contract impact" status="active"/>,
    ],
  },
  {
    body: [
      <p key="1">Understood. I'll keep the change scoped — no contract drift —
        and have Reviewer focus on persistence round-tripping.</p>,
      <Dispatch key="d1" target="Builder/ios" task="T-231"
                blurb="implement against existing contract" status="active"/>,
      <Dispatch key="d2" target="Reviewer" task="T-231·review"
                blurb="dc-07 + round-trip" status="queued"/>,
    ],
  },
  {
    body: [
      <p key="1">I've logged that as a decision. Verifier is queued to run
        against the local profile once Builder lands the change.</p>,
      <ToolCard key="t"
        fn="memory_log"
        args={{ type: "decision", agent: "conductor", summary: "%PROMPT%" }}
        status="ok"
        results={[{ kind: 'event', ref: 'events#12,442', title: 'logged', score: null }]}
      />,
    ],
  },
];

// ─── ChatView ───────────────────────────────────────────────────────
function ChatView({ activeIndex }) {
  const [msgs, setMsgs] = React.useState(() => seedChat());
  const [typing, setTyping] = React.useState(false);
  const scrollRef = React.useRef(null);

  React.useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [msgs, typing]);

  const send = (text) => {
    const now = fmtChatTime(new Date());
    const id = 'm-' + Date.now();
    setMsgs(m => [...m, { id, kind: 'human', time: now, body: [<p key="1">{text}</p>] }]);
    setTyping(true);
    setTimeout(() => {
      const reply = CANNED_REPLIES[Math.floor(Math.random() * CANNED_REPLIES.length)];
      // patch %PROMPT% in tool-card args (recursive walk would be overkill — args are React children)
      const body = reply.body.map((node, i) => {
        if (node.type === ToolCard && node.props.args && Object.values(node.props.args).some(v => typeof v === 'string' && v.includes('%PROMPT%'))) {
          const args = Object.fromEntries(
            Object.entries(node.props.args).map(([k, v]) => [k, typeof v === 'string' ? v.replace('%PROMPT%', text.slice(0, 40)) : v])
          );
          return React.cloneElement(node, { ...node.props, args, key: i });
        }
        return React.cloneElement(node, { key: i });
      });
      setMsgs(m => [...m, {
        id: 'm-' + Date.now() + '-c',
        kind: 'conductor',
        time: fmtChatTime(new Date()),
        body,
      }]);
      setTyping(false);
    }, 1800 + Math.random() * 800);
  };

  return (
    <div className="chat-shell-pad">
      {/* Strip */}
      <div className="chat-strip">
        <div className="who">
          <div className="av"/>
          <span>Conductor</span>
        </div>
        <span className="meta">claude-sonnet-4-6 · api</span>
        <span style={{ width: 1, height: 12, background: 'var(--rule-strong)' }}/>
        <span className="meta">workload · atlas-port-ios</span>
        <span style={{ width: 1, height: 12, background: 'var(--rule-strong)' }}/>
        <MiniLane activeIndex={activeIndex}/>
        <span className="meta">cycle 14 · running</span>
        <span className="spacer"/>
        <button className="dc-btn is-ghost">+ new cycle</button>
        <button className="dc-btn">share</button>
      </div>

      {/* Messages */}
      <div className="chat-messages" ref={scrollRef}>
        <div className="chat-col">
          {msgs.map(m => <MsgRow key={m.id} m={m}/>)}
          {typing && (
            <div className="msg dc-fade-in">
              <div className="msg-av is-conductor">◐</div>
              <div>
                <div className="msg-h"><span className="name is-conductor">Conductor</span> <span>· thinking</span></div>
                <div className="typing">
                  Recalling, planning
                  <span className="dots"><i/><i/><i/></span>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Composer */}
      <Composer onSend={send}/>
    </div>
  );
}

Object.assign(window, { ChatView });
