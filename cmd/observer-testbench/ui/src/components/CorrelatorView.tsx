import { useMemo, useState } from 'react';
import { CorrelatorSection } from './CorrelatorSection';
import { AnomalySwimlane } from './AnomalySwimlane';
import { CompressedGroupCard } from './CompressedGroupCard';
import type { ObserverState, ObserverActions } from '../hooks/useObserver';
import type { ScenarioInfo, Correlation } from '../api/client';

interface CorrelatorViewProps {
  state: ObserverState;
  actions: ObserverActions;
  sidebarWidth: number;
}

export function CorrelatorView({ state, actions, sidebarWidth }: CorrelatorViewProps) {
  const scenarios = state.scenarios ?? [];
  const components = state.components ?? [];
  const correlations = state.correlations ?? [];
  const compressedGroups = state.compressedGroups ?? [];
  const anomalies = state.anomalies ?? [];
  const correlatorStats = state.correlatorStats;

  const [showRawData, setShowRawData] = useState(false);

  const correlatorComponents = useMemo(
    () => components.filter((c) => c.category === 'correlator'),
    [components]
  );

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
          <div
            key={i}
            className="bg-slate-700/50 rounded p-3"
          >
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
