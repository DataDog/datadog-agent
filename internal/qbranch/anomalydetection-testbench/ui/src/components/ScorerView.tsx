import { useState, useMemo, useCallback } from 'react';
import { AnomalyScoreTimeline } from './AnomalyScoreTimeline';
import type { PhaseMarker } from './ChartWithAnomalyDetails';
import type { ObserverState, ObserverActions } from '../hooks/useObserver';
import type { ScoreState, SeverityEvent, Anomaly, LogAnomaly } from '../api/client';

// ── ScenarioSelector (reused pattern) ───────────────────────────────────────

function ScenarioSelector({ scenarios, activeScenario, onLoadScenario }: {
  scenarios: { name: string; hasParquet: boolean; hasLogs: boolean; hasEvents: boolean }[];
  activeScenario: string | null;
  onLoadScenario: (name: string) => Promise<void>;
}) {
  return (
    <div className="p-4 border-b border-slate-700">
      <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">Scenarios</h2>
      <div className="space-y-1">
        {scenarios.length === 0 ? (
          <div className="text-sm text-slate-500">No scenarios found</div>
        ) : (
          scenarios.map((s) => (
            <button
              key={s.name}
              onClick={() => onLoadScenario(s.name)}
              className={`w-full text-left px-3 py-2 rounded text-sm transition-colors ${
                activeScenario === s.name
                  ? 'bg-purple-600 text-white'
                  : 'text-slate-300 hover:bg-slate-700'
              }`}
            >
              <div className="font-medium">{s.name}</div>
              <div className="text-xs text-slate-400 mt-0.5">
                {[s.hasParquet && 'parquet', s.hasLogs && 'logs', s.hasEvents && 'events']
                  .filter(Boolean)
                  .join(', ') || 'empty'}
              </div>
            </button>
          ))
        )}
      </div>
    </div>
  );
}

// ── Helpers ──────────────────────────────────────────────────────────────────

const SEVERITY_LEVEL_LABELS = ['Low', 'Medium', 'High'] as const;
const SEVERITY_LEVEL_COLORS = ['#22c55e', '#f59e0b', '#ef4444'] as const;

function formatTs(unix: number) {
  return new Date(unix * 1000).toLocaleTimeString([], {
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  });
}

function formatDuration(secs: number) {
  if (secs < 60) return `${secs}s`;
  if (secs < 3600) return `${Math.floor(secs / 60)}m ${secs % 60}s`;
  return `${(secs / 3600).toFixed(1)}h`;
}

// ── EventList ────────────────────────────────────────────────────────────────

interface EventWindow {
  event: SeverityEvent;
  endSec: number;
  anomalies: Anomaly[];
  logAnomalies: LogAnomaly[];
}

function buildEventWindows(
  events: SeverityEvent[],
  lastSec: number,
  allAnomalies: Anomaly[],
  allLogAnomalies: LogAnomaly[],
): EventWindow[] {
  return events.map((ev, i) => {
    const end = i + 1 < events.length ? events[i + 1].timestamp - 1 : lastSec;
    const anomalies = allAnomalies.filter(
      (a) => a.timestamp >= ev.timestamp && a.timestamp <= end,
    );
    const logAnomalies = allLogAnomalies.filter(
      (a) => a.timestamp >= ev.timestamp && a.timestamp <= end,
    );
    return { event: ev, endSec: end, anomalies, logAnomalies };
  });
}

function AnomalyRow({ anomaly }: { anomaly: Anomaly }) {
  return (
    <div className="flex items-start gap-2 py-1 border-t border-slate-700/50 first:border-0">
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2 flex-wrap">
          <span className="text-xs text-slate-300 font-mono">{formatTs(anomaly.timestamp)}</span>
          <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-700 text-slate-400 font-mono">
            {anomaly.detectorComponent ?? anomaly.detectorName}
          </span>
          <span className="text-xs text-slate-200">{anomaly.title}</span>
        </div>
        <div className="text-[10px] text-slate-500 font-mono truncate mt-0.5">
          {anomaly.source}
        </div>
      </div>
    </div>
  );
}

function LogAnomalyRow({ anomaly }: { anomaly: LogAnomaly }) {
  return (
    <div className="flex items-start gap-2 py-1 border-t border-slate-700/50 first:border-0">
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2 flex-wrap">
          <span className="text-xs text-slate-300 font-mono">{formatTs(anomaly.timestamp)}</span>
          <span className="text-[10px] px-1.5 py-0.5 rounded bg-blue-900/60 text-blue-300 font-mono">
            log
          </span>
          <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-700 text-slate-400 font-mono">
            {anomaly.detectorName}
          </span>
          <span className="text-xs text-slate-200">{anomaly.title}</span>
        </div>
        {anomaly.description && (
          <div className="text-[10px] text-slate-500 truncate mt-0.5">{anomaly.description}</div>
        )}
      </div>
    </div>
  );
}

type AnomalyFilter = 'all' | 'metrics' | 'logs';

function EventRow({ window: w, filter, onHover }: {
  window: EventWindow;
  filter: AnomalyFilter;
  onHover?: (ev: SeverityEvent | null) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const { event, endSec, anomalies, logAnomalies } = w;
  const isUp = event.to_level > event.from_level;
  const duration = endSec - event.timestamp;
  const fromColor = SEVERITY_LEVEL_COLORS[event.from_level];
  const toColor = SEVERITY_LEVEL_COLORS[event.to_level];

  const visibleMetrics = filter !== 'logs' ? anomalies : [];
  const visibleLogs = filter !== 'metrics' ? logAnomalies : [];
  const visibleCount = visibleMetrics.length + visibleLogs.length;
  const totalCount = anomalies.length + logAnomalies.length;

  return (
    <div
      className="border-b border-slate-700/60 last:border-0"
      onMouseEnter={() => onHover?.(event)}
      onMouseLeave={() => onHover?.(null)}
    >
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className="w-full text-left px-4 py-2.5 flex items-center gap-3 hover:bg-slate-700/40 transition-colors"
      >
        <span className="text-slate-500 w-4 flex-shrink-0 text-xs select-none">
          {expanded ? '▼' : '▶'}
        </span>

        {/* Transition badge */}
        <span className="flex items-center gap-1 flex-shrink-0">
          <span className="text-xs font-semibold" style={{ color: fromColor }}>
            {SEVERITY_LEVEL_LABELS[event.from_level]}
          </span>
          <span className={`text-xs font-bold ${isUp ? 'text-red-400' : 'text-green-400'}`}>
            {isUp ? '▲' : '▼'}
          </span>
          <span className="text-xs font-semibold" style={{ color: toColor }}>
            {SEVERITY_LEVEL_LABELS[event.to_level]}
          </span>
        </span>

        {/* Timestamp + duration */}
        <span className="text-xs text-slate-300 font-mono flex-shrink-0">{formatTs(event.timestamp)}</span>
        <span className="text-[10px] text-slate-500 flex-shrink-0">for {formatDuration(duration)}</span>

        {/* Anomaly count */}
        <span className="ml-auto text-[10px] text-slate-500 flex-shrink-0 tabular-nums">
          {visibleCount !== totalCount
            ? `${visibleCount} / ${totalCount}`
            : totalCount}{' '}
          anomal{totalCount !== 1 ? 'ies' : 'y'}
        </span>
      </button>

      {expanded && (
        <div className="px-4 pb-3 ml-7">
          {visibleCount === 0 ? (
            <p className="text-xs text-slate-500 italic">
              {totalCount === 0 ? 'No anomalies in this window.' : 'No anomalies match the current filter.'}
            </p>
          ) : (
            <div className="bg-slate-900/40 rounded p-2">
              {visibleMetrics.map((a, i) => (
                <AnomalyRow key={`m-${i}`} anomaly={a} />
              ))}
              {visibleLogs.map((a, i) => (
                <LogAnomalyRow key={`l-${i}`} anomaly={a} />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// ── ScorerView ───────────────────────────────────────────────────────────────

interface TimeRange {
  start: number;
  end: number;
}

interface ScorerViewProps {
  state: ObserverState;
  actions: ObserverActions;
  sidebarWidth: number;
  phaseMarkers?: PhaseMarker[];
  timeRange?: TimeRange | null;
  onTimeRangeChange?: (range: TimeRange | null) => void;
}

export function ScorerView({ state, actions, sidebarWidth, phaseMarkers, timeRange, onTimeRangeChange }: ScorerViewProps) {
  const [scoreState, setScoreState] = useState<ScoreState | null>(null);
  const [anomalyFilter, setAnomalyFilter] = useState<AnomalyFilter>('all');
  const [hoveredEvent, setHoveredEvent] = useState<SeverityEvent | null>(null);

  const handleScoreState = useCallback((ss: ScoreState) => {
    setScoreState(ss);
  }, []);

  const eventWindows = useMemo(() => {
    if (!scoreState || !(scoreState.events ?? []).length) return [];
    const buckets = scoreState.buckets ?? [];
    const lastSec = buckets.length > 0 ? buckets[buckets.length - 1].second : 0;
    return buildEventWindows(
      scoreState.events,
      lastSec,
      state.anomalies ?? [],
      state.logAnomalies ?? [],
    );
  }, [scoreState, state.anomalies, state.logAnomalies]);

  const ewmaPeak = useMemo(() =>
    (scoreState?.buckets ?? []).reduce((m, b) => Math.max(m, b.ewma), 0),
    [scoreState],
  );

  return (
    <div className="flex-1 flex">
      <aside
        className="bg-slate-800 border-r border-slate-700 overflow-y-auto"
        style={{ width: sidebarWidth }}
      >
        <ScenarioSelector
          scenarios={state.scenarios}
          activeScenario={state.activeScenario}
          onLoadScenario={actions.loadScenario}
        />

        {scoreState && (
          <div className="p-4 border-b border-slate-700">
            <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">
              Summary
            </h2>
            <div className="space-y-1 text-xs text-slate-300">
              <div className="flex justify-between">
                <span className="text-slate-500">Buckets</span>
                <span>{(scoreState.buckets ?? []).length}s</span>
              </div>
              <div className="flex justify-between">
                <span className="text-slate-500">EWMA peak</span>
                <span className="text-cyan-300">{ewmaPeak.toFixed(4)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-slate-500">Events</span>
                <span>{(scoreState.events ?? []).length}</span>
              </div>
              <div className="mt-2 pt-2 border-t border-slate-700 space-y-1">
                {(['Low', 'Medium', 'High'] as const).map((label, lvl) => {
                  const up = (scoreState.events ?? []).filter(
                    (e) => e.to_level === lvl && e.to_level > e.from_level,
                  ).length;
                  const down = (scoreState.events ?? []).filter(
                    (e) => e.from_level === lvl && e.to_level < e.from_level,
                  ).length;
                  if (up + down === 0) return null;
                  return (
                    <div key={label} className="flex justify-between">
                      <span style={{ color: SEVERITY_LEVEL_COLORS[lvl] }}>{label}</span>
                      <span className="text-slate-400">
                        {up > 0 && <span className="text-red-400">▲{up}</span>}
                        {up > 0 && down > 0 && ' '}
                        {down > 0 && <span className="text-green-400">▼{down}</span>}
                      </span>
                    </div>
                  );
                })}
              </div>
            </div>
          </div>
        )}
      </aside>

      <main className="flex-1 flex flex-col min-h-0 overflow-hidden">
        <div className="flex-1 flex flex-col min-h-0 overflow-y-auto">
          {/* Timeline widget */}
          <div className="p-4 shrink-0">
            <AnomalyScoreTimeline
              scenarioDataVersion={state.scenarioDataVersion}
              phaseMarkers={phaseMarkers}
              timeRange={timeRange}
              hoveredEvent={hoveredEvent}
              onTimeRangeChange={onTimeRangeChange}
              onScoreState={handleScoreState}
            />
          </div>

          {/* Event list */}
          {state.connectionState === 'ready' && (
            <div className="px-4 pb-6">
              <div className="bg-slate-800 rounded-lg border border-slate-700 overflow-hidden">
                <div className="px-4 py-2 border-b border-slate-700 flex items-center gap-2">
                  <span className="text-sm font-medium text-slate-300">
                    Severity events
                  </span>
                  {scoreState && (
                    <span className="text-xs text-slate-500">
                      · {(scoreState.events ?? []).length} transition{(scoreState.events ?? []).length !== 1 ? 's' : ''}
                      · click to expand
                    </span>
                  )}
                  <div className="ml-auto flex gap-1">
                    {(['all', 'metrics', 'logs'] as const).map((f) => (
                      <button
                        key={f}
                        onClick={() => setAnomalyFilter(f)}
                        className={`px-2 py-0.5 rounded text-[10px] font-medium transition-colors ${
                          anomalyFilter === f
                            ? 'bg-purple-600 text-white'
                            : 'text-slate-400 hover:bg-slate-700'
                        }`}
                      >
                        {f === 'all' ? 'All' : f === 'metrics' ? 'Metrics' : 'Logs'}
                      </button>
                    ))}
                  </div>
                </div>

                {!scoreState || !(scoreState.events ?? []).length ? (
                  <div className="px-4 py-8 text-center text-slate-500 text-sm">
                    {state.scenarioDataVersion === 0
                      ? 'Load a scenario to see severity events'
                      : 'No severity transitions detected'}
                  </div>
                ) : (
                  <div>
                    {eventWindows.map((w, i) => (
                      <EventRow key={i} window={w} filter={anomalyFilter} onHover={setHoveredEvent} />
                    ))}
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
      </main>
    </div>
  );
}
