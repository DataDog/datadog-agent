import { useState, useMemo, useRef, useEffect } from 'react';
import type { ScenarioInfo, LogAnomaly, LogEntry } from '../api/client';
import type { ObserverState, ObserverActions } from '../hooks/useObserver';

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

function processorBadgeColor(name: string): string {
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

function processorLineColor(name: string): string {
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
}: {
  logs: LogEntry[];
  anomalies: LogAnomaly[];
  scenarioStart: number | null;
  scenarioEnd: number | null;
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

  // Derive unique processors for the legend
  const processors = Array.from(new Set(anomalies.map((a) => a.processorName)));

  return (
    <div className="bg-slate-800/60 rounded p-3 mb-4">
      <div className="flex items-center gap-4 text-xs text-slate-400 mb-1.5">
        <span>Log rate ({logs.length} log{logs.length !== 1 ? 's' : ''} total)</span>
        {processors.map((name) => (
          <span key={name} className="flex items-center gap-1">
            <span className="inline-block w-2 h-2 rounded-sm" style={{ backgroundColor: processorLineColor(name) }} />
            <span style={{ color: processorLineColor(name) }}>{name}</span>
          </span>
        ))}
      </div>
      <div className="relative h-10">
        {/* Log rate bars — low alpha background */}
        <div className="absolute inset-0 flex items-end gap-px">
          {logBuckets.map((count, i) => (
            <div
              key={i}
              className={`flex-1 rounded-sm ${count > 0 ? 'bg-teal-500/25' : 'bg-slate-700/20'}`}
              style={{ height: count > 0 ? `${Math.max(3, (count / maxLog) * 40)}px` : '2px' }}
              title={count > 0 ? `${count} log${count > 1 ? 's' : ''}` : undefined}
            />
          ))}
        </div>
        {/* Anomaly lines — one vertical line per anomaly, colored by processor */}
        {anomalies.map((a, i) => {
          const pct = ((a.timestamp - scenarioStart) / (scenarioEnd - scenarioStart)) * 100;
          if (pct < 0 || pct > 100) return null;
          const color = processorLineColor(a.processorName);
          return (
            <div
              key={i}
              className="absolute top-0 bottom-0"
              style={{ left: `${pct}%` }}
              title={`${a.processorName}: ${a.title}`}
            >
              {/* Downward triangle at the top of the line */}
              <div style={{
                position: 'absolute',
                top: 0,
                left: '-3px',
                width: 0,
                height: 0,
                borderLeft: '3px solid transparent',
                borderRight: '3px solid transparent',
                borderTop: `5px solid ${color}`,
              }} />
              {/* Vertical line */}
              <div className="absolute top-0 bottom-0 w-px" style={{ backgroundColor: color }} />
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
}

function LogEntryRow({ entry, isExpanded, onToggle }: LogEntryRowProps) {
  const contentPreview = entry.content.length > 120 && !isExpanded
    ? entry.content.slice(0, 120) + '…'
    : entry.content;

  return (
    <div className="bg-slate-700/30 rounded overflow-hidden">
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
          <span className="text-xs text-slate-300 font-mono leading-relaxed break-all">
            {contentPreview}
          </span>
          {entry.content.length > 120 && (
            <span className="text-slate-500 flex-shrink-0 text-xs ml-auto">{isExpanded ? '▼' : '▶'}</span>
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
}

function LogAnomalyCard({ anomaly, isExpanded, onToggle }: LogAnomalyCardProps) {
  return (
    <div className="bg-slate-700/50 rounded overflow-hidden">
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
              <span className={`text-xs px-1.5 py-0.5 rounded font-medium ${processorBadgeColor(anomaly.processorName)}`}>
                {anomaly.processorName}
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

const ALL_LEVELS = ['error', 'warn', 'info', 'debug'];
const LOG_PAGE_SIZE = 50;

export function LogView({ state, actions, sidebarWidth }: LogViewProps) {
  const scenarios = state.scenarios ?? [];
  const allLogs = state.logs ?? [];
  const allLogAnomalies = state.logAnomalies ?? [];

  const [enabledLevels, setEnabledLevels] = useState<Set<string>>(new Set(ALL_LEVELS));
  const [expandedLogIndex, setExpandedLogIndex] = useState<number | null>(null);
  const [expandedAnomalyIndex, setExpandedAnomalyIndex] = useState<number | null>(null);
  const [anomaliesExpanded, setAnomaliesExpanded] = useState(true);
  const [logsExpanded, setLogsExpanded] = useState(true);
  const [logPage, setLogPage] = useState(1);
  const initializedScenarioRef = useRef<string | null>(null);

  // Reset state when scenario changes
  useEffect(() => {
    if (state.activeScenario && initializedScenarioRef.current !== state.activeScenario) {
      initializedScenarioRef.current = state.activeScenario;
      setEnabledLevels(new Set(ALL_LEVELS));
      setExpandedLogIndex(null);
      setExpandedAnomalyIndex(null);
      setLogPage(1);
    }
  }, [state.activeScenario]);

  const filteredLogs = useMemo(() => {
    return allLogs
      .filter((l) => enabledLevels.has(l.status.toLowerCase()))
      .sort((a, b) => a.timestamp - b.timestamp);
  }, [allLogs, enabledLevels]);

  const countByLevel = useMemo(() => {
    const counts = new Map<string, number>();
    for (const l of allLogs) {
      const lvl = l.status.toLowerCase();
      counts.set(lvl, (counts.get(lvl) ?? 0) + 1);
    }
    return counts;
  }, [allLogs]);

  const sortedAnomalies = useMemo(
    () => [...allLogAnomalies].sort((a, b) => a.timestamp - b.timestamp),
    [allLogAnomalies]
  );

  const toggleLevel = (level: string) => {
    const next = new Set(enabledLevels);
    if (next.has(level)) next.delete(level);
    else next.add(level);
    setEnabledLevels(next);
    setExpandedLogIndex(null);
    setLogPage(1);
  };

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

        {/* Level filter */}
        <div className="p-4 border-b border-slate-700">
          <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">
            Log Level
          </h2>
          <div className="space-y-1">
            {ALL_LEVELS.map((level) => {
              const count = countByLevel.get(level) ?? 0;
              return (
                <label
                  key={level}
                  className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-slate-700 cursor-pointer"
                >
                  <input
                    type="checkbox"
                    checked={enabledLevels.has(level)}
                    onChange={() => toggleLevel(level)}
                    className="rounded border-slate-600 bg-slate-700 text-teal-500 focus:ring-teal-500"
                  />
                  <span className={`text-xs px-1.5 py-0.5 rounded font-medium uppercase ${levelBadgeColor(level)}`}>
                    {level}
                  </span>
                  {count > 0 && (
                    <span className="text-xs text-slate-500 flex-shrink-0 ml-auto">{count}</span>
                  )}
                </label>
              );
            })}
          </div>
        </div>

        {/* Summary */}
        <div className="p-4">
          <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2">
            Summary
          </h2>
          <div className="space-y-1.5">
            <div className="text-sm text-slate-300">
              {allLogs.length} log{allLogs.length !== 1 ? 's' : ''} total
            </div>
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
              logs={allLogs}
              anomalies={sortedAnomalies}
              scenarioStart={scenarioStart ?? null}
              scenarioEnd={scenarioEnd ?? null}
            />

            {/* Detected Anomalies collapsible section */}
            {allLogAnomalies.length > 0 && (
              <div className="mb-6">
                <button
                  onClick={() => setAnomaliesExpanded(!anomaliesExpanded)}
                  className="flex items-center gap-2 text-sm font-medium text-slate-300 hover:text-white mb-3 transition-colors"
                >
                  <span className="text-slate-500">{anomaliesExpanded ? '▼' : '▶'}</span>
                  Detected Anomalies ({allLogAnomalies.length})
                </button>
                {anomaliesExpanded && (
                  <div className="space-y-1.5">
                    {sortedAnomalies.map((anomaly, idx) => (
                      <LogAnomalyCard
                        key={`${anomaly.processorName}-${anomaly.timestamp}-${idx}`}
                        anomaly={anomaly}
                        isExpanded={expandedAnomalyIndex === idx}
                        onToggle={() =>
                          setExpandedAnomalyIndex(expandedAnomalyIndex === idx ? null : idx)
                        }
                      />
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* Raw log entries */}
            <div>
              <button
                onClick={() => setLogsExpanded(!logsExpanded)}
                className="flex items-center gap-2 text-sm font-medium text-slate-300 hover:text-white mb-3 transition-colors"
              >
                <span className="text-slate-500">{logsExpanded ? '▼' : '▶'}</span>
                Raw Logs ({filteredLogs.length}{filteredLogs.length !== allLogs.length ? ` of ${allLogs.length}` : ''})
              </button>

              {logsExpanded && (
                allLogs.length === 0 ? (
                  <div className="text-center py-8 text-slate-500 text-sm">
                    No log entries. Load a scenario with log files or the demo scenario.
                  </div>
                ) : filteredLogs.length === 0 ? (
                  <div className="text-center py-8 text-slate-500 text-sm">
                    No logs match the selected levels.
                  </div>
                ) : (
                  <>
                    <div className="overflow-y-auto max-h-[480px] space-y-0.5 pr-1">
                      {filteredLogs.slice(0, logPage * LOG_PAGE_SIZE).map((entry, idx) => (
                        <LogEntryRow
                          key={`${entry.timestamp}-${idx}`}
                          entry={entry}
                          isExpanded={expandedLogIndex === idx}
                          onToggle={() => setExpandedLogIndex(expandedLogIndex === idx ? null : idx)}
                        />
                      ))}
                    </div>
                    {filteredLogs.length > logPage * LOG_PAGE_SIZE && (
                      <button
                        onClick={() => setLogPage((p) => p + 1)}
                        className="mt-2 w-full py-1.5 text-xs text-slate-400 hover:text-slate-200 bg-slate-700/40 hover:bg-slate-700/70 rounded transition-colors"
                      >
                        Show more ({filteredLogs.length - logPage * LOG_PAGE_SIZE} remaining)
                      </button>
                    )}
                  </>
                )
              )}
            </div>
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
