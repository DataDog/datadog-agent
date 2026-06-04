import { useMemo, useState } from 'react';

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

const BUCKET_COUNT = 80;
const SCORE_BINS = 5;
const SCORE_COLORS = [
  'bg-slate-500/70',
  'bg-yellow-500/75',
  'bg-orange-500/75',
  'bg-red-500/80',
  'bg-fuchsia-500/85',
];

// SVG viewBox is 100 × CHART_H (unitless). Each bucket occupies 100/BUCKET_COUNT width.
const CHART_H = 96;

const TYPE_LABELS: Record<TimelineAnomalyType, string> = {
  standard: 'standard metrics',
  'log-derived': 'log metrics',
  telemetry: 'telemetry',
};

function formatTimestamp(ts: number): string {
  return new Date(ts * 1000).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });
}

function scoreToBin(score: number | undefined, scalePower: number): number {
  if (score === undefined || Number.isNaN(score)) return 0;
  const clamped = Math.max(0, Math.min(1, score));
  const transformed = Math.pow(clamped, scalePower);
  return Math.min(SCORE_BINS - 1, Math.floor(transformed * SCORE_BINS));
}

function formatPercent(value: number): string {
  return `${Math.round(value * 100)}%`;
}

/** Compute EWMA over an array. Returns same-length array. */
function computeEWMA(values: number[], alpha: number): number[] {
  if (values.length === 0) return [];
  const out: number[] = new Array(values.length);
  out[0] = values[0];
  for (let i = 1; i < values.length; i++) {
    out[i] = alpha * values[i] + (1 - alpha) * out[i - 1];
  }
  return out;
}

export function AnomalyScoreTimeline({
  anomalies,
  scenarioStart,
  scenarioEnd,
  timeRange,
  phaseMarkers = [],
}: AnomalyScoreTimelineProps) {
  const [scalePower, setScalePower] = useState(1.0);
  const [ewmaAlpha, setEwmaAlpha] = useState(0.15);

  const bounds = useMemo(() => {
    let start = timeRange?.start ?? scenarioStart ?? null;
    let end = timeRange?.end ?? scenarioEnd ?? null;

    if ((start === null || end === null || end <= start) && anomalies.length > 0) {
      start = Math.min(...anomalies.map((a) => a.timestamp));
      end = Math.max(...anomalies.map((a) => a.timestamp));
    }

    return start !== null && end !== null && end > start ? { start, end } : null;
  }, [anomalies, scenarioEnd, scenarioStart, timeRange]);

  // Gaussian normalization of scores
  const normalizedScores = useMemo(() => {
    const validScores = anomalies
      .map((a) => a.score)
      .filter((s) => s !== undefined && !Number.isNaN(s)) as number[];

    if (validScores.length === 0) return new Map<number, number>();

    const mean = validScores.reduce((sum, s) => sum + s, 0) / validScores.length;
    const variance =
      validScores.reduce((sum, s) => sum + Math.pow(s - mean, 2), 0) / validScores.length;
    const stddev = Math.sqrt(variance);

    const scoreMap = new Map<number, number>();
    anomalies.forEach((anomaly, idx) => {
      if (anomaly.score !== undefined && !Number.isNaN(anomaly.score)) {
        const zScore = stddev > 0 ? (anomaly.score - mean) / stddev : 0;
        scoreMap.set(idx, 1 / (1 + Math.exp(-zScore)));
      } else {
        scoreMap.set(idx, 0);
      }
    });

    return scoreMap;
  }, [anomalies]);

  const buckets = useMemo(() => {
    if (!bounds) return [];

    const bucketSize = (bounds.end - bounds.start) / BUCKET_COUNT;
    const next = Array.from({ length: BUCKET_COUNT }, (_, index) => ({
      start: bounds.start + index * bucketSize,
      end: bounds.start + (index + 1) * bucketSize,
      bins: new Array(SCORE_BINS).fill(0) as number[],
      total: 0,
      scoreSum: 0,
    }));

    anomalies.forEach((anomaly, anomalyIdx) => {
      if (!bounds || anomaly.timestamp < bounds.start || anomaly.timestamp > bounds.end) return;
      const idx = Math.min(
        BUCKET_COUNT - 1,
        Math.max(0, Math.floor((anomaly.timestamp - bounds.start) / bucketSize))
      );
      const normalizedScore = normalizedScores.get(anomalyIdx) ?? 0;
      next[idx].bins[scoreToBin(normalizedScore, scalePower)] += 1;
      next[idx].total += 1;
      next[idx].scoreSum += normalizedScore;
    });

    return next;
  }, [anomalies, bounds, scalePower, normalizedScores]);

  // Per-bucket mean score (0 when empty), then EWMA
  const ewmaValues = useMemo(() => {
    const means = buckets.map((b) => (b.total > 0 ? b.scoreSum / b.total : 0));
    return computeEWMA(means, ewmaAlpha);
  }, [buckets, ewmaAlpha]);

  // Build SVG polyline points string for EWMA (viewBox 100 × CHART_H)
  const ewmaPolyline = useMemo(() => {
    if (ewmaValues.length === 0) return '';
    const bucketW = 100 / BUCKET_COUNT;
    return ewmaValues
      .map((v, i) => {
        const x = (i + 0.5) * bucketW;
        const y = CHART_H - v * CHART_H;
        return `${x.toFixed(2)},${y.toFixed(2)}`;
      })
      .join(' ');
  }, [ewmaValues]);

  if (!bounds) return null;

  const maxBucketCount = Math.max(1, ...buckets.map((b) => b.total));

  const normalizedScoreList = Array.from(normalizedScores.values());
  const avgNormalizedScore =
    normalizedScoreList.length === 0
      ? 0
      : normalizedScoreList.reduce((acc, s) => acc + s, 0) / normalizedScoreList.length;
  const highScoreCount = normalizedScoreList.filter((s) => s >= 0.8).length;
  const detectors = new Set(anomalies.map((a) => a.detectorName));
  const typeCounts = anomalies.reduce((acc, a) => {
    acc.set(a.type, (acc.get(a.type) ?? 0) + 1);
    return acc;
  }, new Map<TimelineAnomalyType, number>());

  const ewmaEnd = ewmaValues.length > 0 ? ewmaValues[ewmaValues.length - 1] : 0;
  const ewmaPeak = ewmaValues.length > 0 ? Math.max(...ewmaValues) : 0;

  const getBinRange = (binIdx: number) => {
    if (Math.abs(scalePower - 1.0) < 0.01) {
      return `${binIdx * 20}-${binIdx === SCORE_BINS - 1 ? 100 : (binIdx + 1) * 20}%`;
    }
    const start = binIdx / SCORE_BINS;
    const end = (binIdx + 1) / SCORE_BINS;
    const startScore = Math.pow(start, 1 / scalePower);
    const endScore = Math.pow(end, 1 / scalePower);
    return `${Math.round(startScore * 100)}-${Math.round(endScore * 100)}%`;
  };

  // Map a Unix-second timestamp to viewBox x (0..100).
  const tsToX = (ts: number) =>
    ((ts - bounds.start) / (bounds.end - bounds.start)) * 100;

  return (
    <div className="bg-slate-800 rounded-lg border border-slate-700 p-4">
      {/* Header */}
      <div className="flex items-start justify-between gap-4 mb-3">
        <div>
          <h3 className="text-sm font-semibold text-slate-200">Anomaly intensity timeline</h3>
          <div className="text-xs text-slate-500 mt-0.5">
            {anomalies.length} anomal{anomalies.length === 1 ? 'y' : 'ies'} across{' '}
            {detectors.size} detector{detectors.size === 1 ? '' : 's'}
          </div>
          <div className="text-xs text-slate-600 mt-0.5">
            Scores normalized using Gaussian Z-score → sigmoid transformation
          </div>
        </div>
        <div className="flex gap-2 text-xs text-slate-400 flex-wrap justify-end">
          <span className="px-2 py-1 rounded bg-slate-900/70">max bucket: {maxBucketCount}</span>
          <span className="px-2 py-1 rounded bg-slate-900/70">
            avg norm. score: {formatPercent(avgNormalizedScore)}
          </span>
          <span className="px-2 py-1 rounded bg-slate-900/70">high score: {highScoreCount}</span>
          <span className="px-2 py-1 rounded bg-cyan-900/70 text-cyan-300">
            EWMA end: {formatPercent(ewmaEnd)}
          </span>
          <span className="px-2 py-1 rounded bg-cyan-900/70 text-cyan-300">
            EWMA peak: {formatPercent(ewmaPeak)}
          </span>
        </div>
      </div>

      {/* Controls */}
      <div className="mb-2 space-y-2">
        {/* Score scale */}
        <div className="flex items-center gap-3">
          <label className="text-xs text-slate-400 min-w-[72px]">Score scale:</label>
          <div className="flex items-center gap-2 flex-1">
            <span className="text-xs text-slate-600">Spread</span>
            <input
              type="range"
              min="0.1"
              max="10"
              step="0.1"
              value={scalePower}
              onChange={(e) => setScalePower(parseFloat(e.target.value))}
              className="flex-1 h-1 bg-slate-700 rounded appearance-none cursor-pointer
                         [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:w-3
                         [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-purple-500
                         [&::-moz-range-thumb]:h-3 [&::-moz-range-thumb]:w-3 [&::-moz-range-thumb]:rounded-full
                         [&::-moz-range-thumb]:bg-purple-500 [&::-moz-range-thumb]:border-none"
            />
            <span className="text-xs text-slate-600">Compress</span>
          </div>
          <span className="text-xs text-slate-500 font-mono min-w-[48px] text-right">
            x^{scalePower.toFixed(2)}
          </span>
        </div>

        {/* EWMA alpha */}
        <div className="flex items-center gap-3">
          <label className="text-xs text-slate-400 min-w-[72px]">EWMA α:</label>
          <div className="flex items-center gap-2 flex-1">
            <span className="text-xs text-slate-600">Smooth</span>
            <input
              type="range"
              min="0.01"
              max="1"
              step="0.01"
              value={ewmaAlpha}
              onChange={(e) => setEwmaAlpha(parseFloat(e.target.value))}
              className="flex-1 h-1 bg-slate-700 rounded appearance-none cursor-pointer
                         [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:w-3
                         [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-cyan-500
                         [&::-moz-range-thumb]:h-3 [&::-moz-range-thumb]:w-3 [&::-moz-range-thumb]:rounded-full
                         [&::-moz-range-thumb]:bg-cyan-500 [&::-moz-range-thumb]:border-none"
            />
            <span className="text-xs text-slate-600">Raw</span>
          </div>
          <span className="text-xs text-slate-500 font-mono min-w-[48px] text-right">
            {ewmaAlpha.toFixed(2)}
          </span>
        </div>
      </div>

      {/* Chart: bars + SVG overlay */}
      <div className="relative" style={{ height: `${CHART_H}px` }}>
        {/* Stacked bar chart */}
        <div className="absolute inset-0 flex items-end gap-px">
          {buckets.map((bucket, bucketIdx) => {
            const height =
              bucket.total > 0 ? Math.max(5, (bucket.total / maxBucketCount) * CHART_H) : 2;
            const title =
              bucket.total > 0
                ? `${formatTimestamp(bucket.start)}-${formatTimestamp(bucket.end)}: ${bucket.total} anomalies`
                : undefined;

            return (
              <div
                key={bucketIdx}
                className="flex-1 flex items-end"
                style={{ height: `${CHART_H}px` }}
                title={title}
              >
                {bucket.total === 0 ? (
                  <div className="w-full rounded-sm bg-slate-700/40" style={{ height: '2px' }} />
                ) : (
                  <div
                    className="w-full flex flex-col-reverse rounded-sm overflow-hidden"
                    style={{ height: `${height}px` }}
                  >
                    {bucket.bins.map(
                      (count, binIdx) =>
                        count > 0 && (
                          <div
                            key={binIdx}
                            className={SCORE_COLORS[binIdx]}
                            style={{ height: `${(count / bucket.total) * 100}%` }}
                          />
                        )
                    )}
                  </div>
                )}
              </div>
            );
          })}
        </div>

        {/* SVG overlay: EWMA line + phase markers */}
        <svg
          className="absolute inset-0 w-full h-full pointer-events-none"
          viewBox={`0 0 100 ${CHART_H}`}
          preserveAspectRatio="none"
        >
          {/* Phase markers */}
          {phaseMarkers.map((marker) => {
            const x = tsToX(marker.timestamp);
            if (x < -2 || x > 102) return null;
            return (
              <g key={marker.key}>
                <line
                  x1={x}
                  x2={x}
                  y1={0}
                  y2={CHART_H}
                  stroke={marker.color}
                  strokeWidth="0.6"
                  strokeDasharray="3,2"
                  opacity="0.85"
                />
              </g>
            );
          })}

          {/* EWMA polyline */}
          {ewmaPolyline && (
            <polyline
              points={ewmaPolyline}
              fill="none"
              stroke="#67e8f9"
              strokeWidth="0.8"
              strokeLinejoin="round"
              strokeLinecap="round"
              opacity="0.9"
            />
          )}
        </svg>

        {/* Phase marker labels — rendered in HTML above SVG so text is sharp */}
        {phaseMarkers.map((marker) => {
          const pct = ((marker.timestamp - bounds.start) / (bounds.end - bounds.start)) * 100;
          if (pct < -2 || pct > 102) return null;
          return (
            <span
              key={marker.key}
              className="absolute top-0 text-[9px] font-mono leading-none pointer-events-none"
              style={{
                left: `${pct}%`,
                transform: 'translateX(2px)',
                color: marker.color,
                opacity: 0.9,
              }}
            >
              {marker.label}
            </span>
          );
        })}
      </div>

      {/* Time axis */}
      <div className="flex justify-between text-xs text-slate-600 mt-1.5">
        <span>{formatTimestamp(bounds.start)}</span>
        <span>{formatTimestamp(bounds.end)}</span>
      </div>

      {/* Legend */}
      <div className="flex items-center justify-between gap-4 mt-3 text-xs text-slate-500 flex-wrap">
        <div className="flex items-center gap-1.5 flex-wrap">
          <span>score bins:</span>
          {SCORE_COLORS.map((color, idx) => (
            <span key={idx} className="inline-flex items-center gap-1">
              <span className={`inline-block w-2.5 h-2.5 rounded-sm ${color}`} />
              {getBinRange(idx)}
            </span>
          ))}
          <span className="inline-flex items-center gap-1 ml-1">
            <svg width="16" height="8" viewBox="0 0 16 8">
              <polyline
                points="0,7 4,5 8,3 12,4 16,2"
                fill="none"
                stroke="#67e8f9"
                strokeWidth="1.5"
                strokeLinejoin="round"
              />
            </svg>
            EWMA
          </span>
        </div>
        <div className="flex gap-2 flex-wrap">
          {Array.from(typeCounts.entries()).map(([type, count]) => (
            <span key={type}>
              {TYPE_LABELS[type]}: {count}
            </span>
          ))}
        </div>
      </div>
    </div>
  );
}
