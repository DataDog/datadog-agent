import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import type { TimeRange, PhaseMarker } from './ChartWithAnomalyDetails';

export type TimelineAnomalyType = 'standard' | 'log-derived' | 'telemetry';

export interface TimelineAnomaly {
  timestamp: number;
  detectorName: string;
  title: string;
  score?: number;
  type: TimelineAnomalyType;
  sourceSeriesId?: string;
}

interface AnomalyScoreTimelineProps {
  anomalies: TimelineAnomaly[];
  scenarioStart: number | null;
  scenarioEnd: number | null;
  timeRange?: TimeRange | null;
  phaseMarkers?: PhaseMarker[];
}

interface SeverityEvent {
  bucketIdx: number;
  timestamp: number;
  direction: 'increase' | 'decrease';
  fromLevel: 0 | 1 | 2;
  toLevel: 0 | 1 | 2;
}

// ── Anomaly level weights ──────────────────────────────────────────────────────
// Each anomaly is mapped to one of 5 levels (0=VeryLow … 4=XHigh).
// The weight is the only value used for both bar coloring (bin index = level)
// and the EWMA input (mean weight per bucket × saturation factor).
// This keeps all detectors on the same proportionate scale.
const LEVEL_WEIGHTS = [0.2, 0.5, 1.0, 2.0, 3.0] as const;
const LEVEL_LABELS  = ['VeryLow', 'Low', 'Medium', 'High', 'XHigh'] as const;

// Score → level thresholds (from 3-scenario calibration).
// Both holt_residual and tukey_biweight emit scores in the same numeric range
// (global means 14.4 and 12.7 respectively), so a single threshold table works
// for all scored detectors.
//   baseline: mean=8.3 P95=15.8 | disruption: P50=13.1 P95=36.8 P99=49.4
const SCORE_THRESHOLDS = [6, 12, 20, 35] as const; // [<6→0, <12→1, <20→2, <35→3, ≥35→4]

// Fixed level for detectors that emit no score at all.
//   bocpd: no Score field → use Medium as a reliable change-point signal
const DETECTOR_FIXED_LEVEL: Record<string, number> = {
  bocpd: 2, // Medium (1.0)
};

// Detectors whose score should be used with SCORE_THRESHOLDS.
const SCORED_DETECTORS = new Set(['holt_residual', 'tukey_biweight']);

// Slider ceiling for EWMA thresholds (extended dynamically when EWMA exceeds this).
const EWMA_SLIDER_MAX = 5.0;
// Default thresholds (approximate; tune with sliders after observing live data).
//   With level weights, baseline EWMA stays near 0.1–0.3; disruption peaks 1–2.
const DEFAULT_LOW_THRESHOLD  = 0.25;
const DEFAULT_HIGH_THRESHOLD = 0.5;
const DEFAULT_MARGIN         = 0.15;
const DEFAULT_EWMA_ALPHA     = 0.16;
const DEFAULT_SATURATION_K   = 5;

const SCORE_BINS = 5;
const CHART_H = 192;
const EVENTS_H = 22;
const TOTAL_H = CHART_H + EVENTS_H;
const TRI_SIZE = 5;
// Default display bar count when aggregation window is auto.
const DEFAULT_DISPLAY_BARS = 80;

const SCORE_COLORS = [
  'bg-slate-500/70',
  'bg-yellow-500/75',
  'bg-orange-500/75',
  'bg-red-500/80',
  'bg-fuchsia-500/85',
];

const SEVERITY_LABELS = ['Low', 'Medium', 'High'] as const;
const SEVERITY_COLORS = ['#22c55e', '#f97316', '#ef4444'] as const;

const TYPE_LABELS: Record<TimelineAnomalyType, string> = {
  standard: 'standard metrics',
  'log-derived': 'log metrics',
  telemetry: 'telemetry',
};

function formatTimestamp(ts: number): string {
  return new Date(ts * 1000).toLocaleTimeString([], {
    hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  });
}

function formatDuration(secs: number): string {
  if (secs < 60) return `${secs}s`;
  return `${Math.floor(secs / 60)}m${secs % 60 > 0 ? `${secs % 60}s` : ''}`;
}

// Maps a raw detector score to a level index 0–4.
function scoreToLevel(score: number): number {
  for (let i = 0; i < SCORE_THRESHOLDS.length; i++) {
    if (score < SCORE_THRESHOLDS[i]) return i;
  }
  return SCORE_THRESHOLDS.length; // 4 = XHigh
}

// Single entry-point: returns the level index [0–4] for any anomaly.
// Level drives both bar coloring (bin index) and EWMA weight.
function perDetectorLevel(a: TimelineAnomaly): number {
  if (SCORED_DETECTORS.has(a.detectorName) && a.score != null && !Number.isNaN(a.score)) {
    return scoreToLevel(a.score);
  }
  return DETECTOR_FIXED_LEVEL[a.detectorName] ?? 2; // unknown or no score → Medium
}

function perDetectorWeight(a: TimelineAnomaly): number {
  return LEVEL_WEIGHTS[perDetectorLevel(a)];
}

function computeEWMA(values: number[], alpha: number): number[] {
  if (values.length === 0) return [];
  const out = new Array<number>(values.length);
  out[0] = values[0];
  for (let i = 1; i < values.length; i++) out[i] = alpha * values[i] + (1 - alpha) * out[i - 1];
  return out;
}

interface SeverityEventsResult {
  events: SeverityEvent[];
  initialLevel: 0 | 1 | 2;
}

function computeSeverityEvents(
  ewmaValues: number[],
  bucketWidthSecs: number,
  bucketStartTs: number,
  lowT: number,
  highT: number,
  margin: number,
  cooldownSecs: number,
): SeverityEventsResult {
  if (ewmaValues.length === 0) return { events: [], initialLevel: 0 };

  function rawLevel(v: number): 0 | 1 | 2 {
    if (v >= highT) return 2;
    if (v >= lowT) return 1;
    return 0;
  }

  function nextLevel(v: number, cur: 0 | 1 | 2): 0 | 1 | 2 {
    if (cur === 0) {
      if (v >= highT + margin) return 2;
      if (v >= lowT + margin) return 1;
      return 0;
    }
    if (cur === 1) {
      if (v >= highT + margin) return 2;
      if (v < lowT - margin) return 0;
      return 1;
    }
    // cur === 2 (High): cap decrease to one level at a time so we always pass through Medium
    if (v < highT - margin) return 1;
    return 2;
  }

  const initialLevel = rawLevel(ewmaValues[0]);
  let cur = initialLevel;
  // Track when we entered the current state; cooldown prevents leaving an elevated state
  // too soon, enforcing minimum dwell time before each step down.
  let lastStateEntryTs = -Infinity;
  const events: SeverityEvent[] = [];

  for (let i = 1; i < ewmaValues.length; i++) {
    const ts = bucketStartTs + (i + 0.5) * bucketWidthSecs;
    const target = nextLevel(ewmaValues[i], cur);
    if (target === cur) continue;
    const dir = target > cur ? 'increase' : 'decrease';
    if (dir === 'decrease' && ts - lastStateEntryTs < cooldownSecs) continue;
    events.push({ bucketIdx: i, timestamp: ts, direction: dir, fromLevel: cur, toLevel: target });
    lastStateEntryTs = ts;
    cur = target;
  }
  return { events, initialLevel };
}

/** Up-pointing triangle polygon points centered at (cx, cy). */
function triUp(cx: number, cy: number, s: number): string {
  return `${cx},${cy - s} ${cx - s * 0.8},${cy + s * 0.6} ${cx + s * 0.8},${cy + s * 0.6}`;
}
/** Down-pointing triangle polygon points centered at (cx, cy). */
function triDown(cx: number, cy: number, s: number): string {
  return `${cx},${cy + s} ${cx - s * 0.8},${cy - s * 0.6} ${cx + s * 0.8},${cy - s * 0.6}`;
}

interface SecondBucket {
  bins: number[];    // SCORE_BINS counts, index = anomaly level (0=VeryLow…4=XHigh)
  scoreSum: number;  // sum of LEVEL_WEIGHTS (feeds EWMA)
  total: number;
}

interface DisplayBucket {
  start: number;
  end: number;
  bins: number[];
  scoreSum: number;
  total: number;
}

interface Hovered {
  bucketIdx: number;
  mouseX: number;
  mouseY: number;
  total: number;
  bins: number[];
  meanWeight: number;
  ewmaValue: number;
  bucketStart: number;
  bucketEnd: number;
  events: SeverityEvent[];
}

function usePersistedState(key: string, defaultValue: number): [number, (v: number) => void] {
  const [value, setValueRaw] = useState<number>(() => {
    try {
      const stored = localStorage.getItem(`ast:${key}`);
      return stored !== null ? parseFloat(stored) : defaultValue;
    } catch {
      return defaultValue;
    }
  });
  const setValue = useCallback((v: number) => {
    setValueRaw(v);
    try { localStorage.setItem(`ast:${key}`, String(v)); } catch { /* ignore */ }
  }, [key]);
  return [value, setValue];
}

export function AnomalyScoreTimeline({
  anomalies,
  scenarioStart,
  scenarioEnd,
  timeRange,
  phaseMarkers = [],
}: AnomalyScoreTimelineProps) {
  // ── display controls ──────────────────────────────────────────────────────
  const [ewmaAlpha, setEwmaAlpha] = usePersistedState('ewmaAlpha', DEFAULT_EWMA_ALPHA);
  const [saturationK, setSaturationK] = usePersistedState('saturationK', DEFAULT_SATURATION_K);
  // aggregationWindow: 0 = auto (fit ~80 bars), >0 = manual seconds per display bar
  const [aggregationWindow, setAggregationWindow] = usePersistedState('aggregationWindow', 0);

  // ── event-detection controls (persisted across scenario loads) ─────────────
  const [lowThreshold, setLowThresholdRaw] = usePersistedState('lowThreshold', DEFAULT_LOW_THRESHOLD);
  const [highThreshold, setHighThresholdRaw] = usePersistedState('highThreshold', DEFAULT_HIGH_THRESHOLD);
  const [margin, setMargin] = usePersistedState('margin', DEFAULT_MARGIN);
  const [cooldownSecs, setCooldownSecs] = usePersistedState('cooldownSecs', 300);

  const setLowThreshold = (v: number) => setLowThresholdRaw(Math.min(v, highThreshold - 0.05));
  const setHighThreshold = (v: number) => setHighThresholdRaw(Math.max(v, lowThreshold + 0.05));

  // ── layout ────────────────────────────────────────────────────────────────
  const [chartWidth, setChartWidth] = useState(600);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => setChartWidth(e.contentRect.width));
    ro.observe(el);
    setChartWidth(el.getBoundingClientRect().width);
    return () => ro.disconnect();
  }, []);

  // ── hover ─────────────────────────────────────────────────────────────────
  const [hovered, setHovered] = useState<Hovered | null>(null);
  const [hoveredEventIdx, setHoveredEventIdx] = useState<number | null>(null);

  // ── time bounds ───────────────────────────────────────────────────────────
  const bounds = useMemo(() => {
    let start = timeRange?.start ?? scenarioStart ?? null;
    let end = timeRange?.end ?? scenarioEnd ?? null;
    if ((start === null || end === null || end <= start) && anomalies.length > 0) {
      start = Math.min(...anomalies.map((a) => a.timestamp));
      end = Math.max(...anomalies.map((a) => a.timestamp));
    }
    return start !== null && end !== null && end > start ? { start, end } : null;
  }, [anomalies, scenarioEnd, scenarioStart, timeRange]);

  // ── Deduplication: same series × same second, multiple detectors → keep highest level ──
  const dedupedAnomalies = useMemo(() => {
    // Key: "floor(ts):sourceSeriesId" — groups co-firing detectors on the same series.
    // Anomalies without a sourceSeriesId are never merged (no identity to match on).
    const best = new Map<string, TimelineAnomaly>();
    for (const a of anomalies) {
      if (!a.sourceSeriesId) continue;
      const key = `${Math.floor(a.timestamp)}:${a.sourceSeriesId}`;
      const existing = best.get(key);
      if (!existing || perDetectorLevel(a) > perDetectorLevel(existing)) {
        best.set(key, a);
      }
    }
    // Re-add anomalies without a sourceSeriesId unchanged.
    const unkeyed = anomalies.filter((a) => !a.sourceSeriesId);
    return [...best.values(), ...unkeyed];
  }, [anomalies]);

  // ── 1-second raw buckets ──────────────────────────────────────────────────
  // level = perDetectorLevel(a) drives both the bin color (index) and the
  // EWMA weight (LEVEL_WEIGHTS[level]), so there is a single unified score path.
  const secondBuckets = useMemo(() => {
    if (!bounds) return new Map<number, SecondBucket>();
    const map = new Map<number, SecondBucket>();
    for (const a of dedupedAnomalies) {
      if (a.timestamp < bounds.start || a.timestamp > bounds.end) continue;
      const sec = Math.floor(a.timestamp);
      if (!map.has(sec)) {
        map.set(sec, { bins: new Array<number>(SCORE_BINS).fill(0), scoreSum: 0, total: 0 });
      }
      const b = map.get(sec)!;
      const level = perDetectorLevel(a);
      b.bins[level] += 1;
      b.scoreSum += LEVEL_WEIGHTS[level];
      b.total += 1;
    }
    return map;
  }, [dedupedAnomalies, bounds]);

  // ── resolved aggregation window (seconds per display bar) ─────────────────
  const resolvedWindow = useMemo(() => {
    if (!bounds) return 1;
    const durationSecs = Math.ceil(bounds.end - bounds.start);
    if (aggregationWindow > 0) return aggregationWindow;
    return Math.max(1, Math.ceil(durationSecs / DEFAULT_DISPLAY_BARS));
  }, [bounds, aggregationWindow]);

  // ── display buckets (aggregated from 1-second bins) ───────────────────────
  const displayBuckets = useMemo((): DisplayBucket[] => {
    if (!bounds) return [];
    const durationSecs = Math.ceil(bounds.end - bounds.start);
    const count = Math.ceil(durationSecs / resolvedWindow);
    return Array.from({ length: count }, (_, i) => {
      const start = bounds.start + i * resolvedWindow;
      const end = start + resolvedWindow;
      const bins = new Array<number>(SCORE_BINS).fill(0);
      let scoreSum = 0;
      let total = 0;
      for (let sec = Math.floor(start); sec < Math.ceil(end); sec++) {
        const sb = secondBuckets.get(sec);
        if (!sb) continue;
        for (let bi = 0; bi < SCORE_BINS; bi++) bins[bi] += sb.bins[bi];
        scoreSum += sb.scoreSum;
        total += sb.total;
      }
      return { start, end, bins, scoreSum, total };
    });
  }, [bounds, secondBuckets, resolvedWindow]);

  const displayCount = displayBuckets.length;

  // ── EWMA ──────────────────────────────────────────────────────────────────
  const ewmaValues = useMemo(
    () => computeEWMA(
      displayBuckets.map((b) => {
        if (b.total === 0) return 0;
        const meanRaw = b.scoreSum / b.total;
        return meanRaw * (1 - Math.exp(-b.total / saturationK));
      }),
      ewmaAlpha,
    ),
    [displayBuckets, ewmaAlpha, saturationK],
  );

  // Dynamic display ceiling: high threshold sits near the top; expands if EWMA exceeds it.
  const ewmaDisplayMax = useMemo(() => {
    const peak = ewmaValues.length > 0 ? Math.max(...ewmaValues) : 0;
    return Math.max(highThreshold, peak) * 1.15;
  }, [highThreshold, ewmaValues]);

  const ewmaToY = useCallback(
    (v: number) => CHART_H - Math.min(1, Math.max(0, v / ewmaDisplayMax)) * CHART_H,
    [ewmaDisplayMax],
  );

  const ewmaYValues = useMemo(() => ewmaValues.map(ewmaToY), [ewmaValues, ewmaToY]);

  const ewmaPolyline = useMemo(() => {
    if (ewmaYValues.length === 0 || chartWidth === 0 || displayCount === 0) return '';
    const bw = chartWidth / displayCount;
    return ewmaYValues.map((y, i) => `${((i + 0.5) * bw).toFixed(1)},${y.toFixed(1)}`).join(' ');
  }, [ewmaYValues, chartWidth, displayCount]);

  // ── threshold y positions (raw EWMA space, fixed scale) ───────────────────
  const lowThresholdY  = CHART_H - (lowThreshold  / ewmaDisplayMax) * CHART_H;
  const highThresholdY = CHART_H - (highThreshold / ewmaDisplayMax) * CHART_H;

  // ── severity events (run on raw EWMA with raw thresholds) ─────────────────
  const { severityEvents, stateSegments } = useMemo(() => {
    if (!bounds || displayCount === 0) return { severityEvents: [], stateSegments: [] };

    const { events, initialLevel } = computeSeverityEvents(
      ewmaValues, resolvedWindow, bounds.start,
      lowThreshold, highThreshold, margin, cooldownSecs,
    );

    const segs: { fromBucket: number; toBucket: number; level: 0 | 1 | 2 }[] = [];
    let cur: 0 | 1 | 2 = initialLevel;
    let curFrom = 0;
    for (const ev of events) {
      segs.push({ fromBucket: curFrom, toBucket: ev.bucketIdx, level: cur });
      curFrom = ev.bucketIdx;
      cur = ev.toLevel;
    }
    segs.push({ fromBucket: curFrom, toBucket: displayCount, level: cur });

    return { severityEvents: events, stateSegments: segs };
  }, [ewmaValues, bounds, displayCount, resolvedWindow, lowThreshold, highThreshold, margin, cooldownSecs]);

  const eventsByBucket = useMemo(() => {
    const map = new Map<number, SeverityEvent[]>();
    severityEvents.forEach((ev) => {
      const arr = map.get(ev.bucketIdx) ?? [];
      arr.push(ev);
      map.set(ev.bucketIdx, arr);
    });
    return map;
  }, [severityEvents]);

  // ── hover handlers ────────────────────────────────────────────────────────
  const handleMouseMove = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (!bounds || displayBuckets.length === 0) return;
      const rect = e.currentTarget.getBoundingClientRect();
      const relX = e.clientX - rect.left;
      const relY = e.clientY - rect.top;
      const idx = Math.min(displayCount - 1, Math.max(0, Math.floor((relX / rect.width) * displayCount)));
      const b = displayBuckets[idx];
      setHovered({
        bucketIdx: idx,
        mouseX: relX,
        mouseY: relY,
        total: b.total,
        bins: b.bins,
        meanWeight: b.total > 0 ? b.scoreSum / b.total : 0,
        ewmaValue: ewmaValues[idx] ?? 0,
        bucketStart: b.start,
        bucketEnd: b.end,
        events: eventsByBucket.get(idx) ?? [],
      });
    },
    [bounds, displayBuckets, displayCount, ewmaValues, eventsByBucket],
  );

  const handleMouseLeave = useCallback(() => {
    setHovered(null);
    setHoveredEventIdx(null);
  }, []);

  if (!bounds) return null;

  const maxBucketCount = Math.max(1, ...displayBuckets.map((b) => b.total));
  const detectors = new Set(anomalies.map((a) => a.detectorName));
  const typeCounts = anomalies.reduce((acc, a) => {
    acc.set(a.type, (acc.get(a.type) ?? 0) + 1);
    return acc;
  }, new Map<TimelineAnomalyType, number>());
  const ewmaEnd  = ewmaValues.length > 0 ? ewmaValues[ewmaValues.length - 1] : 0;
  const ewmaPeak = ewmaValues.length > 0 ? Math.max(...ewmaValues) : 0;

  const tsToX    = (ts: number) => ((ts - bounds.start) / (bounds.end - bounds.start)) * chartWidth;
  const bucketCx = (idx: number) => ((idx + 0.5) * chartWidth) / displayCount;

  const hoveredEvent = hoveredEventIdx !== null
    ? severityEvents[hoveredEventIdx]
    : hovered?.events[0] ?? null;

  const windowLabel = aggregationWindow > 0
    ? `window: ${formatDuration(aggregationWindow)}`
    : `window: auto (${formatDuration(resolvedWindow)})`;

  return (
    <div className="bg-slate-800 rounded-lg border border-slate-700 p-4">
      {/* ── Header ────────────────────────────────────────────────────────── */}
      <div className="flex items-start justify-between gap-4 mb-3">
        <div>
          <h3 className="text-sm font-semibold text-slate-200">Anomaly intensity timeline</h3>
          <div className="text-xs text-slate-500 mt-0.5">
            {anomalies.length} anomal{anomalies.length === 1 ? 'y' : 'ies'} across {detectors.size} detector{detectors.size === 1 ? '' : 's'}
            {' · '}
            <span className="text-amber-400">{severityEvents.filter(e => e.direction === 'increase').length} ▲</span>
            {' '}
            <span className="text-slate-400">{severityEvents.filter(e => e.direction === 'decrease').length} ▼</span>
            {' events'}
          </div>
        </div>
        <div className="flex gap-2 text-xs flex-wrap justify-end">
          <span className="px-2 py-1 rounded bg-slate-900/70 text-slate-400">max bucket: {maxBucketCount}</span>
          <span className="px-2 py-1 rounded bg-slate-900/70 text-slate-400">{windowLabel}</span>
          <span className="px-2 py-1 rounded bg-cyan-900/70 text-cyan-300">EWMA end: {ewmaEnd.toFixed(2)}</span>
          <span className="px-2 py-1 rounded bg-cyan-900/70 text-cyan-300">EWMA peak: {ewmaPeak.toFixed(2)}</span>
        </div>
      </div>

      {/* ── Controls ──────────────────────────────────────────────────────── */}
      <div className="grid grid-cols-2 gap-x-8 gap-y-2 mb-3">
        {/* Left: display */}
        <div className="space-y-2">
          <div className="text-[10px] font-semibold text-slate-500 uppercase tracking-wider mb-1">Display</div>
          <SliderRow label="EWMA α" leftLabel="Smooth" rightLabel="Raw"
            min={0.01} max={1} step={0.01} value={ewmaAlpha} onChange={setEwmaAlpha}
            valueLabel={ewmaAlpha.toFixed(2)} thumbHex="#67e8f9" />
          <SliderRow label="Count saturation k" leftLabel="1 (fast)" rightLabel="30 (slow)"
            min={1} max={30} step={1} value={saturationK} onChange={setSaturationK}
            valueLabel={`k=${saturationK}`} thumbHex="#f97316" />
          <SliderRow label="Aggregation window" leftLabel="1s" rightLabel="10m"
            min={0} max={600} step={1} value={aggregationWindow} onChange={setAggregationWindow}
            valueLabel={aggregationWindow === 0 ? 'auto' : formatDuration(aggregationWindow)} thumbHex="#94a3b8" />
        </div>

        {/* Right: event detection */}
        <div className="space-y-2">
          <div className="text-[10px] font-semibold text-slate-500 uppercase tracking-wider mb-1">Event detection</div>
          <SliderRow label="Low threshold" leftLabel="0" rightLabel={ewmaDisplayMax.toFixed(2)}
            min={0} max={Math.max(EWMA_SLIDER_MAX, ewmaDisplayMax)} step={0.05} value={lowThreshold} onChange={setLowThreshold}
            valueLabel={lowThreshold.toFixed(2)} thumbHex="#22c55e" />
          <SliderRow label="High threshold" leftLabel="0" rightLabel={ewmaDisplayMax.toFixed(2)}
            min={0} max={Math.max(EWMA_SLIDER_MAX, ewmaDisplayMax)} step={0.05} value={highThreshold} onChange={setHighThreshold}
            valueLabel={highThreshold.toFixed(2)} thumbHex="#ef4444" />
          <SliderRow label="Margin (hysteresis)" leftLabel="0" rightLabel="0.5"
            min={0} max={0.5} step={0.01} value={margin} onChange={setMargin}
            valueLabel={margin.toFixed(2)} thumbHex="#f59e0b" />
          <SliderRow label="Cooldown (decrease)" leftLabel="0s" rightLabel="10m"
            min={0} max={600} step={10} value={cooldownSecs} onChange={setCooldownSecs}
            valueLabel={formatDuration(cooldownSecs)} thumbHex="#3b82f6" />
        </div>
      </div>

      {/* ── Chart + events strip ───────────────────────────────────────────── */}
      <div
        ref={containerRef}
        className="relative cursor-crosshair"
        style={{ height: TOTAL_H }}
        onMouseMove={handleMouseMove}
        onMouseLeave={handleMouseLeave}
      >
        {/* Stacked bars (top CHART_H px) */}
        <div className="absolute top-0 left-0 right-0 flex items-end gap-px" style={{ height: CHART_H }}>
          {displayBuckets.map((bucket, bi) => {
            const h = bucket.total > 0 ? Math.max(5, (bucket.total / maxBucketCount) * CHART_H) : 2;
            return (
              <div key={bi} className="flex-1 flex items-end" style={{ height: CHART_H }}>
                {bucket.total === 0 ? (
                  <div className="w-full rounded-sm bg-slate-700/40" style={{ height: '2px' }} />
                ) : (
                  <div className="w-full flex flex-col-reverse rounded-sm overflow-hidden" style={{ height: h }}>
                    {bucket.bins.map((count, binIdx) =>
                      count > 0 && (
                        <div key={binIdx} className={SCORE_COLORS[binIdx]}
                          style={{ height: `${(count / bucket.total) * 100}%` }} />
                      )
                    )}
                  </div>
                )}
              </div>
            );
          })}
        </div>

        {/* Events strip separator */}
        <div className="absolute left-0 right-0 border-t border-slate-700/60" style={{ top: CHART_H }} />

        {/* SVG overlay */}
        <svg
          className="absolute inset-0 w-full pointer-events-none"
          style={{ height: TOTAL_H }}
          viewBox={`0 0 ${chartWidth} ${TOTAL_H}`}
        >
          {/* Hover bucket highlight */}
          {hovered !== null && displayCount > 0 && (
            <rect
              x={(hovered.bucketIdx * chartWidth) / displayCount} y={0}
              width={chartWidth / displayCount} height={TOTAL_H}
              fill="white" fillOpacity={0.05}
            />
          )}

          {/* Threshold horizontal lines */}
          {(() => {
            const lines: React.ReactNode[] = [];
            const clampedLowY  = Math.max(0, Math.min(CHART_H, lowThresholdY));
            const clampedHighY = Math.max(0, Math.min(CHART_H, highThresholdY));
            if (lowThresholdY >= 0 && lowThresholdY <= CHART_H) {
              lines.push(
                <line key="low-t" x1={0} x2={chartWidth} y1={clampedLowY} y2={clampedLowY}
                  stroke={SEVERITY_COLORS[0]} strokeWidth={0.75} strokeDasharray="3,4" opacity={0.5} />,
                <text key="low-t-label" x={chartWidth - 2} y={clampedLowY - 2}
                  fill={SEVERITY_COLORS[0]} fontSize={8} fontFamily="monospace" textAnchor="end" opacity={0.7}>
                  {lowThreshold.toFixed(1)}
                </text>
              );
            }
            if (highThresholdY >= 0 && highThresholdY <= CHART_H) {
              lines.push(
                <line key="high-t" x1={0} x2={chartWidth} y1={clampedHighY} y2={clampedHighY}
                  stroke={SEVERITY_COLORS[2]} strokeWidth={0.75} strokeDasharray="3,4" opacity={0.5} />,
                <text key="high-t-label" x={chartWidth - 2} y={clampedHighY - 2}
                  fill={SEVERITY_COLORS[2]} fontSize={8} fontFamily="monospace" textAnchor="end" opacity={0.7}>
                  {highThreshold.toFixed(1)}
                </text>
              );
            }
            return lines;
          })()}

          {/* Phase marker lines */}
          {phaseMarkers.map((m) => {
            const x = tsToX(m.timestamp);
            if (x < -10 || x > chartWidth + 10) return null;
            return (
              <line key={m.key} x1={x} x2={x} y1={0} y2={CHART_H}
                stroke={m.color} strokeWidth={1} strokeDasharray="4,3" opacity={0.8} />
            );
          })}

          {/* EWMA polyline */}
          {ewmaPolyline && (
            <polyline points={ewmaPolyline} fill="none" stroke="#67e8f9"
              strokeWidth={1.5} strokeLinejoin="round" strokeLinecap="round" opacity={0.9} />
          )}

          {/* EWMA dot at hover */}
          {hovered !== null && ewmaYValues[hovered.bucketIdx] !== undefined && (
            <circle cx={bucketCx(hovered.bucketIdx)} cy={ewmaYValues[hovered.bucketIdx]}
              r={3} fill="#67e8f9" />
          )}

          {/* State segments */}
          {stateSegments.map((seg, i) => {
            const x1 = (seg.fromBucket * chartWidth) / displayCount;
            const x2 = (seg.toBucket * chartWidth) / displayCount;
            const cy = CHART_H + EVENTS_H / 2;
            return (
              <line key={i}
                x1={x1} x2={x2} y1={cy} y2={cy}
                stroke={SEVERITY_COLORS[seg.level]}
                strokeWidth={2}
                opacity={0.6}
              />
            );
          })}

          {/* Event triangles */}
          {severityEvents.map((ev, i) => {
            const cx = bucketCx(ev.bucketIdx);
            const cy = CHART_H + EVENTS_H / 2;
            const color = SEVERITY_COLORS[ev.toLevel];
            const isHovered = hoveredEventIdx === i;
            return (
              <polygon
                key={i}
                points={ev.direction === 'increase' ? triUp(cx, cy, TRI_SIZE) : triDown(cx, cy, TRI_SIZE)}
                fill={color}
                opacity={isHovered ? 1 : 0.85}
                stroke={isHovered ? 'white' : color}
                strokeWidth={isHovered ? 0.8 : 0}
                style={{ pointerEvents: 'all', cursor: 'pointer' }}
                onMouseEnter={() => setHoveredEventIdx(i)}
                onMouseLeave={() => setHoveredEventIdx(null)}
              />
            );
          })}
        </svg>

        {/* Phase marker labels */}
        {phaseMarkers.map((m) => {
          const pct = ((m.timestamp - bounds.start) / (bounds.end - bounds.start)) * 100;
          if (pct < -2 || pct > 102) return null;
          return (
            <span key={m.key}
              className="absolute top-0 text-[9px] font-mono leading-none pointer-events-none select-none"
              style={{ left: `${pct}%`, transform: 'translateX(2px)', color: m.color, opacity: 0.9 }}>
              {m.label}
            </span>
          );
        })}

        {/* Hover tooltip */}
        {(hovered !== null || hoveredEvent !== null) && (() => {
          const anchorX = hoveredEvent !== null
            ? bucketCx(hoveredEvent.bucketIdx)
            : hovered!.mouseX;
          const anchorY = hoveredEvent !== null
            ? CHART_H + EVENTS_H / 2
            : hovered!.mouseY;
          const flipLeft = anchorX + 10 > chartWidth - 180;

          return (
            <div
              className="absolute z-10 pointer-events-none bg-slate-900 border border-slate-600 rounded px-2.5 py-1.5 text-xs shadow-lg min-w-[170px]"
              style={{
                left: flipLeft ? anchorX - 180 : anchorX + 10,
                top: Math.max(0, anchorY - 10),
              }}
            >
              {hovered !== null && (
                <>
                  <div className="text-slate-400 font-mono mb-1 text-[10px]">
                    {formatTimestamp(hovered.bucketStart)}–{formatTimestamp(hovered.bucketEnd)}
                  </div>
                  <div className="flex justify-between gap-3">
                    <span className="text-slate-400">anomalies</span>
                    <span className="text-slate-200 font-semibold">{hovered.total}</span>
                  </div>
                  {hovered.total > 0 && (
                    <div className="flex items-center gap-0.5 mt-0.5 mb-0.5">
                      {hovered.bins.map((count, binIdx) => {
                        const binColors = ['bg-slate-500/70','bg-yellow-500/75','bg-orange-500/75','bg-red-500/80','bg-fuchsia-500/85'];
                        return (
                          <div key={binIdx} className="flex flex-col items-center gap-0.5 flex-1">
                            <span className="text-[9px] text-slate-300 font-mono leading-none">{count > 0 ? count : ''}</span>
                            <div className={`w-full h-1.5 rounded-sm ${binColors[binIdx]}`} style={{ opacity: count > 0 ? 1 : 0.2 }} />
                          </div>
                        );
                      })}
                    </div>
                  )}
                  <div className="flex justify-between gap-3">
                    <span className="text-slate-400">mean weight</span>
                    <span className="text-slate-200">{hovered.meanWeight.toFixed(2)}</span>
                  </div>
                  <div className="flex justify-between gap-3">
                    <span className="text-cyan-400">EWMA</span>
                    <span className="text-cyan-200">{hovered.ewmaValue.toFixed(2)}</span>
                  </div>
                </>
              )}
              {(hovered?.events ?? (hoveredEvent ? [hoveredEvent] : [])).map((ev, i) => (
                <div key={i} className="mt-1.5 pt-1.5 border-t border-slate-700">
                  <div className="flex items-center gap-1.5">
                    <span style={{ color: SEVERITY_COLORS[ev.toLevel] }} className="font-bold">
                      {ev.direction === 'increase' ? '▲' : '▼'}
                    </span>
                    <span className="font-semibold" style={{ color: SEVERITY_COLORS[ev.toLevel] }}>
                      {SEVERITY_LABELS[ev.fromLevel]} → {SEVERITY_LABELS[ev.toLevel]}
                    </span>
                  </div>
                  <div className="text-slate-500 text-[10px] mt-0.5">{formatTimestamp(ev.timestamp)}</div>
                </div>
              ))}
            </div>
          );
        })()}
      </div>

      {/* Time axis */}
      <div className="flex justify-between text-xs text-slate-600 mt-1.5">
        <span>{formatTimestamp(bounds.start)}</span>
        <span>{formatTimestamp(bounds.end)}</span>
      </div>

      {/* Legend */}
      <div className="flex items-center justify-between gap-4 mt-3 text-xs text-slate-500 flex-wrap">
        <div className="flex items-center gap-2 flex-wrap">
          <span>levels:</span>
          {SCORE_COLORS.map((color, idx) => (
            <span key={idx} className="inline-flex items-center gap-1">
              <span className={`inline-block w-2.5 h-2.5 rounded-sm ${color}`} />
              <span className="text-slate-400">{LEVEL_LABELS[idx]}</span>
              <span className="text-slate-600">({LEVEL_WEIGHTS[idx]})</span>
            </span>
          ))}
          <span className="inline-flex items-center gap-1 ml-2">
            <svg width="16" height="8" viewBox="0 0 16 8">
              <polyline points="0,7 4,5 8,3 12,4 16,2" fill="none" stroke="#67e8f9" strokeWidth="1.5" strokeLinejoin="round" />
            </svg>
            EWMA (ceil={ewmaDisplayMax.toFixed(2)})
          </span>
        </div>
        <div className="flex items-center gap-3 flex-wrap">
          {([0, 1, 2] as const).map((level) => (
            <span key={level} className="inline-flex items-center gap-1">
              <svg width="12" height="12" viewBox="0 0 12 12">
                <polygon points={triUp(6, 6, 4)} fill={SEVERITY_COLORS[level]} />
              </svg>
              <svg width="12" height="12" viewBox="0 0 12 12">
                <polygon points={triDown(6, 6, 4)} fill={SEVERITY_COLORS[level]} />
              </svg>
              {SEVERITY_LABELS[level]}
            </span>
          ))}
          <span className="text-slate-600">▲=increase · ▼=decrease</span>
        </div>
        <div className="flex gap-2 flex-wrap">
          {Array.from(typeCounts.entries()).map(([type, count]) => (
            <span key={type}>{TYPE_LABELS[type]}: {count}</span>
          ))}
        </div>
      </div>
    </div>
  );
}

// ── Shared slider row component ────────────────────────────────────────────
interface SliderRowProps {
  label: string;
  leftLabel: string;
  rightLabel: string;
  min: number;
  max: number;
  step: number;
  value: number;
  onChange: (v: number) => void;
  valueLabel: string;
  /** Hex color for the thumb, e.g. "#a855f7". Uses accent-color (safe from Tailwind JIT stripping). */
  thumbHex: string;
}

function SliderRow({ label, leftLabel, rightLabel, min, max, step, value, onChange, valueLabel, thumbHex }: SliderRowProps) {
  return (
    <div className="flex items-center gap-2">
      <label className="text-xs text-slate-400 min-w-[130px] shrink-0">{label}:</label>
      <span className="text-[10px] text-slate-600 shrink-0">{leftLabel}</span>
      <input
        type="range" min={min} max={max} step={step} value={value}
        onChange={(e) => onChange(parseFloat(e.target.value))}
        style={{ '--thumb-color': thumbHex } as React.CSSProperties}
        className="flex-1 h-1 bg-slate-700 rounded appearance-none cursor-pointer
          [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:w-3
          [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-[var(--thumb-color)]
          [&::-moz-range-thumb]:h-3 [&::-moz-range-thumb]:w-3 [&::-moz-range-thumb]:rounded-full
          [&::-moz-range-thumb]:bg-[var(--thumb-color)] [&::-moz-range-thumb]:border-none"
      />
      <span className="text-[10px] text-slate-600 shrink-0">{rightLabel}</span>
      <span className="text-xs text-slate-500 font-mono min-w-[40px] text-right shrink-0">{valueLabel}</span>
    </div>
  );
}
