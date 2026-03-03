import { useState, useMemo, useRef, useEffect } from 'react';
import type { ScenarioInfo, LogAnomaly, LogEntry } from '../api/client';
import type { ObserverState, ObserverActions } from '../hooks/useObserver';
import { MAIN_TAG_FILTER_KEYS } from '../constants';
import { parseTagFilter, extractTagGroups, toggleTagInInput, matchesTagFilter } from '../filters';

interface LogViewProps {
  state: ObserverState;
  actions: ObserverActions;
  sidebarWidth: number;
}

function formatTimestamp(ts: number): string {
  return new Date(ts * 1000).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });
}

function levelBadgeColor(status: string): string {
  switch (status.toLowerCase()) {
    case 'error':
      return 'text-red-400 bg-red-900/40';
    case 'warn':
    case 'warning':
      return 'text-amber-400 bg-amber-900/40';
    case 'info':
      return 'text-blue-400 bg-blue-900/40';
    case 'debug':
    case 'trace':
      return 'text-slate-400 bg-slate-700/40';
    default:
      return 'text-slate-400 bg-slate-700/40';
  }
}

function scoreColor(score: number): string {
  if (score >= 0.9) return 'text-red-400 bg-red-900/40';
  if (score >= 0.7) return 'text-orange-400 bg-orange-900/40';
  if (score >= 0.5) return 'text-yellow-400 bg-yellow-900/40';
  return 'text-slate-400 bg-slate-700/40';
}

function detectorBadgeColor(name: string): string {
  const colors = [
    'text-purple-400 bg-purple-900/40',
    'text-blue-400 bg-blue-900/40',
    'text-cyan-400 bg-cyan-900/40',
    'text-teal-400 bg-teal-900/40',
    'text-green-400 bg-green-900/40',
  ];
  let hash = 0;
  for (let i = 0; i < name.length; i++) hash = (hash * 31 + name.charCodeAt(i)) & 0xffffffff;
  return colors[Math.abs(hash) % colors.length];
}

function detectorLineColor(name: string): string {
  const colors = ['#c084fc', '#60a5fa', '#22d3ee', '#2dd4bf', '#4ade80'];
  let hash = 0;
  for (let i = 0; i < name.length; i++) hash = (hash * 31 + name.charCodeAt(i)) & 0xffffffff;
  return colors[Math.abs(hash) % colors.length];
}

// Combined log rate + anomaly timeline chart
function LogRateChart({
  logs,
  anomalies,
  scenarioStart,
  scenarioEnd,
  hoveredTimestamp,
  hoveredAnomalyIndex,
}: {
  logs: LogEntry[];
  anomalies: LogAnomaly[];
  scenarioStart: number | null;
  scenarioEnd: number | null;
  hoveredTimestamp?: number | null;
  hoveredAnomalyIndex?: number | null;
}) {
  if (!scenarioStart || !scenarioEnd || scenarioEnd <= scenarioStart) {
    return null;
  }

  const bucketCount = 60;
  const bucketSize = (scenarioEnd - scenarioStart) / bucketCount;

  const logBuckets = new Array(bucketCount).fill(0);
  for (const l of logs) {
    const idx = Math.min(Math.floor((l.timestamp - scenarioStart) / bucketSize), bucketCount - 1);
    if (idx >= 0) logBuckets[idx]++;
  }

  const maxLog = Math.max(1, ...logBuckets);

  const hoveredBucket =
    hoveredTimestamp != null
      ? Math.min(Math.floor((hoveredTimestamp - scenarioStart) / bucketSize), bucketCount - 1)
      : null;

  // Derive unique detectors for the legend
  const detectors = Array.from(new Set(anomalies.map((a) => a.detectorName)));

  return (
    <div className="bg-slate-800/60 rounded p-3 mb-4">
      <div className="flex items-center gap-4 text-xs text-slate-400 mb-1.5">
        <span>Log rate ({logs.length} log{logs.length !== 1 ? 's' : ''} total)</span>
        {detectors.map((name) => (
          <span key={name} className="flex items-center gap-1">
            <span className="inline-block w-2 h-2 rounded-sm" style={{ backgroundColor: detectorLineColor(name) }} />
            <span style={{ color: detectorLineColor(name) }}>{name}</span>
          </span>
        ))}
      </div>
      <div className="relative h-10">
        {/* Log rate bars — low alpha background */}
        <div className="absolute inset-0 flex items-end gap-px">
          {logBuckets.map((count, i) => {
            const isHovered = hoveredBucket !== null && i === hoveredBucket;
            let barClass: string;
            if (isHovered) {
              barClass = 'flex-1 rounded-sm bg-amber-400/80';
            } else if (count > 0) {
              barClass = 'flex-1 rounded-sm bg-teal-500/25';
            } else {
              barClass = 'flex-1 rounded-sm bg-slate-700/20';
            }
            return (
              <div
                key={i}
                className={barClass}
                style={{ height: count > 0 ? `${Math.max(3, (count / maxLog) * 40)}px` : '2px' }}
                title={count > 0 ? `${count} log${count > 1 ? 's' : ''}` : undefined}
              />
            );
          })}
        </div>
        {/* Anomaly lines — one vertical line per anomaly, colored by detector */}
        {anomalies.map((a, i) => {
          const pct = ((a.timestamp - scenarioStart) / (scenarioEnd - scenarioStart)) * 100;
          if (pct < 0 || pct > 100) return null;
          const color = detectorLineColor(a.detectorName);
          const isHovered = hoveredAnomalyIndex === i;
          return (
            <div
              key={i}
              className="absolute top-0 bottom-0"
              style={{ left: `${pct}%`, opacity: hoveredAnomalyIndex != null && !isHovered ? 0.35 : 1 }}
              title={`${a.detectorName}: ${a.title}`}
            >
              {/* Downward triangle at the top of the line */}
              <div style={{
                position: 'absolute',
                top: 0,
                left: isHovered ? '-5px' : '-3px',
                width: 0,
                height: 0,
                borderLeft: `${isHovered ? 5 : 3}px solid transparent`,
                borderRight: `${isHovered ? 5 : 3}px solid transparent`,
                borderTop: `${isHovered ? 8 : 5}px solid ${color}`,
              }} />
              {/* Vertical line */}
              <div
                className={`absolute top-0 bottom-0 ${isHovered ? 'w-0.5' : 'w-px'}`}
                style={{ backgroundColor: color }}
              />
            </div>
          );
        })}
      </div>
      <div className="flex justify-between text-xs text-slate-600 mt-1">
        <span>{formatTimestamp(scenarioStart)}</span>
        <span>{formatTimestamp(scenarioEnd)}</span>
      </div>
    </div>
  );
}

interface LogEntryRowProps {
  entry: LogEntry;
  isExpanded: boolean;
  onToggle: () => void;
  isTelemetry?: boolean;
  onHoverEnter?: () => void;
  onHoverLeave?: () => void;
}

function LogEntryRow({ entry, isExpanded, onToggle, isTelemetry = false, onHoverEnter, onHoverLeave }: LogEntryRowProps) {
  const contentPreview = entry.content.length > 120 && !isExpanded
    ? entry.content.slice(0, 120) + '…'
    : entry.content;

  return (
    <div className="bg-slate-700/30 rounded overflow-hidden" onMouseEnter={onHoverEnter} onMouseLeave={onHoverLeave}>
      <button
        onClick={onToggle}
        className="w-full text-left px-3 py-2 hover:bg-slate-700/50 transition-colors"
      >
        <div className="flex items-start gap-2">
          <span className="flex-shrink-0 text-xs text-slate-500 font-mono pt-0.5 w-20 text-right">
            {formatTimestamp(entry.timestamp)}
          </span>
          <span className={`flex-shrink-0 text-xs px-1.5 py-0.5 rounded font-medium uppercase ${levelBadgeColor(entry.status)}`}>
            {entry.status}
          </span>
          {isTelemetry && (
            <span className="flex-shrink-0 inline-flex items-center justify-center w-4 h-4 rounded-full bg-purple-600 text-white text-[9px] font-bold mt-0.5" title="Telemetry log">T</span>
          )}
          <span className="text-xs text-slate-300 font-mono leading-relaxed break-all flex-1">
            {contentPreview}
          </span>
          {entry.tags && entry.tags.length > 0 && (() => {
            const headerTags = entry.tags.filter((tag) =>
              MAIN_TAG_FILTER_KEYS.has(tag.slice(0, tag.indexOf(':')))
            );
            return headerTags.length > 0 ? (
              <div className="flex gap-1 flex-wrap flex-shrink-0">
                {headerTags.map((tag) => (
                  <span key={tag} className="px-1 py-0.5 rounded text-[9px] bg-slate-600/50 text-slate-400 font-mono">
                    {tag}
                  </span>
                ))}
              </div>
            ) : null;
          })()}
          {entry.content.length > 120 && (
            <span className="text-slate-500 flex-shrink-0 text-xs">{isExpanded ? '▼' : '▶'}</span>
          )}
        </div>
      </button>

      {isExpanded && entry.tags && entry.tags.length > 0 && (
        <div className="px-3 pb-2 border-t border-slate-600/30">
          <div className="flex gap-1 flex-wrap mt-1.5">
            {entry.tags.map((tag) => (
              <span
                key={tag}
                className="text-xs px-1.5 py-0.5 rounded bg-slate-600/50 text-slate-400 font-mono"
              >
                {tag}
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

interface LogAnomalyCardProps {
  anomaly: LogAnomaly;
  isExpanded: boolean;
  onToggle: () => void;
  onHoverEnter?: () => void;
  onHoverLeave?: () => void;
}

function LogAnomalyCard({ anomaly, isExpanded, onToggle, onHoverEnter, onHoverLeave }: LogAnomalyCardProps) {
  return (
    <div className="bg-slate-700/50 rounded overflow-hidden" onMouseEnter={onHoverEnter} onMouseLeave={onHoverLeave}>
      <button
        onClick={onToggle}
        className="w-full text-left px-4 py-3 hover:bg-slate-700/70 transition-colors"
      >
        <div className="flex items-start gap-3">
          <div className="flex-shrink-0 text-xs text-slate-500 font-mono pt-0.5 w-20 text-right">
            {formatTimestamp(anomaly.timestamp)}
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 flex-wrap mb-1">
              <span className={`text-xs px-1.5 py-0.5 rounded font-medium ${detectorBadgeColor(anomaly.detectorName)}`}>
                {anomaly.detectorName}
              </span>
              {anomaly.score !== undefined && (
                <span className={`text-xs px-1.5 py-0.5 rounded font-mono ${scoreColor(anomaly.score)}`}>
                  score: {anomaly.score.toFixed(2)}
                </span>
              )}
            </div>
            <div className="text-sm text-slate-200 font-medium leading-snug">
              {anomaly.title}
            </div>
            {!isExpanded && (
              <div className="text-xs text-slate-400 mt-0.5 truncate">
                {anomaly.description}
              </div>
            )}
          </div>
          {anomaly.tags && anomaly.tags.length > 0 && (() => {
            const headerTags = anomaly.tags.filter((tag) =>
              MAIN_TAG_FILTER_KEYS.has(tag.slice(0, tag.indexOf(':')))
            );
            return headerTags.length > 0 ? (
              <div className="flex gap-1 flex-wrap flex-shrink-0">
                {headerTags.map((tag) => (
                  <span key={tag} className="px-1 py-0.5 rounded text-[9px] bg-slate-600/50 text-slate-400 font-mono">
                    {tag}
                  </span>
                ))}
              </div>
            ) : null;
          })()}
          <span className="text-slate-500 flex-shrink-0 text-xs">{isExpanded ? '▼' : '▶'}</span>
        </div>
      </button>

      {isExpanded && (
        <div className="px-4 pb-3 border-t border-slate-600/50">
          <div className="mt-2 mb-2">
            <div className="text-xs text-slate-400 font-medium mb-1">Log Content</div>
            <pre className="text-xs text-slate-300 bg-slate-800/60 rounded p-2 whitespace-pre-wrap break-all font-mono leading-relaxed max-h-40 overflow-y-auto">
              {anomaly.description}
            </pre>
          </div>
          {anomaly.tags && anomaly.tags.length > 0 && (
            <div className="mt-2">
              <div className="text-xs text-slate-400 font-medium mb-1">Tags</div>
              <div className="flex gap-1 flex-wrap">
                {anomaly.tags.map((tag) => (
                  <span
                    key={tag}
                    className="text-xs px-1.5 py-0.5 rounded bg-slate-600/50 text-slate-400 font-mono"
                  >
                    {tag}
                  </span>
                ))}
              </div>
            </div>
          )}
          <div className="mt-2 text-xs text-slate-500">
            Source: <span className="text-slate-400">{anomaly.source}</span>
          </div>
        </div>
      )}
    </div>
  );
}

const LOG_PAGE_SIZE = 50;

/** Synthesize a `status:<value>` tag from the entry's status field so it can be filtered like any other tag. */
function getEffectiveTags(tags: string[], status: string): string[] {
  const statusTag = `status:${status.toLowerCase()}`;
  return tags.includes(statusTag) ? tags : [statusTag, ...tags];
}

export function LogView({ state, actions, sidebarWidth }: LogViewProps) {
  const scenarios = state.scenarios ?? [];
  const allLogs = state.logs ?? [];
  const allLogAnomalies = state.logAnomalies ?? [];

  const [tagFilterInput, setTagFilterInput] = useState('');
  const [expandedLogIndex, setExpandedLogIndex] = useState<number | null>(null);
  const [expandedAnomalyIndex, setExpandedAnomalyIndex] = useState<number | null>(null);
  const [hoveredLogTimestamp, setHoveredLogTimestamp] = useState<number | null>(null);
  const [hoveredAnomalyIndex, setHoveredAnomalyIndex] = useState<number | null>(null);
  const [anomaliesExpanded, setAnomaliesExpanded] = useState(true);
  const [logsExpanded, setLogsExpanded] = useState(true);
  const [logPage, setLogPage] = useState(1);
  const [telemetryLogsExpanded, setTelemetryLogsExpanded] = useState(true);
  const [telemetryLogPage, setTelemetryLogPage] = useState(1);
  const [expandedTelemetryLogIndex, setExpandedTelemetryLogIndex] = useState<number | null>(null);
  const initializedScenarioRef = useRef<string | null>(null);

  // Reset state when scenario changes
  useEffect(() => {
    if (state.activeScenario && initializedScenarioRef.current !== state.activeScenario) {
      initializedScenarioRef.current = state.activeScenario;
      setTagFilterInput('');
      setExpandedLogIndex(null);
      setExpandedAnomalyIndex(null);
      setLogPage(1);
      setTelemetryLogPage(1);
      setExpandedTelemetryLogIndex(null);
    }
  }, [state.activeScenario]);

  const logTagGroups = useMemo(() => {
    const all = extractTagGroups(allLogs.map((l) => getEffectiveTags(l.tags ?? [], l.status)));
    return new Map([...all.entries()].filter(([k]) => MAIN_TAG_FILTER_KEYS.has(k)));
  }, [allLogs]);

  const filteredLogs = useMemo(() => {
    const filter = parseTagFilter(tagFilterInput);
    return allLogs
      .filter((l) => {
        if (filter.include.size === 0 && filter.exclude.size === 0) return true;
        return matchesTagFilter(getEffectiveTags(l.tags ?? [], l.status), filter);
      })
      .sort((a, b) => a.timestamp - b.timestamp);
  }, [allLogs, tagFilterInput]);

  const regularLogs = useMemo(
    () => filteredLogs.filter((l) => !(l.tags ?? []).includes('telemetry:true')),
    [filteredLogs]
  );

  const telemetryLogs = useMemo(
    () => filteredLogs.filter((l) => (l.tags ?? []).includes('telemetry:true')),
    [filteredLogs]
  );

  const sortedAnomalies = useMemo(() => {
    const filter = parseTagFilter(tagFilterInput);
    const anomalies = (filter.include.size === 0 && filter.exclude.size === 0)
      ? allLogAnomalies
      : allLogAnomalies.filter((a) => matchesTagFilter(a.tags ?? [], filter));
    return [...anomalies].sort((a, b) => a.timestamp - b.timestamp);
  }, [allLogAnomalies, tagFilterInput]);

  const scenarioStart = state.status?.scenarioStart ?? null;
  const scenarioEnd = state.status?.scenarioEnd ?? null;

  return (
    <div className="flex-1 flex">
      {/* Sidebar */}
      <aside
        className="bg-slate-800 border-r border-slate-700 flex flex-col"
        style={{ width: sidebarWidth }}
      >
        <ScenarioSelector
          scenarios={scenarios}
          activeScenario={state.activeScenario}
          onLoadScenario={actions.loadScenario}
        />

        {/* Tag filter */}
        <div className="p-4 border-b border-slate-700">
          <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2">
            Tag Filter
          </h2>
          <div className="relative mb-2">
            <input
              type="text"
              value={tagFilterInput}
              onChange={(e) => {
                setTagFilterInput(e.target.value);
                setExpandedLogIndex(null);
                setLogPage(1);
              }}
              placeholder="host:web-1 service:api"
              className="w-full bg-slate-700 text-slate-200 text-xs rounded px-2 py-1.5 placeholder-slate-500 focus:outline-none focus:ring-1 focus:ring-teal-500 font-mono pr-6"
            />
            {tagFilterInput && (
              <button
                onClick={() => { setTagFilterInput(''); setLogPage(1); }}
                className="absolute right-1.5 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-300"
              >
                ×
              </button>
            )}
          </div>
          {logTagGroups.size > 0 && (
            <div className="space-y-2">
              {[...logTagGroups.entries()].map(([key, tags]) => {
                const { include: activeTags, exclude: excludedTags } = parseTagFilter(tagFilterInput);
                return (
                  <div key={key}>
                    <div className="text-[10px] text-slate-500 mb-1">{key}</div>
                    <div className="flex flex-wrap gap-1">
                      {tags.map((tag) => {
                        const active = activeTags.get(key)?.has(tag) ?? false;
                        const excluded = excludedTags.has(tag) || excludedTags.has(key);
                        const value = tag.slice(tag.indexOf(':') + 1);
                        const isStatus = key === 'status';
                        return (
                          <button
                            key={tag}
                            onClick={() => {
                              setTagFilterInput(toggleTagInInput(tagFilterInput, tag));
                              setExpandedLogIndex(null);
                              setLogPage(1);
                            }}
                            className={`text-[10px] px-1.5 py-0.5 rounded font-mono font-medium transition-colors ring-1 ${isStatus ? 'uppercase' : ''} ${
                              excluded
                                ? 'bg-red-600/40 text-red-300 ring-red-500/60'
                                : active
                                ? `${isStatus ? levelBadgeColor(value) : 'bg-teal-600/40 text-teal-300'} ring-teal-500/60`
                                : isStatus
                                ? `${levelBadgeColor(value)} ring-transparent opacity-60 hover:opacity-100`
                                : 'bg-slate-700 text-slate-400 ring-transparent hover:bg-slate-600 hover:text-slate-300'
                            }`}
                          >
                            {isStatus ? value : tag}
                          </button>
                        );
                      })}
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>

        {/* Summary */}
        <div className="p-4">
          <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2">
            Summary
          </h2>
          <div className="space-y-1.5">
            <div className="text-sm text-slate-300">
              {allLogs.filter((l) => !(l.tags ?? []).includes('telemetry:true')).length} log{allLogs.filter((l) => !(l.tags ?? []).includes('telemetry:true')).length !== 1 ? 's' : ''} total
            </div>
            {allLogs.some((l) => (l.tags ?? []).includes('telemetry:true')) && (
              <div className="text-sm text-purple-400 flex items-center gap-1.5">
                <span className="inline-flex items-center justify-center w-3.5 h-3.5 rounded-full bg-purple-600 text-white text-[8px] font-bold">T</span>
                {allLogs.filter((l) => (l.tags ?? []).includes('telemetry:true')).length} telemetry log{allLogs.filter((l) => (l.tags ?? []).includes('telemetry:true')).length !== 1 ? 's' : ''}
              </div>
            )}
            <div className="text-sm text-slate-300">
              {allLogAnomalies.length} anomal{allLogAnomalies.length !== 1 ? 'ies' : 'y'} detected
            </div>
          </div>
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 p-6 overflow-y-auto">
        {state.error && (
          <div className="bg-red-900/50 border border-red-700 rounded-lg p-4 mb-6">
            <div className="text-red-400">{state.error}</div>
          </div>
        )}

        {state.connectionState === 'disconnected' && (
          <div className="text-center py-20">
            <div className="text-slate-400 text-lg">Waiting for observer connection...</div>
          </div>
        )}

        {state.connectionState === 'connected' && !state.activeScenario && (
          <div className="text-center py-20">
            <div className="text-slate-400 text-lg">Select a scenario to begin</div>
          </div>
        )}

        {state.connectionState === 'loading' && (
          <div className="text-center py-20">
            <div className="text-blue-400 text-lg">Loading scenario...</div>
          </div>
        )}

        {state.connectionState === 'ready' && (
          <div>
            {/* Log rate + anomaly timeline */}
            <LogRateChart
              logs={allLogs.filter((l) => !(l.tags ?? []).includes('telemetry:true'))}
              anomalies={sortedAnomalies}
              scenarioStart={scenarioStart ?? null}
              scenarioEnd={scenarioEnd ?? null}
              hoveredTimestamp={hoveredLogTimestamp}
              hoveredAnomalyIndex={hoveredAnomalyIndex}
            />

            {/* Detected Anomalies collapsible section */}
            {allLogAnomalies.length > 0 && (
              <div className="mb-6">
                <button
                  onClick={() => setAnomaliesExpanded(!anomaliesExpanded)}
                  className="flex items-center gap-2 text-sm font-medium text-slate-300 hover:text-white mb-3 transition-colors"
                >
                  <span className="text-slate-500">{anomaliesExpanded ? '▼' : '▶'}</span>
                  Detected Anomalies ({sortedAnomalies.length}{sortedAnomalies.length !== allLogAnomalies.length ? ` of ${allLogAnomalies.length}` : ''})
                </button>
                {anomaliesExpanded && (
                  <div className="space-y-1.5">
                    {sortedAnomalies.map((anomaly, idx) => (
                      <LogAnomalyCard
                        key={`${anomaly.detectorName}-${anomaly.timestamp}-${idx}`}
                        anomaly={anomaly}
                        isExpanded={expandedAnomalyIndex === idx}
                        onToggle={() =>
                          setExpandedAnomalyIndex(expandedAnomalyIndex === idx ? null : idx)
                        }
                        onHoverEnter={() => setHoveredAnomalyIndex(idx)}
                        onHoverLeave={() => setHoveredAnomalyIndex(null)}
                      />
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* Raw log entries (regular only) */}
            <div>
              <button
                onClick={() => setLogsExpanded(!logsExpanded)}
                className="flex items-center gap-2 text-sm font-medium text-slate-300 hover:text-white mb-3 transition-colors"
              >
                <span className="text-slate-500">{logsExpanded ? '▼' : '▶'}</span>
                Raw Logs ({regularLogs.length}{regularLogs.length !== allLogs.length - telemetryLogs.length ? ` of ${allLogs.length - telemetryLogs.length}` : ''})
              </button>

              {logsExpanded && (
                allLogs.filter((l) => !(l.tags ?? []).includes('telemetry:true')).length === 0 ? (
                  <div className="text-center py-8 text-slate-500 text-sm">
                    No log entries. Load a scenario with log files or the demo scenario.
                  </div>
                ) : regularLogs.length === 0 ? (
                  <div className="text-center py-8 text-slate-500 text-sm">
                    No logs match the selected levels.
                  </div>
                ) : (
                  <>
                    <div className="overflow-y-auto max-h-[480px] space-y-0.5 pr-1">
                      {regularLogs.slice(0, logPage * LOG_PAGE_SIZE).map((entry, idx) => (
                        <LogEntryRow
                          key={`${entry.timestamp}-${idx}`}
                          entry={entry}
                          isExpanded={expandedLogIndex === idx}
                          onToggle={() => setExpandedLogIndex(expandedLogIndex === idx ? null : idx)}
                          onHoverEnter={() => setHoveredLogTimestamp(entry.timestamp)}
                          onHoverLeave={() => setHoveredLogTimestamp(null)}
                        />
                      ))}
                    </div>
                    {regularLogs.length > logPage * LOG_PAGE_SIZE && (
                      <button
                        onClick={() => setLogPage((p) => p + 1)}
                        className="mt-2 w-full py-1.5 text-xs text-slate-400 hover:text-slate-200 bg-slate-700/40 hover:bg-slate-700/70 rounded transition-colors"
                      >
                        Show more ({regularLogs.length - logPage * LOG_PAGE_SIZE} remaining)
                      </button>
                    )}
                  </>
                )
              )}
            </div>

            {/* Telemetry log entries */}
            {(allLogs.some((l) => (l.tags ?? []).includes('telemetry:true')) || telemetryLogs.length > 0) && (
              <div>
                <div className="flex items-center gap-3 mb-3">
                  <div className="flex-1 border-t border-purple-800/50" />
                  <button
                    onClick={() => setTelemetryLogsExpanded(!telemetryLogsExpanded)}
                    className="flex items-center gap-1.5 text-xs text-purple-400 font-medium hover:text-purple-300 transition-colors"
                  >
                    <span className="text-purple-600">{telemetryLogsExpanded ? '▼' : '▶'}</span>
                    <span className="inline-flex items-center justify-center w-4 h-4 rounded-full bg-purple-600 text-white text-[9px] font-bold">T</span>
                    Telemetry Logs ({telemetryLogs.length})
                  </button>
                  <div className="flex-1 border-t border-purple-800/50" />
                </div>

                {telemetryLogsExpanded && (
                  telemetryLogs.length === 0 ? (
                    <div className="text-center py-8 text-slate-500 text-sm">
                      No telemetry logs match the current filters.
                    </div>
                  ) : (
                    <>
                      <div className="overflow-y-auto max-h-[480px] space-y-0.5 pr-1">
                        {telemetryLogs.slice(0, telemetryLogPage * LOG_PAGE_SIZE).map((entry, idx) => (
                          <LogEntryRow
                            key={`telem-${entry.timestamp}-${idx}`}
                            entry={entry}
                            isExpanded={expandedTelemetryLogIndex === idx}
                            onToggle={() => setExpandedTelemetryLogIndex(expandedTelemetryLogIndex === idx ? null : idx)}
                            isTelemetry
                            onHoverEnter={() => setHoveredLogTimestamp(entry.timestamp)}
                            onHoverLeave={() => setHoveredLogTimestamp(null)}
                          />
                        ))}
                      </div>
                      {telemetryLogs.length > telemetryLogPage * LOG_PAGE_SIZE && (
                        <button
                          onClick={() => setTelemetryLogPage((p) => p + 1)}
                          className="mt-2 w-full py-1.5 text-xs text-purple-400 hover:text-purple-200 bg-purple-900/20 hover:bg-purple-900/40 rounded transition-colors"
                        >
                          Show more ({telemetryLogs.length - telemetryLogPage * LOG_PAGE_SIZE} remaining)
                        </button>
                      )}
                    </>
                  )
                )}
              </div>
            )}
          </div>
        )}
      </main>
    </div>
  );
}

function ScenarioSelector({
  scenarios,
  activeScenario,
  onLoadScenario,
}: {
  scenarios: ScenarioInfo[];
  activeScenario: string | null;
  onLoadScenario: (name: string) => Promise<void>;
}) {
  return (
    <div className="p-4 border-b border-slate-700">
      <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">
        Scenarios
      </h2>
      <div className="space-y-1">
        {scenarios.length === 0 ? (
          <div className="text-sm text-slate-500">No scenarios found</div>
        ) : (
          scenarios.map((scenario) => (
            <button
              key={scenario.name}
              onClick={() => onLoadScenario(scenario.name)}
              className={`w-full text-left px-3 py-2 rounded text-sm transition-colors ${
                activeScenario === scenario.name
                  ? 'bg-purple-600 text-white'
                  : 'text-slate-300 hover:bg-slate-700'
              }`}
            >
              <div className="font-medium">{scenario.name}</div>
              <div className="text-xs text-slate-400 mt-0.5">
                {[
                  scenario.hasParquet && 'parquet',
                  scenario.hasLogs && 'logs',
                  scenario.hasEvents && 'events',
                ]
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
