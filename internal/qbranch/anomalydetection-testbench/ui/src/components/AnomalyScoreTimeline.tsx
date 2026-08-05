/**
 * AnomalyScoreTimeline — renders the Go scorer's ScoreState.
 *
 * Rendering only: no scoring logic here. The Go scorer (observer/impl/anomaly_scorer.go)
 * runs steps 0–5 (dedup → bucketing → saturation → EWMA → severity state machine).
 * This component performs ONLY Step 6 (display-window aggregation) for chart rendering.
 *
 * Config values are read-only; they are driven by the server-returned ScoreState.config.
 * On each scenarioDataVersion bump the component POSTs the default config to
 * /api/scores/replay and replaces its local ScoreState with the response.
 */

import { useState, useMemo, useCallback, useEffect, useRef } from 'react';
import { api } from '../api/client';
import type { ScoreState, ScoreBucket, ScorerConfig, SeverityEvent } from '../api/client';
import type { PhaseMarker } from './ChartWithAnomalyDetails';

// ── Constants ──────────────────────────────────────────────────────────────

const LEVEL_WEIGHTS = [0.2, 0.5, 1.0, 2.0, 3.0] as const;
const LEVEL_LABELS = ['VeryLow', 'Low', 'Medium', 'High', 'XHigh'] as const;
const LEVEL_COLORS = ['#64748b', '#eab308', '#f97316', '#ef4444', '#c026d3'] as const;

// Fallback used only while GET /api/scores/config is in-flight.
// These values are never sent to the backend for replay.
const DISPLAY_FALLBACK_CONFIG: ScorerConfig = {
  alpha: 0.014,
  saturation_k: 5,
  low_threshold: 0.15,
  high_threshold: 0.40,
  margin_pct: 0.20,
  cooldown_secs: 300,
};

const CHART_H = 240;
const CHART_TOP_PADDING = 0.20;

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

// ── Read-only config row ────────────────────────────────────────────────────

function ConfigRow({ label, value, color }: { label: string; value: string; color?: string }) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-xs text-slate-400 min-w-[130px] shrink-0">{label}:</span>
      <span className="text-xs font-mono ml-auto" style={color ? { color } : undefined}>
        {value}
      </span>
    </div>
  );
}

// ── Main component ──────────────────────────────────────────────────────────

interface TimeRange {
  start: number;
  end: number;
}

export function AnomalyScoreTimeline({
  scenarioDataVersion,
  phaseMarkers = [],
  timeRange,
  hoveredEvent,
  onTimeRangeChange,
  onScoreState,
}: {
  scenarioDataVersion: number;
  phaseMarkers?: PhaseMarker[];
  timeRange?: TimeRange | null;
  hoveredEvent?: SeverityEvent | null;
  onTimeRangeChange?: (range: TimeRange | null) => void;
  onScoreState?: (ss: ScoreState) => void;
}) {
  const [scoreState, setScoreState] = useState<ScoreState | null>(null);
  const [replayConfig, setReplayConfig] = useState<ScorerConfig | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Fetch the canonical default config from the backend once on mount so we
  // never need to duplicate threshold values in the frontend.
  useEffect(() => {
    api.getScoresConfig()
      .then(cfg => setReplayConfig(cfg))
      .catch(() => setReplayConfig(DISPLAY_FALLBACK_CONFIG));
  }, []);

  // Responsive chart width via ResizeObserver
  const chartContainerRef = useRef<HTMLDivElement>(null);
  const [chartW, setChartW] = useState(800);
  useEffect(() => {
    const el = chartContainerRef.current;
    if (!el) return;
    const ro = new ResizeObserver(() => {
      const w = el.clientWidth;
      if (w > 0) setChartW(w);
    });
    ro.observe(el);
    const w = el.clientWidth;
    if (w > 0) setChartW(w);
    return () => ro.disconnect();
  }, []);

  // Replay whenever a new scenario is loaded or the config first arrives.
  useEffect(() => {
    if (scenarioDataVersion === 0 || !replayConfig) return;
    setLoading(true);
    api.replayScores(replayConfig)
      .then(st => { setScoreState(st); setError(null); onScoreState?.(st); })
      .catch(e => setError(String(e)))
      .finally(() => setLoading(false));
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scenarioDataVersion, replayConfig]);

  const allBuckets = scoreState?.buckets ?? [];
  const allEvents = scoreState?.events ?? [];
  // Merge: use replayConfig (fetched from /api/scores/config) as the base so that
  // cooldown_secs is always present, then overlay EWMA fields from scoreState.config.
  // scoreState.config only contains the def-level AnomalyScorerConfig (no cooldown).
  const cfg: ScorerConfig = { ...(replayConfig ?? DISPLAY_FALLBACK_CONFIG), ...(scoreState?.config ?? {}) };

  // Filter buckets to the selected time range (client-side zoom)
  const buckets = useMemo(() => {
    if (!timeRange) return allBuckets;
    return allBuckets.filter(b => b.second >= timeRange.start && b.second <= timeRange.end);
  }, [allBuckets, timeRange]);

  // Step 6: display aggregation
  const resolvedWindow = autoWindow(buckets);
  const displayBuckets = useMemo(() => aggregateBuckets(buckets, resolvedWindow), [buckets, resolvedWindow]);
  const displayCount = displayBuckets.length;

  // EWMA chart scale
  const ewmaDisplayMax = useMemo(() => {
    const peak = buckets.reduce((m, b) => Math.max(m, b.ewma), 0);
    return Math.max(cfg.high_threshold, peak) * (1 + CHART_TOP_PADDING);
  }, [buckets, cfg.high_threshold]);

  const ewmaPeak = useMemo(() => buckets.reduce((m, b) => Math.max(m, b.ewma), 0), [buckets]);
  const ewmaEnd = buckets.length > 0 ? buckets[buckets.length - 1].ewma : 0;
  const maxCount = useMemo(() => displayBuckets.reduce((m, b) => Math.max(m, b.count), 0), [displayBuckets]);

  // Layout
  const bucketStart = buckets.length > 0 ? buckets[0].second : 0;
  const bucketEnd = buckets.length > 0 ? buckets[buckets.length - 1].second : 0;

  // Extend left edge to scenario start (earliest phase marker) unless zoomed
  const displayStart = useMemo(() => {
    if (buckets.length === 0) return 0;
    if (timeRange) return Math.min(timeRange.start, bucketStart);
    return phaseMarkers.reduce((min, m) => Math.min(min, m.timestamp), bucketStart);
  }, [buckets, phaseMarkers, bucketStart, timeRange]);

  const tsToX = useCallback((ts: number): number => {
    if (bucketEnd === displayStart) return 0;
    return ((ts - displayStart) / (bucketEnd - displayStart)) * chartW;
  }, [displayStart, bucketEnd, chartW]);

  const xToTs = useCallback((x: number): number => {
    if (bucketEnd <= displayStart) return displayStart;
    return displayStart + (x / chartW) * (bucketEnd - displayStart);
  }, [displayStart, bucketEnd, chartW]);

  const barW = useMemo(() => {
    if (displayCount === 0 || bucketEnd <= displayStart) return 0;
    return (resolvedWindow / (bucketEnd - displayStart)) * chartW;
  }, [displayCount, resolvedWindow, displayStart, bucketEnd, chartW]);

  const ewmaToY = useCallback((v: number): number => {
    return CHART_H - Math.min(1, Math.max(0, v / ewmaDisplayMax)) * CHART_H;
  }, [ewmaDisplayMax]);

  const lowThresholdY = CHART_H - (cfg.low_threshold / ewmaDisplayMax) * CHART_H;
  const highThresholdY = CHART_H - (cfg.high_threshold / ewmaDisplayMax) * CHART_H;

  // Severity background segments
  const stateSegments = useMemo(() => {
    if (buckets.length === 0 || allEvents.length === 0) return [];
    const rangeStart = buckets[0].second;
    const rangeEnd = buckets[buckets.length - 1].second;
    let curLevel = 0;
    for (const ev of allEvents) {
      if (ev.timestamp <= rangeStart) curLevel = ev.to_level;
    }
    const segs: { x1: number; x2: number; level: number }[] = [];
    let curX = 0;
    for (const ev of allEvents) {
      if (ev.timestamp <= rangeStart) continue;
      if (ev.timestamp > rangeEnd) break;
      const x = tsToX(ev.timestamp);
      segs.push({ x1: curX, x2: x, level: curLevel });
      curLevel = ev.to_level;
      curX = x;
    }
    segs.push({ x1: curX, x2: chartW, level: curLevel });
    return segs.filter(s => s.level > 0);
  }, [buckets, allEvents, tsToX, chartW]);

  // Hovered severity event region for highlight overlay
  const hoveredEventRegion = useMemo(() => {
    if (!hoveredEvent || buckets.length === 0) return null;
    const idx = allEvents.findIndex(
      e => e.timestamp === hoveredEvent.timestamp && e.to_level === hoveredEvent.to_level,
    );
    if (idx < 0) return null;
    const start = hoveredEvent.timestamp;
    const end = idx + 1 < allEvents.length ? allEvents[idx + 1].timestamp : bucketEnd;
    return { start, end };
  }, [hoveredEvent, allEvents, bucketEnd, buckets.length]);

  // EWMA polyline with optional flat-zero prefix
  const ewmaPoints = useMemo(() => {
    if (displayBuckets.length === 0) return '';
    const dataPts = displayBuckets.map(b => `${tsToX(b.startSec) + barW / 2},${ewmaToY(b.ewma)}`);
    if (displayStart < bucketStart) {
      const y0 = ewmaToY(0);
      dataPts.unshift(`${tsToX(bucketStart)},${y0}`, `${tsToX(displayStart)},${y0}`);
    }
    return dataPts.join(' ');
  }, [displayBuckets, barW, tsToX, ewmaToY, displayStart, bucketStart]);

  // Pre-anomaly warmup region: the portion of the chart before the first bucket
  const warmupRegion = useMemo(() => {
    if (displayStart >= bucketStart) return null;
    return { x1: 0, x2: tsToX(bucketStart), durationSecs: bucketStart - displayStart };
  }, [displayStart, bucketStart, tsToX]);

  const totalAnomalies = buckets.reduce((s, b) => s + b.count, 0);
  const upEvents = allEvents.filter(e => e.to_level > e.from_level).length;
  const downEvents = allEvents.filter(e => e.to_level < e.from_level).length;

  // ── Interaction state ────────────────────────────────────────────────────

  const svgRef = useRef<SVGSVGElement>(null);
  const [hoverX, setHoverX] = useState<number | null>(null);
  const [dragStartX, setDragStartX] = useState<number | null>(null);
  const [dragCurrentX, setDragCurrentX] = useState<number | null>(null);
  const isDragging = dragStartX !== null;

  const getEventX = useCallback((e: React.MouseEvent): number => {
    const rect = svgRef.current?.getBoundingClientRect();
    if (!rect) return 0;
    return Math.max(0, Math.min(chartW, e.clientX - rect.left));
  }, [chartW]);

  const handleMouseMove = useCallback((e: React.MouseEvent) => {
    const x = getEventX(e);
    setHoverX(x);
    if (dragStartX !== null) setDragCurrentX(x);
  }, [getEventX, dragStartX]);

  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    if (e.button !== 0 || displayBuckets.length === 0) return;
    e.preventDefault();
    const x = getEventX(e);
    setDragStartX(x);
    setDragCurrentX(x);
  }, [getEventX, displayBuckets.length]);

  const handleMouseUp = useCallback((e: React.MouseEvent) => {
    if (dragStartX === null || dragCurrentX === null) return;
    const x1 = Math.min(dragStartX, dragCurrentX);
    const x2 = Math.max(dragStartX, dragCurrentX);
    if (x2 - x1 > 8 && onTimeRangeChange) {
      const t1 = Math.round(xToTs(x1));
      const t2 = Math.round(xToTs(x2));
      if (t2 - t1 >= 2) onTimeRangeChange({ start: t1, end: t2 });
    }
    setDragStartX(null);
    setDragCurrentX(null);
  }, [dragStartX, dragCurrentX, xToTs, onTimeRangeChange]);

  const handleMouseLeave = useCallback(() => {
    setHoverX(null);
    if (dragStartX !== null && dragCurrentX !== null) {
      // Complete drag if meaningful
      const x1 = Math.min(dragStartX, dragCurrentX);
      const x2 = Math.max(dragStartX, dragCurrentX);
      if (x2 - x1 > 8 && onTimeRangeChange) {
        const t1 = Math.round(xToTs(x1));
        const t2 = Math.round(xToTs(x2));
        if (t2 - t1 >= 2) onTimeRangeChange({ start: t1, end: t2 });
      }
    }
    setDragStartX(null);
    setDragCurrentX(null);
  }, [dragStartX, dragCurrentX, xToTs, onTimeRangeChange]);

  // Tooltip: find the display bucket under the cursor
  const hoverBucket = useMemo(() => {
    if (hoverX === null || isDragging || displayBuckets.length === 0) return null;
    const ts = xToTs(hoverX);
    return displayBuckets.find(b => ts >= b.startSec && ts < b.startSec + resolvedWindow) ?? null;
  }, [hoverX, isDragging, displayBuckets, xToTs, resolvedWindow]);

  const hoverTs = hoverX !== null && !isDragging ? xToTs(hoverX) : null;

  return (
    <div className="bg-slate-800 rounded-lg border border-slate-700 p-4">
      {/* Header */}
      <div className="flex items-center justify-between mb-3">
        <div>
          <h3 className="text-sm font-semibold text-white">Anomaly intensity timeline</h3>
          <div className="text-xs text-slate-400 mt-0.5">
            {loading
              ? 'Updating…'
              : error
              ? <span className="text-red-400">{error}</span>
              : allBuckets.length === 0
              ? 'No data'
              : `${totalAnomalies} anomalies · ${buckets.length}s · ${upEvents}↑ ${downEvents}↓ severity transitions`}
          </div>
        </div>
        <div className="text-right text-xs space-y-0.5">
          <div className="text-slate-400">window: auto ({resolvedWindow}s)</div>
          <div className="text-cyan-300">EWMA end: {ewmaEnd.toFixed(3)}</div>
          <div className="text-cyan-400 font-medium">EWMA peak: {ewmaPeak.toFixed(3)}</div>
        </div>
      </div>

      {/* Read-only config */}
      <div className="grid grid-cols-2 gap-x-8 gap-y-2 mb-3">
        <div className="space-y-1.5">
          <div className="text-[10px] font-semibold text-slate-500 uppercase tracking-wider mb-1">Signal</div>
          <ConfigRow label="EWMA α" value={cfg.alpha.toFixed(3)} color="#67e8f9" />
          <ConfigRow label="Count saturation k" value={`k=${cfg.saturation_k}`} color="#f97316" />
        </div>
        <div className="space-y-1.5">
          <div className="text-[10px] font-semibold text-slate-500 uppercase tracking-wider mb-1">Event detection</div>
          <ConfigRow label="Low threshold" value={cfg.low_threshold.toFixed(3)} color="#22c55e" />
          <ConfigRow label="High threshold" value={cfg.high_threshold.toFixed(3)} color="#ef4444" />
          <ConfigRow label="Margin (hysteresis)" value={`${(cfg.margin_pct * 100).toFixed(0)}% of high`} color="#f59e0b" />
          <ConfigRow label="Cooldown (decrease)" value={formatDuration(cfg.cooldown_secs)} color="#a78bfa" />
        </div>
      </div>

      {/* Chart */}
      {displayBuckets.length === 0 ? (
        <div className="h-24 flex items-center justify-center text-slate-500 text-sm">
          {scenarioDataVersion === 0 ? 'Load a scenario to see the timeline' : 'No anomalies detected yet'}
        </div>
      ) : (
        <div ref={chartContainerRef} className="relative select-none">
          {/* Hover tooltip */}
          {hoverBucket && hoverX !== null && (
            <div
              className="absolute z-20 pointer-events-none bg-slate-900/95 border border-slate-600 rounded-lg p-2 shadow-xl text-xs min-w-[140px]"
              style={{
                top: 4,
                left: hoverX + 10,
              }}
            >
              <div className="font-mono text-white font-medium mb-1">{formatTs(hoverBucket.startSec)}</div>
              <div className="text-cyan-300 mb-1.5">
                EWMA: <span className="font-mono">{hoverBucket.ewma.toFixed(4)}</span>
              </div>
              {hoverBucket.count > 0 ? (
                <div className="space-y-0.5">
                  {LEVEL_LABELS.map((label, i) =>
                    hoverBucket.bins[i] > 0 ? (
                      <div key={label} className="flex items-center gap-1.5">
                        <span className="w-2 h-2 rounded-sm shrink-0" style={{ background: LEVEL_COLORS[i] }} />
                        <span className="text-slate-400">{label}</span>
                        <span className="font-mono text-slate-200 ml-auto">{hoverBucket.bins[i]}</span>
                      </div>
                    ) : null,
                  )}
                  <div className="text-slate-500 pt-0.5 border-t border-slate-700 flex justify-between">
                    <span>Total</span>
                    <span className="font-mono text-slate-300">{hoverBucket.count}</span>
                  </div>
                </div>
              ) : (
                <div className="text-slate-500 text-[10px]">No anomalies</div>
              )}
            </div>
          )}

          <svg
            ref={svgRef}
            width={chartW}
            height={CHART_H + 26}
            className="block"
            style={{ cursor: isDragging ? 'ew-resize' : 'crosshair' }}
            onMouseMove={handleMouseMove}
            onMouseDown={handleMouseDown}
            onMouseUp={handleMouseUp}
            onMouseLeave={handleMouseLeave}
          >
            {/* Pre-anomaly warmup shading: no scorer data yet (detector warmup) */}
            {warmupRegion && warmupRegion.x2 > 0 && (
              <g>
                <rect x={0} y={0} width={warmupRegion.x2} height={CHART_H}
                  fill="#94a3b8" fillOpacity={0.07} />
                <line x1={warmupRegion.x2} y1={0} x2={warmupRegion.x2} y2={CHART_H}
                  stroke="#94a3b8" strokeWidth={1} strokeDasharray="2,3" opacity={0.4} />
                {warmupRegion.x2 > 60 && (
                  <text
                    x={warmupRegion.x2 / 2} y={CHART_H / 2}
                    fontSize={9} fill="#64748b" textAnchor="middle"
                    style={{ pointerEvents: 'none' }}
                  >
                    no anomalies ({formatDuration(warmupRegion.durationSecs)})
                  </text>
                )}
              </g>
            )}

            {/* Severity background */}
            {stateSegments.map((seg, i) => (
              <rect key={i} x={seg.x1} y={0} width={seg.x2 - seg.x1} height={CHART_H}
                fill={SEVERITY_COLORS[seg.level]} fillOpacity={0.08} />
            ))}

            {/* Hovered severity event region highlight */}
            {hoveredEventRegion && (() => {
              const x1 = Math.max(0, tsToX(hoveredEventRegion.start));
              const x2 = Math.min(chartW, tsToX(hoveredEventRegion.end));
              return x2 > x1 ? (
                <rect x={x1} y={0} width={x2 - x1} height={CHART_H}
                  fill="#fbbf24" fillOpacity={0.15}
                  stroke="#fbbf24" strokeWidth={1} strokeOpacity={0.5} />
              ) : null;
            })()}

            {/* Phase marker lines */}
            {phaseMarkers.map((marker) => {
              const x = tsToX(marker.timestamp);
              if (x < -20 || x > chartW + 20) return null;
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
              const bx = tsToX(b.startSec);
              const bw = Math.max(1, barW - 1);
              const totalH = maxCount > 0 ? (b.count / maxCount) * CHART_H * 0.9 : 0;
              let yOff = 0;
              return (
                <g key={i}>
                  {[4, 3, 2, 1, 0].map(l => {
                    const h = maxCount > 0 ? (b.bins[l] / maxCount) * CHART_H * 0.9 : 0;
                    if (h < 0.5) return null;
                    const rect = (
                      <rect key={l}
                        x={bx} y={CHART_H - totalH + yOff}
                        width={bw} height={h}
                        fill={LEVEL_COLORS[l]} fillOpacity={0.85} />
                    );
                    yOff += h;
                    return rect;
                  })}
                </g>
              );
            })}

            {/* Threshold lines */}
            <line x1={0} y1={lowThresholdY} x2={chartW} y2={lowThresholdY}
              stroke="#22c55e" strokeWidth={1} strokeDasharray="4 2" opacity={0.7} />
            <line x1={0} y1={highThresholdY} x2={chartW} y2={highThresholdY}
              stroke="#ef4444" strokeWidth={1} strokeDasharray="4 2" opacity={0.7} />

            {/* EWMA line */}
            {ewmaPoints && (
              <polyline points={ewmaPoints} fill="none" stroke="#67e8f9" strokeWidth={1.5} opacity={0.9} />
            )}

            {/* Severity event markers */}
            {allEvents.map((ev, i) => {
              const x = tsToX(ev.timestamp);
              if (x < 0 || x > chartW) return null;
              const isUp = ev.to_level > ev.from_level;
              const color = SEVERITY_COLORS[ev.to_level] ?? '#94a3b8';
              const BASE_Y = CHART_H + 10;
              const pts = isUp
                ? `${x},${CHART_H + 3} ${x - 5},${BASE_Y} ${x + 5},${BASE_Y}`
                : `${x},${CHART_H + 17} ${x - 5},${BASE_Y} ${x + 5},${BASE_Y}`;
              return <polygon key={i} points={pts} fill={color} opacity={0.9} />;
            })}

            {/* Threshold labels */}
            <text x={chartW - 2} y={lowThresholdY - 2} fontSize={9} fill="#22c55e" textAnchor="end">
              {cfg.low_threshold.toFixed(3)}
            </text>
            <text x={chartW - 2} y={highThresholdY - 2} fontSize={9} fill="#ef4444" textAnchor="end">
              {cfg.high_threshold.toFixed(3)}
            </text>

            {/* Time axis labels */}
            {buckets.length > 0 && (
              <>
                <text x={2} y={CHART_H + 24} fontSize={9} fill="#64748b">{formatTs(displayStart)}</text>
                <text x={chartW - 2} y={CHART_H + 24} fontSize={9} fill="#64748b" textAnchor="end">
                  {formatTs(bucketEnd)}
                </text>
              </>
            )}

            {/* Drag selection rectangle */}
            {isDragging && dragStartX !== null && dragCurrentX !== null && (
              <rect
                x={Math.min(dragStartX, dragCurrentX)} y={0}
                width={Math.abs(dragCurrentX - dragStartX)} height={CHART_H}
                fill="#60a5fa" fillOpacity={0.15}
                stroke="#60a5fa" strokeWidth={1} strokeOpacity={0.6}
              />
            )}

            {/* Hover cursor line + time label */}
            {hoverTs !== null && !isDragging && (
              <g>
                <line x1={hoverX!} y1={0} x2={hoverX!} y2={CHART_H}
                  stroke="#94a3b8" strokeWidth={1} strokeDasharray="2,2" opacity={0.5} />
              </g>
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
        {onTimeRangeChange && (
          <span className="text-slate-500 ml-auto">drag to zoom</span>
        )}
      </div>
    </div>
  );
}
