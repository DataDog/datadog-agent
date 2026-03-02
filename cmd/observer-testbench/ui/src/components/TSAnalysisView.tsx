import { useState, useEffect, useMemo, useRef } from 'react';
import { ChartWithAnomalyDetails } from './ChartWithAnomalyDetails';
import { SeriesTree } from './SeriesTree';
import { api } from '../api/client';
import type { SeriesData, SeriesInfo, ScenarioInfo } from '../api/client';
import type { SeriesVariant } from './TimeSeriesChart';
import { getAnalyzerColorStable } from './TimeSeriesChart';
import type { TimeRange } from './ChartWithAnomalyDetails';
import type { ObserverState, ObserverActions } from '../hooks/useObserver';
import { MAIN_TAG_FILTER_KEYS } from '../constants';
import { parseTagFilter, extractTagGroups, toggleTagInInput, matchesTagFilter } from '../filters';

const AGGREGATION_TYPES = ['avg', 'count', 'sum', 'min', 'max'] as const;
type AggregationType = typeof AGGREGATION_TYPES[number];

function getBaseMetricName(name: string): string {
  const match = name.match(/^(.+):(avg|sum|count|min|max)$/);
  return match ? match[1] : name;
}

function getAggregationType(name: string): AggregationType | null {
  const match = name.match(/:(avg|sum|count|min|max)$/);
  return match ? (match[1] as AggregationType) : null;
}

function getAnalyzerComponent(anomaly: { analyzerName: string; analyzerComponent?: string }): string {
  return anomaly.analyzerComponent ?? anomaly.analyzerName;
}

function formatSeriesLabel(tags: string[]): string {
  if (!tags || tags.length === 0) return 'untagged';
  return tags.join(', ');
}


interface MetricGroup {
  key: string;
  namespace: string;
  baseName: string;
  members: SeriesInfo[];
}

interface TSAnalysisViewProps {
  state: ObserverState;
  actions: ObserverActions;
  sidebarWidth: number;
  timeRange: TimeRange | null;
  onTimeRangeChange: (range: TimeRange | null) => void;
  smoothLines: boolean;
}

export function TSAnalysisView({
  state,
  actions,
  sidebarWidth,
  timeRange,
  onTimeRangeChange,
  smoothLines,
}: TSAnalysisViewProps) {
  const [selectedGroups, setSelectedGroups] = useState<Set<string>>(new Set());
  const [enabledAnalyzers, setEnabledAnalyzers] = useState<Set<string>>(new Set());
  const [groupSeriesData, setGroupSeriesData] = useState<Map<string, SeriesData[]>>(new Map());
  const [aggregationType, setAggregationType] = useState<AggregationType>('avg');
  const [showAnomalyOnlyGroups, setShowAnomalyOnlyGroups] = useState(false);
  const [showAnomalyOnlySeriesLines, setShowAnomalyOnlySeriesLines] = useState(false);
  const [tagFilterInput, setTagFilterInput] = useState('');

  const scenarios = state.scenarios ?? [];
  const components = state.components ?? [];
  const allSeries = state.series ?? [];
  const allAnomalies = state.anomalies ?? [];

  const analyzerComponents = useMemo(
    () => components.filter((c) => c.category === 'analyzer'),
    [components]
  );

  const anomalies = useMemo(
    () => allAnomalies.filter((a) => enabledAnalyzers.has(getAnalyzerComponent(a))),
    [allAnomalies, enabledAnalyzers]
  );

  const tsAnalyzerNames = useMemo(
    () => analyzerComponents.map((c) => c.name),
    [analyzerComponents]
  );

  // Map comp.name (analyzerComponent) → analyzerName used by the timeline for coloring
  const analyzerNameByComponent = useMemo(() => {
    const map = new Map<string, string>();
    for (const a of allAnomalies) {
      const component = getAnalyzerComponent(a);
      if (!map.has(component)) map.set(component, a.analyzerName);
    }
    return map;
  }, [allAnomalies]);

  const tagGroups = useMemo(() => {
    const all = extractTagGroups(allSeries.map((s) => s.tags));
    return new Map([...all.entries()].filter(([k]) => MAIN_TAG_FILTER_KEYS.has(k)));
  }, [allSeries]);

  const filteredSeries = useMemo(() => {
    const byAggType = allSeries.filter((s) => getAggregationType(s.name) === aggregationType);
    const filter = parseTagFilter(tagFilterInput);
    if (filter.include.size === 0 && filter.exclude.size === 0) return byAggType;
    return byAggType.filter((s) => matchesTagFilter(s.tags ?? [], filter));
  }, [allSeries, aggregationType, tagFilterInput]);

  const metricGroups = useMemo(() => {
    const groups = new Map<string, MetricGroup>();
    filteredSeries.forEach((s) => {
      const baseName = getBaseMetricName(s.name);
      const key = `${s.namespace}/${baseName}`;
      if (!groups.has(key)) {
        groups.set(key, {
          key,
          namespace: s.namespace,
          baseName,
          members: [],
        });
      }
      groups.get(key)!.members.push(s);
    });

    return Array.from(groups.values()).sort((a, b) => a.baseName.localeCompare(b.baseName));
  }, [filteredSeries]);

  const groupByKey = useMemo(() => {
    const map = new Map<string, MetricGroup>();
    metricGroups.forEach((g) => map.set(g.key, g));
    return map;
  }, [metricGroups]);

  const anomalyCountByGroup = useMemo(() => {
    const counts = new Map<string, number>();
    metricGroups.forEach((group) => {
      const memberIDs = new Set(group.members.map((m) => m.id));
      const count = anomalies.filter((a) => a.sourceSeriesId && memberIDs.has(a.sourceSeriesId)).length;
      counts.set(group.key, count);
    });
    return counts;
  }, [metricGroups, anomalies]);

  const anomalyCountBySeriesID = useMemo(() => {
    const counts = new Map<string, number>();
    anomalies.forEach((a) => {
      if (!a.sourceSeriesId) return;
      counts.set(a.sourceSeriesId, (counts.get(a.sourceSeriesId) ?? 0) + 1);
    });
    return counts;
  }, [anomalies]);

  const visibleGroups = useMemo(() => {
    if (!showAnomalyOnlyGroups) return metricGroups;
    return metricGroups.filter((g) => (anomalyCountByGroup.get(g.key) ?? 0) > 0);
  }, [metricGroups, showAnomalyOnlyGroups, anomalyCountByGroup]);

  const displayGroups = useMemo(
    () => visibleGroups.map((g) => ({ key: g.key, name: g.baseName, displayName: g.baseName })),
    [visibleGroups]
  );

  const initializedScenarioRef = useRef<string | null>(null);
  useEffect(() => {
    if (tsAnalyzerNames.length > 0 && state.activeScenario && initializedScenarioRef.current !== state.activeScenario) {
      initializedScenarioRef.current = state.activeScenario;
      setEnabledAnalyzers(new Set(tsAnalyzerNames));
      setTagFilterInput('');
    }
  }, [tsAnalyzerNames, state.activeScenario]);

  const [autoSelectedScenario, setAutoSelectedScenario] = useState<string | null>(null);
  useEffect(() => {
    if (!state.activeScenario || state.connectionState !== 'ready') return;
    if (autoSelectedScenario === state.activeScenario) return;

    const ranked = [...visibleGroups].sort((a, b) => {
      const countDiff = (anomalyCountByGroup.get(b.key) ?? 0) - (anomalyCountByGroup.get(a.key) ?? 0);
      if (countDiff !== 0) return countDiff;
      return a.baseName.localeCompare(b.baseName);
    });

    setSelectedGroups(new Set(ranked.slice(0, 6).map((g) => g.key)));
    setAutoSelectedScenario(state.activeScenario);
    onTimeRangeChange(null);
  }, [state.activeScenario, state.connectionState, visibleGroups, anomalyCountByGroup, autoSelectedScenario, onTimeRangeChange]);

  const prevSelectedGroupsRef = useRef<Set<string>>(new Set());
  useEffect(() => {
    if (selectedGroups.size === 0 || state.connectionState !== 'ready') {
      if (groupSeriesData.size > 0) setGroupSeriesData(new Map());
      return;
    }

    const selectionChanged = selectedGroups.size !== prevSelectedGroupsRef.current.size ||
      [...selectedGroups].some((k) => !prevSelectedGroupsRef.current.has(k));
    if (!selectionChanged) return;
    prevSelectedGroupsRef.current = new Set(selectedGroups);

    const fetchSeriesData = async () => {
      const next = new Map<string, SeriesData[]>();
      for (const groupKey of selectedGroups) {
        const group = groupByKey.get(groupKey);
        if (!group) continue;
        const seriesList: SeriesData[] = [];
        for (const s of group.members) {
          try {
            const data = await api.getSeriesDataByID(s.id);
            seriesList.push(data);
          } catch (e) {
            console.error(`Failed to fetch series ${s.id}:`, e);
          }
        }
        next.set(groupKey, seriesList);
      }
      setGroupSeriesData(next);
    };

    fetchSeriesData();
  }, [selectedGroups, state.connectionState, state.activeScenario, groupByKey, groupSeriesData.size]);

  const toggleAnalyzer = (name: string) => {
    const next = new Set(enabledAnalyzers);
    if (next.has(name)) {
      next.delete(name);
    } else {
      next.add(name);
    }
    setEnabledAnalyzers(next);
  };

  const anomalousGroupKeys = useMemo(() => {
    const keys = new Set<string>();
    metricGroups.forEach((g) => {
      if ((anomalyCountByGroup.get(g.key) ?? 0) > 0) keys.add(g.key);
    });
    return keys;
  }, [metricGroups, anomalyCountByGroup]);

  return (
    <div className="flex-1 flex">
      <aside
        className="bg-slate-800 border-r border-slate-700 flex flex-col"
        style={{ width: sidebarWidth }}
      >
        <ScenarioSelector
          scenarios={scenarios}
          activeScenario={state.activeScenario}
          onLoadScenario={actions.loadScenario}
        />

        <div className="p-4 border-b border-slate-700">
          <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">
            Analyzers
          </h2>
          <div className="space-y-1">
            {analyzerComponents.map((comp) => {
              const count = allAnomalies.filter((a) => getAnalyzerComponent(a) === comp.name).length;
              const analyzerName = analyzerNameByComponent.get(comp.name);
              const color = analyzerName ? getAnalyzerColorStable(analyzerName) : null;
              return (
                <label
                  key={comp.name}
                  className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-slate-700 cursor-pointer"
                >
                  <input
                    type="checkbox"
                    checked={enabledAnalyzers.has(comp.name)}
                    onChange={() => toggleAnalyzer(comp.name)}
                    className="rounded border-slate-600 bg-slate-700 text-purple-600 focus:ring-purple-500"
                  />
                  {color ? (
                    <span
                      className="text-xs px-1.5 py-0.5 rounded font-medium flex-1"
                      style={{ backgroundColor: color.fill, color: color.stroke }}
                    >
                      {comp.displayName}
                    </span>
                  ) : (
                    <span className="text-sm text-slate-300 flex-1">{comp.displayName}</span>
                  )}
                  {count > 0 && (
                    <span className="text-xs text-slate-500">{count}</span>
                  )}
                </label>
              );
            })}
          </div>
        </div>

        <div className="p-4 border-b border-slate-700 space-y-3">
          <div>
            <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">
              Aggregation
            </h2>
            <div className="flex gap-1 flex-wrap">
              {AGGREGATION_TYPES.map((type) => (
                <button
                  key={type}
                  onClick={() => setAggregationType(type)}
                  className={`text-xs px-2 py-1 rounded transition-colors ${
                    aggregationType === type
                      ? 'bg-purple-600 text-white'
                      : 'bg-slate-700 text-slate-400 hover:bg-slate-600'
                  }`}
                >
                  {type}
                </button>
              ))}
            </div>
          </div>
          <label className="flex items-center justify-between text-xs text-slate-300 bg-slate-700/40 rounded px-2 py-1.5 cursor-pointer">
            <span>Show only groups with anomalies</span>
            <input
              type="checkbox"
              checked={showAnomalyOnlyGroups}
              onChange={(e) => setShowAnomalyOnlyGroups(e.target.checked)}
              className="rounded border-slate-600 bg-slate-700 text-purple-600 focus:ring-purple-500"
            />
          </label>
          <label className="flex items-center justify-between text-xs text-slate-300 bg-slate-700/40 rounded px-2 py-1.5 cursor-pointer">
            <span>In charts, show only anomalous series</span>
            <input
              type="checkbox"
              checked={showAnomalyOnlySeriesLines}
              onChange={(e) => setShowAnomalyOnlySeriesLines(e.target.checked)}
              className="rounded border-slate-600 bg-slate-700 text-purple-600 focus:ring-purple-500"
            />
          </label>
        </div>

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
                const { include: activeTags } = parseTagFilter(tagFilterInput);
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

        <div className="flex-1 p-4 overflow-hidden flex flex-col min-h-0">
          <div className="flex items-center justify-between mb-2">
            <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider">
              Metric Groups ({displayGroups.length})
            </h2>
            <div className="flex gap-1">
              <button
                onClick={() => {
                  const anomalousKeys = displayGroups
                    .filter((g) => anomalousGroupKeys.has(g.key))
                    .map((g) => g.key);
                  setSelectedGroups(new Set(anomalousKeys));
                }}
                className="text-xs px-1.5 py-0.5 bg-slate-700 hover:bg-slate-600 rounded text-slate-400"
                title="Select groups with anomalies"
              >
                !
              </button>
              <button
                onClick={() => setSelectedGroups(new Set(displayGroups.map((g) => g.key)))}
                className="text-xs px-1.5 py-0.5 bg-slate-700 hover:bg-slate-600 rounded text-slate-400"
                title="Select all groups"
              >
                All
              </button>
              <button
                onClick={() => setSelectedGroups(new Set())}
                className="text-xs px-1.5 py-0.5 bg-slate-700 hover:bg-slate-600 rounded text-slate-400"
                title="Clear selection"
              >
                None
              </button>
            </div>
          </div>
          <SeriesTree
            series={displayGroups}
            selectedSeries={selectedGroups}
            anomalousSources={anomalousGroupKeys}
            onSelectionChange={setSelectedGroups}
          />
        </div>
      </aside>

      <main className="flex-1 p-6 overflow-y-auto">
        {state.error && (
          <div className="bg-red-900/50 border border-red-700 rounded-lg p-4 mb-6">
            <div className="text-red-400">{state.error}</div>
          </div>
        )}

        {state.connectionState === 'disconnected' && (
          <div className="text-center py-20">
            <div className="text-slate-400 text-lg">Waiting for observer connection...</div>
            <div className="text-slate-500 mt-2">
              Start the observer: <code className="bg-slate-800 px-2 py-1 rounded">./bin/observer-testbench</code>
            </div>
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
          <div className="space-y-6">
            {selectedGroups.size === 0 ? (
              <div className="text-center py-10 text-slate-500">
                Select metric groups from the sidebar to view charts
              </div>
            ) : groupSeriesData.size === 0 ? (
              <div className="text-center py-10 text-slate-500">Loading series data...</div>
            ) : (
              <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                {Array.from(selectedGroups).map((groupKey) => {
                  const group = groupByKey.get(groupKey);
                  if (!group) return null;
                  const dataList = groupSeriesData.get(groupKey) ?? [];
                  if (dataList.length === 0) return null;

                  const chartSeries = showAnomalyOnlySeriesLines
                    ? dataList.filter((d) => (anomalyCountBySeriesID.get(d.id) ?? 0) > 0)
                    : dataList;
                  if (chartSeries.length === 0) return null;

                  const seriesIDs = new Set(chartSeries.map((d) => d.id));
                  const seriesAnomalies = anomalies.filter((a) => a.sourceSeriesId && seriesIDs.has(a.sourceSeriesId));
                  const anomalyMarkers = chartSeries.flatMap((d) => d.anomalies);

                  const seriesVariants: SeriesVariant[] = chartSeries.map((d) => ({
                    label: formatSeriesLabel(d.tags),
                    points: d.points,
                    seriesId: d.id,
                  }));

                  const primary = chartSeries[0];

                  return (
                    <ChartWithAnomalyDetails
                      key={groupKey}
                      name={primary.name}
                      points={primary.points}
                      anomalyMarkers={anomalyMarkers}
                      anomalies={seriesAnomalies}
                      correlationRanges={[]}
                      enabledAnalyzers={enabledAnalyzers}
                      timeRange={timeRange}
                      onTimeRangeChange={onTimeRangeChange}
                      smoothLines={smoothLines}
                      seriesVariants={seriesVariants}
                    />
                  );
                })}
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
