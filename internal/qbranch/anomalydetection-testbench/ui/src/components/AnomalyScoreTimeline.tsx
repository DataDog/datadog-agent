import { useMemo, useState } from 'react';

import type { TimeRange } from './ChartWithAnomalyDetails';

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
  // Apply power transformation: x^n compresses lower values into first bins
  const transformed = Math.pow(clamped, scalePower);
  return Math.min(SCORE_BINS - 1, Math.floor(transformed * SCORE_BINS));
}

function formatPercent(value: number): string {
  return `${Math.round(value * 100)}%`;
}

export function AnomalyScoreTimeline({
  anomalies,
  scenarioStart,
  scenarioEnd,
  timeRange,
}: AnomalyScoreTimelineProps) {
  // Scale power: 1.0 = linear (middle), <1.0 = expand lower scores, >1.0 = compress lower scores
  const [scalePower, setScalePower] = useState(1.0);
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
    const validScores = anomalies.map(a => a.score).filter(s => s !== undefined && !Number.isNaN(s)) as number[];
    
    if (validScores.length === 0) return new Map();
    
    const mean = validScores.reduce((sum, score) => sum + score, 0) / validScores.length;
    const variance = validScores.reduce((sum, score) => sum + Math.pow(score - mean, 2), 0) / validScores.length;
    const stddev = Math.sqrt(variance);
    
    const scoreMap = new Map<number, number>();
    
    anomalies.forEach((anomaly, idx) => {
      if (anomaly.score !== undefined && !Number.isNaN(anomaly.score)) {
        // Z-score normalization: (x - mean) / stddev
        const zScore = stddev > 0 ? (anomaly.score - mean) / stddev : 0;
        // Convert to 0-1 range using sigmoid-like transformation
        const normalized = 1 / (1 + Math.exp(-zScore));
        scoreMap.set(idx, normalized);
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
    }));

    anomalies.forEach((anomaly, anomalyIdx) => {
      if (anomaly.timestamp < bounds.start || anomaly.timestamp > bounds.end) return;
      const idx = Math.min(
        BUCKET_COUNT - 1,
        Math.max(0, Math.floor((anomaly.timestamp - bounds.start) / bucketSize))
      );
      const normalizedScore = normalizedScores.get(anomalyIdx) ?? 0;
      next[idx].bins[scoreToBin(normalizedScore, scalePower)] += 1;
      next[idx].total += 1;
    });

    return next;
  }, [anomalies, bounds, scalePower, normalizedScores]);

  if (!bounds) return null;

  const maxBucketCount = Math.max(1, ...buckets.map((b) => b.total));
  
  // Calculate stats using normalized scores
  const normalizedScoreList = Array.from(normalizedScores.values());
  const avgNormalizedScore = normalizedScoreList.length === 0 
    ? 0 
    : normalizedScoreList.reduce((acc, score) => acc + score, 0) / normalizedScoreList.length;
  const highScoreCount = normalizedScoreList.filter(score => score >= 0.8).length;
  const detectors = new Set(anomalies.map((a) => a.detectorName));
  const typeCounts = anomalies.reduce((acc, a) => {
    acc.set(a.type, (acc.get(a.type) ?? 0) + 1);
    return acc;
  }, new Map<TimelineAnomalyType, number>());

  // Compute bin ranges based on current power scale
  const getBinRange = (binIdx: number) => {
    if (Math.abs(scalePower - 1.0) < 0.01) {
      // Linear scale: evenly spaced 20% intervals
      return `${binIdx * 20}-${binIdx === SCORE_BINS - 1 ? 100 : (binIdx + 1) * 20}%`;
    } else {
      // Power scale: compute actual thresholds
      const start = binIdx / SCORE_BINS;
      const end = (binIdx + 1) / SCORE_BINS;
      // Inverse transform: y = x^(1/n) gives original score range
      const startScore = Math.pow(start, 1 / scalePower);
      const endScore = Math.pow(end, 1 / scalePower);
      return `${Math.round(startScore * 100)}-${Math.round(endScore * 100)}%`;
    }
  };

  return (
    <div className="bg-slate-800 rounded-lg border border-slate-700 p-4">
      <div className="flex items-start justify-between gap-4 mb-3">
        <div>
          <h3 className="text-sm font-semibold text-slate-200">Anomaly intensity timeline</h3>
          <div className="text-xs text-slate-500 mt-0.5">
            {anomalies.length} anomal{anomalies.length === 1 ? 'y' : 'ies'} across {detectors.size} detector{detectors.size === 1 ? '' : 's'}
          </div>
          <div className="text-xs text-slate-600 mt-0.5">
            Scores normalized using Gaussian Z-score → sigmoid transformation
          </div>
        </div>
        <div className="flex gap-2 text-xs text-slate-400 flex-wrap justify-end">
          <span className="px-2 py-1 rounded bg-slate-900/70">max bucket: {maxBucketCount}</span>
          <span className="px-2 py-1 rounded bg-slate-900/70">avg norm. score: {formatPercent(avgNormalizedScore)}</span>
          <span className="px-2 py-1 rounded bg-slate-900/70">high score: {highScoreCount}</span>
        </div>
      </div>

      {/* Scale control */}
      <div className="mb-3 flex items-center gap-3">
        <label className="text-xs text-slate-400 min-w-fit">Score scale:</label>
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
        <span className="text-xs text-slate-500 font-mono min-w-fit">x^{scalePower.toFixed(2)}</span>
      </div>

      <div className="flex items-end gap-px h-24">
        {buckets.map((bucket, bucketIdx) => {
          const height = bucket.total > 0 ? Math.max(5, (bucket.total / maxBucketCount) * 96) : 2;
          const title =
            bucket.total > 0
              ? `${formatTimestamp(bucket.start)}-${formatTimestamp(bucket.end)}: ${bucket.total} anomalies`
              : undefined;

          return (
            <div key={bucketIdx} className="flex-1 flex items-end" style={{ height: '96px' }} title={title}>
              {bucket.total === 0 ? (
                <div className="w-full rounded-sm bg-slate-700/40" style={{ height: '2px' }} />
              ) : (
                <div className="w-full flex flex-col-reverse rounded-sm overflow-hidden" style={{ height: `${height}px` }}>
                  {bucket.bins.map((count, binIdx) => (
                    count > 0 && (
                      <div
                        key={binIdx}
                        className={SCORE_COLORS[binIdx]}
                        style={{ height: `${(count / bucket.total) * 100}%` }}
                      />
                    )
                  ))}
                </div>
              )}
            </div>
          );
        })}
      </div>

      <div className="flex justify-between text-xs text-slate-600 mt-1.5">
        <span>{formatTimestamp(bounds.start)}</span>
        <span>{formatTimestamp(bounds.end)}</span>
      </div>

      <div className="flex items-center justify-between gap-4 mt-3 text-xs text-slate-500 flex-wrap">
        <div className="flex items-center gap-1.5 flex-wrap">
          <span>score bins:</span>
          {SCORE_COLORS.map((color, idx) => (
            <span key={idx} className="inline-flex items-center gap-1">
              <span className={`inline-block w-2.5 h-2.5 rounded-sm ${color}`} />
              {getBinRange(idx)}
            </span>
          ))}
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