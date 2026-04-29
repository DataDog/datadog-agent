import { useState, useEffect, useMemo, useRef } from 'react';
import { ChartWithAnomalyDetails } from './ChartWithAnomalyDetails';
import { SeriesTree } from './SeriesTree';
import { api } from '../api/client';
import type { SeriesData, SeriesInfo, ScenarioInfo, Point } from '../api/client';
import type { SeriesVariant } from './MetricsChart';
import { getDetectorColorStable } from './MetricsChart';
import type { TimeRange, PhaseMarker } from './ChartWithAnomalyDetails';
import type { ObserverState, ObserverActions } from '../hooks/useObserver';
import { MAIN_TAG_FILTER_KEYS } from '../constants';
import { parseTagFilter, extractTagGroups, toggleTagInInput, matchesTagFilter } from '../filters';
import { TagFilterGroups } from './TagFilterGroups';

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

function isPatternCounterBaseName(baseName: string): boolean {
  return /^log\.log_pattern_extractor\.[0-9a-f]+\.count$/.test(baseName);
}

function getDetectorComponent(anomaly: { detectorName: string; detectorComponent?: string }): string {
  return anomaly.detectorComponent ?? anomaly.detectorName;
}

function formatSeriesLabel(tags: string[]): string {
  if (!tags || tags.length === 0) return 'untagged';
  return tags.join(', ');
}

/** Prefix sum of per-bucket deltas (time-ordered) — total from scenario start. */
function cumulativeFromStart(points: Point[]): Point[] {
  if (points.length === 0) return points;
  const sorted = [...points].sort((a, b) => a.timestamp - b.timestamp);
  let acc = 0;
  return sorted.map((p) => {
    acc += p.value;
    return { timestamp: p.timestamp, value: acc };
  });
}

interface MetricGroup {
  key: string;
  namespace: string;
  baseName: string;
  members: SeriesInfo[];
  virtual: boolean;
}

interface MetricsViewProps {
  state: ObserverState;
  actions: ObserverActions;
  sidebarWidth: number;
  timeRange: TimeRange | null;
  onTimeRangeChange: (range: TimeRange | null) => void;
  smoothLines: boolean;
  phaseMarkers?: PhaseMarker[];
  /** One-shot jump from Logs tab; cleared via onRequestedFocusedGroupKeyConsumed after applying. */
  requestedFocusedGroupKey?: string | null;
  onRequestedFocusedGroupKeyConsumed?: () => void;
  onJumpToPattern?: (patternHash: string) => void;
}

export function MetricsView({
  state,
  actions,
  sidebarWidth,
  timeRange,
  onTimeRangeChange,
  smoothLines,
  phaseMarkers,
  requestedFocusedGroupKey,
  onRequestedFocusedGroupKeyConsumed,
  onJumpToPattern,
}: MetricsViewProps) {
  const [selectedGroups, setSelectedGroups] = useState<Set<string>>(new Set());
  const [groupSeriesData, setGroupSeriesData] = useState<Map<string, SeriesData[]>>(new Map());
  const [aggregationType, setAggregationType] = useState<AggregationType>('avg');
  const [showAnomalyOnlyGroups, setShowAnomalyOnlyGroups] = useState(false);
  const [showAnomalyOnlySeriesLines, setShowAnomalyOnlySeriesLines] = useState(false);
  const [tagFilterInput, setTagFilterInput] = useState('');

  // Parse a pattern metric group key and return its log pattern hash, or null.
  // Current: "log_pattern_extractor/log.log_pattern_extractor.<hash>.count"
  // Legacy:  "parquet/_virtual.log.log_pattern_extractor.<hash>.count"
  const getPatternHash = (groupKey: string): string | null => {
    let m = groupKey.match(/^log_pattern_extractor\/log\.log_pattern_extractor\.([0-9a-f]+)\.count$/);
    if (m) return m[1];
    return null;
  };

  // Fill the bucket grid: snap stored timestamps to slots and emit 0 for missing ones.
  // Returns [bucketSec, denseSeries] where denseSeries covers [first, last] completely.
  const fillBuckets = (points: Point[]): [number, Point[]] => {
    if (points.length < 2) return [0, points];
    const bucketSec = Math.round(points[1].timestamp - points[0].timestamp);
    if (bucketSec <= 0) return [0, points];
    const first = Math.round(points[0].timestamp);
    const last = Math.round(points[points.length - 1].timestamp);
    const bySlot = new Map<number, number>();
    for (const p of points) {
      const slot = first + Math.round((p.timestamp - first) / bucketSec) * bucketSec;
      bySlot.set(slot, p.value);
    }
    const dense: Point[] = [];
    for (let ts = first; ts <= last; ts += bucketSec) {
      dense.push({ timestamp: ts, value: bySlot.get(ts) ?? 0 });
    }
    return [bucketSec, dense];
  };

  // Rate (evt/s): count per bucket ÷ bucket size in seconds.
  const toRateSeries = (points: Point[]): Point[] => {
    if (points.length === 0) return points;
    if (points.length === 1) return [{ timestamp: points[0].timestamp, value: 0 }];
    const [bucketSec, dense] = fillBuckets(points);
    if (bucketSec <= 0) return points;
    return dense.map((p) => ({ timestamp: p.timestamp, value: p.value / bucketSec }));
  };

  // Rate (evt/min): same per-bucket count normalized to one minute.
  const toRatePerMinSeries = (points: Point[]): Point[] => {
    if (points.length === 0) return points;
    if (points.length === 1) return [{ timestamp: points[0].timestamp, value: 0 }];
    const [bucketSec, dense] = fillBuckets(points);
    if (bucketSec <= 0) return points;
    return dense.map((p) => ({ timestamp: p.timestamp, value: (p.value / bucketSec) * 60 }));
  };

  type PatternViewMode = 'raw' | 'rate-sec' | 'rate-min';

  // Per-group view mode for pattern series. Defaults to 'rate-sec' when first selected.
  const [patternViewMode, setPatternViewMode] = useState<Map<string, PatternViewMode>>(new Map());
  const setGroupViewMode = (groupKey: string, mode: PatternViewMode) => {
    setPatternViewMode((prev) => new Map([...prev, [groupKey, mode]]));
  };

  // When LogView requests a jump to a specific series group, select it, then clear the request
  // so the same groupKey can be requested again (same pattern as requestedPatternFilter in LogView).
  // Virtual series (pattern counts) use the :count aggregation, so switch to it.
  // Default to rate-sec view for pattern series.
  useEffect(() => {
    if (!requestedFocusedGroupKey) return;
    setAggregationType('count');
    setSelectedGroups((prev) => new Set([...prev, requestedFocusedGroupKey]));
    if (getPatternHash(requestedFocusedGroupKey)) {
      setPatternViewMode((prev) => new Map([...prev, [requestedFocusedGroupKey, 'rate-sec']]));
    }
    onRequestedFocusedGroupKeyConsumed?.();
  }, [requestedFocusedGroupKey, onRequestedFocusedGroupKeyConsumed]);

  // Auto-set rate-sec view for pattern groups when they're newly selected from the sidebar.
  const prevSelectedForRateRef = useRef<Set<string>>(new Set());
  useEffect(() => {
    const prev = prevSelectedForRateRef.current;
    const added = [...selectedGroups].filter((k) => !prev.has(k) && getPatternHash(k) !== null);
    if (added.length > 0) {
      setPatternViewMode((m) => {
        const next = new Map(m);
        for (const k of added) if (!next.has(k)) next.set(k, 'rate-sec');
        return next;
      });
    }
    prevSelectedForRateRef.current = new Set(selectedGroups);
  }, [selectedGroups]);

  // Fetch log patterns to annotate pattern metric series with human-readable labels.
  const [logPatternByHash, setLogPatternByHash] = useState<Map<string, string>>(new Map());
  useEffect(() => {
    if (state.connectionState !== 'ready') return;
    api.getLogPatterns().then((patterns) => {
      setLogPatternByHash(new Map(patterns.map((p) => [p.hash, p.patternString])));
    }).catch(console.error);
  }, [state.connectionState, state.activeScenario, state.scenarioDataVersion]);

  const scenarios = state.scenarios ?? [];
  const components = state.components ?? [];
  const allSeries = state.series ?? [];
  const allAnomalies = state.anomalies ?? [];

  const detectorComponents = useMemo(
    () => components.filter((c) => c.category === 'detector'),
    [components]
  );

  // Derive enabled detectors from backend component state (source of truth)
  const enabledDetectors = useMemo(
    () => new Set(detectorComponents.filter((c) => c.enabled).map((c) => c.name)),
    [detectorComponents]
  );

  const anomalies = useMemo(
    () => allAnomalies.filter((a) => enabledDetectors.has(getDetectorComponent(a))),
    [allAnomalies, enabledDetectors]
  );

  // Map comp.name (detectorComponent) → detectorName used by the timeline for coloring
  const detectorNameByComponent = useMemo(() => {
    const map = new Map<string, string>();
    for (const a of allAnomalies) {
      const component = getDetectorComponent(a);
      if (!map.has(component)) map.set(component, a.detectorName);
    }
    return map;
  }, [allAnomalies]);

  const tagGroups = useMemo(() => {
    const all = extractTagGroups(allSeries.map((s) => s.tags));
    return new Map([...all.entries()].filter(([k]) => MAIN_TAG_FILTER_KEYS.has(k)));
  }, [allSeries]);

  const filteredSeries = useMemo(
    () =>
      allSeries.filter((s) => {
        const agg = getAggregationType(s.name);
        const baseName = getBaseMetricName(s.name);
        if (s.metricKind === 'counter') {
          return agg === 'sum' || agg === 'avg' || agg === 'count';
        }
        if (isPatternCounterBaseName(baseName)) {
          return agg === 'count';
        }
        return agg === aggregationType;
      }),
    [allSeries, aggregationType]
  );

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
          virtual: s.virtual === true,
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

  const telemetryGroups = useMemo(
    () => visibleGroups.filter((g) => g.namespace === 'telemetry'),
    [visibleGroups]
  );

  const virtualGroups = useMemo(
    () => visibleGroups.filter((g) => g.virtual),
    [visibleGroups]
  );

  const displayGroups = useMemo(
    () =>
      visibleGroups.map((g) => ({
        key: g.key,
        name: g.baseName,
        displayName: g.baseName,
        virtual: g.virtual,
      })),
    [visibleGroups]
  );

  const tagFilter = useMemo(() => parseTagFilter(tagFilterInput), [tagFilterInput]);

  const initializedScenarioRef = useRef<string | null>(null);
  useEffect(() => {
    if (state.activeScenario && initializedScenarioRef.current !== state.activeScenario) {
      initializedScenarioRef.current = state.activeScenario;
      setTagFilterInput('');
    }
  }, [state.activeScenario]);

  const [autoSelectedScenario, setAutoSelectedScenario] = useState<string | null>(null);
  useEffect(() => {
    if (!state.activeScenario || state.connectionState !== 'ready') return;
    if (autoSelectedScenario === state.activeScenario) return;
    const scenarioHasSeries = (state.status?.seriesCount ?? 0) > 0;
    if (scenarioHasSeries && allSeries.length === 0) return;

    const mainGroups = [...visibleGroups]
      .filter((g) => g.namespace !== 'telemetry' && !g.virtual);

    const DEFAULT_METRIC = 'datadog.dogstatsd.client.metrics';
    const defaultGroup = mainGroups.find((g) => g.baseName === DEFAULT_METRIC);
    const defaultKeys = defaultGroup
      ? [defaultGroup.key]
      : mainGroups
          .sort((a, b) => {
            const countDiff = (anomalyCountByGroup.get(b.key) ?? 0) - (anomalyCountByGroup.get(a.key) ?? 0);
            if (countDiff !== 0) return countDiff;
            return a.baseName.localeCompare(b.baseName);
          })
          .slice(0, 6)
          .map((g) => g.key);

    setSelectedGroups(new Set(defaultKeys));
    setGroupSeriesData(new Map());
    setAutoSelectedScenario(state.activeScenario);
    onTimeRangeChange(null);
  }, [
    state.activeScenario,
    state.connectionState,
    state.status?.seriesCount,
    visibleGroups,
    anomalyCountByGroup,
    autoSelectedScenario,
    onTimeRangeChange,
    allSeries.length,
  ]);

  useEffect(() => {
    prevSelectedGroupsRef.current = new Set();
    setGroupSeriesData(new Map());
  }, [state.activeScenario]);

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

    let cancelled = false;
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
      if (!cancelled) {
        setGroupSeriesData(next);
      }
    };

    fetchSeriesData();
    return () => {
      cancelled = true;
    };
  }, [selectedGroups, state.connectionState, state.activeScenario, groupByKey, groupSeriesData.size]);

  const toggleDetector = (name: string) => {
    actions.toggleComponent(name);
  };

  const [expandedConfigs, setExpandedConfigs] = useState<Set<string>>(new Set());
  const toggleConfigPanel = (name: string) => {
    setExpandedConfigs(prev => {
      const next = new Set(prev);
      next.has(name) ? next.delete(name) : next.add(name);
      return next;
    });
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
        className="bg-slate-800 border-r border-slate-700 overflow-y-auto"
        style={{ width: sidebarWidth }}
      >
        <ScenarioSelector
          scenarios={scenarios}
          activeScenario={state.activeScenario}
          onLoadScenario={actions.loadScenario}
        />

        <div className="p-4 border-b border-slate-700">
          <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">
            Detectors
          </h2>
          <div className="space-y-1">
            {detectorComponents.map((comp) => {
              const count = allAnomalies.filter((a) => getDetectorComponent(a) === comp.name).length;
              const detectorName = detectorNameByComponent.get(comp.name);
              const color = detectorName ? getDetectorColorStable(detectorName) : null;
              const configEntries = comp.config ? Object.entries(comp.config) : [];
              const isExpanded = expandedConfigs.has(comp.name);
              return (
                <div key={comp.name}>
                  <label className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-slate-700 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={enabledDetectors.has(comp.name)}
                      onChange={() => toggleDetector(comp.name)}
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
                    {configEntries.length > 0 && (
                      <button
                        type="button"
                        onClick={e => { e.preventDefault(); toggleConfigPanel(comp.name); }}
                        title={isExpanded ? 'Hide config' : 'Show config'}
                        className="text-slate-500 hover:text-slate-300 transition-colors text-xs leading-none"
                      >
                        {isExpanded ? '▴' : '▾'}
                      </button>
                    )}
                  </label>
                  {isExpanded && configEntries.length > 0 && (
                    <div className="ml-6 mb-1 px-2 py-1.5 bg-slate-900/60 rounded border border-slate-700/50 font-mono text-xs space-y-0.5">
                      {configEntries.map(([k, v]) => (
                        <div key={k} className="flex gap-2">
                          <span className="text-slate-500 shrink-0">{k}</span>
                          <span className="text-slate-300">{String(v)}</span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
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
          <div className="flex items-center justify-between mb-2">
            <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider">
              Metric Groups ({displayGroups.length})
            </h2>
            <div className="flex gap-1">
              <button
                onClick={() => {
                  const telKeys = telemetryGroups.map((g) => g.key);
                  const anySelected = telKeys.some((k) => selectedGroups.has(k));
                  setSelectedGroups((prev) => {
                    const next = new Set(prev);
                    if (anySelected) {
                      telKeys.forEach((k) => next.delete(k));
                    } else {
                      telKeys.forEach((k) => next.add(k));
                    }
                    return next;
                  });
                }}
                className={`text-xs px-1.5 py-0.5 rounded font-bold transition-colors ${
                  telemetryGroups.some((g) => selectedGroups.has(g.key))
                    ? 'bg-purple-600 text-white'
                    : 'bg-slate-700 text-slate-400 hover:bg-slate-600'
                }`}
                title={telemetryGroups.some((g) => selectedGroups.has(g.key)) ? 'Deselect telemetry groups' : 'Select telemetry groups'}
              >
                T
              </button>
              <button
                onClick={() => {
                  const virtKeys = virtualGroups.map((g) => g.key);
                  const anySelected = virtKeys.some((k) => selectedGroups.has(k));
                  setSelectedGroups((prev) => {
                    const next = new Set(prev);
                    if (anySelected) {
                      virtKeys.forEach((k) => next.delete(k));
                    } else {
                      virtKeys.forEach((k) => next.add(k));
                    }
                    return next;
                  });
                }}
                className={`text-xs px-1.5 py-0.5 rounded font-bold transition-colors ${
                  virtualGroups.some((g) => selectedGroups.has(g.key))
                    ? 'bg-cyan-600 text-white'
                    : 'bg-slate-700 text-slate-400 hover:bg-slate-600'
                }`}
                title={virtualGroups.some((g) => selectedGroups.has(g.key)) ? 'Deselect virtual groups' : 'Select virtual groups'}
              >
                V
              </button>
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

        <div className="p-4 border-t border-slate-700">
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
          <TagFilterGroups
            tagGroups={tagGroups}
            tagFilterInput={tagFilterInput}
            onToggleTag={(tag) => setTagFilterInput(toggleTagInInput(tagFilterInput, tag))}
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
              <>
                {/* Regular metric groups */}
                {(() => {
                  const cards = Array.from(selectedGroups)
                    .filter((key) => {
                      const g = groupByKey.get(key);
                      return g && g.namespace !== 'telemetry' && !g.virtual;
                    })
                    .map((groupKey) => {
                      const dataList = groupSeriesData.get(groupKey) ?? [];
                      if (dataList.length === 0) return null;
                      const tagFiltered = (tagFilter.include.size > 0 || tagFilter.exclude.size > 0)
                        ? dataList.filter((d) => matchesTagFilter(d.tags ?? [], tagFilter))
                        : dataList;
                      const chartSeries = showAnomalyOnlySeriesLines
                        ? tagFiltered.filter((d) => (anomalyCountBySeriesID.get(d.id) ?? 0) > 0)
                        : tagFiltered;
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
                          enabledDetectors={enabledDetectors}
                          timeRange={timeRange}
                          onTimeRangeChange={onTimeRangeChange}
                          smoothLines={smoothLines}
                          seriesVariants={seriesVariants}
                          phaseMarkers={phaseMarkers}
                        />
                      );
                    })
                    .filter(Boolean);
                  return cards.length > 0 ? (
                    <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">{cards}</div>
                  ) : null;
                })()}

                {/* Virtual metric groups — shown after a separator */}
                {virtualGroups.some((g) => selectedGroups.has(g.key)) && (
                  <>
                    <div className="flex items-center gap-3">
                      <div className="flex-1 border-t border-cyan-800/50" />
                      <div className="flex items-center gap-1.5 text-xs text-cyan-400 font-medium">
                        <span className="inline-flex items-center justify-center w-4 h-4 rounded-full bg-cyan-600 text-white text-[9px] font-bold">V</span>
                        Virtual Metrics
                      </div>
                      <div className="flex-1 border-t border-cyan-800/50" />
                    </div>
                    <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                      {Array.from(selectedGroups)
                        .filter((key) => {
                          const g = groupByKey.get(key);
                          return g && g.virtual;
                        })
                        .map((groupKey) => {
                          const dataList = groupSeriesData.get(groupKey) ?? [];
                          if (dataList.length === 0) return null;
                          const chartSeries = showAnomalyOnlySeriesLines
                            ? dataList.filter((d) => (anomalyCountBySeriesID.get(d.id) ?? 0) > 0)
                            : dataList;
                          if (chartSeries.length === 0) return null;
                          const seriesIDs = new Set(chartSeries.map((d) => d.id));
                          const seriesAnomalies = anomalies.filter((a) => a.sourceSeriesId && seriesIDs.has(a.sourceSeriesId));
                          const anomalyMarkers = chartSeries.flatMap((d) => d.anomalies);
                          const patternHash = getPatternHash(groupKey);
                          const isPatternSeries = patternHash !== null;
                          const viewMode: PatternViewMode = (isPatternSeries && patternViewMode.get(groupKey)) || 'raw';
                          const transformPoints =
                            viewMode === 'rate-sec' ? toRateSeries :
                            viewMode === 'rate-min' ? toRatePerMinSeries :
                            (pts: Point[]) => pts;
                          const seriesVariants: SeriesVariant[] = chartSeries.map((d) => ({
                            label: formatSeriesLabel(d.tags),
                            points: transformPoints(d.points),
                            seriesId: d.id,
                          }));
                          const primary = chartSeries[0];
                          const primaryPoints = transformPoints(primary.points);
                          const patternString = patternHash ? logPatternByHash.get(patternHash) : undefined;
                          const modeLabel = viewMode === 'rate-sec' ? ' (evt/s)' : viewMode === 'rate-min' ? ' (evt/min)' : '';
                          const subtitle = (patternString || isPatternSeries) ? (
                            <div className="space-y-2">
                              {patternString && (
                                <div>
                                  <div className="text-[10px] text-slate-500 uppercase tracking-wide mb-1">Log Pattern</div>
                                  <pre className="text-xs text-slate-200 font-mono bg-slate-900/60 rounded px-2 py-1 whitespace-pre-wrap break-all">
                                    {patternString}
                                  </pre>
                                </div>
                              )}
                              <div className="flex items-center gap-2">
                                <div className="flex rounded overflow-hidden border border-slate-600 text-xs">
                                  {(
                                    [
                                      { mode: 'raw' as PatternViewMode, label: 'Raw', title: 'Count per bucket' },
                                      { mode: 'rate-sec' as PatternViewMode, label: 'evt/s', title: 'Events per second (count ÷ bucket size)' },
                                      { mode: 'rate-min' as PatternViewMode, label: 'evt/min', title: 'Events per minute (60 s sliding window)' },
                                    ] as const
                                  ).map(({ mode, label, title }) => (
                                    <button
                                      key={mode}
                                      onClick={() => setGroupViewMode(groupKey, mode)}
                                      className={`px-2.5 py-1 transition-colors ${viewMode === mode ? 'bg-cyan-700 text-white' : 'bg-slate-800 text-slate-400 hover:bg-slate-700'}`}
                                      title={title}
                                    >
                                      {label}
                                    </button>
                                  ))}
                                </div>
                                <div className="flex-1" />
                                {onJumpToPattern && patternHash && (
                                  <button
                                    onClick={() => onJumpToPattern(patternHash)}
                                    className="text-xs px-2.5 py-1.5 rounded bg-slate-700 text-slate-300 hover:bg-slate-600 transition-colors"
                                    title="Filter logs by this pattern in the Logs tab"
                                  >
                                    ↗ View in logs
                                  </button>
                                )}
                              </div>
                            </div>
                          ) : undefined;
                          return (
                            <ChartWithAnomalyDetails
                              key={`${groupKey}-${viewMode}`}
                              name={`${primary.name}${modeLabel}`}
                              points={primaryPoints}
                              anomalyMarkers={anomalyMarkers}
                              anomalies={seriesAnomalies}
                              correlationRanges={[]}
                              enabledDetectors={enabledDetectors}
                              timeRange={timeRange}
                              onTimeRangeChange={onTimeRangeChange}
                              smoothLines={smoothLines}
                              seriesVariants={seriesVariants}
                              phaseMarkers={phaseMarkers}
                              subtitle={subtitle}
                            />
                          );
                        })}
                    </div>
                  </>
                )}

                {/* Telemetry metric groups — shown after a separator */}
                {telemetryGroups.some((g) => selectedGroups.has(g.key)) && (
                  <>
                    <div className="flex items-center gap-3">
                      <div className="flex-1 border-t border-purple-800/50" />
                      <div className="flex flex-col items-center gap-0.5 shrink-0">
                        <div className="flex items-center gap-1.5 text-xs text-purple-400 font-medium">
                          <span className="inline-flex items-center justify-center w-4 h-4 rounded-full bg-purple-600 text-white text-[9px] font-bold">T</span>
                          Telemetry Metrics
                        </div>
                        <span
                          className="text-[10px] text-slate-500 max-w-md text-center px-2"
                          title="Counter metrics: sum of deltas per time bucket, displayed as a running total from scenario start. Gauges follow the aggregation control."
                        >
                          Counters: cumulative from start · Gauges: aggregation above
                        </span>
                      </div>
                      <div className="flex-1 border-t border-purple-800/50" />
                    </div>
                    <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                      {Array.from(selectedGroups)
                        .filter((key) => {
                          const g = groupByKey.get(key);
                          return g && g.namespace === 'telemetry';
                        })
                        .map((groupKey) => {
                          const group = groupByKey.get(groupKey);
                          const isCounterTelemetry = group?.members.some((m) => m.metricKind === 'counter');
                          const dataList = groupSeriesData.get(groupKey) ?? [];
                          if (dataList.length === 0) return null;
                          const chartSeries = showAnomalyOnlySeriesLines
                            ? dataList.filter((d) => (anomalyCountBySeriesID.get(d.id) ?? 0) > 0)
                            : dataList;
                          if (chartSeries.length === 0) return null;
                          const seriesIDs = new Set(chartSeries.map((d) => d.id));
                          const seriesAnomalies = anomalies.filter((a) => a.sourceSeriesId && seriesIDs.has(a.sourceSeriesId));
                          const anomalyMarkers = chartSeries.flatMap((d) => d.anomalies);
                          const mapPoints = (pts: Point[]) =>
                            isCounterTelemetry ? cumulativeFromStart(pts) : pts;
                          const seriesVariants: SeriesVariant[] = chartSeries.map((d) => ({
                            label: formatSeriesLabel(d.tags),
                            points: mapPoints(d.points),
                            seriesId: d.id,
                          }));
                          const primary = chartSeries[0];
                          const chartTitleBase = getBaseMetricName(primary.name);
                          const chartTitle = isCounterTelemetry ? `${chartTitleBase} (cumulative)` : primary.name;
                          return (
                            <ChartWithAnomalyDetails
                              key={groupKey}
                              name={chartTitle}
                              points={mapPoints(primary.points)}
                              anomalyMarkers={anomalyMarkers}
                              anomalies={seriesAnomalies}
                              correlationRanges={[]}
                              enabledDetectors={enabledDetectors}
                              timeRange={timeRange}
                              onTimeRangeChange={onTimeRangeChange}
                              smoothLines={smoothLines}
                              seriesVariants={seriesVariants}
                              isTelemetry
                              phaseMarkers={phaseMarkers}
                            />
                          );
                        })}
                    </div>
                  </>
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
