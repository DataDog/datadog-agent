import { useMemo, useRef, useEffect, useState } from 'react';
import { CorrelatorSection } from './CorrelatorSection';
import { AnomalySwimlane } from './AnomalySwimlane';
import { CompressedGroupCard } from './CompressedGroupCard';
import type { ObserverState, ObserverActions } from '../hooks/useObserver';
import type { ScenarioInfo, Correlation } from '../api/client';
import type { TimeRange } from './ChartWithAnomalyDetails';

function parseTagFilter(input: string): Map<string, Set<string>> {
  const byKey = new Map<string, Set<string>>();
  for (const token of input.trim().split(/\s+/)) {
    const sep = token.indexOf(':');
    if (sep <= 0 || sep === token.length - 1) continue;
    const key = token.slice(0, sep);
    if (!byKey.has(key)) byKey.set(key, new Set());
    byKey.get(key)!.add(token);
  }
  return byKey;
}

function extractTagGroups(tagLists: string[][]): Map<string, string[]> {
  const groups = new Map<string, Set<string>>();
  for (const tags of tagLists) {
    for (const tag of tags ?? []) {
      const sep = tag.indexOf(':');
      if (sep === -1) continue;
      const key = tag.slice(0, sep);
      if (!groups.has(key)) groups.set(key, new Set());
      groups.get(key)!.add(tag);
    }
  }
  return new Map([...groups.entries()].map(([k, v]) => [k, [...v].sort()]));
}

function toggleTagInInput(input: string, tag: string): string {
  const tokens = input.trim().split(/\s+/).filter(Boolean);
  const idx = tokens.indexOf(tag);
  if (idx >= 0) tokens.splice(idx, 1);
  else tokens.push(tag);
  return tokens.join(' ');
}

function matchesTags(tags: string[], byKey: Map<string, Set<string>>): boolean {
  if (byKey.size === 0) return true;
  const tagSet = new Set(tags);
  for (const [, tagValues] of byKey) {
    if (![...tagValues].some((t) => tagSet.has(t))) return false;
  }
  return true;
}

interface CorrelatorViewProps {
  state: ObserverState;
  actions: ObserverActions;
  sidebarWidth: number;
  timeRange: TimeRange | null;
}

export function CorrelatorView({ state, actions, sidebarWidth, timeRange }: CorrelatorViewProps) {
  const scenarios = state.scenarios ?? [];
  const components = state.components ?? [];
  const allCorrelations = state.correlations ?? [];
  const compressedGroups = state.compressedGroups ?? [];
  const allAnomalies = state.anomalies ?? [];
  const correlatorStats = state.correlatorStats;

  const [showRawData, setShowRawData] = useState(false);
  const [tagFilterInput, setTagFilterInput] = useState('');
  const initializedScenarioRef = useRef<string | null>(null);

  useEffect(() => {
    if (state.activeScenario && initializedScenarioRef.current !== state.activeScenario) {
      initializedScenarioRef.current = state.activeScenario;
      setTagFilterInput('');
    }
  }, [state.activeScenario]);

  const correlatorComponents = useMemo(
    () => components.filter((c) => c.category === 'correlator'),
    [components]
  );

  const tagGroups = useMemo(
    () => extractTagGroups(allAnomalies.map((a) => a.tags ?? [])),
    [allAnomalies]
  );

  const anomalies = useMemo(() => {
    const byKey = parseTagFilter(tagFilterInput);
    if (byKey.size === 0) return allAnomalies;
    return allAnomalies.filter((a) => matchesTags(a.tags ?? [], byKey));
  }, [allAnomalies, tagFilterInput]);

  const correlations = useMemo(() => {
    const byKey = parseTagFilter(tagFilterInput);
    if (byKey.size === 0) return allCorrelations;
    return allCorrelations.filter((c) =>
      c.anomalies.some((a) => matchesTags(a.tags, byKey))
    );
  }, [allCorrelations, tagFilterInput]);

  return (
    <div className="flex-1 flex">
      {/* Sidebar */}
      <aside
        className="bg-slate-800 border-r border-slate-700 flex flex-col"
        style={{ width: sidebarWidth }}
      >
        {/* Scenarios */}
        <ScenarioSelector
          scenarios={scenarios}
          activeScenario={state.activeScenario}
          onLoadScenario={actions.loadScenario}
        />

        {/* Correlator toggles */}
        <div className="p-4 border-b border-slate-700">
          <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">
            Correlators
          </h2>
          <div className="space-y-1">
            {correlatorComponents.map((comp) => {
              const stats = correlatorStats?.[comp.name];
              return (
                <div key={comp.name} className="flex items-center gap-2 px-2 py-1.5">
                  <button
                    onClick={() => actions.toggleComponent(comp.name)}
                    className={`relative inline-flex h-4 w-7 items-center rounded-full transition-colors flex-shrink-0 ${
                      comp.enabled ? 'bg-purple-600' : 'bg-slate-600'
                    }`}
                  >
                    <span
                      className={`inline-block h-3 w-3 transform rounded-full bg-white transition-transform ${
                        comp.enabled ? 'translate-x-3.5' : 'translate-x-0.5'
                      }`}
                    />
                  </button>
                  <span className="text-sm text-slate-300 flex-1">{comp.displayName}</span>
                  {stats && (
                    <StatsLabel stats={stats} />
                  )}
                </div>
              );
            })}
          </div>
        </div>

        {/* Analysis settings */}
        <div className="p-4 border-b border-slate-700">
          <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">
            Settings
          </h2>
          <div className="space-y-1 text-xs">
            <div className="flex items-center gap-2 px-2 py-1">
              <div className={`w-2 h-2 rounded-full ${state.status?.serverConfig?.cusumSkipCount ? 'bg-amber-500' : 'bg-slate-600'}`} />
              <span className="text-slate-300">:count metrics</span>
              <span className="text-slate-500 ml-auto">
                {state.status?.serverConfig?.cusumSkipCount ? 'filtered' : 'included'}
              </span>
            </div>
          </div>
        </div>

        {/* Tag filter */}
        <div className="p-4 border-b border-slate-700">
          <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2">
            Tag Filter
          </h2>
          <div className="relative mb-2">
            <input
              type="text"
              value={tagFilterInput}
              onChange={(e) => setTagFilterInput(e.target.value)}
              placeholder="host:web-1 service:api"
              className="w-full bg-slate-700 text-slate-200 text-xs rounded px-2 py-1.5 placeholder-slate-500 focus:outline-none focus:ring-1 focus:ring-purple-500 font-mono pr-6"
            />
            {tagFilterInput && (
              <button
                onClick={() => setTagFilterInput('')}
                className="absolute right-1.5 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-300"
              >
                ×
              </button>
            )}
          </div>
          {tagGroups.size > 0 && (
            <div className="space-y-2">
              {[...tagGroups.entries()].map(([key, tags]) => {
                const activeTags = parseTagFilter(tagFilterInput);
                return (
                  <div key={key}>
                    <div className="text-[10px] text-slate-500 mb-1">{key}</div>
                    <div className="flex flex-wrap gap-1">
                      {tags.map((tag) => {
                        const active = activeTags.get(key)?.has(tag) ?? false;
                        return (
                          <button
                            key={tag}
                            onClick={() => setTagFilterInput(toggleTagInInput(tagFilterInput, tag))}
                            className={`text-[10px] px-1.5 py-0.5 rounded font-mono transition-colors ${
                              active
                                ? 'bg-purple-600/40 text-purple-300 ring-1 ring-purple-500/60'
                                : 'bg-slate-700 text-slate-400 hover:bg-slate-600 hover:text-slate-300'
                            }`}
                          >
                            {tag}
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
      </aside>

      {/* Main content */}
      <main className="flex-1 p-6 overflow-y-auto">
        {state.connectionState !== 'ready' && (
          <EmptyState connectionState={state.connectionState} activeScenario={state.activeScenario} error={state.error} />
        )}

        {state.connectionState === 'ready' && (
          <div className="space-y-4">
            {/* 1. Anomaly Swimlane - always visible */}
            <AnomalySwimlane
              anomalies={anomalies}
              compressedGroups={compressedGroups}
              correlations={correlations}
              timeRange={timeRange}
            />

            {/* 2. Compressed Groups */}
            {compressedGroups.length > 0 && (
              <div>
                <h2 className="text-sm font-semibold text-slate-300 mb-3">
                  Compressed Groups ({compressedGroups.length})
                </h2>
                <div className="grid grid-cols-1 lg:grid-cols-2 gap-3">
                  {compressedGroups.map((group) => (
                    <CompressedGroupCard
                      key={group.groupId}
                      group={group}
                    />
                  ))}
                </div>
              </div>
            )}

            {/* 3. Raw Correlator Data - collapsed by default */}
            <div>
              <button
                onClick={() => setShowRawData(!showRawData)}
                className="flex items-center gap-2 text-xs text-slate-500 hover:text-slate-400 mb-2"
              >
                <span>{showRawData ? '▼' : '▶'}</span>
                <span>Raw Correlator Data</span>
              </button>

              {showRawData && (
                <div className="space-y-4">
                  {/* Time Cluster Correlations */}
                  {correlations.length > 0 && (
                    <TimeClusterSection correlations={correlations} />
                  )}

                  {/* Dynamic correlator sections */}
                  {correlatorComponents.map((comp) => {
                    const corrData = state.correlatorData.get(comp.name);
                    return (
                      <CorrelatorSection
                        key={comp.name}
                        name={comp.name}
                        displayName={comp.displayName}
                        enabled={comp.enabled}
                        data={corrData?.data ?? null}
                        onToggle={() => actions.toggleComponent(comp.name)}
                      />
                    );
                  })}
                </div>
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

function StatsLabel({ stats }: { stats: Record<string, unknown> }) {
  // Show the most interesting stat as a label
  const entries = Object.entries(stats).filter(([k, v]) =>
    typeof v === 'number' && k !== 'enabled' && v > 0
  );
  if (entries.length === 0) return null;
  // Prefer known display fields
  const preferred = ['clusterCount', 'edgeCount', 'pairCount'];
  const best = preferred.find((k) => stats[k] && (stats[k] as number) > 0);
  const [key, value] = best ? [best, stats[best]] : entries[0];
  const label = key.replace(/Count$/, 's').replace(/([A-Z])/g, ' $1').trim().toLowerCase();
  return (
    <span className="text-xs text-slate-500">{value as number} {label}</span>
  );
}

function EmptyState({
  connectionState,
  activeScenario,
  error,
}: {
  connectionState: string;
  activeScenario: string | null;
  error: string | null;
}) {
  return (
    <>
      {error && (
        <div className="bg-red-900/50 border border-red-700 rounded-lg p-4 mb-6">
          <div className="text-red-400">{error}</div>
        </div>
      )}
      {connectionState === 'disconnected' && (
        <div className="text-center py-20">
          <div className="text-slate-400 text-lg">Waiting for observer connection...</div>
          <div className="text-slate-500 mt-2">
            Start the observer: <code className="bg-slate-800 px-2 py-1 rounded">./bin/observer-testbench</code>
          </div>
        </div>
      )}
      {connectionState === 'connected' && !activeScenario && (
        <div className="text-center py-20">
          <div className="text-slate-400 text-lg">Select a scenario to begin</div>
        </div>
      )}
      {connectionState === 'loading' && (
        <div className="text-center py-20">
          <div className="text-blue-400 text-lg">Loading scenario...</div>
        </div>
      )}
    </>
  );
}

function TimeClusterSection({ correlations }: { correlations: Correlation[] }) {
  return (
    <div className="bg-slate-800 rounded-lg p-4">
      <h2 className="text-sm font-semibold text-slate-300 mb-3">
        Time Clusters ({correlations.length})
      </h2>
      <div className="space-y-2">
        {correlations.map((c, i) => (
          <div key={i} className="bg-slate-700/50 rounded p-3">
            <div className="flex items-center justify-between">
              <div className="font-medium text-purple-400 text-sm">{c.title}</div>
              <span className="text-xs text-slate-500">
                {c.memberSeriesIds.length} series
              </span>
            </div>
            <div className="text-sm text-slate-400 mt-1">
              Pattern: {c.pattern}
            </div>
            <div className="flex flex-wrap gap-1 mt-2">
              {c.metricNames.map((metricName, j) => (
                <span
                  key={j}
                  className="text-xs px-2 py-0.5 bg-slate-600/50 rounded text-slate-300"
                >
                  {metricName}
                </span>
              ))}
            </div>
            {c.anomalies.length > 0 && (
              <div className="mt-2 space-y-1">
                {c.anomalies.map((a, j) => (
                  <div key={j} className="flex items-start gap-2 text-xs">
                    <span className="text-slate-500 font-mono w-20 flex-shrink-0 text-right pt-0.5">
                      {new Date(a.timestamp * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })}
                    </span>
                    <span className="text-slate-300 flex-1">{a.title}</span>
                    {a.tags && a.tags.length > 0 && (
                      <div className="flex gap-1 flex-wrap flex-shrink-0">
                        {a.tags.map((tag) => (
                          <span key={tag} className="px-1 py-0.5 rounded text-[9px] bg-slate-600/50 text-slate-400 font-mono">
                            {tag}
                          </span>
                        ))}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
            <div className="text-xs text-slate-500 mt-2">
              {new Date(c.firstSeen * 1000).toLocaleTimeString()} -{' '}
              {new Date(c.lastUpdated * 1000).toLocaleTimeString()}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
