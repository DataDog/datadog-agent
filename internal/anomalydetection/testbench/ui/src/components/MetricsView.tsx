import { ScenarioSelector } from './ScenarioSelector';
import { useState, useEffect, useMemo, useRef, useCallback, memo } from 'react';
import { ChartWithAnomalyDetails } from './ChartWithAnomalyDetails';
import { SeriesTree } from './SeriesTree';
import { api } from '../api/client';
import type { SeriesData, SeriesInfo, Point, SeriesID, Anomaly } from '../api/client';
import type { SeriesVariant } from './MetricsChart';
import { getDetectorColorStable } from './MetricsChart';
import type { TimeRange, PhaseMarker } from './ChartWithAnomalyDetails';
import type { ObserverState, ObserverActions } from '../hooks/useObserver';
import { MAIN_TAG_FILTER_KEYS } from '../constants';
import { parseTagFilter, extractTagGroups, toggleTagInInput, matchesTagFilter } from '../filters';
import { TagFilterGroups } from './TagFilterGroups';
import type { ParsedTagFilter } from '../filters';
import { countHitsBySeries } from '../investigation';
import { inspectionWindow, recordingWindow, selectMetricGroups, INSPECTION_LABELS } from '../metricInspection';
import type { InspectionScope } from '../metricInspection';

const MAX_METRIC_GROUPS = 50;
const INITIAL_METRIC_GROUPS = 5;

function formatHits(count: number): string { return `${count} detector hit${count === 1 ? '' : 's'}`; }

const AGGREGATION_TYPES = ['avg', 'count', 'sum'] as const;
type AggregationType = typeof AGGREGATION_TYPES[number];

function getBaseMetricName(name: string): string {
  const match = name.match(/^(.+):(avg|sum|count)$/);
  return match ? match[1] : name;
}

function getAggregationType(name: string): AggregationType | null {
  const match = name.match(/:(avg|sum|count)$/);
  return match ? (match[1] as AggregationType) : null;
}

function isPatternCounterBaseName(baseName: string): boolean {
  return /^log\.log_pattern_extractor\.[0-9a-f]+\.count$/.test(baseName);
}

function getDetectorComponent(anomaly: { detectorName: string; detectorComponent?: string }): string {
  return anomaly.detectorComponent ?? anomaly.detectorName;
}

function effectiveSeriesTags(tags: string[] | null | undefined, host?: string): string[] {
	const baseTags = tags ?? [];
	return host ? [`host:${host}`, ...baseTags] : baseTags;
}

function formatSeriesLabel(tags: string[] | null | undefined, host?: string): string {
	const displayTags = effectiveSeriesTags(tags, host);
	if (displayTags.length === 0) return 'untagged';
	return displayTags.join(', ');
}

function formatMetricTypes(members: SeriesInfo[]): string[] {
  return [...new Set(members.flatMap((member) => member.metricTypes ?? []))].sort();
}

function filterCachedSeriesData(dataList: SeriesData[], group: MetricGroup): SeriesData[] {
  const currentSeriesIDs = new Set(group.members.map((member) => member.id));
  return dataList.filter((data) => currentSeriesIDs.has(data.id));
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

const EMPTY_SERIES_DATA: SeriesData[] = [];
interface RegularMetricCardProps {
  group: MetricGroup;
  data: SeriesData[];
  tagFilter: ParsedTagFilter;
  showAnomalyOnlySeriesLines: boolean;
  focusedSeriesID: string | null;
  anomalyCountBySeriesID: Map<SeriesID, number>;
  anomalies: Anomaly[];
  selectedMetricTypes: Set<string>;
  toggleMetricType: (type: string) => void;
  enabledDetectors: Set<string>;
  timeRange: TimeRange | null;
  onTimeRangeChange: (range: TimeRange | null) => void;
  smoothLines: boolean;
  phaseMarkers?: PhaseMarker[];
  hitCount: number;
  rankingLabel: string;
  inspectionRange: TimeRange | null;
  onInspectRange: (range: TimeRange) => void;
  pinned: boolean;
  onTogglePin: (key: string) => void;
  pinLimitReached: boolean;
}
// Loading another group must not redraw every chart already on the page.
const RegularMetricCard = memo(function RegularMetricCard({ group, data, tagFilter,
  showAnomalyOnlySeriesLines, focusedSeriesID, anomalyCountBySeriesID, anomalies,
  selectedMetricTypes, toggleMetricType, enabledDetectors, timeRange, onTimeRangeChange,
  smoothLines, phaseMarkers, hitCount, rankingLabel, inspectionRange, onInspectRange, pinned, onTogglePin, pinLimitReached,
}: RegularMetricCardProps) {
  const focusedGroup = focusedSeriesID && group.members.some(member => member.id === focusedSeriesID);
  const chartSeries = filterCachedSeriesData(data, group).filter(series =>
    (!focusedGroup || series.id === focusedSeriesID) &&
    matchesTagFilter(effectiveSeriesTags(series.tags, series.host), tagFilter) &&
    (!showAnomalyOnlySeriesLines || (anomalyCountBySeriesID.get(series.id) ?? 0) > 0)
  );
  if (chartSeries.length === 0) return null;
  const seriesIDs = new Set(chartSeries.map((d) => d.id));
  const seriesAnomalies = anomalies.filter((a) => a.sourceSeriesId && seriesIDs.has(a.sourceSeriesId));
  const anomalyMarkers = chartSeries.flatMap((d) => d.anomalies ?? []);
  const metricTypes = formatMetricTypes(group.members.filter((member) => seriesIDs.has(member.id)));
  const metricTypeSubtitle = metricTypes.length > 0 ? (
    <div className="flex items-center gap-1.5 flex-wrap text-xs text-slate-400">
      <span>{metricTypes.length === 1 ? 'Metric type:' : 'Metric types:'}</span>
      {metricTypes.map((type) => {
        const active = selectedMetricTypes.has(type);
        return (
          <button
            key={type}
            type="button"
            aria-pressed={active}
            onClick={() => toggleMetricType(type)}
            className={`text-[10px] px-1.5 py-0.5 rounded font-mono transition-colors ${
              active ? 'bg-indigo-600/50 text-indigo-200 ring-1 ring-indigo-400/60' : 'bg-slate-700 text-slate-300 hover:bg-indigo-600/40 hover:text-indigo-200'
            }`}
          >
            {type}
          </button>
        );
      })}
    </div>
  ) : undefined;
  const seriesVariants: SeriesVariant[] = chartSeries.map((d) => ({
    label: formatSeriesLabel(d.tags, d.host),
    points: d.points,
    seriesId: d.id,
  }));
  const primary = chartSeries[0];
  return (
    <ChartWithAnomalyDetails
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
      inspectionRange={inspectionRange}
      onInspectRange={onInspectRange}
      subtitle={<div className="space-y-1"><div className="flex items-center justify-between gap-3 text-xs text-slate-400"><span>{formatHits(focusedSeriesID && group.members.some(member => member.id === focusedSeriesID) ? anomalyCountBySeriesID.get(focusedSeriesID as SeriesID) ?? 0 : hitCount)} across {chartSeries.length} tagged series · {rankingLabel}</span><button type="button" aria-label={`${pinned ? 'Unpin' : 'Pin'} metric ${group.baseName}`} aria-pressed={pinned} disabled={!pinned && pinLimitReached} onClick={() => onTogglePin(group.key)} className={`shrink-0 hover:underline disabled:opacity-40 ${pinned ? 'text-purple-300' : 'text-slate-300'}`}>{pinned ? 'Pinned · Unpin' : 'Pin metric'}</button></div>{metricTypeSubtitle}</div>}
    />
  );
});

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
  requestedSeries?: SeriesInfo | null;
  onRequestedSeriesConsumed?: () => void;
  onBackToScorer?: () => void;
  onResumeInspection?: () => void;
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
  requestedSeries,
  onRequestedSeriesConsumed,
  onBackToScorer,
  onResumeInspection,
}: MetricsViewProps) {
  const [pinnedGroups, updatePinnedGroups] = useState<Set<string>>(new Set());
  const setPinnedGroups = useCallback((selection: Set<string> | ((current: Set<string>) => Set<string>)) => {
    updatePinnedGroups(current => new Set([...(typeof selection === 'function' ? selection(current) : selection)].slice(0, MAX_METRIC_GROUPS)));
  }, []);
  const [rankedLimit, setRankedLimit] = useState(INITIAL_METRIC_GROUPS);
  const [inspectionScope, setInspectionScope] = useState<InspectionScope>('recording');
  const [customWindow, setCustomWindow] = useState<TimeRange | null>(null);
  const [focusedSeriesID, setFocusedSeriesID] = useState<string | null>(null);
  const [focusedGroupKey, setFocusedGroupKey] = useState<string | null>(null);
  const [selectedMetricTypes, setSelectedMetricTypes] = useState<Set<string>>(new Set());
  const [groupSeriesData, setGroupSeriesData] = useState<Map<string, SeriesData[]>>(new Map());
  const seriesDataCacheRef = useRef(new Map<string, SeriesData>());
  const cacheVersionRef = useRef(0);
  const groupDataCacheRef = useRef(new Map<string, SeriesData[]>());
  const [groupErrors, setGroupErrors] = useState<Map<string, string>>(new Map());
  const [retryVersion, setRetryVersion] = useState(0);
  const [aggregationType, setAggregationType] = useState<AggregationType>('avg');
  const [showAnomalyOnlyGroups, setShowAnomalyOnlyGroups] = useState(false);
  const [showAnomalyOnlySeriesLines, setShowAnomalyOnlySeriesLines] = useState(false);
  const [tagFilterInput, setTagFilterInput] = useState('');
  const [metricSearch, setMetricSearch] = useState('');

  const focusContextRef = useRef<{ aggregation: AggregationType; types: Set<string>; tags: string; onlyHits: boolean } | null>(null);
  const saveFocusContext = useCallback(() => {
    if (!focusContextRef.current) focusContextRef.current = {
      aggregation: aggregationType, types: selectedMetricTypes, tags: tagFilterInput, onlyHits: showAnomalyOnlySeriesLines,
    };
  }, [aggregationType, selectedMetricTypes, tagFilterInput, showAnomalyOnlySeriesLines]);
  const clearSourceFocus = useCallback(() => {
    const previous = focusContextRef.current;
    focusContextRef.current = null;
    if (previous) {
      setAggregationType(previous.aggregation);
      setSelectedMetricTypes(previous.types);
      setTagFilterInput(previous.tags);
      setShowAnomalyOnlySeriesLines(previous.onlyHits);
    }
    setFocusedGroupKey(null);
    setFocusedSeriesID(null);
  }, []);

  // Parse a pattern metric group key and return its log pattern hash, or null.
  // Current: "log_pattern_extractor/log.log_pattern_extractor.<hash>.count"
  // Legacy:  "parquet/_virtual.log.log_pattern_extractor.<hash>.count"
  const getPatternHash = (groupKey: string): string | null => {
    const m = groupKey.match(/^log_pattern_extractor\/log\.log_pattern_extractor\.([0-9a-f]+)\.count$/);
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
  const allAnomalies = useMemo(() => state.anomalies ?? [], [state.anomalies]);
  const useMetricPipeline = state.status?.serverConfig?.useMetricPipeline === true;

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
    () => useMetricPipeline
      ? allAnomalies
      : allAnomalies.filter((a) => enabledDetectors.has(getDetectorComponent(a))),
    [allAnomalies, enabledDetectors, useMetricPipeline]
  );

  const visibleDetectors = useMemo(
    () => useMetricPipeline
      ? new Set(allAnomalies.map(getDetectorComponent))
      : enabledDetectors,
    [allAnomalies, enabledDetectors, useMetricPipeline]
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
	const all = extractTagGroups(allSeries.map((s) => effectiveSeriesTags(s.tags, s.host)));
    return new Map([...all.entries()].filter(([k]) => MAIN_TAG_FILTER_KEYS.has(k)));
  }, [allSeries]);

  const metricTypeOptions = useMemo(
    () => [...new Set(allSeries.flatMap((series) => series.metricTypes ?? []))].sort(),
    [allSeries]
  );

  const toggleMetricType = useCallback((metricType: string) => {
    setSelectedMetricTypes((current) => {
      const next = new Set(current);
      if (next.has(metricType)) next.delete(metricType);
      else next.add(metricType);
      return next;
    });
  }, []);

  const filteredSeries = useMemo(
    () =>
      allSeries.filter((s) => {
        if (selectedMetricTypes.size > 0 && !(s.metricTypes ?? []).some((type) => selectedMetricTypes.has(type))) {
          return false;
        }
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
    [allSeries, aggregationType, selectedMetricTypes]
  );

  const allMetricGroups = useMemo(() => {
    const groups = new Map<string, MetricGroup>();
    allSeries.forEach((s) => {
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
  }, [allSeries]);

  const metricGroups = useMemo(() => {
    const filteredSeriesIDs = new Set(filteredSeries.map((series) => series.id));
    return allMetricGroups
      .map((group) => ({ ...group, members: group.members.filter((member) => filteredSeriesIDs.has(member.id)) }))
      .filter((group) => group.members.length > 0);
  }, [allMetricGroups, filteredSeries]);

  const groupByKey = useMemo(() => {
    const map = new Map<string, MetricGroup>();
    metricGroups.forEach((g) => map.set(g.key, g));
    return map;
  }, [metricGroups]);

  const tagFilter = useMemo(() => parseTagFilter(tagFilterInput), [tagFilterInput]);
  const episode = state.status?.episodeInfo;
  const fullWindow = useMemo(() => recordingWindow(episode,
    state.status?.scenarioStart != null && state.status?.scenarioEnd != null
      ? { start: state.status.scenarioStart, end: state.status.scenarioEnd } : null),
  [episode, state.status?.scenarioStart, state.status?.scenarioEnd]);
  const rankingRange = useMemo(() => focusedGroupKey && timeRange ? timeRange : inspectionWindow(inspectionScope, episode, customWindow, fullWindow),
    [focusedGroupKey, timeRange, inspectionScope, episode, customWindow, fullWindow]);
  const rankingLabel = focusedGroupKey ? 'Source window' : INSPECTION_LABELS[inspectionScope];
  const highlightedWindow = focusedGroupKey || inspectionScope !== 'recording' ? rankingRange : null;
  const handleInspectRange = useCallback((range: TimeRange) => {
    setCustomWindow(range);
    setInspectionScope('custom');
    setRankedLimit(limit => limit || INITIAL_METRIC_GROUPS);
  }, []);
  const chooseInspectionScope = (scope: InspectionScope) => {
    clearSourceFocus();
    setInspectionScope(scope);
    setRankedLimit(limit => limit || INITIAL_METRIC_GROUPS);
    if (fullWindow && (timeRange?.start !== fullWindow.start || timeRange?.end !== fullWindow.end)) onTimeRangeChange(fullWindow);
  };
  const togglePin = useCallback((key: string) => {
    setPinnedGroups(current => {
      const next = new Set(current);
      if (next.has(key)) next.delete(key);
      else if (next.size < MAX_METRIC_GROUPS) next.add(key);
      return next;
    });
  }, [setPinnedGroups]);
  const anomalyCountBySeriesID = useMemo(() => countHitsBySeries(anomalies, rankingRange), [anomalies, rankingRange]);
  const anomalyCountByGroup = useMemo(() => new Map(metricGroups.map(group => [group.key,
    group.members.reduce((total, member) => total + (matchesTagFilter(effectiveSeriesTags(member.tags, member.host), tagFilter) ? anomalyCountBySeriesID.get(member.id) ?? 0 : 0), 0),
  ])), [metricGroups, anomalyCountBySeriesID, tagFilter]);

  const visibleGroups = useMemo(() => {
    if (!showAnomalyOnlyGroups) return metricGroups;
    return metricGroups.filter((g) => (anomalyCountByGroup.get(g.key) ?? 0) > 0);
  }, [metricGroups, showAnomalyOnlyGroups, anomalyCountByGroup]);

  const topAnomalousGroupKeys = useMemo(
    () => visibleGroups
      .filter((g) => g.namespace !== 'telemetry' && !g.virtual && (anomalyCountByGroup.get(g.key) ?? 0) > 0)
      .sort((a, b) => {
        const countDiff = (anomalyCountByGroup.get(b.key) ?? 0) - (anomalyCountByGroup.get(a.key) ?? 0);
        if (countDiff !== 0) return countDiff;
        return a.baseName.localeCompare(b.baseName);
      })
      .slice(0, MAX_METRIC_GROUPS)
      .map((g) => g.key),
    [visibleGroups, anomalyCountByGroup]
  );

  const selection = useMemo(() => selectMetricGroups(pinnedGroups, topAnomalousGroupKeys, rankedLimit, MAX_METRIC_GROUPS),
    [pinnedGroups, topAnomalousGroupKeys, rankedLimit]);
  const selectedGroups = useMemo(() => focusedGroupKey ? new Set([focusedGroupKey]) : selection.selected, [focusedGroupKey, selection.selected]);

  // When LogView requests a jump to a specific series group, select it, then clear the request
  // so the same groupKey can be requested again (same pattern as requestedPatternFilter in LogView).
  // Virtual series (pattern counts) use the :count aggregation, so switch to it.
  // Default to rate-sec view for pattern series.
  useEffect(() => {
    if (!requestedFocusedGroupKey) return;
    saveFocusContext();
    setSelectedMetricTypes(new Set());
    setTagFilterInput('');
    setShowAnomalyOnlySeriesLines(false);
    setAggregationType('count');
    setFocusedGroupKey(requestedFocusedGroupKey);
    setFocusedSeriesID(null);
    if (getPatternHash(requestedFocusedGroupKey)) {
      setPatternViewMode((prev) => new Map([...prev, [requestedFocusedGroupKey, 'rate-sec']]));
    }
    onRequestedFocusedGroupKeyConsumed?.();
  }, [requestedFocusedGroupKey, onRequestedFocusedGroupKeyConsumed, saveFocusContext]);

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

  const telemetryGroups = useMemo(
    () => metricGroups.filter((g) => g.namespace === 'telemetry'),
    [metricGroups]
  );

  const virtualGroups = useMemo(
    () => metricGroups.filter((g) => g.virtual),
    [metricGroups]
  );

  const displayGroups = useMemo(
    () =>
      visibleGroups.filter(group => `${group.namespace}/${group.baseName}`.toLowerCase().includes(metricSearch.trim().toLowerCase())).map((g) => ({
        key: g.key,
        name: g.baseName,
        displayName: g.baseName,
        virtual: g.virtual,
        metricTypes: formatMetricTypes(g.members),
      })),
    [visibleGroups, metricSearch]
  );

  const initializedScenarioRef = useRef<string | null>(null);
  useEffect(() => {
    if (state.activeScenario && initializedScenarioRef.current !== state.activeScenario) {
      initializedScenarioRef.current = state.activeScenario;
      setTagFilterInput('');
      setMetricSearch('');
      setSelectedMetricTypes(new Set());
    }
  }, [state.activeScenario]);

  const [selectionScenario, setSelectionScenario] = useState<string | null>(null);
  useEffect(() => {
    if (!state.activeScenario || selectionScenario === state.activeScenario) return;
    setPinnedGroups(new Set());
    focusContextRef.current = null;
    setAggregationType('avg');
    setShowAnomalyOnlySeriesLines(false);
    setRankedLimit(INITIAL_METRIC_GROUPS);
    setInspectionScope('recording');
    setCustomWindow(null);
    setFocusedGroupKey(null);
    setFocusedSeriesID(null);
    setGroupSeriesData(new Map());
    seriesDataCacheRef.current = new Map();
    groupDataCacheRef.current.clear();
    setSelectionScenario(state.activeScenario);
  }, [
    state.activeScenario,
    selectionScenario,
    onTimeRangeChange,
    setPinnedGroups,
  ]);

  useEffect(() => {
    if (!requestedSeries || state.connectionState !== 'ready' || selectionScenario !== state.activeScenario) return;
    const groupKey = `${requestedSeries.namespace}/${getBaseMetricName(requestedSeries.name)}`;
    saveFocusContext();
    setAggregationType(getAggregationType(requestedSeries.name) ?? 'avg');
    setSelectedMetricTypes(new Set());
    setTagFilterInput('');
    setShowAnomalyOnlySeriesLines(false);
    setFocusedGroupKey(groupKey);
    setFocusedSeriesID(requestedSeries.id);
    onRequestedSeriesConsumed?.();
  }, [requestedSeries, onRequestedSeriesConsumed, state.connectionState, selectionScenario, state.activeScenario, saveFocusContext]);

  const canShowMoreMetrics = !focusedGroupKey && selection.more > 0 && rankedLimit > 0;

  const selectedVisibleCount = [...selectedGroups].filter((key) => groupByKey.has(key)).length;

  useEffect(() => {
    if (selectionScenario !== state.activeScenario) return;
    if (selectedGroups.size === 0 || state.connectionState !== 'ready') {
      setGroupSeriesData(new Map());
      setGroupErrors(new Map());
      return;
    }

    if (cacheVersionRef.current !== state.scenarioDataVersion) {
      cacheVersionRef.current = state.scenarioDataVersion;
      seriesDataCacheRef.current.clear();
      groupDataCacheRef.current.clear();
      setGroupSeriesData(new Map());
    } else {
      setGroupSeriesData((current) => new Map(
        [...current].filter(([key]) => selectedGroups.has(key) && groupByKey.has(key))
      ));
    }
    setGroupErrors(new Map());
    const controller = new AbortController();
    const seriesCache = seriesDataCacheRef.current;
    const groups = [...selectedGroups].flatMap((key) => {
      const group = groupByKey.get(key);
      return group ? [group] : [];
    });
    let nextGroup = 0;

    // Bound requests across groups, load only the displayed aggregation, and
    // abort queued work when selection, recording, or detector results change.
    const worker = async () => {
      while (!controller.signal.aborted && nextGroup < groups.length) {
        const group = groups[nextGroup++];
        const members = focusedSeriesID && group.members.some(member => member.id === focusedSeriesID)
          ? group.members.filter(member => member.id === focusedSeriesID) : group.members;
        const cached = groupDataCacheRef.current.get(group.key);
        const cachedIDs = new Set(cached?.map(member => member.id));
        if (cached?.length === members.length && members.every(member => cachedIDs.has(member.id))) {
          setGroupSeriesData(current => current.get(group.key) === cached ? current : new Map(current).set(group.key, cached));
          continue;
        }
        const seriesList: SeriesData[] = [];
        let failure: string | null = null;
        for (const member of members) {
          if (controller.signal.aborted) return;
          try {
            let data = seriesCache.get(member.id);
            if (!data) {
              data = await api.getSeriesDataByID(member.id, controller.signal);
              if (controller.signal.aborted) return;
              seriesCache.set(member.id, data);
            }
            seriesList.push(data);
          } catch (error) {
            if (controller.signal.aborted) return;
            failure = error instanceof Error ? error.message : 'Request failed';
          }
        }
        if (controller.signal.aborted) return;
        if (failure) {
          setGroupErrors((current) => new Map(current).set(group.key, `${group.baseName}: ${failure}`));
        }
        if (!failure) groupDataCacheRef.current.set(group.key, seriesList);
        setGroupSeriesData((current) => {
          const previous = current.get(group.key);
          if (previous?.length === seriesList.length && previous.every((data, i) => data === seriesList[i])) return current;
          return new Map(current).set(group.key, seriesList);
        });
      }
    };

    void Promise.all(Array.from({ length: Math.min(8, groups.length) }, worker));
    return () => controller.abort();
  }, [selectedGroups, selectionScenario, state.connectionState, state.activeScenario, state.scenarioDataVersion, groupByKey, retryVersion, focusedSeriesID]);

  const toggleDetector = (name: string) => {
    actions.toggleComponent(name);
  };

  const [expandedConfigs, setExpandedConfigs] = useState<Set<string>>(new Set());
  const toggleConfigPanel = (name: string) => {
    setExpandedConfigs(prev => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
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
    <div className="flex-1 flex min-w-0">
      <aside
        className="bg-slate-800 border-r border-slate-700 overflow-y-auto shrink-0"
        style={{ width: sidebarWidth }}
      >
        <ScenarioSelector
          scenarios={scenarios}
          activeScenario={state.activeScenario}
          onLoadScenario={actions.loadScenario}
          busy={state.connectionState === 'loading' || state.pendingComponents.size > 0}
        />

        <div className="p-4 border-b border-slate-700">
          <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">
            {useMetricPipeline ? 'metrics pipeline' : 'legacy pipeline'}
          </h2>
          <div className="space-y-1">
            {detectorComponents.map((comp) => {
              const count = allAnomalies.filter((a) => getDetectorComponent(a) === comp.name).length;
              const detectorName = detectorNameByComponent.get(comp.name);
              const color = detectorName ? getDetectorColorStable(detectorName) : null;
              const configEntries = comp.config ? Object.entries(comp.config) : [];
              const isExpanded = expandedConfigs.has(comp.name);
              const updating = state.pendingComponents.has(comp.name);
              return (
                <div key={comp.name}>
                  <label aria-busy={updating} className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-slate-700 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={enabledDetectors.has(comp.name)}
                      disabled={state.pendingComponents.size > 0 || state.connectionState === 'loading'}
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
                    {updating ? (
                      <span role="status" aria-label={`${comp.displayName} updating`} className="flex items-center gap-1.5 text-xs text-purple-300 shrink-0">
                        <span aria-hidden="true" className="h-3.5 w-3.5 rounded-full border-2 border-purple-400/30 border-t-purple-300 animate-spin motion-reduce:animate-none" />
                        Updating…
                      </span>
                    ) : count > 0 && (
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

        {state.status?.baseline?.enabled && (
          <div className="p-4 border-b border-slate-700">
            <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2">
              Baseline analysis
            </h2>
            {state.status.baseline.active ? (
              <div className="flex items-center gap-2 text-xs text-amber-400">
                <span className="inline-block w-2 h-2 rounded-full bg-amber-400 animate-pulse" />
                <span>Window active · {Math.round(state.status.baseline.durationSec / 60)}m</span>
              </div>
            ) : (
              <div className="space-y-1">
                <div className="flex items-center gap-2 text-xs text-slate-300">
                  <span className="text-green-400">✓</span>
                  <span>
                    {(state.status.baseline.mutedSeries?.length ?? 0)} series muted from anomaly detection
                  </span>
                </div>
                {(state.status.baseline.mutedSeries?.length ?? 0) > 0 && (
                  <MutedSeriesList series={state.status.baseline.mutedSeries!} />
                )}
              </div>
            )}
          </div>
        )}

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
          {metricTypeOptions.length > 0 && (
            <div>
              <div className="flex items-center justify-between mb-2">
                <h3 className="text-xs font-semibold text-slate-400">Metric type</h3>
                {selectedMetricTypes.size > 0 && (
                  <button
                    type="button"
                    onClick={() => setSelectedMetricTypes(new Set())}
                    className="text-[10px] text-slate-500 hover:text-slate-300"
                  >
                    Clear
                  </button>
                )}
              </div>
              <div className="flex gap-1 flex-wrap">
                {metricTypeOptions.map((type) => {
                  const active = selectedMetricTypes.has(type);
                  return (
                    <button
                      key={type}
                      type="button"
                      aria-pressed={active}
                      onClick={() => toggleMetricType(type)}
                      className={`text-[10px] px-1.5 py-0.5 rounded font-mono transition-colors ${
                        active ? 'bg-indigo-600/50 text-indigo-200 ring-1 ring-indigo-400/60' : 'bg-slate-700 text-slate-400 hover:bg-slate-600 hover:text-slate-300'
                      }`}
                    >
                      {type}
                    </button>
                  );
                })}
              </div>
            </div>
          )}
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
              Pin metrics ({displayGroups.length})
            </h2>
          </div>
            <div className="flex flex-wrap gap-1 mb-2">
              <button
                onClick={() => {
                  const telKeys = telemetryGroups.map((g) => g.key);
                  const anySelected = telKeys.some((k) => pinnedGroups.has(k));
                  setPinnedGroups((prev) => {
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
                  telemetryGroups.some((g) => pinnedGroups.has(g.key))
                    ? 'bg-purple-600 text-white'
                    : 'bg-slate-700 text-slate-400 hover:bg-slate-600'
                }`}
                title={telemetryGroups.some((g) => pinnedGroups.has(g.key)) ? 'Unpin telemetry groups' : 'Pin telemetry groups'}
              >
                Telemetry
              </button>
              <button
                onClick={() => {
                  const virtKeys = virtualGroups.map((g) => g.key);
                  const anySelected = virtKeys.some((k) => pinnedGroups.has(k));
                  setPinnedGroups((prev) => {
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
                  virtualGroups.some((g) => pinnedGroups.has(g.key))
                    ? 'bg-cyan-600 text-white'
                    : 'bg-slate-700 text-slate-400 hover:bg-slate-600'
                }`}
                title={virtualGroups.some((g) => pinnedGroups.has(g.key)) ? 'Unpin virtual groups' : 'Pin virtual groups'}
              >
                Log-derived
              </button>
              <button
                onClick={() => {
                  const anomalousKeys = displayGroups
                    .filter((g) => anomalousGroupKeys.has(g.key))
                    .map((g) => g.key);
                  setPinnedGroups(current => new Set([...current, ...anomalousKeys]));
                }}
                className="text-xs px-1.5 py-0.5 bg-slate-700 hover:bg-slate-600 rounded text-slate-400"
                title="Pin groups with detector hits"
              >
                With hits
              </button>
              <button
                onClick={() => setPinnedGroups(current => new Set([...current, ...displayGroups.map(group => group.key)]))}
                className="text-xs px-1.5 py-0.5 bg-slate-700 hover:bg-slate-600 rounded text-slate-400"
                title={`Pin the first ${MAX_METRIC_GROUPS} matching groups`}
              >
                First {MAX_METRIC_GROUPS}
              </button>
              <button
                onClick={() => setPinnedGroups(new Set())}
                className="text-xs px-1.5 py-0.5 bg-slate-700 hover:bg-slate-600 rounded text-slate-400"
                title="Clear pinned metrics"
              >
                Clear pins
              </button>
            </div>
          <input type="search" aria-label="Find metrics" placeholder="Find metrics…" value={metricSearch} onChange={event => setMetricSearch(event.target.value)} className="mb-2 w-full rounded border border-slate-600 bg-slate-900 px-2 py-1.5 text-xs text-slate-200 focus:outline-none focus:ring-2 focus:ring-purple-500" />
          <p className="mb-2 text-[10px] text-slate-500">Checked metrics are pinned. Red ! means detector hits.</p>
          <SeriesTree
            series={displayGroups}
            selectedSeries={pinnedGroups}
            selectedMetricTypes={selectedMetricTypes}
            anomalousSources={anomalousGroupKeys}
            onToggleMetricType={toggleMetricType}
            onSelectionChange={setPinnedGroups}
            maxSelected={MAX_METRIC_GROUPS}
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

      <main className="flex-1 min-w-0 min-w-0 p-6 overflow-y-auto">
        {state.error && (
          <div className="bg-red-900/50 border border-red-700 rounded-lg p-4 mb-6">
            <div className="text-red-400">{state.error}</div>
          </div>
        )}

        {state.connectionState === 'disconnected' && (
          <div className="text-center py-20">
            <div className="text-slate-400 text-lg">Waiting for observer connection...</div>
            <div className="text-slate-500 mt-2">
              Start the observer: <code className="bg-slate-800 px-2 py-1 rounded">./bin/anomalydetection-testbench</code>
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
            <section className="rounded-lg border border-slate-700 bg-slate-800/60 p-4 space-y-3" aria-label="Metric inspection">
              <div className="flex items-center gap-3 flex-wrap justify-between">
                <div><h2 className="text-sm font-semibold text-white">Inspect a window</h2><p className="text-xs text-slate-400 mt-1">{focusedGroupKey ? 'Inspecting a source metric. Your saved pins are preserved.' : 'Ranked metrics update automatically. Pin metrics to keep them as you explore.'}</p></div>
                <label className="text-xs text-slate-300">Inspect window <select aria-label="Inspect window" value={focusedGroupKey ? 'source' : inspectionScope} onChange={e => chooseInspectionScope(e.target.value as InspectionScope)} className="ml-2 p-2 bg-slate-900 rounded border border-slate-600">
                  {focusedGroupKey && <option value="source" disabled>Source window</option>}
                  {(['recording', ...(episode?.mode === 'healthy-control' ? ['evaluation'] as const : ['baseline', 'disruption', 'cooldown'] as const), 'custom'] as const).map(scope => <option key={scope} value={scope} disabled={scope !== 'recording' && !inspectionWindow(scope, episode, customWindow, fullWindow)}>{INSPECTION_LABELS[scope]}</option>)}
                </select></label>
              </div>
              <div className="flex items-center gap-3 flex-wrap text-xs">
                <span className="font-mono text-purple-200" aria-label="Inspected timestamps">{rankingRange ? `${new Date(rankingRange.start * 1000).toLocaleString()} – ${new Date(rankingRange.end * 1000).toLocaleTimeString()}` : 'Waiting for recording timestamps'}</span>
                {rankingRange && (focusedGroupKey || inspectionScope !== 'recording') && (timeRange?.start !== rankingRange.start || timeRange?.end !== rankingRange.end) && <button type="button" className="text-purple-300 hover:underline" onClick={() => onTimeRangeChange(rankingRange)}>Zoom to window</button>}
                {fullWindow && (timeRange?.start !== fullWindow.start || timeRange?.end !== fullWindow.end) && <button type="button" className="text-slate-300 hover:underline" onClick={() => onTimeRangeChange(fullWindow)}>Show recording context</button>}
              </div>
              <div className="flex items-center justify-between gap-3 flex-wrap">
                <p className="text-xs text-slate-300" aria-label="Metric selection summary">{focusedGroupKey ? `${selection.pinned.length} pin${selection.pinned.length === 1 ? '' : 's'} saved · 1 focused chart` : `${selection.pinned.length} pinned + ${selection.ranked.length} ranked · ${selectedGroups.size}/${MAX_METRIC_GROUPS} charts`}{tagFilterInput && ' · tag filter applied'}</p>
                <div className="flex items-center gap-3 text-xs">
                  {!focusedGroupKey && rankedLimit === 0 && pinnedGroups.size < MAX_METRIC_GROUPS && <button className="text-purple-300 hover:underline" onClick={() => { setRankedLimit(INITIAL_METRIC_GROUPS); setFocusedSeriesID(null); }}>Show ranked metrics</button>}
                  {!focusedGroupKey && selection.pinned.length > 0 && selection.ranked.length > 0 && <button className="text-purple-300 hover:underline" onClick={() => setRankedLimit(0)}>Show only pinned</button>}
                  {!focusedGroupKey && selection.ranked.length > INITIAL_METRIC_GROUPS && <button className="text-purple-300 hover:underline" onClick={() => setRankedLimit(INITIAL_METRIC_GROUPS)}>Show fewer</button>}
                  {canShowMoreMetrics && rankedLimit > 0 && <button type="button" onClick={() => setRankedLimit(MAX_METRIC_GROUPS)} disabled={selectionScenario !== state.activeScenario} className="px-3 py-2 font-medium rounded bg-purple-600 text-white hover:bg-purple-500 disabled:opacity-50">Show {selection.more} more metrics</button>}
                </div>
              </div>
              {!focusedGroupKey && rankedLimit > 0 && topAnomalousGroupKeys.length === 0 && <p className="text-xs text-slate-400">No detector hits in the inspected window. Pinned charts remain available for comparison.</p>}
              <p className="text-xs text-slate-400">Drag a chart to inspect a custom window. The purple band marks the inspected time; zoom is separate. Ranking counts detector hits across tagged series.</p>
              {selection.pinned.length > 0 && <div className="flex gap-2 flex-wrap max-h-28 overflow-y-auto" aria-label="Pinned metrics">{selection.pinned.map(key => <button key={key} type="button" aria-label={`Unpin ${key.slice(key.indexOf('/') + 1)}`} title={`Unpin ${key}`} onClick={() => togglePin(key)} className="text-xs text-purple-200 bg-purple-900/40 border border-purple-500/40 rounded px-2 py-1 break-all">{key.slice(key.indexOf('/') + 1)} ×</button>)}</div>}
              {focusedGroupKey && <div className="flex gap-3 items-center text-xs"><span className="text-cyan-300">Focused source metric.</span>{focusedSeriesID && <button className="text-purple-300 hover:underline" onClick={() => setFocusedSeriesID(null)}>Show all tagged series</button>}<button className="text-purple-300 hover:underline" onClick={() => { clearSourceFocus(); onResumeInspection?.(); }}>Resume metric inspection</button></div>}
              {onBackToScorer && <button className="text-xs text-purple-300 hover:underline" onClick={() => { clearSourceFocus(); onBackToScorer?.(); }}>← Back to scorer event</button>}
            </section>
            {selectedGroups.size > 0 && groupSeriesData.size < selectedVisibleCount && (
              <div role="status" className="text-sm text-slate-400">
                Loading metrics ({groupSeriesData.size}/{selectedVisibleCount})…
              </div>
            )}
            {groupErrors.size > 0 && (
              <div role="alert" className="rounded border border-amber-700 bg-amber-950/40 p-3 text-xs text-amber-200">
                <p>Some metric data could not be loaded.</p>
                <ul className="my-2 max-h-24 overflow-y-auto break-all">
                  {[...groupErrors].map(([key, message]) => <li key={key}>{message}</li>)}
                </ul>
                <button type="button" onClick={() => setRetryVersion((version) => version + 1)} className="text-purple-300 hover:underline">Retry metrics</button>
              </div>
            )}
            {selectedGroups.size > 0 && selectedVisibleCount === 0 && (
              <p className="py-6 text-center text-sm text-slate-400">No selected metrics match the current aggregation and metric type filters.</p>
            )}
            {selectedGroups.size === 0 ? (
              <div className="text-center py-10 text-slate-500">
                {rankedLimit === 0 ? 'No pinned metrics. Pin a metric from the sidebar or show ranked metrics.' : 'No detector hits in this window. Inspect another window or pin metrics from the sidebar to compare.'}
              </div>
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
                      const group = groupByKey.get(groupKey);
                      if (!group) return null;
                      return <RegularMetricCard key={groupKey} group={group} data={groupSeriesData.get(groupKey) ?? EMPTY_SERIES_DATA}
                        tagFilter={tagFilter} showAnomalyOnlySeriesLines={showAnomalyOnlySeriesLines} focusedSeriesID={focusedSeriesID}
                        anomalyCountBySeriesID={anomalyCountBySeriesID} anomalies={anomalies} selectedMetricTypes={selectedMetricTypes}
                        toggleMetricType={toggleMetricType} enabledDetectors={visibleDetectors} timeRange={timeRange}
                        onTimeRangeChange={onTimeRangeChange} smoothLines={smoothLines} phaseMarkers={phaseMarkers}
                        hitCount={anomalyCountByGroup.get(groupKey) ?? 0} rankingLabel={rankingLabel}
                        inspectionRange={highlightedWindow} onInspectRange={handleInspectRange} pinned={pinnedGroups.has(groupKey)}
                        onTogglePin={togglePin} pinLimitReached={pinnedGroups.size >= MAX_METRIC_GROUPS} />;
                    })
                    .filter(Boolean);
                  return cards.length > 0 ? (
                    <div className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,440px),1fr))] gap-4">{cards}</div>
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
                    <div className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,440px),1fr))] gap-4">
                      {Array.from(selectedGroups)
                        .filter((key) => {
                          const g = groupByKey.get(key);
                          return g && g.virtual;
                        })
                        .map((groupKey) => {
                          const group = groupByKey.get(groupKey);
                          if (!group) return null;
                          const dataList = filterCachedSeriesData(groupSeriesData.get(groupKey) ?? [], group);
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
								label: formatSeriesLabel(d.tags, d.host),
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
                              enabledDetectors={visibleDetectors}
                              timeRange={timeRange}
                              onTimeRangeChange={onTimeRangeChange}
                              smoothLines={smoothLines}
                              seriesVariants={seriesVariants}
                              phaseMarkers={phaseMarkers}
                              inspectionRange={highlightedWindow}
                              onInspectRange={handleInspectRange}
                              subtitle={<>{subtitle}<button type="button" className="text-xs text-purple-300 hover:underline" onClick={() => togglePin(groupKey)}>Unpin metric</button></>}
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
                    <div className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,440px),1fr))] gap-4">
                      {Array.from(selectedGroups)
                        .filter((key) => {
                          const g = groupByKey.get(key);
                          return g && g.namespace === 'telemetry';
                        })
                        .map((groupKey) => {
                          const group = groupByKey.get(groupKey);
                          if (!group) return null;
                          const isCounterTelemetry = group.members.some((m) => m.metricKind === 'counter');
                          const dataList = filterCachedSeriesData(groupSeriesData.get(groupKey) ?? [], group);
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
								label: formatSeriesLabel(d.tags, d.host),
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
                              enabledDetectors={visibleDetectors}
                              timeRange={timeRange}
                              onTimeRangeChange={onTimeRangeChange}
                              smoothLines={smoothLines}
                              seriesVariants={seriesVariants}
                              isTelemetry
                              phaseMarkers={phaseMarkers}
                              inspectionRange={highlightedWindow}
                              onInspectRange={handleInspectRange}
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

function MutedSeriesList({ series }: { series: string[] }) {
  const [expanded, setExpanded] = useState(false);
  const preview = series.slice(0, 3);
  const rest = series.length - preview.length;
  return (
    <div className="text-xs text-slate-500 font-mono space-y-0.5 ml-4">
      {(expanded ? series : preview).map((s) => (
        <div key={s} className="truncate opacity-60" title={s}>{s}</div>
      ))}
      {!expanded && rest > 0 && (
        <button
          className="text-slate-400 hover:text-slate-200 transition-colors"
          onClick={() => setExpanded(true)}
        >
          +{rest} more
        </button>
      )}
    </div>
  );
}
