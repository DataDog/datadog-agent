/**
 * AnomalyScoreTimeline — renders the Go scorer's ScoreState.
 *
 * Rendering only: no scoring logic here. The Go scorer (observer/impl/anomaly_scorer.go)
 * runs steps 0–5 (dedup → bucketing → saturation → EWMA → severity state machine).
 * This component performs ONLY Step 6 (display-window aggregation) for chart rendering.
 *
 * Sliders control the ScorerConfig posted to /api/scores/replay.
 * On slider change the component re-POSTs the config and replaces its local ScoreState.
 * On each SSE advance (scenarioDataVersion bump) the component re-fetches live state.
 */

import { useState, useMemo, useCallback, useEffect, useRef } from 'react';
import { api } from '../api/client';
import type { ScoreState, ScoreBucket, ScorerConfig } from '../api/client';
import type { PhaseMarker } from './ChartWithAnomalyDetails';

// ── Constants ──────────────────────────────────────────────────────────────

const LEVEL_WEIGHTS = [0.2, 0.5, 1.0, 2.0, 3.0] as const;
const LEVEL_LABELS = ['VeryLow', 'Low', 'Medium', 'High', 'XHigh'] as const;
const LEVEL_COLORS = ['#64748b', '#eab308', '#f97316', '#ef4444', '#c026d3'] as const;

const DEFAULT_EWMA_ALPHA = 0.014;
const DEFAULT_SATURATION_K = 5;
const DEFAULT_LOW_THRESHOLD = 0.040;
const DEFAULT_HIGH_THRESHOLD = 0.060;
const DEFAULT_MARGIN_PCT = 0.20;
const DEFAULT_COOLDOWN = 300;

const CHART_H = 240;
const CHART_TOP_PADDING = 0.20;

const EWMA_SLIDER_MAX = 5.0;

// ── Persistent state ────────────────────────────────────────────────────────

function usePersistedState<T>(key: string, def: T): [T, (v: T) => void] {
  const storageKey = 'ast:' + key;
  const [val, setVal] = useState<T>(() => {
    try {
      const raw = localStorage.getItem(storageKey);
      return raw !== null ? (JSON.parse(raw) as T) : def;
    } catch {
      return def;
    }
  });
  const set = useCallback((v: T) => {
    setVal(v);
    try { localStorage.setItem(storageKey, JSON.stringify(v)); } catch { /* ignore */ }
  }, [storageKey]);
  return [val, set];
}

// ── Display aggregation (Step 6 — testbench only) ──────────────────────────

interface DisplayBucket {
  bins: [number, number, number, number, number];
  count: number;
  ewma: number;
  startSec: number;
}

function aggregateBuckets(buckets: ScoreBucket[], windowSecs: number): DisplayBucket[] {
  if (buckets.length === 0) return [];
  const start = buckets[0].second;
  const end = buckets[buckets.length - 1].second;
  const result: DisplayBucket[] = [];
  for (let t = start; t <= end; t += windowSecs) {
    const slice = buckets.filter(b => b.second >= t && b.second < t + windowSecs);
    const bins: [number, number, number, number, number] = [0, 0, 0, 0, 0];
    let count = 0;
    let ewma = 0;
    for (const b of slice) {
      for (let l = 0; l < 5; l++) bins[l] += b.bins[l];
      count += b.count;
      ewma = b.ewma; // last EWMA in the window
    }
    result.push({ bins, count, ewma, startSec: t });
  }
  return result;
}

function autoWindow(buckets: ScoreBucket[], targetBars = 80): number {
  if (buckets.length === 0) return 1;
  const span = buckets[buckets.length - 1].second - buckets[0].second + 1;
  return Math.max(1, Math.ceil(span / targetBars));
}

// ── Helpers ─────────────────────────────────────────────────────────────────

function formatDuration(secs: number): string {
  if (secs < 60) return `${secs}s`;
  if (secs < 3600) return `${Math.round(secs / 60)}m`;
  return `${(secs / 3600).toFixed(1)}h`;
}

function formatTs(unix: number): string {
  return new Date(unix * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

const SEVERITY_COLORS: Record<number, string> = {
  0: '#22c55e',
  1: '#f59e0b',
  2: '#ef4444',
};
const SEVERITY_LABELS: Record<number, string> = { 0: 'Low', 1: 'Medium', 2: 'High' };

// ── Main component ──────────────────────────────────────────────────────────

export function AnomalyScoreTimeline({
  scenarioDataVersion,
  phaseMarkers = [],
}: {
  scenarioDataVersion: number;
  phaseMarkers?: PhaseMarker[];
}) {
  // Scorer config sliders (persisted)
  const [ewmaAlpha, setEwmaAlpha] = usePersistedState('ewmaAlpha', DEFAULT_EWMA_ALPHA);
  const [saturationK, setSaturationK] = usePersistedState('saturationK', DEFAULT_SATURATION_K);
  const [aggregationWindow, setAggregationWindow] = usePersistedState('aggregationWindow', 0);
  const [lowThreshold, setLowThresholdRaw] = usePersistedState('lowThreshold', DEFAULT_LOW_THRESHOLD);
  const [highThreshold, setHighThresholdRaw] = usePersistedState('highThreshold', DEFAULT_HIGH_THRESHOLD);
  const [marginPct, setMarginPct] = usePersistedState('marginPct', DEFAULT_MARGIN_PCT);
  const [cooldownSecs, setCooldownSecs] = usePersistedState('cooldownSecs', DEFAULT_COOLDOWN);

  const setLowThreshold = (v: number) => setLowThresholdRaw(Math.min(v, highThreshold * 0.95));
  const setHighThreshold = (v: number) => setHighThresholdRaw(Math.max(v, lowThreshold * 1.05));

  // Fetched state
  const [scoreState, setScoreState] = useState<ScoreState | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Current config (derived from sliders)
  const currentConfig = useMemo((): ScorerConfig => ({
    alpha: ewmaAlpha,
    saturation_k: saturationK,
    low_threshold: lowThreshold,
    high_threshold: highThreshold,
    margin_pct: marginPct,
    cooldown_secs: cooldownSecs,
  }), [ewmaAlpha, saturationK, lowThreshold, highThreshold, marginPct, cooldownSecs]);

  // Fetch live scores on scenario data version change
  useEffect(() => {
    if (scenarioDataVersion === 0) return;
    setLoading(true);
    api.replayScores(currentConfig)
      .then(st => { setScoreState(st); setError(null); })
      .catch(e => setError(String(e)))
      .finally(() => setLoading(false));
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scenarioDataVersion]);

  // Re-replay on slider change (debounced 250ms)
  const replayWithConfig = useCallback((cfg: ScorerConfig) => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      setLoading(true);
      api.replayScores(cfg)
        .then(st => { setScoreState(st); setError(null); })
        .catch(e => setError(String(e)))
        .finally(() => setLoading(false));
    }, 250);
  }, []);

  // Trigger replay when any slider changes
  useEffect(() => {
    if (scenarioDataVersion === 0) return;
    replayWithConfig(currentConfig);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ewmaAlpha, saturationK, lowThreshold, highThreshold, marginPct, cooldownSecs]);

  const buckets = scoreState?.buckets ?? [];
  const events = scoreState?.events ?? [];

  // Step 6: display aggregation
  const resolvedWindow = aggregationWindow === 0 ? autoWindow(buckets) : aggregationWindow;
  const displayBuckets = useMemo(() => aggregateBuckets(buckets, resolvedWindow), [buckets, resolvedWindow]);

  const displayCount = displayBuckets.length;

  // EWMA chart scale: max(ewmaPeak, high) × (1 + CHART_TOP_PADDING)
  const ewmaDisplayMax = useMemo(() => {
    const peak = buckets.reduce((m, b) => Math.max(m, b.ewma), 0);
    return Math.max(highThreshold, peak) * (1 + CHART_TOP_PADDING);
  }, [buckets, highThreshold]);

  const ewmaPeak = useMemo(() => buckets.reduce((m, b) => Math.max(m, b.ewma), 0), [buckets]);
  const ewmaEnd = buckets.length > 0 ? buckets[buckets.length - 1].ewma : 0;

  // Max anomaly count (for bar height scaling)
  const maxCount = useMemo(() => displayBuckets.reduce((m, b) => Math.max(m, b.count), 0), [displayBuckets]);

  // Layout
  const CHART_W = 800;
  const barW = displayCount > 0 ? CHART_W / displayCount : 0;
  const bucketStart = buckets.length > 0 ? buckets[0].second : 0;
  const bucketEnd = buckets.length > 0 ? buckets[buckets.length - 1].second : 0;

  const tsToX = useCallback((ts: number): number => {
    if (bucketEnd === bucketStart) return 0;
    return ((ts - bucketStart) / (bucketEnd - bucketStart)) * CHART_W;
  }, [bucketStart, bucketEnd]);

  const ewmaToY = useCallback((v: number): number => {
    return CHART_H - Math.min(1, Math.max(0, v / ewmaDisplayMax)) * CHART_H;
  }, [ewmaDisplayMax]);

  const lowThresholdY = CHART_H - (lowThreshold / ewmaDisplayMax) * CHART_H;
  const highThresholdY = CHART_H - (highThreshold / ewmaDisplayMax) * CHART_H;

  // Severity background segments
  const stateSegments = useMemo(() => {
    if (displayBuckets.length === 0 || events.length === 0) return [];
    // Map event timestamps to x positions
    const segs: { x1: number; x2: number; level: number }[] = [];
    let curLevel = 0;
    let curX = 0;
    for (const ev of events) {
      const x = tsToX(ev.timestamp);
      segs.push({ x1: curX, x2: x, level: curLevel });
      curLevel = ev.to_level;
      curX = x;
    }
    segs.push({ x1: curX, x2: CHART_W, level: curLevel });
    return segs.filter(s => s.level > 0);
  }, [displayBuckets, events, tsToX]);

  // EWMA polyline
  const ewmaPoints = useMemo(() => {
    if (displayBuckets.length === 0) return '';
    return displayBuckets
      .map((b, i) => `${i * barW + barW / 2},${ewmaToY(b.ewma)}`)
      .join(' ');
  }, [displayBuckets, barW, ewmaToY]);

  const totalAnomalies = buckets.reduce((s, b) => s + b.count, 0);
  const upEvents = events.filter(e => e.to_level > e.from_level).length;
  const downEvents = events.filter(e => e.to_level < e.from_level).length;

  return (
    <div className="bg-slate-800 rounded-lg border border-slate-700 p-4">
      {/* Header */}
      <div className="flex items-center justify-between mb-3">
        <div>
          <h3 className="text-sm font-semibold text-white">Anomaly intensity timeline</h3>
          <div className="text-xs text-slate-400 mt-0.5">
            {totalAnomalies} anomalies · {upEvents} ▲ {downEvents} ▼ events
            {loading && <span className="ml-2 text-slate-500">updating…</span>}
            {error && <span className="ml-2 text-red-400">{error}</span>}
          </div>
        </div>
        <div className="flex gap-4 text-xs">
          <span className="text-slate-400">window: {aggregationWindow === 0 ? `auto (${resolvedWindow}s)` : `${resolvedWindow}s`}</span>
          <span className="text-cyan-300">EWMA end: {ewmaEnd.toFixed(3)}</span>
          <span className="text-cyan-400 font-medium">EWMA peak: {ewmaPeak.toFixed(3)}</span>
        </div>
      </div>

      {/* Controls */}
      <div className="grid grid-cols-2 gap-x-8 gap-y-2 mb-3">
        <div className="space-y-2">
          <div className="text-[10px] font-semibold text-slate-500 uppercase tracking-wider mb-1">Display</div>
          <SliderRow label="EWMA α" leftLabel="Smooth" rightLabel="Raw"
            min={0.001} max={1} step={0.001} value={ewmaAlpha}
            onChange={v => { setEwmaAlpha(v); }}
            valueLabel={ewmaAlpha.toFixed(3)} thumbHex="#67e8f9" logScale />
          <SliderRow label="Count saturation k" leftLabel="1 (fast)" rightLabel="30 (slow)"
            min={1} max={30} step={1} value={saturationK}
            onChange={setSaturationK}
            valueLabel={`k=${saturationK}`} thumbHex="#f97316" />
          <SliderRow label="Aggregation window" leftLabel="1s" rightLabel="10m"
            min={0} max={600} step={1} value={aggregationWindow}
            onChange={setAggregationWindow}
            valueLabel={aggregationWindow === 0 ? 'auto' : formatDuration(aggregationWindow)} thumbHex="#94a3b8" />
        </div>
        <div className="space-y-2">
          <div className="text-[10px] font-semibold text-slate-500 uppercase tracking-wider mb-1">Event detection</div>
          <SliderRow label="Low threshold" leftLabel="0.01" rightLabel={ewmaDisplayMax.toFixed(2)}
            min={0.01} max={Math.max(EWMA_SLIDER_MAX, ewmaDisplayMax)} step={0.001}
            value={lowThreshold} onChange={setLowThreshold}
            valueLabel={lowThreshold.toFixed(3)} thumbHex="#22c55e" logScale />
          <SliderRow label="High threshold" leftLabel="0.01" rightLabel={ewmaDisplayMax.toFixed(2)}
            min={0.01} max={Math.max(EWMA_SLIDER_MAX, ewmaDisplayMax)} step={0.001}
            value={highThreshold} onChange={setHighThreshold}
            valueLabel={highThreshold.toFixed(3)} thumbHex="#ef4444" logScale />
          <SliderRow label="Margin (hysteresis)" leftLabel="0%" rightLabel="50%"
            min={0} max={0.50} step={0.01} value={marginPct} onChange={setMarginPct}
            valueLabel={`${(marginPct * 100).toFixed(0)}% of high`} thumbHex="#f59e0b" />
          <SliderRow label="Cooldown (decrease)" leftLabel="0s" rightLabel="10m"
            min={0} max={600} step={5} value={cooldownSecs} onChange={setCooldownSecs}
            valueLabel={formatDuration(cooldownSecs)} thumbHex="#a78bfa" />
        </div>
      </div>

      {/* Chart */}
      {displayBuckets.length === 0 ? (
        <div className="h-24 flex items-center justify-center text-slate-500 text-sm">
          {scenarioDataVersion === 0 ? 'Load a scenario to see the timeline' : 'No anomalies detected yet'}
        </div>
      ) : (
        <div className="overflow-x-auto">
          <svg width={CHART_W} height={CHART_H + 20} className="block">
            {/* Severity background */}
            {stateSegments.map((seg, i) => (
              <rect key={i} x={seg.x1} y={0} width={seg.x2 - seg.x1} height={CHART_H}
                fill={SEVERITY_COLORS[seg.level]} fillOpacity={0.08} />
            ))}

            {/* Phase marker lines */}
            {phaseMarkers.map((marker) => {
              const x = tsToX(marker.timestamp);
              if (x < -20 || x > CHART_W + 20) return null;
              return (
                <g key={marker.key}>
                  <line x1={x} y1={0} x2={x} y2={CHART_H}
                    stroke={marker.color} strokeWidth={1} strokeDasharray="4,3" opacity={0.75} />
                  <text x={x + 3} y={10} fontSize={9} fill={marker.color}
                    fontFamily="monospace" opacity={0.9} style={{ pointerEvents: 'none' }}>
                    {marker.label}
                  </text>
                </g>
              );
            })}

            {/* Stacked anomaly bars */}
            {displayBuckets.map((b, i) => {
              if (b.count === 0) return null;
              const totalH = maxCount > 0 ? (b.count / maxCount) * CHART_H * 0.9 : 0;
              let yOff = 0;
              return (
                <g key={i}>
                  {[4, 3, 2, 1, 0].map(l => {
                    const h = maxCount > 0 ? (b.bins[l] / maxCount) * CHART_H * 0.9 : 0;
                    if (h < 0.5) return null;
                    const rect = (
                      <rect key={l}
                        x={i * barW + 1} y={CHART_H - totalH + yOff}
                        width={Math.max(1, barW - 2)} height={h}
                        fill={LEVEL_COLORS[l]} fillOpacity={0.85} />
                    );
                    yOff += h;
                    return rect;
                  })}
                </g>
              );
            })}

            {/* Threshold lines */}
            <line x1={0} y1={lowThresholdY} x2={CHART_W} y2={lowThresholdY}
              stroke="#22c55e" strokeWidth={1} strokeDasharray="4 2" opacity={0.7} />
            <line x1={0} y1={highThresholdY} x2={CHART_W} y2={highThresholdY}
              stroke="#ef4444" strokeWidth={1} strokeDasharray="4 2" opacity={0.7} />

            {/* EWMA line */}
            {ewmaPoints && (
              <polyline points={ewmaPoints} fill="none" stroke="#67e8f9" strokeWidth={1.5} opacity={0.9} />
            )}

            {/* Severity event triangles */}
            {events.map((ev, i) => {
              const x = tsToX(ev.timestamp);
              const isUp = ev.to_level > ev.from_level;
              const color = SEVERITY_COLORS[ev.to_level] ?? '#94a3b8';
              const y = isUp ? CHART_H + 2 : CHART_H + 12;
              const pts = isUp
                ? `${x},${y - 8} ${x - 5},${y} ${x + 5},${y}`
                : `${x},${y + 8} ${x - 5},${y} ${x + 5},${y}`;
              return <polygon key={i} points={pts} fill={color} opacity={0.9} />;
            })}

            {/* Threshold labels (right side) */}
            <text x={CHART_W - 2} y={lowThresholdY - 2} fontSize={9} fill="#22c55e" textAnchor="end">
              {lowThreshold.toFixed(3)}
            </text>
            <text x={CHART_W - 2} y={highThresholdY - 2} fontSize={9} fill="#ef4444" textAnchor="end">
              {highThreshold.toFixed(3)}
            </text>

            {/* Time axis labels */}
            {buckets.length > 0 && (
              <>
                <text x={2} y={CHART_H + 15} fontSize={9} fill="#64748b">{formatTs(bucketStart)}</text>
                <text x={CHART_W - 2} y={CHART_H + 15} fontSize={9} fill="#64748b" textAnchor="end">
                  {formatTs(bucketEnd)}
                </text>
              </>
            )}
          </svg>
        </div>
      )}

      {/* Legend */}
      <div className="flex flex-wrap gap-x-4 gap-y-1 mt-2 text-[10px]">
        {LEVEL_LABELS.map((label, i) => (
          <span key={label} className="flex items-center gap-1">
            <span className="w-2.5 h-2.5 rounded-sm inline-block" style={{ background: LEVEL_COLORS[i] }} />
            <span className="text-slate-400">{label} ({LEVEL_WEIGHTS[i]})</span>
          </span>
        ))}
        <span className="flex items-center gap-1 text-cyan-300">
          <span className="w-6 h-0.5 inline-block bg-cyan-300" />
          EWMA (ceil={ewmaDisplayMax.toFixed(3)})
        </span>
        {Object.entries(SEVERITY_LABELS).map(([lvl, label]) => (
          <span key={lvl} className="flex items-center gap-1">
            <span style={{ color: SEVERITY_COLORS[Number(lvl)] }}>▲▼</span>
            <span className="text-slate-400">{label}</span>
          </span>
        ))}
      </div>
    </div>
  );
}

// ── SliderRow ────────────────────────────────────────────────────────────────

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
  thumbHex: string;
  logScale?: boolean;
}

function SliderRow({ label, leftLabel, rightLabel, min, max, step, value, onChange, valueLabel, thumbHex, logScale = false }: SliderRowProps) {
  const toPos = (v: number) => logScale ? (Math.log(v) - Math.log(min)) / (Math.log(max) - Math.log(min)) : v;
  const toVal = (p: number) => logScale ? min * Math.pow(max / min, p) : p;

  return (
    <div className="flex items-center gap-2">
      <label className="text-xs text-slate-400 min-w-[130px] shrink-0">{label}:</label>
      <span className="text-[10px] text-slate-600 shrink-0">{leftLabel}</span>
      <input
        type="range"
        min={logScale ? 0 : min} max={logScale ? 1 : max} step={logScale ? 0.001 : step}
        value={toPos(value)}
        onChange={e => onChange(toVal(parseFloat(e.target.value)))}
        style={{ '--thumb-color': thumbHex } as React.CSSProperties}
        className="flex-1 h-1 bg-slate-700 rounded appearance-none cursor-pointer
          [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:w-3
          [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-[var(--thumb-color)]
          [&::-moz-range-thumb]:h-3 [&::-moz-range-thumb]:w-3 [&::-moz-range-thumb]:rounded-full
          [&::-moz-range-thumb]:bg-[var(--thumb-color)] [&::-moz-range-thumb]:border-none"
      />
      <span className="text-[10px] text-slate-600 shrink-0">{rightLabel}</span>
      <span className="text-xs text-white min-w-[60px] text-right">{valueLabel}</span>
    </div>
  );
}
