import { useState, useMemo, useRef, useEffect } from 'react';
import type { ScenarioInfo, LogAnomaly } from '../api/client';
import type { ObserverState, ObserverActions } from '../hooks/useObserver';

interface LogAnomalyViewProps {
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

function formatDate(ts: number): string {
  return new Date(ts * 1000).toLocaleDateString([], {
    month: 'short',
    day: 'numeric',
  });
}

function scoreColor(score: number): string {
  if (score >= 0.9) return 'text-red-400 bg-red-900/40';
  if (score >= 0.7) return 'text-orange-400 bg-orange-900/40';
  if (score >= 0.5) return 'text-yellow-400 bg-yellow-900/40';
  return 'text-slate-400 bg-slate-700/40';
}

function processorBadgeColor(name: string): string {
  // Simple deterministic color from processor name
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
          {/* Timestamp */}
          <div className="flex-shrink-0 text-xs text-slate-500 font-mono pt-0.5 w-20 text-right">
            {formatTimestamp(anomaly.timestamp)}
          </div>

          {/* Content */}
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

      {/* Expanded details */}
      {isExpanded && (
        <div className="px-4 pb-3 border-t border-slate-600/50">
          {/* Full description */}
          <div className="mt-2 mb-2">
            <div className="text-xs text-slate-400 font-medium mb-1">Log Content</div>
            <pre className="text-xs text-slate-300 bg-slate-800/60 rounded p-2 whitespace-pre-wrap break-all font-mono leading-relaxed max-h-40 overflow-y-auto">
              {anomaly.description}
            </pre>
          </div>

          {/* Tags */}
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

          {/* Source */}
          <div className="mt-2 text-xs text-slate-500">
            Source: <span className="text-slate-400">{anomaly.source}</span>
          </div>
        </div>
      )}
    </div>
  );
}

// Mini timeline bar showing anomaly density over time
function AnomalyTimeline({
  anomalies,
  scenarioStart,
  scenarioEnd,
}: {
  anomalies: LogAnomaly[];
  scenarioStart: number | null;
  scenarioEnd: number | null;
}) {
  if (anomalies.length === 0 || !scenarioStart || !scenarioEnd || scenarioEnd <= scenarioStart) {
    return null;
  }

  const bucketCount = 60;
  const bucketSize = (scenarioEnd - scenarioStart) / bucketCount;
  const buckets = new Array(bucketCount).fill(0);

  for (const a of anomalies) {
    const idx = Math.min(
      Math.floor((a.timestamp - scenarioStart) / bucketSize),
      bucketCount - 1
    );
    if (idx >= 0) buckets[idx]++;
  }

  const maxCount = Math.max(1, ...buckets);

  return (
    <div className="bg-slate-800/60 rounded p-3 mb-4">
      <div className="text-xs text-slate-400 mb-1.5">
        Timeline ({anomalies.length} anomalies)
      </div>
      <div className="flex items-end gap-px h-8">
        {buckets.map((count, i) => (
          <div
            key={i}
            className={`flex-1 rounded-sm transition-colors ${count > 0 ? 'bg-orange-500/70' : 'bg-slate-700/40'}`}
            style={{ height: count > 0 ? `${Math.max(4, (count / maxCount) * 32)}px` : '2px' }}
            title={count > 0 ? `${count} anomaly${count > 1 ? 'ies' : 'y'}` : undefined}
          />
        ))}
      </div>
      <div className="flex justify-between text-xs text-slate-600 mt-1">
        <span>{formatTimestamp(scenarioStart)}</span>
        <span>{formatTimestamp(scenarioEnd)}</span>
      </div>
    </div>
  );
}

export function LogAnomalyView({ state, actions, sidebarWidth }: LogAnomalyViewProps) {
  const scenarios = state.scenarios ?? [];
  const allLogAnomalies = state.logAnomalies ?? [];

  const [enabledProcessors, setEnabledProcessors] = useState<Set<string>>(new Set());
  const [expandedIndex, setExpandedIndex] = useState<number | null>(null);
  const initializedScenarioRef = useRef<string | null>(null);

  // Derive all unique processor names from the anomalies
  const processorNames = useMemo(() => {
    const names = new Set<string>();
    for (const a of allLogAnomalies) names.add(a.processorName);
    return Array.from(names).sort();
  }, [allLogAnomalies]);

  // Auto-enable all processors when a new scenario is loaded
  useEffect(() => {
    if (processorNames.length > 0 && state.activeScenario && initializedScenarioRef.current !== state.activeScenario) {
      initializedScenarioRef.current = state.activeScenario;
      setEnabledProcessors(new Set(processorNames));
      setExpandedIndex(null);
    }
  }, [processorNames, state.activeScenario]);

  // Filter anomalies by enabled processors, sorted by timestamp
  const filteredAnomalies = useMemo(() => {
    return allLogAnomalies
      .filter((a) => enabledProcessors.has(a.processorName))
      .sort((a, b) => a.timestamp - b.timestamp);
  }, [allLogAnomalies, enabledProcessors]);

  const countByProcessor = useMemo(() => {
    const counts = new Map<string, number>();
    for (const a of allLogAnomalies) {
      counts.set(a.processorName, (counts.get(a.processorName) ?? 0) + 1);
    }
    return counts;
  }, [allLogAnomalies]);

  const toggleProcessor = (name: string) => {
    const next = new Set(enabledProcessors);
    if (next.has(name)) {
      next.delete(name);
    } else {
      next.add(name);
    }
    setEnabledProcessors(next);
    setExpandedIndex(null);
  };

  const scenarioStart = state.status?.scenarioStart ?? null;
  const scenarioEnd = state.status?.scenarioEnd ?? null;

  // Group anomalies by date for timeline headers
  const groupedByDate = useMemo(() => {
    const groups: { date: string; ts: number; anomalies: { anomaly: LogAnomaly; idx: number }[] }[] = [];
    let currentDate = '';
    let currentGroup: { date: string; ts: number; anomalies: { anomaly: LogAnomaly; idx: number }[] } | null = null;

    filteredAnomalies.forEach((anomaly, idx) => {
      const date = formatDate(anomaly.timestamp);
      if (date !== currentDate) {
        currentDate = date;
        currentGroup = { date, ts: anomaly.timestamp, anomalies: [] };
        groups.push(currentGroup);
      }
      currentGroup!.anomalies.push({ anomaly, idx });
    });

    return groups;
  }, [filteredAnomalies]);

  return (
    <div className="flex-1 flex">
      {/* Sidebar */}
      <aside
        className="bg-slate-800 border-r border-slate-700 flex flex-col"
        style={{ width: sidebarWidth }}
      >
        {/* Scenario selector */}
        <ScenarioSelector
          scenarios={scenarios}
          activeScenario={state.activeScenario}
          onLoadScenario={actions.loadScenario}
        />

        {/* Processor filter */}
        <div className="p-4 border-b border-slate-700">
          <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">
            Log Processors
          </h2>
          {processorNames.length === 0 ? (
            <div className="text-sm text-slate-500">No log anomalies detected</div>
          ) : (
            <div className="space-y-1">
              {processorNames.map((name) => {
                const count = countByProcessor.get(name) ?? 0;
                return (
                  <label
                    key={name}
                    className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-slate-700 cursor-pointer"
                  >
                    <input
                      type="checkbox"
                      checked={enabledProcessors.has(name)}
                      onChange={() => toggleProcessor(name)}
                      className="rounded border-slate-600 bg-slate-700 text-orange-500 focus:ring-orange-500"
                    />
                    <span className="text-sm text-slate-300 flex-1 truncate">{name}</span>
                    {count > 0 && (
                      <span className="text-xs text-slate-500 flex-shrink-0">{count}</span>
                    )}
                  </label>
                );
              })}
            </div>
          )}
        </div>

        {/* Summary */}
        {filteredAnomalies.length > 0 && (
          <div className="p-4 border-b border-slate-700">
            <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2">
              Summary
            </h2>
            <div className="text-sm text-slate-300">
              {filteredAnomalies.length} anomal{filteredAnomalies.length === 1 ? 'y' : 'ies'}
            </div>
            {/* Score distribution */}
            {(() => {
              const withScore = filteredAnomalies.filter((a) => a.score !== undefined);
              if (withScore.length === 0) return null;
              const critical = withScore.filter((a) => (a.score ?? 0) >= 0.9).length;
              const high = withScore.filter((a) => (a.score ?? 0) >= 0.7 && (a.score ?? 0) < 0.9).length;
              const other = withScore.length - critical - high;
              return (
                <div className="mt-2 space-y-1">
                  {critical > 0 && (
                    <div className="flex items-center gap-2">
                      <span className="w-2 h-2 rounded-full bg-red-500 flex-shrink-0" />
                      <span className="text-xs text-slate-400">{critical} critical (≥0.9)</span>
                    </div>
                  )}
                  {high > 0 && (
                    <div className="flex items-center gap-2">
                      <span className="w-2 h-2 rounded-full bg-orange-500 flex-shrink-0" />
                      <span className="text-xs text-slate-400">{high} high (≥0.7)</span>
                    </div>
                  )}
                  {other > 0 && (
                    <div className="flex items-center gap-2">
                      <span className="w-2 h-2 rounded-full bg-slate-500 flex-shrink-0" />
                      <span className="text-xs text-slate-400">{other} other</span>
                    </div>
                  )}
                </div>
              );
            })()}
          </div>
        )}
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
            {allLogAnomalies.length === 0 ? (
              <div className="text-center py-20">
                <div className="text-slate-400 text-lg">No log anomalies detected</div>
                <div className="text-slate-500 mt-2 text-sm">
                  Log anomalies are emitted directly by log processors when critical patterns are detected.
                  Load a scenario with log files to see results.
                </div>
              </div>
            ) : (
              <>
                {/* Timeline overview */}
                <AnomalyTimeline
                  anomalies={filteredAnomalies}
                  scenarioStart={scenarioStart ?? null}
                  scenarioEnd={scenarioEnd ?? null}
                />

                {/* Event list */}
                {filteredAnomalies.length === 0 ? (
                  <div className="text-center py-10 text-slate-500">
                    No anomalies match the selected processors
                  </div>
                ) : (
                  <div className="space-y-4">
                    {groupedByDate.map((group) => (
                      <div key={group.date}>
                        <div className="text-xs text-slate-500 font-medium mb-2 flex items-center gap-2">
                          <span className="text-slate-400">{group.date}</span>
                          <span className="h-px flex-1 bg-slate-700" />
                          <span>{group.anomalies.length} event{group.anomalies.length !== 1 ? 's' : ''}</span>
                        </div>
                        <div className="space-y-1.5">
                          {group.anomalies.map(({ anomaly, idx }) => (
                            <LogAnomalyCard
                              key={`${anomaly.processorName}-${anomaly.timestamp}-${idx}`}
                              anomaly={anomaly}
                              isExpanded={expandedIndex === idx}
                              onToggle={() => setExpandedIndex(expandedIndex === idx ? null : idx)}
                            />
                          ))}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </>
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
