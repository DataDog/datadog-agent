import { useState, useEffect, useMemo, useRef } from 'react';
import { useObserver } from './hooks/useObserver';
import { ChartWithAnomalyDetails } from './components/ChartWithAnomalyDetails';
import { SeriesTree } from './components/SeriesTree';
import { api } from './api/client';
import type { SeriesData, SeriesInfo } from './api/client';
import type { SplitSeries } from './components/TimeSeriesChart';

// Parse tag string "key:value" into parts
function parseTag(tag: string): { key: string; value: string } | null {
  const idx = tag.indexOf(':');
  if (idx === -1) return null;
  return { key: tag.slice(0, idx), value: tag.slice(idx + 1) };
}

// Aggregation types available for metrics
const AGGREGATION_TYPES = ['avg', 'count', 'sum', 'min', 'max'] as const;
type AggregationType = typeof AGGREGATION_TYPES[number];

// Get the base metric name without aggregation suffix
function getBaseMetricName(name: string): string {
  // Series names often end with :avg, :sum, :count, :min, :max
  const match = name.match(/^(.+):(avg|sum|count|min|max)$/);
  return match ? match[1] : name;
}

// Get the aggregation type from a series name
function getAggregationType(name: string): AggregationType | null {
  const match = name.match(/:(avg|sum|count|min|max)$/);
  return match ? (match[1] as AggregationType) : null;
}

// Find all series variants (same base name, different tags)
function findSeriesVariants(
  baseName: string,
  allSeries: SeriesInfo[],
  splitByTag: string
): SeriesInfo[] {
  const base = getBaseMetricName(baseName);
  return allSeries.filter((s) => {
    const sBase = getBaseMetricName(s.name);
    if (sBase !== base) return false;
    // Must have the split tag key
    return (s.tags ?? []).some((t) => {
      const parsed = parseTag(t);
      return parsed?.key === splitByTag;
    });
  });
}

function ConnectionStatus({ state }: { state: string }) {
  const colors: Record<string, string> = {
    disconnected: 'bg-red-500',
    connected: 'bg-yellow-500',
    loading: 'bg-blue-500',
    ready: 'bg-green-500',
  };

  const labels: Record<string, string> = {
    disconnected: 'Disconnected',
    connected: 'Connected (no scenario)',
    loading: 'Loading...',
    ready: 'Ready',
  };

  return (
    <div className="flex items-center gap-2">
      <div className={`w-2 h-2 rounded-full ${colors[state]} animate-pulse`} />
      <span className="text-sm text-slate-400">{labels[state]}</span>
    </div>
  );
}

// Time range for global zoom
interface TimeRange {
  start: number; // Unix timestamp in seconds
  end: number;
}

function formatTimeRange(range: TimeRange): string {
  const formatTime = (ts: number) => new Date(ts * 1000).toLocaleTimeString();
  return `${formatTime(range.start)} - ${formatTime(range.end)}`;
}

function App() {
  const [state, actions] = useObserver();
  const [selectedSeries, setSelectedSeries] = useState<Set<string>>(new Set());
  const [enabledAnalyzers, setEnabledAnalyzers] = useState<Set<string>>(new Set());
  const [seriesData, setSeriesData] = useState<Map<string, SeriesData>>(new Map());
  const [timeRange, setTimeRange] = useState<TimeRange | null>(null);
  const [sidebarWidth, setSidebarWidth] = useState(320);
  const [correlationsExpanded, setCorrelationsExpanded] = useState(true);
  const [smoothLines, setSmoothLines] = useState(true);
  const [splitByTag, setSplitByTag] = useState<string | null>(null);
  const [splitSeriesData, setSplitSeriesData] = useState<Map<string, SeriesData[]>>(new Map());
  const [aggregationType, setAggregationType] = useState<AggregationType>('avg');
  const isResizingRef = useRef(false);

  // Safely access arrays with fallbacks
  const scenarios = state.scenarios ?? [];
  const components = state.components ?? [];
  const series = state.series ?? [];
  const allAnomalies = state.anomalies ?? [];
  const correlations = state.correlations ?? [];

  // Filter anomalies by enabled analyzers
  const anomalies = useMemo(
    () => allAnomalies.filter((a) => enabledAnalyzers.has(a.analyzerName)),
    [allAnomalies, enabledAnalyzers]
  );

  // Get unique analyzers from components
  const tsAnalyzers = useMemo(
    () => components.filter((c) => c.type === 'ts_analysis').map((c) => c.name),
    [components]
  );

  // Extract available tag keys from all series
  const availableTagKeys = useMemo(() => {
    const tagKeys = new Set<string>();
    series.forEach((s) => {
      (s.tags ?? []).forEach((t) => {
        const parsed = parseTag(t);
        if (parsed) tagKeys.add(parsed.key);
      });
    });
    return Array.from(tagKeys).sort();
  }, [series]);

  // Filter series by selected aggregation type and deduplicate by base name
  const filteredSeries = useMemo(() => {
    // First, filter to only include series with the selected aggregation type
    const withAggType = series.filter((s) => {
      const aggType = getAggregationType(s.name);
      return aggType === aggregationType;
    });

    // Deduplicate by base name (in case there are multiple with same base but different tags)
    const seen = new Set<string>();
    return withAggType.filter((s) => {
      const baseName = getBaseMetricName(s.name);
      const key = `${s.namespace}/${baseName}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
  }, [series, aggregationType]);

  // Create display series with stripped aggregation suffix for the tree
  const displaySeries = useMemo(() => {
    return filteredSeries.map((s) => ({
      ...s,
      displayName: getBaseMetricName(s.name),
    }));
  }, [filteredSeries]);

  // Initialize enabled analyzers when components load
  useEffect(() => {
    if (tsAnalyzers.length > 0 && enabledAnalyzers.size === 0) {
      setEnabledAnalyzers(new Set(tsAnalyzers));
    }
  }, [tsAnalyzers, enabledAnalyzers.size]);

  // Track which scenario we've auto-selected for
  const [autoSelectedScenario, setAutoSelectedScenario] = useState<string | null>(null);

  // Auto-select series with anomalies when scenario data loads
  useEffect(() => {
    // Skip if no scenario or already auto-selected for this scenario
    if (!state.activeScenario || state.connectionState !== 'ready') {
      return;
    }

    // If scenario changed, auto-select series with anomalies
    if (autoSelectedScenario !== state.activeScenario) {
      // Auto-select series with anomalies, prioritizing those with more detections
      if (anomalies.length > 0 && series.length > 0) {
        // Count anomalies per series (more detections = more interesting)
        const anomalyCount = new Map<string, number>();
        anomalies.forEach((a) => {
          anomalyCount.set(a.source, (anomalyCount.get(a.source) || 0) + 1);
        });

        // Find matching series and sort by anomaly count (desc), then name (asc) for determinism
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
          // No anomalies matched, just select first few series alphabetically
          const sorted = [...series].sort((a, b) => a.name.localeCompare(b.name));
          setSelectedSeries(new Set(sorted.slice(0, 6).map((s) => `${s.namespace}/${s.name}`)));
        }
        setAutoSelectedScenario(state.activeScenario);
        // Reset time range when scenario changes
        setTimeRange(null);
      }
    }
  }, [state.activeScenario, state.connectionState, anomalies, series, autoSelectedScenario]);

  // Fetch data for selected series
  useEffect(() => {
    // Clear data immediately when selection changes
    setSeriesData(new Map());

    if (selectedSeries.size === 0 || state.connectionState !== 'ready') {
      return;
    }

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
  }, [selectedSeries, state.connectionState, state.activeScenario]);

  // Fetch split series data when splitByTag is enabled
  useEffect(() => {
    setSplitSeriesData(new Map());

    if (!splitByTag || selectedSeries.size === 0 || state.connectionState !== 'ready') {
      return;
    }

    const fetchSplitData = async () => {
      const newSplitData = new Map<string, SeriesData[]>();

      for (const key of selectedSeries) {
        const [namespace, ...nameParts] = key.split('/');
        const name = nameParts.join('/');

        // Find all series variants with different tag values for the split key
        const variants = findSeriesVariants(name, series, splitByTag);

        if (variants.length > 1) {
          // Fetch data for each variant
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
  }, [splitByTag, selectedSeries, series, state.connectionState]);

  const toggleAnalyzer = (name: string) => {
    const newSet = new Set(enabledAnalyzers);
    if (newSet.has(name)) {
      newSet.delete(name);
    } else {
      newSet.add(name);
    }
    setEnabledAnalyzers(newSet);
  };

  // Sidebar resize handlers
  const handleResizeStart = (e: React.MouseEvent) => {
    e.preventDefault();
    isResizingRef.current = true;
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
  };

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (!isResizingRef.current) return;
      const newWidth = Math.max(200, Math.min(600, e.clientX));
      setSidebarWidth(newWidth);
    };

    const handleMouseUp = () => {
      if (isResizingRef.current) {
        isResizingRef.current = false;
        document.body.style.cursor = '';
        document.body.style.userSelect = '';
      }
    };

    window.addEventListener('mousemove', handleMouseMove);
    window.addEventListener('mouseup', handleMouseUp);

    return () => {
      window.removeEventListener('mousemove', handleMouseMove);
      window.removeEventListener('mouseup', handleMouseUp);
    };
  }, []);

  // Compute anomalous sources for the tree
  const anomalousSources = useMemo(
    () => new Set(anomalies.map((a) => a.source)),
    [anomalies]
  );

  return (
    <div className="min-h-screen flex flex-col">
      {/* Header */}
      <header className="bg-slate-800 border-b border-slate-700 px-4 py-3">
        <div className="flex justify-between items-center">
          <h1 className="text-lg font-semibold text-white">Observer Test Bench</h1>
          <div className="flex items-center gap-4">
            {/* Time Range Zoom Control */}
            {timeRange && (
              <div className="flex items-center gap-2 bg-slate-700/50 rounded px-3 py-1.5">
                <span className="text-xs text-slate-400">Zoom:</span>
                <span className="text-sm text-slate-200 font-mono">
                  {formatTimeRange(timeRange)}
                </span>
                <span className="text-xs text-slate-500 ml-1">
                  (middle-drag to pan)
                </span>
                <button
                  onClick={() => setTimeRange(null)}
                  className="ml-2 text-xs px-2 py-0.5 bg-slate-600 hover:bg-slate-500 rounded text-slate-300"
                  title="Reset zoom"
                >
                  Reset
                </button>
              </div>
            )}
            {!timeRange && state.connectionState === 'ready' && (
              <span className="text-xs text-slate-500">
                Drag to zoom, middle-drag to pan
              </span>
            )}
            {/* Smooth Lines Toggle */}
            <label className="flex items-center gap-2 cursor-pointer">
              <span className="text-xs text-slate-400">Smooth</span>
              <button
                onClick={() => setSmoothLines(!smoothLines)}
                className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${
                  smoothLines ? 'bg-purple-600' : 'bg-slate-600'
                }`}
              >
                <span
                  className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white transition-transform ${
                    smoothLines ? 'translate-x-5' : 'translate-x-1'
                  }`}
                />
              </button>
            </label>
            {/* Split by Tag Dropdown */}
            {availableTagKeys.length > 0 && (
              <div className="flex items-center gap-2">
                <span className="text-xs text-slate-400">Split by</span>
                <select
                  value={splitByTag ?? ''}
                  onChange={(e) => setSplitByTag(e.target.value || null)}
                  className="text-xs bg-slate-700 text-slate-300 rounded px-2 py-1 border border-slate-600 focus:outline-none focus:ring-1 focus:ring-purple-500"
                >
                  <option value="">None</option>
                  {availableTagKeys.map((key) => (
                    <option key={key} value={key}>
                      {key}
                    </option>
                  ))}
                </select>
              </div>
            )}
            <ConnectionStatus state={state.connectionState} />
            {state.status && (
              <span className="text-sm text-slate-400">
                {series.length} series, {anomalies.length}
                {anomalies.length !== allAnomalies.length && `/${allAnomalies.length}`} anomalies
              </span>
            )}
          </div>
        </div>
      </header>

      <div className="flex-1 flex">
        {/* Left Sidebar - Scenarios & Components */}
        <aside
          className="bg-slate-800 border-r border-slate-700 flex flex-col relative"
          style={{ width: sidebarWidth }}
        >
          {/* Resize handle */}
          <div
            className="absolute right-0 top-0 bottom-0 w-1 cursor-col-resize hover:bg-purple-500/50 active:bg-purple-500"
            onMouseDown={handleResizeStart}
          />
          {/* Scenarios */}
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
                    onClick={() => actions.loadScenario(scenario.name)}
                    className={`w-full text-left px-3 py-2 rounded text-sm transition-colors ${
                      state.activeScenario === scenario.name
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

          {/* Analyzers */}
          <div className="p-4 border-b border-slate-700">
            <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">
              Analyzers
            </h2>
            <div className="space-y-1">
              {tsAnalyzers.map((name) => {
                const count = allAnomalies.filter((a) => a.analyzerName === name).length;
                return (
                  <label
                    key={name}
                    className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-slate-700 cursor-pointer"
                  >
                    <input
                      type="checkbox"
                      checked={enabledAnalyzers.has(name)}
                      onChange={() => toggleAnalyzer(name)}
                      className="rounded border-slate-600 bg-slate-700 text-purple-600 focus:ring-purple-500"
                    />
                    <span className="text-sm text-slate-300 flex-1">{name}</span>
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
              {/* Quick selection buttons */}
              <div className="flex gap-1">
                <button
                  onClick={() => {
                    const anomalousKeys = displaySeries
                      .filter((s) => anomalies.some((a) => a.source === s.name || a.source === s.displayName))
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
              {/* Correlations - clickable to select related series */}
              {correlations.length > 0 && (
                <div className="bg-slate-800 rounded-lg">
                  <button
                    onClick={() => setCorrelationsExpanded(!correlationsExpanded)}
                    className="w-full p-4 flex items-center justify-between hover:bg-slate-700/30 rounded-lg transition-colors"
                  >
                    <div className="flex items-center gap-2">
                      <span className="text-slate-500">{correlationsExpanded ? '▼' : '▶'}</span>
                      <h2 className="text-sm font-semibold text-slate-300">
                        Correlations ({correlations.length})
                      </h2>
                    </div>
                    <span className="text-xs text-slate-500">
                      {correlationsExpanded ? 'Shift+click to select multiple' : 'Click to expand'}
                    </span>
                  </button>
                  {correlationsExpanded && <div className="space-y-2 px-4 pb-4">
                    {correlations.map((c, i) => {
                      // Extract series names from the correlation's anomalies
                      const correlatedSources = new Set(c.anomalies.map((a) => a.source));
                      const correlatedSeriesKeys = series
                        .filter((s) => correlatedSources.has(s.name))
                        .map((s) => `${s.namespace}/${s.name}`);
                      const isSelected = correlatedSeriesKeys.length > 0 &&
                        correlatedSeriesKeys.every((k) => selectedSeries.has(k));

                      return (
                        <button
                          key={i}
                          onClick={(e) => {
                            if (correlatedSeriesKeys.length > 0) {
                              if (e.shiftKey || e.metaKey || e.ctrlKey) {
                                // Add to existing selection
                                setSelectedSeries((prev) => {
                                  const next = new Set(prev);
                                  correlatedSeriesKeys.forEach((k) => next.add(k));
                                  return next;
                                });
                              } else {
                                // Replace selection
                                setSelectedSeries(new Set(correlatedSeriesKeys));
                              }
                            }
                          }}
                          className={`w-full text-left rounded p-3 transition-colors ${
                            isSelected
                              ? 'bg-purple-900/30 border border-purple-500/50'
                              : 'bg-slate-700/50 hover:bg-slate-700 border border-transparent'
                          }`}
                        >
                          <div className="flex items-center justify-between">
                            <div className="font-medium text-purple-400">{c.title}</div>
                            <span className="text-xs text-slate-500">
                              {correlatedSeriesKeys.length} series
                            </span>
                          </div>
                          <div className="text-sm text-slate-400 mt-1">
                            Pattern: {c.pattern}
                          </div>
                          <div className="flex flex-wrap gap-1 mt-2">
                            {c.signals.map((signal, j) => (
                              <span
                                key={j}
                                className="text-xs px-2 py-0.5 bg-slate-600/50 rounded text-slate-300"
                              >
                                {signal}
                              </span>
                            ))}
                          </div>
                          <div className="text-xs text-slate-500 mt-2">
                            {new Date(c.firstSeen * 1000).toLocaleTimeString()} -{' '}
                            {new Date(c.lastUpdated * 1000).toLocaleTimeString()}
                          </div>
                        </button>
                      );
                    })}
                  </div>}
                </div>
              )}

              {/* Charts with inline anomaly details */}
              {selectedSeries.size === 0 ? (
                <div className="text-center py-10 text-slate-500">
                  Select series from the sidebar or click a correlation to view charts
                </div>
              ) : (
                <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                  {Array.from(selectedSeries).map((key) => {
                    const data = seriesData.get(key);
                    if (!data) return null;
                    // Find anomalies for this series from the full anomaly list
                    const seriesAnomalies = anomalies.filter((a) => a.source === data.name);
                    // Find correlations that involve this series
                    const seriesCorrelations = correlations
                      .filter((c) => c.anomalies.some((a) => a.source === data.name))
                      .map((c, idx) => ({
                        id: idx,
                        title: c.title,
                        start: c.firstSeen,
                        end: c.lastUpdated,
                      }));

                    // Build split series if tag splitting is enabled
                    let splitSeries: SplitSeries[] | undefined;
                    if (splitByTag) {
                      const variants = splitSeriesData.get(key);
                      if (variants && variants.length > 1) {
                        splitSeries = variants.map((v) => {
                          // Find the value for the split tag
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
                        correlationRanges={seriesCorrelations}
                        enabledAnalyzers={enabledAnalyzers}
                        timeRange={timeRange}
                        onTimeRangeChange={setTimeRange}
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
    </div>
  );
}

export default App;
