import { useState, useEffect, useMemo, useRef } from 'react';
import { ChartWithAnomalyDetails } from './ChartWithAnomalyDetails';
import { SeriesTree } from './SeriesTree';
import { api } from '../api/client';
import type { SeriesData, SeriesInfo, ScenarioInfo } from '../api/client';
import type { SplitSeries } from './TimeSeriesChart';
import type { TimeRange } from './ChartWithAnomalyDetails';
import type { ObserverState, ObserverActions } from '../hooks/useObserver';

// Parse tag string "key:value" into parts
function parseTag(tag: string): { key: string; value: string } | null {
  const idx = tag.indexOf(':');
  if (idx === -1) return null;
  return { key: tag.slice(0, idx), value: tag.slice(idx + 1) };
}

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

function findSeriesVariants(
  baseName: string,
  allSeries: SeriesInfo[],
  splitByTag: string
): SeriesInfo[] {
  const base = getBaseMetricName(baseName);
  return allSeries.filter((s) => {
    const sBase = getBaseMetricName(s.name);
    if (sBase !== base) return false;
    return (s.tags ?? []).some((t) => {
      const parsed = parseTag(t);
      return parsed?.key === splitByTag;
    });
  });
}

interface TSAnalysisViewProps {
  state: ObserverState;
  actions: ObserverActions;
  sidebarWidth: number;
  timeRange: TimeRange | null;
  onTimeRangeChange: (range: TimeRange | null) => void;
  smoothLines: boolean;
  splitByTag: string | null;
}

export function TSAnalysisView({
  state,
  actions,
  sidebarWidth,
  timeRange,
  onTimeRangeChange,
  smoothLines,
  splitByTag,
}: TSAnalysisViewProps) {
  const [selectedSeries, setSelectedSeries] = useState<Set<string>>(new Set());
  const [enabledAnalyzers, setEnabledAnalyzers] = useState<Set<string>>(new Set());
  const [seriesData, setSeriesData] = useState<Map<string, SeriesData>>(new Map());
  const [splitSeriesData, setSplitSeriesData] = useState<Map<string, SeriesData[]>>(new Map());
  const [aggregationType, setAggregationType] = useState<AggregationType>('avg');

  const scenarios = state.scenarios ?? [];
  const components = state.components ?? [];
  const series = state.series ?? [];
  const allAnomalies = state.anomalies ?? [];

  // Filter anomalies by enabled analyzers
  const anomalies = useMemo(
    () => allAnomalies.filter((a) => enabledAnalyzers.has(a.analyzerName)),
    [allAnomalies, enabledAnalyzers]
  );

  // Get unique analyzers from components
  const analyzerComponents = useMemo(
    () => components.filter((c) => c.category === 'analyzer'),
    [components]
  );

  const tsAnalyzerNames = useMemo(
    () => analyzerComponents.map((c) => c.name),
    [analyzerComponents]
  );

  // Filter series by selected aggregation type and deduplicate by base name
  const filteredSeries = useMemo(() => {
    const withAggType = series.filter((s) => {
      const aggType = getAggregationType(s.name);
      return aggType === aggregationType;
    });
    const seen = new Set<string>();
    return withAggType.filter((s) => {
      const baseName = getBaseMetricName(s.name);
      const key = `${s.namespace}/${baseName}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
  }, [series, aggregationType]);

  // Create display series with stripped aggregation suffix
  const displaySeries = useMemo(() => {
    return filteredSeries.map((s) => ({
      ...s,
      displayName: getBaseMetricName(s.name),
    }));
  }, [filteredSeries]);

  // Track which scenario we initialized analyzers for
  const initializedScenarioRef = useRef<string | null>(null);

  // Initialize enabled analyzers when components load (once per scenario)
  useEffect(() => {
    if (tsAnalyzerNames.length > 0 && state.activeScenario && initializedScenarioRef.current !== state.activeScenario) {
      initializedScenarioRef.current = state.activeScenario;
      setEnabledAnalyzers(new Set(tsAnalyzerNames));
    }
  }, [tsAnalyzerNames, state.activeScenario]);

  // Track which scenario we've auto-selected for
  const [autoSelectedScenario, setAutoSelectedScenario] = useState<string | null>(null);

  // Auto-select series with anomalies when scenario data loads
  useEffect(() => {
    if (!state.activeScenario || state.connectionState !== 'ready') return;
    if (autoSelectedScenario !== state.activeScenario) {
      if (series.length > 0) {
        const anomalyCount = new Map<string, number>();
        allAnomalies.forEach((a) => {
          anomalyCount.set(a.source, (anomalyCount.get(a.source) || 0) + 1);
        });
        const matching = series
          .filter((s) => anomalyCount.has(s.name))
          .sort((a, b) => {
            const countDiff = (anomalyCount.get(b.name) || 0) - (anomalyCount.get(a.name) || 0);
            if (countDiff !== 0) return countDiff;
            return a.name.localeCompare(b.name);
          });
        if (matching.length > 0) {
          setSelectedSeries(new Set(matching.slice(0, 6).map((s) => `${s.namespace}/${s.name}`)));
        } else {
          const sorted = [...series].sort((a, b) => a.name.localeCompare(b.name));
          setSelectedSeries(new Set(sorted.slice(0, 6).map((s) => `${s.namespace}/${s.name}`)));
        }
        setAutoSelectedScenario(state.activeScenario);
        onTimeRangeChange(null);
      }
    }
  }, [state.activeScenario, state.connectionState, allAnomalies, series, autoSelectedScenario, onTimeRangeChange]);

  // Track previous selection to detect changes
  const prevSelectedSeriesRef = useRef<Set<string>>(new Set());

  // Fetch data for selected series
  useEffect(() => {
    if (selectedSeries.size === 0 || state.connectionState !== 'ready') {
      if (seriesData.size > 0) setSeriesData(new Map());
      return;
    }
    const selectionChanged = selectedSeries.size !== prevSelectedSeriesRef.current.size ||
      [...selectedSeries].some(k => !prevSelectedSeriesRef.current.has(k));
    if (!selectionChanged) return;
    prevSelectedSeriesRef.current = new Set(selectedSeries);

    const fetchSeriesData = async () => {
      const newData = new Map<string, SeriesData>();
      for (const key of selectedSeries) {
        const [namespace, ...nameParts] = key.split('/');
        const name = nameParts.join('/');
        try {
          const data = await api.getSeriesData(namespace, name);
          newData.set(key, data);
        } catch (e) {
          console.error(`Failed to fetch series ${key}:`, e);
        }
      }
      setSeriesData(newData);
    };
    fetchSeriesData();
  }, [selectedSeries, state.connectionState, state.activeScenario, seriesData.size]);

  // Track previous split tag to detect changes
  const prevSplitByTagRef = useRef<string | null>(null);

  // Fetch split series data when splitByTag is enabled
  useEffect(() => {
    if (!splitByTag || selectedSeries.size === 0 || state.connectionState !== 'ready') {
      if (splitSeriesData.size > 0) setSplitSeriesData(new Map());
      prevSplitByTagRef.current = splitByTag;
      return;
    }
    if (splitByTag === prevSplitByTagRef.current) return;
    prevSplitByTagRef.current = splitByTag;

    const fetchSplitData = async () => {
      const newSplitData = new Map<string, SeriesData[]>();
      for (const key of selectedSeries) {
        const [_namespace, ...nameParts] = key.split('/');
        void _namespace;
        const name = nameParts.join('/');
        const variants = findSeriesVariants(name, series, splitByTag);
        if (variants.length > 1) {
          const variantData: SeriesData[] = [];
          for (const variant of variants) {
            try {
              const data = await api.getSeriesData(variant.namespace, variant.name);
              variantData.push(data);
            } catch (e) {
              console.error(`Failed to fetch variant ${variant.name}:`, e);
            }
          }
          newSplitData.set(key, variantData);
        }
      }
      setSplitSeriesData(newSplitData);
    };
    fetchSplitData();
  }, [splitByTag, selectedSeries, series, state.connectionState, splitSeriesData.size]);

  const toggleAnalyzer = (name: string) => {
    const newSet = new Set(enabledAnalyzers);
    if (newSet.has(name)) {
      newSet.delete(name);
    } else {
      newSet.add(name);
    }
    setEnabledAnalyzers(newSet);
  };

  // Compute anomalous sources for the tree
  const anomalousSources = useMemo(
    () => new Set(anomalies.map((a) => a.source)),
    [anomalies]
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

        {/* Analyzers */}
        <div className="p-4 border-b border-slate-700">
          <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">
            Analyzers
          </h2>
          <div className="space-y-1">
            {analyzerComponents.map((comp) => {
              const count = allAnomalies.filter((a) => a.analyzerName === comp.name).length;
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
                  <span className="text-sm text-slate-300 flex-1">{comp.displayName}</span>
                  {count > 0 && (
                    <span className="text-xs text-slate-500">{count}</span>
                  )}
                </label>
              );
            })}
          </div>
        </div>

        {/* Aggregation Type */}
        <div className="p-4 border-b border-slate-700">
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

        {/* Series Tree */}
        <div className="flex-1 p-4 overflow-hidden flex flex-col min-h-0">
          <div className="flex items-center justify-between mb-2">
            <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider">
              Series ({displaySeries.length})
            </h2>
            <div className="flex gap-1">
              <button
                onClick={() => {
                  const anomalousKeys = displaySeries
                    .filter((s) => anomalies.some((a) => a.source === s.name || a.source === (s as any).displayName))
                    .map((s) => `${s.namespace}/${s.name}`);
                  setSelectedSeries(new Set(anomalousKeys));
                }}
                className="text-xs px-1.5 py-0.5 bg-slate-700 hover:bg-slate-600 rounded text-slate-400"
                title="Select all series with anomalies"
              >
                !
              </button>
              <button
                onClick={() => {
                  const allKeys = displaySeries.map((s) => `${s.namespace}/${s.name}`);
                  setSelectedSeries(new Set(allKeys));
                }}
                className="text-xs px-1.5 py-0.5 bg-slate-700 hover:bg-slate-600 rounded text-slate-400"
                title="Select all series"
              >
                All
              </button>
              <button
                onClick={() => setSelectedSeries(new Set())}
                className="text-xs px-1.5 py-0.5 bg-slate-700 hover:bg-slate-600 rounded text-slate-400"
                title="Clear selection"
              >
                None
              </button>
            </div>
          </div>
          <SeriesTree
            series={displaySeries}
            selectedSeries={selectedSeries}
            anomalousSources={anomalousSources}
            onSelectionChange={setSelectedSeries}
          />
        </div>
      </aside>

      {/* Main Content - Charts */}
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
            {/* Charts */}
            {selectedSeries.size === 0 ? (
              <div className="text-center py-10 text-slate-500">
                Select series from the sidebar to view charts
              </div>
            ) : seriesData.size === 0 ? (
              <div className="text-center py-10 text-slate-500">
                Loading series data...
              </div>
            ) : (
              <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                {Array.from(selectedSeries).map((key) => {
                  const data = seriesData.get(key);
                  if (!data) return null;
                  const seriesAnomalies = anomalies.filter((a) => a.source === data.name);

                  let splitSeries: SplitSeries[] | undefined;
                  if (splitByTag) {
                    const variants = splitSeriesData.get(key);
                    if (variants && variants.length > 1) {
                      splitSeries = variants.map((v) => {
                        const tagValue = (v.tags ?? [])
                          .map((t) => parseTag(t))
                          .find((p) => p?.key === splitByTag)?.value ?? 'unknown';
                        return {
                          label: `${splitByTag}:${tagValue}`,
                          points: v.points,
                        };
                      });
                    }
                  }

                  return (
                    <ChartWithAnomalyDetails
                      key={key}
                      name={data.name}
                      points={data.points}
                      anomalyMarkers={data.anomalies}
                      anomalies={seriesAnomalies}
                      correlationRanges={[]}
                      enabledAnalyzers={enabledAnalyzers}
                      timeRange={timeRange}
                      onTimeRangeChange={onTimeRangeChange}
                      smoothLines={smoothLines}
                      splitSeries={splitSeries}
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

