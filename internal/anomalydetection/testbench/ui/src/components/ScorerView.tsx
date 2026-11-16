import { ScenarioSelector } from './ScenarioSelector';
import { useState, useMemo, useCallback, useRef, useEffect } from 'react';
import { visibleSeverityEvents } from '../scorerDisplay';
import { resolveMetricContributor, contributorLogTags, onsetWindow } from '../investigation';
import type { SourceInspectionActions } from '../investigation';
import { AnomalyScoreTimeline } from './AnomalyScoreTimeline';
import type { ScorerTimeMode } from './AnomalyScoreTimeline';
import { BaselineTimelineWidget } from './BaselineView';
import type { PhaseMarker } from './ChartWithAnomalyDetails';
import type { ObserverState, ObserverActions } from '../hooks/useObserver';
import type { ScoreState, SeverityEvent, Anomaly, LogAnomaly, ScorerReport, ScorerInput, MetricName, SeriesID, SeriesInfo } from '../api/client';

// ── ScenarioSelector (reused pattern) ───────────────────────────────────────


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
  startSec: number;
  endSec: number;
  anomalies: Anomaly[];
  logAnomalies: LogAnomaly[];
}

function buildEventWindows(
  events: SeverityEvent[],
  lastSec: number,
  allAnomalies: Anomaly[],
  allLogAnomalies: LogAnomaly[],
  range?: TimeRange | null,
  mode: ScorerTimeMode = 'detection',
): EventWindow[] {
  const inputTime = (input: Anomaly | LogAnomaly) => mode === 'detection' ? input.scoringSecond ?? input.timestamp : input.timestamp;
  return events.map((ev, i) => {
    const end = Math.min(i + 1 < events.length ? events[i + 1].timestamp - 1 : lastSec, range?.end ?? lastSec);
    const start = Math.max(ev.timestamp, range?.start ?? ev.timestamp);
    const anomalies = allAnomalies.filter(
      (a) => inputTime(a) >= start && inputTime(a) <= end,
    );
    const logAnomalies = allLogAnomalies.filter(
      (a) => inputTime(a) >= start && inputTime(a) <= end,
    );
    return { event: ev, startSec: start, endSec: end, anomalies, logAnomalies };
  });
}

function scorerInputToAnomaly(input: ScorerInput): Anomaly {
  return {
    source: input.source as MetricName,
    sourceSeriesId: input.sourceSeriesId as SeriesID | undefined,
    detectorName: input.detectorName,
    detectorComponent: input.detectorComponent,
    title: input.title,
    description: input.description,
    host: input.host,
    tags: input.tags,
    timestamp: input.timestamp,
    scoringSecond: input.scoringSecond,
  };
}

function scorerInputToLogAnomaly(input: ScorerInput): LogAnomaly {
  return {
    source: input.source,
    detectorName: input.detectorName,
    title: input.title,
    description: input.description,
    tags: input.tags,
    timestamp: input.timestamp,
    scoringSecond: input.scoringSecond,
  };
}

function AnomalyRow({ anomaly, series, onInspectSeries }: { anomaly: Anomaly; series?: SeriesInfo } & SourceInspectionActions) {
  return (
    <div className="flex items-start gap-2 py-1 border-t border-slate-700/50 first:border-0">
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2 flex-wrap">
          <span className="text-xs text-slate-300 font-mono">{formatTs(anomaly.timestamp)}</span>
          {anomaly.scoringSecond !== undefined && anomaly.scoringSecond !== anomaly.timestamp && (
            <span className="text-[10px] text-cyan-400 font-mono" title="Effective scorer second">
              scored {formatTs(anomaly.scoringSecond)}
            </span>
          )}
          <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-700 text-slate-400 font-mono">
            {anomaly.detectorComponent ?? anomaly.detectorName}
          </span>
          <span className="text-xs text-slate-200">{anomaly.title}</span>
        </div>
        <div className="text-[10px] text-slate-500 font-mono truncate mt-0.5">
          {anomaly.source}
        </div>
      </div>
      {series && onInspectSeries && <button type="button" className="shrink-0 text-xs text-purple-300 hover:underline" onClick={() => onInspectSeries(series, onsetWindow(anomaly.timestamp))}>View metric →</button>}
    </div>
  );
}

function LogAnomalyRow({ anomaly, onInspectLogs }: { anomaly: LogAnomaly } & SourceInspectionActions) {
  return (
    <div className="flex items-start gap-2 py-1 border-t border-slate-700/50 first:border-0">
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2 flex-wrap">
          <span className="text-xs text-slate-300 font-mono">{formatTs(anomaly.timestamp)}</span>
          {anomaly.scoringSecond !== undefined && anomaly.scoringSecond !== anomaly.timestamp && (
            <span className="text-[10px] text-cyan-400 font-mono" title="Effective scorer second">
              scored {formatTs(anomaly.scoringSecond)}
            </span>
          )}
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
      {onInspectLogs && <button type="button" className="shrink-0 text-xs text-purple-300 hover:underline" onClick={() => onInspectLogs(onsetWindow(anomaly.timestamp), (anomaly.tags ?? []).filter(tag => /^(host|service|container_name):/.test(tag)).join(' '))}>View logs →</button>}
    </div>
  );
}

type AnomalyFilter = 'all' | 'metrics' | 'logs';

function EventRow({ window: w, filter, report, onHover, expanded, onToggle, series, onInspectSeries, onInspectLogs }: {
  window: EventWindow;
  filter: AnomalyFilter;
  report?: ScorerReport;
  onHover?: (ev: SeverityEvent | null) => void;
  expanded: boolean;
  onToggle: () => void;
  series: SeriesInfo[];
} & SourceInspectionActions) {
  const [visibleLimit, setVisibleLimit] = useState(50);
  const { event, startSec, endSec, anomalies, logAnomalies } = w;
  const isUp = event.to_level > event.from_level;
  const duration = Math.max(0, endSec - startSec + 1);
  const fromColor = SEVERITY_LEVEL_COLORS[event.from_level];
  const toColor = SEVERITY_LEVEL_COLORS[event.to_level];

  const visibleMetrics = filter !== 'logs' ? anomalies : [];
  const visibleLogs = filter !== 'metrics' ? logAnomalies : [];
  const visibleCount = visibleMetrics.length + visibleLogs.length;
  const totalCount = anomalies.length + logAnomalies.length;

  return (
    <div
      data-event-timestamp={event.timestamp}
      className="border-b border-slate-700/60 last:border-0"
      onMouseEnter={() => onHover?.(event)}
      onMouseLeave={() => onHover?.(null)}
    >
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={expanded}
        className="w-full text-left px-4 py-2.5 flex flex-wrap items-center gap-3 hover:bg-slate-700/40 transition-colors"
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
        <span className="text-[10px] text-slate-500 flex-shrink-0">{startSec > event.timestamp ? 'Continues into selection · ' : ''}for {formatDuration(duration)} in view</span>

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
          {report && (
            <div className="mb-3 bg-slate-900/40 rounded p-2">
              <div className="text-[10px] uppercase tracking-wider text-slate-500 mb-1.5">Top contributors when High began</div>
              <p className="mb-2 text-xs text-slate-400">Captured at episode onset. Log text is a representative example or pattern; its embedded timestamp may differ.</p>
              {report.contributors.length === 0 ? (
                <div className="text-xs text-slate-500">No storage-backed contributors</div>
              ) : (
                <ol className="space-y-1 text-xs text-slate-300 list-decimal list-inside">
                  {report.contributors.map((contributor, index) => {
                    const metric = resolveMetricContributor(contributor.name, series);
                    const isLog = contributor.name.startsWith('log: ');
                    const label = isLog ? contributor.name.slice(5).split(' — {')[0] : contributor.name.split('{')[0];
                    return <li key={`${contributor.name}-${index}`} className="flex min-w-0 items-start gap-2">
                      <span className="shrink-0 text-cyan-300">{Math.round(contributor.share * 100)}%</span>
                      <details className="min-w-0 flex-1 font-mono text-slate-200">
                        <summary className="cursor-pointer break-words" title={contributor.name}>{label}</summary>
                        <p className="mt-1 break-all text-slate-400">{contributor.name}</p>
                      </details>
                      {metric && onInspectSeries && <button type="button" className="shrink-0 text-purple-300 hover:underline" onClick={() => onInspectSeries(metric, onsetWindow(event.timestamp))}>View metric →</button>}
                      {isLog && onInspectLogs && <button type="button" className="shrink-0 text-purple-300 hover:underline" onClick={() => onInspectLogs(onsetWindow(event.timestamp), contributorLogTags(contributor.name))}>View logs →</button>}
                      {!metric && !isLog && <span className="shrink-0 text-slate-500" title="The descriptor does not uniquely identify a current tagged series.">Series unavailable</span>}
                    </li>;
                  })}
                </ol>
              )}
            </div>
          )}
          {visibleCount === 0 ? (
            <p className="text-xs text-slate-500 italic">
              {totalCount === 0 ? 'No anomalies in this window.' : 'No anomalies match the current filter.'}
            </p>
          ) : (
            <div className="bg-slate-900/40 rounded p-2">
              {visibleMetrics.slice(0, visibleLimit).map((a, i) => (
                <AnomalyRow key={`m-${i}`} anomaly={a} series={series.find(candidate => candidate.id === a.sourceSeriesId)} onInspectSeries={onInspectSeries} />
              ))}
              {visibleLogs.slice(0, Math.max(0, visibleLimit - visibleMetrics.length)).map((a, i) => (
                <LogAnomalyRow key={`l-${i}`} anomaly={a} onInspectLogs={onInspectLogs} />
              ))}
              {visibleCount > visibleLimit && (
                <button type="button" className="mt-2 text-xs text-purple-300 hover:underline" onClick={() => setVisibleLimit(limit => limit + 50)}>Show {Math.min(50, visibleCount - visibleLimit)} more detections</button>
              )}
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

interface ScorerViewProps extends SourceInspectionActions {
  isActive?: boolean;
  state: ObserverState;
  actions: ObserverActions;
  sidebarWidth: number;
  phaseMarkers?: PhaseMarker[];
  timeRange?: TimeRange | null;
  onTimeRangeChange?: (range: TimeRange | null) => void;
}

export function ScorerView({ state, actions, sidebarWidth, phaseMarkers, timeRange, onTimeRangeChange, onInspectSeries, onInspectLogs, isActive = true }: ScorerViewProps) {
  const [expandedEvents, setExpandedEvents] = useState<Set<string>>(new Set());
  const viewRef = useRef<HTMLDivElement>(null);
  const returnEventRef = useRef<number | null>(null);
  useEffect(() => {
    if (!isActive || returnEventRef.current === null) return;
    const timestamp = returnEventRef.current;
    returnEventRef.current = null;
    const frame = requestAnimationFrame(() => viewRef.current?.querySelector(`[data-event-timestamp="${timestamp}"]`)?.scrollIntoView({ block: 'nearest' }));
    return () => cancelAnimationFrame(frame);
  }, [isActive]);
  const [scoreState, setScoreState] = useState<ScoreState | null>(null);
  const [timeMode, setTimeMode] = useState<ScorerTimeMode>('detection');
  const [anomalyFilter, setAnomalyFilter] = useState<AnomalyFilter>('all');
  const [hoveredEvent, setHoveredEvent] = useState<SeverityEvent | null>(null);

  const handleScoreState = useCallback((ss: ScoreState | null) => {
    setScoreState(ss);
  }, []);

  const metricAnomalies = useMemo(() => {
    if (timeMode === 'detection' && scoreState?.inputs) {
      return scoreState.inputs.filter(input => !input.isLog).map(scorerInputToAnomaly);
    }
    return state.anomalies ?? [];
  }, [timeMode, scoreState, state.anomalies]);

  const logAnomalies = useMemo(() => {
    if (timeMode === 'detection' && scoreState?.inputs) {
      return scoreState.inputs.filter(input => input.isLog).map(scorerInputToLogAnomaly);
    }
    return state.logAnomalies ?? [];
  }, [timeMode, scoreState, state.logAnomalies]);

  const eventWindows = useMemo(() => {
    if (!scoreState || !(scoreState.events ?? []).length) return [];
    const buckets = scoreState.buckets ?? [];
    const lastSec = buckets.length > 0 ? buckets[buckets.length - 1].second : 0;
    return buildEventWindows(
      visibleSeverityEvents(scoreState.events, timeRange),
      lastSec,
      metricAnomalies,
      logAnomalies,
      timeRange,
      timeMode,
    ).filter(window => window.endSec >= window.startSec);
  }, [scoreState, metricAnomalies, logAnomalies, timeRange, timeMode]);

  const ewmaPeak = useMemo(() =>
    (scoreState?.buckets ?? []).reduce((m, b) => Math.max(m, b.ewma), 0),
    [scoreState],
  );

  const disruption = state.status?.episodeInfo?.disruption;
  const disruptionStart = Date.parse(disruption?.start ?? '') / 1000;
  const disruptionEnd = Date.parse(disruption?.end ?? '') / 1000;
  const firstHigh = (scoreState?.events ?? []).find(event => event.to_level === 2 && event.from_level < 2 && event.timestamp >= disruptionStart && event.timestamp <= disruptionEnd);

  const reportsByTimestamp = useMemo(() => new Map(
    (scoreState?.reports ?? []).map((report) => [report.timestamp, report]),
  ), [scoreState]);

  return (
    <div ref={viewRef} className="flex-1 flex min-w-0">
      <aside
        className="bg-slate-800 border-r border-slate-700 overflow-y-auto shrink-0"
        style={{ width: sidebarWidth }}
      >
        <ScenarioSelector
          scenarios={state.scenarios}
          activeScenario={state.activeScenario}
          onLoadScenario={actions.loadScenario}
          busy={state.connectionState === 'loading' || state.pendingComponents.size > 0}
        />

        {scoreState?.available === false && (
          <div className="p-4 border-b border-slate-700 text-xs text-amber-300">
            Scorer unavailable for this run.
          </div>
        )}

        {scoreState && scoreState.available !== false && (
          <div className="p-4 border-b border-slate-700">
            <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">
              Recording summary
            </h2>
            <div className="space-y-1 text-xs text-slate-300">
              <div className="flex justify-between"><span className="text-slate-500">Metric detections</span><span>{state.anomalies.length}</span></div>
              <div className="flex justify-between"><span className="text-slate-500">Log detections</span><span>{state.logAnomalies.length}</span></div>
              {Number.isFinite(disruptionStart) && <div className="flex justify-between gap-2"><span className="text-slate-500">First High in disruption</span><span>{firstHigh ? `+${formatDuration(Math.round(firstHigh.timestamp - disruptionStart))}` : 'Not detected'}</span></div>}
              <div className="flex justify-between">
                <span className="text-slate-500">Buckets</span>
                <span>{(scoreState.buckets ?? []).length}s</span>
              </div>
              <div className="flex justify-between">
                <span className="text-slate-500">Timing</span>
                <span>{timeMode === 'detection' ? 'Detection' : 'Change point'}</span>
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

      <main className="flex-1 min-w-0 flex flex-col min-h-0 overflow-hidden">
        <div className="flex-1 flex flex-col min-h-0 overflow-y-auto">
          {/* Timeline widget */}
          <div className="p-4 shrink-0">
            <AnomalyScoreTimeline
              scenarioDataVersion={state.scenarioDataVersion}
              timeMode={timeMode}
              onTimeModeChange={setTimeMode}
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
                <div className="px-4 py-2 border-b border-slate-700 flex flex-wrap items-center gap-2">
                  <span className="text-sm font-medium text-slate-300">
                    Severity events
                  </span>
                  {scoreState && (
                    <span className="text-xs text-slate-500">
                      · {eventWindows.length} severity interval{eventWindows.length !== 1 ? 's' : ''} in view
                      {' · '}
                      {timeMode === 'detection' ? 'grouped by scoring second' : 'grouped by change-point time'}
                      {' · click to expand'}
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
                      : scoreState?.available === false
                      ? 'Scorer unavailable for this run'
                      : 'No severity intervals in the selected time range'}
                  </div>
                ) : (
                  <div>
                    {eventWindows.map((w) => (
                      <EventRow
                        key={`${state.activeScenario}:${timeMode}:${w.event.timestamp}`}
                        expanded={expandedEvents.has(`${state.activeScenario}:${timeMode}:${w.event.timestamp}`)}
                        onToggle={() => setExpandedEvents(current => {
                          const next = new Set(current);
                          const key = `${state.activeScenario}:${timeMode}:${w.event.timestamp}`;
                          if (next.has(key)) next.delete(key); else next.add(key);
                          return next;
                        })}
                        series={state.series}
                        onInspectSeries={(series, range) => { returnEventRef.current = w.event.timestamp; onInspectSeries?.(series, range); }}
                        onInspectLogs={(range, tags) => { returnEventRef.current = w.event.timestamp; onInspectLogs?.(range, tags); }}
                        window={w}
                        filter={anomalyFilter}
                        report={w.event.to_level === 2 && w.event.to_level > w.event.from_level
                          ? reportsByTimestamp.get(w.event.timestamp)
                          : undefined}
                        onHover={setHoveredEvent}
                      />
                    ))}
                  </div>
                )}
              </div>
            </div>
          )}

          <div className="px-4 pb-6">
            <BaselineTimelineWidget status={state.status} phaseMarkers={phaseMarkers} />
          </div>
        </div>
      </main>
    </div>
  );
}
