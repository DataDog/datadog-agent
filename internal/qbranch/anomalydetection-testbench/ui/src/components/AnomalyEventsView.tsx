import { useState, useMemo } from 'react';
import type { AnomalyEvent, AnomalySeverity } from '../api/client';
import type { ObserverState, ObserverActions } from '../hooks/useObserver';
import type { PhaseMarker } from './ChartWithAnomalyDetails';

// ---- helpers ---------------------------------------------------------------

const SEVERITY_COLOR: Record<AnomalySeverity, { dot: string; text: string; badge: string }> = {
  low:    { dot: 'bg-slate-400',   text: 'text-slate-300',   badge: 'bg-slate-700 text-slate-300' },
  medium: { dot: 'bg-yellow-400',  text: 'text-yellow-300',  badge: 'bg-yellow-900/60 text-yellow-300' },
  high:   { dot: 'bg-red-500',     text: 'text-red-400',     badge: 'bg-red-900/60 text-red-400' },
};

function formatTs(ts: number): string {
  const d = new Date(ts * 1000);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

function SeverityBadge({ severity }: { severity: AnomalySeverity }) {
  const c = SEVERITY_COLOR[severity] ?? SEVERITY_COLOR.low;
  return (
    <span className={`inline-flex items-center px-1.5 py-0.5 rounded text-xs font-semibold ${c.badge}`}>
      {severity.toUpperCase()}
    </span>
  );
}

function ScorePill({ score }: { score: number }) {
  const pct = Math.round(score * 100);
  const color = pct >= 75 ? 'bg-red-700' : pct >= 40 ? 'bg-yellow-700' : 'bg-slate-600';
  return (
    <span className={`inline-flex items-center px-1.5 py-0.5 rounded text-xs font-mono font-semibold text-white ${color}`}>
      {pct}%
    </span>
  );
}

// ---- Summary cards ---------------------------------------------------------

function SummaryCards({ events }: { events: AnomalyEvent[] }) {
  const total = events.length;
  const high = events.filter(e => e.severity === 'high').length;
  const changed = events.filter(e => e.severityChanged).length;
  const maxSev: AnomalySeverity = events.some(e => e.severity === 'high')
    ? 'high'
    : events.some(e => e.severity === 'medium')
    ? 'medium'
    : 'low';

  return (
    <div className="grid grid-cols-4 gap-2 mb-2">
      {[
        { label: 'Total Events', value: total, color: 'text-slate-200' },
        { label: 'High Severity', value: high, color: 'text-red-400' },
        { label: 'Severity Changes', value: changed, color: 'text-yellow-300' },
        { label: 'Current Max', value: maxSev.toUpperCase(), color: SEVERITY_COLOR[maxSev].text },
      ].map(({ label, value, color }) => (
        <div key={label} className="bg-slate-800 rounded-lg px-3 py-1.5 border border-slate-700 flex items-center gap-3">
          <div className={`text-xl font-bold ${color} leading-none`}>{value}</div>
          <div className="text-xs text-slate-400 leading-tight">{label}</div>
        </div>
      ))}
    </div>
  );
}

// ---- Shared phase-marker helpers -------------------------------------------

const PHASE_COLORS: Record<string, { line: string; label: string }> = {
  baseline:   { line: '#3b82f6', label: '#60a5fa' },
  warmup:     { line: '#6366f1', label: '#818cf8' },
  disruption: { line: '#ef4444', label: '#f87171' },
  cooldown:   { line: '#f59e0b', label: '#fbbf24' },
};

function phaseColor(key: string): { line: string; label: string } {
  return PHASE_COLORS[key.toLowerCase()] ?? { line: '#94a3b8', label: '#cbd5e1' };
}

const mediumSeverityThreshold = 0.40;
const highSeverityThreshold   = 0.75;

// ---- Unified event chart (timeline + rolling score) ------------------------
//
// Geometric elements (bands, lines, score path, dots) live in an SVG with
// preserveAspectRatio="none" so they scale correctly to any container width.
// ALL text labels are HTML elements absolutely-positioned over the wrapper –
// HTML text is never distorted by non-uniform SVG scaling.

function EventChart({ events, selected, onSelect, phaseMarkers, minTs: extMinTs, maxTs: extMaxTs }: {
  events: AnomalyEvent[];
  selected: string | null;
  onSelect: (id: string) => void;
  phaseMarkers?: PhaseMarker[];
  minTs?: number;
  maxTs?: number;
}) {
  if (events.length === 0 && (!phaseMarkers || phaseMarkers.length === 0)) {
    return <div className="text-slate-500 text-sm py-4 text-center">No events in current filter</div>;
  }

  const WIDTH  = 600;
  const HEIGHT = 160;

  // Time constant for the EWMA in seconds.
  // τ = 120 s → half-life ≈ 83 s, smooth over ~2 min of a 30-min scenario.
  const EWMA_TAU = 120;

  const tsValues = events.map(e => e.trigger.timestamp);
  (phaseMarkers ?? []).forEach(m => tsValues.push(m.timestamp));
  const minTs = extMinTs ?? (tsValues.length > 0 ? Math.min(...tsValues) : 0);
  const maxTs = extMaxTs ?? (tsValues.length > 0 ? Math.max(...tsValues) : 1);
  const span  = Math.max(maxTs - minTs, 1);

  // SVG coordinate helpers (viewBox space: 0..WIDTH × 0..HEIGHT)
  const toX    = (t: number) => ((t - minTs) / span) * (WIDTH - 10) + 5;
  const toY    = (s: number) => HEIGHT - s * (HEIGHT - 12) - 6;
  // CSS-% helpers for the HTML overlay (0..100 of the wrapper dimensions)
  const toPctX = (t: number) => ((t - minTs) / span) * 100;
  const toPctY = (s: number) => (toY(s) / HEIGHT) * 100;

  // Continuous-time EWMA score line.
  // Between events the EWMA decays as v(t) = v_prev * exp(-dt/τ).
  // At each new event it blends toward the event score:
  //   v_new = exp(-dt/τ) * v_prev + (1 - exp(-dt/τ)) * score
  // We sample intermediate decay points so the curve looks smooth.
  const ewmaPath = (() => {
    const sorted = [...events].sort((a, b) => a.trigger.timestamp - b.trigger.timestamp);
    if (sorted.length === 0) return '';

    const pts: Array<{ t: number; v: number }> = [];
    let ewmaVal = sorted[0].score;
    let prevT   = sorted[0].trigger.timestamp;
    pts.push({ t: prevT, v: ewmaVal });

    for (let i = 1; i < sorted.length; i++) {
      const t  = sorted[i].trigger.timestamp;
      const dt = t - prevT;
      // Interpolated decay points (one per ~15 s, max 20)
      const steps = Math.max(1, Math.min(Math.round(dt / 15), 20));
      for (let s = 1; s < steps; s++) {
        const partDt = dt * (s / steps);
        pts.push({ t: prevT + partDt, v: ewmaVal * Math.exp(-partDt / EWMA_TAU) });
      }
      // Update EWMA with the new event score
      const decay = Math.exp(-dt / EWMA_TAU);
      ewmaVal = decay * ewmaVal + (1 - decay) * sorted[i].score;
      pts.push({ t, v: ewmaVal });
      prevT = t;
    }

    // Tail decay to end of timeline
    const tailDt = maxTs - prevT;
    if (tailDt > 0) {
      const steps = Math.max(1, Math.min(Math.round(tailDt / 15), 40));
      for (let s = 1; s <= steps; s++) {
        const partDt = tailDt * (s / steps);
        const v = ewmaVal * Math.exp(-partDt / EWMA_TAU);
        if (v < 0.005) break;
        pts.push({ t: prevT + partDt, v });
      }
    }

    return pts.map((p, i) => `${i === 0 ? 'M' : 'L'} ${toX(p.t).toFixed(1)} ${toY(p.v).toFixed(1)}`).join(' ');
  })();

  const yMedium = toY(mediumSeverityThreshold);
  const yHigh   = toY(highSeverityThreshold);
  const pctYMedium = toPctY(mediumSeverityThreshold);
  const pctYHigh   = toPctY(highSeverityThreshold);

  // Diamond path helper for severity-change events
  const diamond = (cx: number, cy: number, r: number) =>
    `M${cx},${cy - r} L${cx + r},${cy} L${cx},${cy + r} L${cx - r},${cy} Z`;

  return (
    <div className="mb-2">
      <div className="text-xs text-slate-400 mb-0.5">EWMA score (τ=2 min) · ● events · ◆ severity change</div>
      {/* Wrapper: fixed height, relative so HTML labels can be absolutely positioned */}
      <div className="relative rounded border border-slate-700 bg-slate-900" style={{ height: HEIGHT, overflow: 'hidden' }}>

        {/* ── SVG layer: geometry only, no text ── */}
        <svg width="100%" height="100%"
          viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
          preserveAspectRatio="none"
          style={{ position: 'absolute', top: 0, left: 0 }}>

          {/* Severity threshold bands */}
          <rect x="0" y={yHigh}   width={WIDTH} height={yMedium - yHigh} fill="rgba(234,179,8,0.07)" />
          <rect x="0" y="0"       width={WIDTH} height={yHigh}           fill="rgba(239,68,68,0.07)" />
          <line x1="0" y1={yMedium} x2={WIDTH} y2={yMedium} stroke="#ca8a04" strokeWidth="0.5" strokeDasharray="3,3" />
          <line x1="0" y1={yHigh}   x2={WIDTH} y2={yHigh}   stroke="#dc2626" strokeWidth="0.5" strokeDasharray="3,3" />

          {/* Phase-marker vertical lines (no labels here – labels are HTML below) */}
          {(phaseMarkers ?? []).map(pm => {
            const x = toX(pm.timestamp);
            if (x < 0 || x > WIDTH) return null;
            const c = phaseColor(pm.key);
            return (
              <line key={pm.key}
                x1={x} y1={0} x2={x} y2={HEIGHT}
                stroke={c.line} strokeWidth="1" strokeDasharray="4,3" opacity="0.7" />
            );
          })}

          {/* EWMA score line */}
          {ewmaPath && <path d={ewmaPath} fill="none" stroke="#8b5cf6" strokeWidth="1.5" opacity="0.9" />}

          {/* Event dots */}
          {events.map(evt => {
            const x   = toX(evt.trigger.timestamp);
            const y   = toY(evt.score);
            const fc  = evt.severity === 'high' ? '#ef4444' : evt.severity === 'medium' ? '#eab308' : '#64748b';
            const sel = evt.id === selected;
            const tip = evt.trigger.logPattern
              ? `${formatTs(evt.trigger.timestamp)} ${evt.severity} – ${evt.trigger.logPattern}`
              : `${formatTs(evt.trigger.timestamp)} ${evt.severity} – ${evt.trigger.title}`;
            return evt.severityChanged
              ? <path key={evt.id} d={diamond(x, y, sel ? 5 : 4)} fill={fc} opacity="0.95"
                  style={{ cursor: 'pointer' }} onClick={() => onSelect(evt.id)}
                  stroke={sel ? 'white' : 'none'} strokeWidth="1">
                  <title>{tip} ◆ change</title>
                </path>
              : <circle key={evt.id} cx={x} cy={y} r={sel ? 4 : 2.5} fill={fc} opacity="0.85"
                  style={{ cursor: 'pointer' }} onClick={() => onSelect(evt.id)}
                  stroke={sel ? 'white' : 'none'} strokeWidth="1">
                  <title>{tip}</title>
                </circle>;
          })}
        </svg>

        {/* ── HTML label layer: never distorted by SVG scaling ── */}

        {/* Threshold labels on the left */}
        <span className="absolute text-[10px] font-medium text-amber-500 pointer-events-none"
          style={{ top: `${pctYMedium}%`, left: 4, transform: 'translateY(-100%)' }}>
          medium
        </span>
        <span className="absolute text-[10px] font-medium text-red-500 pointer-events-none"
          style={{ top: `${pctYHigh}%`, left: 4, transform: 'translateY(-100%)' }}>
          high
        </span>

        {/* Phase-marker labels at the bottom */}
        {(phaseMarkers ?? []).map(pm => {
          const pct = toPctX(pm.timestamp);
          if (pct < 0 || pct > 100) return null;
          const c = phaseColor(pm.key);
          return (
            <span key={pm.key}
              className="absolute text-[10px] font-semibold pointer-events-none whitespace-nowrap"
              style={{ bottom: 3, left: `${pct}%`, transform: 'translateX(3px)', color: c.label,
                       textShadow: '0 0 4px #0f172a' }}>
              {pm.label}
            </span>
          );
        })}
      </div>
    </div>
  );
}

// ---- Event detail panel ----------------------------------------------------

function EventDetailPanel({ event, onClose }: { event: AnomalyEvent; onClose: () => void }) {
  const bd = event.breakdown;
  return (
    <div className="bg-slate-800 border border-slate-600 rounded-lg p-4 text-sm min-w-0 overflow-hidden">
      <div className="flex items-start justify-between mb-3">
        <div>
          <div className="flex items-center gap-2 mb-1">
            <SeverityBadge severity={event.severity} />
            <ScorePill score={event.score} />
            {event.severityChanged && (
              <span className={`text-xs px-1.5 py-0.5 rounded ${
                event.severityDirection === 'up'
                  ? 'bg-red-900/60 text-red-400'
                  : 'bg-blue-900/60 text-blue-400'
              }`}>
                {event.severityDirection === 'up' ? '↑' : '↓'} {event.previousSeverity} → {event.severity}
              </span>
            )}
          </div>
          <div className="text-slate-200 font-medium">
            {event.trigger.logPattern
              ? <span title={event.trigger.title}>{event.trigger.logPattern}</span>
              : event.trigger.title}
          </div>
          {event.trigger.logExample && (
            <div className="text-xs text-slate-400 mt-1 italic truncate" title={event.trigger.logExample}>
              e.g. {event.trigger.logExample}
            </div>
          )}
        </div>
        <button onClick={onClose} className="text-slate-500 hover:text-white ml-4">✕</button>
      </div>

      <div className="grid grid-cols-2 gap-4 min-w-0">
        {/* Trigger */}
        <div className="min-w-0">
          <div className="text-xs text-slate-400 font-semibold uppercase tracking-wide mb-2">Trigger</div>
          <div className="space-y-1 text-xs text-slate-300 min-w-0">
            {event.trigger.logPattern && (
              <div className="mb-1">
                <div className="text-slate-500 mb-0.5">Pattern:</div>
                <div className="font-mono text-slate-200 break-all leading-relaxed">{event.trigger.logPattern}</div>
              </div>
            )}
            {event.trigger.logExample && (
              <div className="mb-1">
                <div className="text-slate-500 mb-0.5">Example log line:</div>
                <div className="font-mono text-slate-300 break-all italic leading-relaxed">{event.trigger.logExample}</div>
              </div>
            )}
            <div className="truncate" title={event.trigger.source}><span className="text-slate-500">Source:</span> {event.trigger.source}</div>
            <div className="truncate" title={event.trigger.detectorName}><span className="text-slate-500">Detector:</span> {event.trigger.detectorName}</div>
            <div><span className="text-slate-500">Type:</span> {event.trigger.type}</div>
            <div><span className="text-slate-500">Time:</span> {formatTs(event.trigger.timestamp)}</div>
            {event.trigger.detectorScore !== undefined && (
              <div><span className="text-slate-500">Detector score:</span> {(event.trigger.detectorScore * 100).toFixed(1)}%</div>
            )}
            {event.trigger.tags && event.trigger.tags.length > 0 && (
              <div className="flex flex-wrap gap-1 mt-1">
                {event.trigger.tags.slice(0, 6).map(t => (
                  <span key={t} className="px-1 py-0.5 bg-slate-700 rounded text-slate-400 font-mono text-[10px]">{t}</span>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Context */}
        <div className="min-w-0">
          <div className="text-xs text-slate-400 font-semibold uppercase tracking-wide mb-2">Context</div>
          <div className="space-y-1 text-xs text-slate-300">
            <div><span className="text-slate-500">Window:</span> {formatTs(event.windowStart)} – {formatTs(event.windowEnd)}</div>
            <div><span className="text-slate-500">Recent anomalies:</span> {event.recentAnomalyCount}</div>
            <div>
              <span className="text-slate-500">Distinct signals:</span> {bd.signalCount}
              {bd.effectiveSignalCount < bd.signalCount && (
                <span className="text-slate-500"> (top {bd.effectiveSignalCount} used)</span>
              )}
            </div>
            {bd.missingScoreCount > 0 && (
              <div className="text-amber-400"><span className="text-slate-500">Missing scores:</span> {bd.missingScoreCount}</div>
            )}
          </div>
        </div>
      </div>

      {/* Per-signal scores */}
      {event.signals.length > 0 && (
        <div className="mt-3">
          <div className="text-xs text-slate-400 font-semibold uppercase tracking-wide mb-2">Signals</div>
          <div className="space-y-1">
            {event.signals.map(sig => (
              <div key={sig.key} className="flex items-center gap-2 text-xs">
                <SeverityBadge severity={sig.severity} />
                <ScorePill score={sig.score} />
                <span className="text-slate-400 font-mono truncate flex-1" title={sig.key}>{sig.key}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Score breakdown */}
      <div className="mt-3 border-t border-slate-700 pt-3">
        <div className="text-xs text-slate-400 font-semibold uppercase tracking-wide mb-2">Score Breakdown</div>
        <div className="text-xs text-slate-300 space-y-1">
          <div><span className="text-slate-500">Window anomalies:</span> {bd.windowAnomalyCount} in 1 min</div>
          <div><span className="text-slate-500">Combined evidence (noisy-OR):</span> {(bd.combinedEvidenceScore * 100).toFixed(1)}%</div>
          <div>
            <span className="text-slate-500">Log-count cap:</span> {(bd.logCountCap * 100).toFixed(1)}%
            {bd.logCountCapApplied && <span className="text-amber-400 ml-1">← clamped</span>}
          </div>
          <div><span className="text-slate-500">Final score:</span> <span className="font-semibold">{(event.score * 100).toFixed(1)}%</span></div>
          <div><span className="text-slate-500">Severity:</span> <span className="font-semibold capitalize">{event.severity}</span></div>
        </div>
      </div>
    </div>
  );
}

// ---- Event row (list) ------------------------------------------------------

function EventRow({ event, selected, onSelect }: {
  event: AnomalyEvent;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      onClick={onSelect}
      className={`w-full text-left px-3 py-2 rounded border transition-colors ${
        selected
          ? 'bg-slate-700 border-purple-500'
          : 'bg-slate-800/50 border-slate-700/50 hover:bg-slate-800 hover:border-slate-600'
      }`}
    >
      <div className="flex items-center gap-2 mb-0.5">
        <SeverityBadge severity={event.severity} />
        <ScorePill score={event.score} />
        {event.severityChanged && (
          <span className={`text-[10px] font-medium px-1 rounded ${
            event.severityDirection === 'up' ? 'text-red-400 bg-red-900/40' : 'text-blue-400 bg-blue-900/40'
          }`}>
            {event.severityDirection === 'up' ? '↑' : '↓'}
          </span>
        )}
        <span className="text-xs text-slate-500 font-mono ml-auto">{formatTs(event.trigger.timestamp)}</span>
      </div>
      <div className="text-xs text-slate-300 truncate" title={event.trigger.title}>
        {event.trigger.logPattern || event.trigger.title}
      </div>
      {event.trigger.logExample && (
        <div className="text-[10px] text-slate-500 truncate italic" title={event.trigger.logExample}>
          e.g. {event.trigger.logExample}
        </div>
      )}
      <div className="text-[10px] text-slate-500 truncate">{event.trigger.detectorName} · {event.breakdown.signalCount} signal{event.breakdown.signalCount !== 1 ? 's' : ''}</div>
    </button>
  );
}

// ---- Main view -------------------------------------------------------------

// Log-extractor namespaces: events triggered by these are categorised as "log"
// even though they flow through the metric detection path.
const LOG_EXTRACTOR_NAMESPACES = new Set(['log_pattern_extractor', 'log_metrics_extractor']);

function isLogTrigger(evt: AnomalyEvent): boolean {
  if (evt.trigger.type === 'log') return true;
  // The trigger source string is "<namespace>/<name>" or just "<name>" – check namespace prefix.
  const src = evt.trigger.source ?? '';
  const ns = src.split('/')[0];
  return LOG_EXTRACTOR_NAMESPACES.has(ns);
}

interface AnomalyEventsViewProps {
  state: ObserverState;
  actions: ObserverActions;
  sidebarWidth: number;
  phaseMarkers?: PhaseMarker[];
}

export function AnomalyEventsView({ state, sidebarWidth, phaseMarkers }: AnomalyEventsViewProps) {
  const events = state.anomalyEvents ?? [];

  // Filters
  const [severityFilter, setSeverityFilter] = useState<'all' | AnomalySeverity>('all');
  const [typeFilter, setTypeFilter] = useState<'all' | 'metric' | 'log'>('all');
  const [changeFilter, setChangeFilter] = useState<'all' | 'changes' | 'upgrades'>('all');
  const [detectorFilter, setDetectorFilter] = useState('');
  const [selectedId, setSelectedId] = useState<string | null>(null);

  // Available detectors for the filter dropdown
  const detectors = useMemo(() => {
    const names = new Set(events.map(e => e.trigger.detectorName));
    return Array.from(names).sort();
  }, [events]);

  const filtered = useMemo(() => events.filter(evt => {
    if (severityFilter !== 'all' && evt.severity !== severityFilter) return false;
    if (typeFilter !== 'all') {
      const isLog = isLogTrigger(evt);
      if (typeFilter === 'log' && !isLog) return false;
      if (typeFilter === 'metric' && isLog) return false;
    }
    if (changeFilter === 'changes' && !evt.severityChanged) return false;
    if (changeFilter === 'upgrades' && evt.severityDirection !== 'up') return false;
    if (detectorFilter && evt.trigger.detectorName !== detectorFilter) return false;
    return true;
  }), [events, severityFilter, typeFilter, changeFilter, detectorFilter]);

  const selectedEvent = filtered.find(e => e.id === selectedId) ?? null;

  // Compute timeline bounds across all events + phase markers so scales align.
  const { timelineMinTs, timelineMaxTs } = useMemo(() => {
    const tsVals: number[] = events.map(e => e.trigger.timestamp);
    (phaseMarkers ?? []).forEach(m => tsVals.push(m.timestamp));
    if (tsVals.length === 0) return { timelineMinTs: undefined, timelineMaxTs: undefined };
    return { timelineMinTs: Math.min(...tsVals), timelineMaxTs: Math.max(...tsVals) };
  }, [events, phaseMarkers]);

  return (
    <div className="flex flex-1 min-h-0">
      {/* Sidebar */}
      <div className="flex flex-col bg-slate-800/40 border-r border-slate-700 overflow-y-auto" style={{ width: sidebarWidth }}>
        <div className="p-3 space-y-3">
          <div className="text-xs font-semibold text-slate-400 uppercase tracking-wide">Filters</div>

          {/* Severity */}
          <div>
            <div className="text-xs text-slate-500 mb-1">Severity</div>
            <div className="flex flex-col gap-1">
              {(['all', 'low', 'medium', 'high'] as const).map(s => (
                <button
                  key={s}
                  onClick={() => setSeverityFilter(s)}
                  className={`text-left px-2 py-1 rounded text-xs transition-colors ${
                    severityFilter === s
                      ? 'bg-purple-700 text-white'
                      : 'text-slate-400 hover:bg-slate-700'
                  }`}
                >
                  {s === 'all' ? 'All severities' : s.charAt(0).toUpperCase() + s.slice(1)}
                </button>
              ))}
            </div>
          </div>

          {/* Type */}
          <div>
            <div className="text-xs text-slate-500 mb-1">Trigger type</div>
            <div className="flex flex-col gap-1">
              {(['all', 'metric', 'log'] as const).map(t => (
                <button
                  key={t}
                  onClick={() => setTypeFilter(t)}
                  className={`text-left px-2 py-1 rounded text-xs transition-colors ${
                    typeFilter === t
                      ? 'bg-purple-700 text-white'
                      : 'text-slate-400 hover:bg-slate-700'
                  }`}
                >
                  {t === 'all' ? 'Metrics & Logs' : t.charAt(0).toUpperCase() + t.slice(1) + 's'}
                </button>
              ))}
            </div>
          </div>

          {/* Changes */}
          <div>
            <div className="text-xs text-slate-500 mb-1">Severity changes</div>
            <div className="flex flex-col gap-1">
              {([
                ['all', 'All events'],
                ['changes', 'Changes only'],
                ['upgrades', 'Upgrades only'],
              ] as const).map(([val, label]) => (
                <button
                  key={val}
                  onClick={() => setChangeFilter(val)}
                  className={`text-left px-2 py-1 rounded text-xs transition-colors ${
                    changeFilter === val
                      ? 'bg-purple-700 text-white'
                      : 'text-slate-400 hover:bg-slate-700'
                  }`}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>

          {/* Detector */}
          {detectors.length > 0 && (
            <div>
              <div className="text-xs text-slate-500 mb-1">Detector</div>
              <select
                value={detectorFilter}
                onChange={e => setDetectorFilter(e.target.value)}
                className="w-full bg-slate-700 border border-slate-600 rounded px-2 py-1 text-xs text-slate-200 focus:outline-none"
              >
                <option value="">All detectors</option>
                {detectors.map(d => (
                  <option key={d} value={d}>{d}</option>
                ))}
              </select>
            </div>
          )}
        </div>
      </div>

      {/* Main panel — flex column: fixed top, scrollable bottom */}
      <div className="flex-1 flex flex-col min-h-0 min-w-0">
        {events.length === 0 ? (
          <div className="flex-1 flex items-center justify-center text-slate-500 text-sm">
            No anomaly events yet. Load a scenario to see scored events.
          </div>
        ) : (
          <>
            {/* ── Fixed top section: cards + timeline + score chart ── */}
            <div className="flex-none px-4 pt-3 pb-1 min-w-0">
              <SummaryCards events={events} />
              <EventChart
                events={filtered}
                selected={selectedId}
                onSelect={id => setSelectedId(prev => prev === id ? null : id)}
                phaseMarkers={phaseMarkers}
                minTs={timelineMinTs}
                maxTs={timelineMaxTs}
              />
            </div>

            {/* ── Scrollable bottom section: detail panel + event list ── */}
            <div className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden px-4 pb-4">
              {selectedEvent && (
                <div className="mb-3">
                  <EventDetailPanel event={selectedEvent} onClose={() => setSelectedId(null)} />
                </div>
              )}
              <div className="text-xs text-slate-500 mb-2">
                {filtered.length} event{filtered.length !== 1 ? 's' : ''}
                {filtered.length !== events.length ? ` (of ${events.length} total)` : ''}
              </div>
              <div className="space-y-1.5">
                {filtered.map(evt => (
                  <EventRow
                    key={evt.id}
                    event={evt}
                    selected={evt.id === selectedId}
                    onSelect={() => setSelectedId(prev => prev === evt.id ? null : evt.id)}
                  />
                ))}
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
