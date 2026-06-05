import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import type { TimeRange, PhaseMarker } from './ChartWithAnomalyDetails';

export type TimelineAnomalyType = 'standard' | 'log-derived' | 'telemetry';

export interface TimelineAnomaly {
  timestamp: number;
  detectorName: string;
  title: string;
  score?: number;
  type: TimelineAnomalyType;
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

// ── Calibration constants (derived from 3-scenario offline calibration) ───────
// Scores are raw unbounded floats from each detector at detection time.
// null = count-only detector (no meaningful score magnitude to normalize).
const DETECTOR_NORM: Record<string, { mu: number; sigma: number } | null> = {
  holt_residual:  { mu: 8.3, sigma: 4.0 }, // calibrated: baseline μ=8.3 σ=4.0
  tukey_biweight: null, // baseline ≈ disruption scores → count-only
  bocpd:          null, // no Score field emitted → uses DETECTOR_FIXED_SCORE
};
// Detectors with no Score that still contribute a fixed raw weight to the EWMA.
const DETECTOR_FIXED_SCORE: Record<string, number> = {
  bocpd: 1,
};
// EWMA lives in raw score space. 15.0 covers postmark max (13.55) with headroom.
const EWMA_DISPLAY_MAX = 15.0;
// Thresholds from calibration (raw EWMA space):
//   baseline P95=4.94, disruption P5=4.34, disruption P50=7.40
const DEFAULT_LOW_THRESHOLD  = 4.5;
const DEFAULT_HIGH_THRESHOLD = 9.0;
const DEFAULT_MARGIN         = 0.5;
const DEFAULT_EWMA_ALPHA     = 0.16;
const DEFAULT_SATURATION_K   = 5;    // k=5 gives best baseline/disruption gap per calibration

const SCORE_BINS = 5;
const CHART_H = 96;
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

// Per-detector sigmoid normalization → [0, 1] used for bin display coloring only.
function perDetectorNormBin(a: TimelineAnomaly): number {
  const n = DETECTOR_NORM[a.detectorName];
  if (!n || a.score == null || Number.isNaN(a.score)) return 0;
  const z = (a.score - n.mu) / n.sigma;
  return 1 / (1 + Math.exp(-z));
}

// Raw score used as EWMA input.
// Known detectors with a normalization entry use their raw score.
// Known detectors without normalization fall back to DETECTOR_FIXED_SCORE (or 0).
// Unknown detectors return 0.
function perDetectorRawScore(a: TimelineAnomaly): number {
  const known = a.detectorName in DETECTOR_NORM || a.detectorName in DETECTOR_FIXED_SCORE;
  if (!known) return 0;
  const n = DETECTOR_NORM[a.detectorName];
  if (n && a.score != null && !Number.isNaN(a.score)) return a.score;
  return DETECTOR_FIXED_SCORE[a.detectorName] ?? 0;
}

function scoreToBin(normScore: number, scalePower: number): number {
  const clamped = Math.max(0, Math.min(1, normScore));
  return Math.min(SCORE_BINS - 1, Math.floor(Math.pow(clamped, scalePower) * SCORE_BINS));
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
  bins: number[];    // SCORE_BINS counts using sigmoid-normalized bin assignment
  scoreSum: number;  // sum of raw scores (for EWMA input)
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
  meanRawScore: number;
  ewmaValue: number;
  bucketStart: number;
  bucketEnd: number;
  events: SeverityEvent[];
}

export function AnomalyScoreTimeline({
  anomalies,
  scenarioStart,
  scenarioEnd,
  timeRange,
  phaseMarkers = [],
}: AnomalyScoreTimelineProps) {
  // ── display controls ──────────────────────────────────────────────────────
  const [scalePower, setScalePower] = useState(1.9);
  const [ewmaAlpha, setEwmaAlpha] = useState(DEFAULT_EWMA_ALPHA);
  const [saturationK, setSaturationK] = useState(DEFAULT_SATURATION_K);
  // aggregationWindow: 0 = auto (fit ~80 bars), >0 = manual seconds per display bar
  const [aggregationWindow, setAggregationWindow] = useState(0);

  // ── event-detection controls ──────────────────────────────────────────────
  const [lowThreshold, setLowThresholdRaw] = useState(DEFAULT_LOW_THRESHOLD);
  const [highThreshold, setHighThresholdRaw] = useState(DEFAULT_HIGH_THRESHOLD);
  const [margin, setMargin] = useState(DEFAULT_MARGIN);
  const [cooldownSecs, setCooldownSecs] = useState(300);

  const setLowThreshold = (v: number) => setLowThresholdRaw(Math.min(v, highThreshold - 0.1));
  const setHighThreshold = (v: number) => setHighThresholdRaw(Math.max(v, lowThreshold + 0.1));

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

  // ── 1-second raw buckets ──────────────────────────────────────────────────
  // Two separate scores per anomaly:
  //   - normBin: sigmoid(raw, mu, sigma) → [0,1] for visual bar coloring
  //   - rawScore: raw detector score (or 0 for count-only) → feeds EWMA
  const secondBuckets = useMemo(() => {
    if (!bounds) return new Map<number, SecondBucket>();
    const map = new Map<number, SecondBucket>();
    for (const a of anomalies) {
      if (a.timestamp < bounds.start || a.timestamp > bounds.end) continue;
      const sec = Math.floor(a.timestamp);
      if (!map.has(sec)) {
        map.set(sec, { bins: new Array<number>(SCORE_BINS).fill(0), scoreSum: 0, total: 0 });
      }
      const b = map.get(sec)!;
      const normBin = perDetectorNormBin(a);
      b.bins[scoreToBin(normBin, scalePower)] += 1;
      b.scoreSum += perDetectorRawScore(a);
      b.total += 1;
    }
    return map;
  }, [anomalies, bounds, scalePower]);

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

  // Fixed display scale — no retrospective min/max needed.
  const ewmaToY = useCallback(
    (v: number) => CHART_H - Math.min(1, Math.max(0, v / EWMA_DISPLAY_MAX)) * CHART_H,
    [],
  );

  const ewmaYValues = useMemo(() => ewmaValues.map(ewmaToY), [ewmaValues, ewmaToY]);

  const ewmaPolyline = useMemo(() => {
    if (ewmaYValues.length === 0 || chartWidth === 0 || displayCount === 0) return '';
    const bw = chartWidth / displayCount;
    return ewmaYValues.map((y, i) => `${((i + 0.5) * bw).toFixed(1)},${y.toFixed(1)}`).join(' ');
  }, [ewmaYValues, chartWidth, displayCount]);

  // ── threshold y positions (raw EWMA space, fixed scale) ───────────────────
  const lowThresholdY  = CHART_H - (lowThreshold  / EWMA_DISPLAY_MAX) * CHART_H;
  const highThresholdY = CHART_H - (highThreshold / EWMA_DISPLAY_MAX) * CHART_H;

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
        meanRawScore: b.total > 0 ? b.scoreSum / b.total : 0,
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

  const getBinRange = (binIdx: number) => {
    if (Math.abs(scalePower - 1.0) < 0.01) return `${binIdx * 20}–${binIdx === SCORE_BINS - 1 ? 100 : (binIdx + 1) * 20}%`;
    const s = Math.pow(binIdx / SCORE_BINS, 1 / scalePower);
    const e = Math.pow((binIdx + 1) / SCORE_BINS, 1 / scalePower);
    return `${Math.round(s * 100)}–${Math.round(e * 100)}%`;
  };

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
          <SliderRow label="Score scale" leftLabel="Spread" rightLabel="Compress"
            min={0.1} max={10} step={0.1} value={scalePower} onChange={setScalePower}
            valueLabel={`x^${scalePower.toFixed(2)}`} thumbHex="#a855f7" />
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
          <SliderRow label="Low threshold" leftLabel="0" rightLabel={EWMA_DISPLAY_MAX.toFixed(0)}
            min={0} max={EWMA_DISPLAY_MAX} step={0.1} value={lowThreshold} onChange={setLowThreshold}
            valueLabel={lowThreshold.toFixed(1)} thumbHex="#22c55e" />
          <SliderRow label="High threshold" leftLabel="0" rightLabel={EWMA_DISPLAY_MAX.toFixed(0)}
            min={0} max={EWMA_DISPLAY_MAX} step={0.1} value={highThreshold} onChange={setHighThreshold}
            valueLabel={highThreshold.toFixed(1)} thumbHex="#ef4444" />
          <SliderRow label="Margin (hysteresis)" leftLabel="0" rightLabel="3"
            min={0} max={3} step={0.1} value={margin} onChange={setMargin}
            valueLabel={margin.toFixed(1)} thumbHex="#f59e0b" />
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
                    <span className="text-slate-400">mean raw score</span>
                    <span className="text-slate-200">{hovered.meanRawScore.toFixed(2)}</span>
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
          <span>bins (norm):</span>
          {SCORE_COLORS.map((color, idx) => (
            <span key={idx} className="inline-flex items-center gap-1">
              <span className={`inline-block w-2.5 h-2.5 rounded-sm ${color}`} />
              {getBinRange(idx)}
            </span>
          ))}
          <span className="inline-flex items-center gap-1 ml-2">
            <svg width="16" height="8" viewBox="0 0 16 8">
              <polyline points="0,7 4,5 8,3 12,4 16,2" fill="none" stroke="#67e8f9" strokeWidth="1.5" strokeLinejoin="round" />
            </svg>
            EWMA (raw, max={EWMA_DISPLAY_MAX})
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
