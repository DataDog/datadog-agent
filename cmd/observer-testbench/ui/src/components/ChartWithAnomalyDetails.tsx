import { useState } from 'react';
import { TimeSeriesChart } from './TimeSeriesChart';
import type { SplitSeries } from './TimeSeriesChart';
import type { Point, AnomalyMarker, Anomaly } from '../api/client';

export interface CorrelationRange {
  id: number;
  title: string;
  start: number;
  end: number;
}

export interface TimeRange {
  start: number;
  end: number;
}

interface ChartWithAnomalyDetailsProps {
  name: string;
  points: Point[];
  anomalyMarkers: AnomalyMarker[];
  anomalies: Anomaly[];
  correlationRanges?: CorrelationRange[];
  enabledAnalyzers: Set<string>;
  timeRange?: TimeRange | null;
  onTimeRangeChange?: (range: TimeRange | null) => void;
  smoothLines?: boolean;
  splitSeries?: SplitSeries[];
}

export function ChartWithAnomalyDetails({
  name,
  points,
  anomalyMarkers,
  anomalies,
  correlationRanges = [],
  enabledAnalyzers,
  timeRange,
  onTimeRangeChange,
  smoothLines = true,
  splitSeries,
}: ChartWithAnomalyDetailsProps) {
  const [expandedIndex, setExpandedIndex] = useState<number | null>(null);

  const formatTimestamp = (ts: number) => {
    return new Date(ts * 1000).toLocaleTimeString();
  };

  const formatValue = (v: number | undefined, decimals = 2) => {
    if (v === undefined) return '-';
    if (Math.abs(v) < 0.01 || Math.abs(v) > 10000) {
      return v.toExponential(decimals);
    }
    return v.toFixed(decimals);
  };

  // Filter anomalies by enabled analyzers
  const filteredAnomalies = anomalies.filter((a) => enabledAnalyzers.has(a.analyzerName));

  return (
    <div className="bg-slate-800 rounded-lg overflow-hidden">
      {/* Chart */}
      <TimeSeriesChart
        name={name}
        points={points}
        anomalies={anomalyMarkers}
        correlationRanges={correlationRanges}
        enabledAnalyzers={enabledAnalyzers}
        timeRange={timeRange}
        onTimeRangeChange={onTimeRangeChange}
        height={200}
        smoothLines={smoothLines}
        splitSeries={splitSeries}
      />

      {/* Anomaly details - compact list below chart */}
      {filteredAnomalies.length > 0 && (
        <div className="border-t border-slate-700 px-4 py-2">
          <div className="text-xs text-slate-500 mb-1">
            {filteredAnomalies.length} anomal{filteredAnomalies.length === 1 ? 'y' : 'ies'} detected
          </div>
          <div className="space-y-1">
            {filteredAnomalies.map((anomaly, idx) => {
              const isExpanded = expandedIndex === idx;
              const debug = anomaly.debugInfo;
              const isCUSUM = anomaly.analyzerName === 'cusum_detector';

              return (
                <div key={`${anomaly.analyzerName}-${anomaly.timestamp}-${idx}`} className="text-xs">
                  {/* Compact header */}
                  <button
                    onClick={() => setExpandedIndex(isExpanded ? null : idx)}
                    className="w-full text-left flex items-center gap-2 py-1 hover:bg-slate-700/50 rounded px-1 -mx-1"
                  >
                    <span
                      className={`px-1.5 py-0.5 rounded text-[10px] ${
                        isCUSUM ? 'bg-red-900/50 text-red-400' : 'bg-blue-900/50 text-blue-400'
                      }`}
                    >
                      {isCUSUM ? 'CUSUM' : 'Z-Score'}
                    </span>
                    <span className="text-slate-400">
                      {formatTimestamp(anomaly.timestamp)}
                    </span>
                    <span className="text-slate-300 flex-1 truncate">
                      {debug ? `${formatValue(debug.deviationSigma)}σ from baseline` : anomaly.title}
                    </span>
                    <span className="text-slate-500">{isExpanded ? '▼' : '▶'}</span>
                  </button>

                  {/* Expanded details */}
                  {isExpanded && debug && (
                    <div className="ml-1 mt-1 mb-2 p-2 bg-slate-900/50 rounded border border-slate-700/50">
                      <div className="grid grid-cols-2 gap-x-4 gap-y-1">
                        <div className="text-slate-500">Baseline:</div>
                        <div className="text-slate-300">
                          {isCUSUM
                            ? `μ=${formatValue(debug.baselineMean)}, σ=${formatValue(debug.baselineStddev)}`
                            : `median=${formatValue(debug.baselineMedian)}, MAD=${formatValue(debug.baselineMAD)}`}
                        </div>
                        <div className="text-slate-500">Threshold:</div>
                        <div className="text-slate-300">{formatValue(debug.threshold)}</div>
                        {isCUSUM && debug.slackParam !== undefined && (
                          <>
                            <div className="text-slate-500">Slack (k):</div>
                            <div className="text-slate-300">{formatValue(debug.slackParam)}</div>
                          </>
                        )}
                        <div className="text-slate-500">Value at trigger:</div>
                        <div className="text-slate-300">{formatValue(debug.currentValue)}</div>
                        <div className="text-slate-500">Deviation:</div>
                        <div className={debug.deviationSigma > 0 ? 'text-red-400' : 'text-blue-400'}>
                          {debug.deviationSigma > 0 ? '+' : ''}
                          {formatValue(debug.deviationSigma)}σ
                        </div>
                      </div>

                      {/* CUSUM sparkline */}
                      {isCUSUM && debug.cusumValues && debug.cusumValues.length > 1 && (
                        <div className="mt-2">
                          <div className="text-slate-500 mb-1">CUSUM accumulator:</div>
                          <CUSUMSparkline values={debug.cusumValues} threshold={debug.threshold} />
                        </div>
                      )}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}

function CUSUMSparkline({ values, threshold }: { values: number[]; threshold: number }) {
  const height = 32;
  const width = 180;
  const padding = 2;

  const maxVal = Math.max(threshold * 1.2, ...values.map(Math.abs));
  const minVal = -maxVal;

  const scaleY = (v: number) => {
    return height - padding - ((v - minVal) / (maxVal - minVal)) * (height - 2 * padding);
  };

  const points = values
    .map((v, i) => {
      const x = padding + (i / (values.length - 1)) * (width - 2 * padding);
      const y = scaleY(v);
      return `${x},${y}`;
    })
    .join(' ');

  const zeroY = scaleY(0);
  const thresholdY = scaleY(threshold);
  const negThresholdY = scaleY(-threshold);

  return (
    <svg width={width} height={height} className="bg-slate-800 rounded">
      <line x1={padding} y1={zeroY} x2={width - padding} y2={zeroY} stroke="#475569" strokeWidth="1" />
      <line
        x1={padding}
        y1={thresholdY}
        x2={width - padding}
        y2={thresholdY}
        stroke="#ef4444"
        strokeWidth="1"
        strokeDasharray="2,2"
        opacity="0.5"
      />
      <line
        x1={padding}
        y1={negThresholdY}
        x2={width - padding}
        y2={negThresholdY}
        stroke="#ef4444"
        strokeWidth="1"
        strokeDasharray="2,2"
        opacity="0.5"
      />
      <polyline points={points} fill="none" stroke="#8b5cf6" strokeWidth="1.5" />
    </svg>
  );
}
