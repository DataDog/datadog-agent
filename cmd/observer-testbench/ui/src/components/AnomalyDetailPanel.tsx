import { useState } from 'react';
import type { Anomaly } from '../api/client';

interface AnomalyDetailPanelProps {
  anomalies: Anomaly[];
}

export function AnomalyDetailPanel({ anomalies }: AnomalyDetailPanelProps) {
  const [expandedIndex, setExpandedIndex] = useState<number | null>(null);

  if (anomalies.length === 0) return null;

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

  return (
    <div className="bg-slate-800 rounded-lg p-4">
      <h2 className="text-sm font-semibold text-slate-300 mb-3">
        Anomaly Details ({anomalies.length})
      </h2>
      <div className="space-y-2 max-h-96 overflow-y-auto">
        {anomalies.map((anomaly, idx) => {
          const isExpanded = expandedIndex === idx;
          const debug = anomaly.debugInfo;
          const isCUSUM = anomaly.analyzerName === 'cusum_detector';

          return (
            <div
              key={`${anomaly.source}-${anomaly.analyzerName}-${anomaly.timestamp}`}
              className="bg-slate-700/50 rounded"
            >
              {/* Header - always visible */}
              <button
                onClick={() => setExpandedIndex(isExpanded ? null : idx)}
                className="w-full text-left px-3 py-2 flex items-center justify-between hover:bg-slate-700/70 rounded"
              >
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span
                      className={`text-xs px-1.5 py-0.5 rounded ${
                        isCUSUM ? 'bg-red-900/50 text-red-400' : 'bg-blue-900/50 text-blue-400'
                      }`}
                    >
                      {anomaly.analyzerName}
                    </span>
                    <span className="text-sm text-slate-300 truncate">{anomaly.source}</span>
                  </div>
                  <div className="text-xs text-slate-500 mt-0.5">
                    {formatTimestamp(anomaly.timestamp)} - {anomaly.description.split(' ').slice(-4).join(' ')}
                  </div>
                </div>
                <span className="text-slate-500 ml-2">{isExpanded ? '▼' : '▶'}</span>
              </button>

              {/* Expanded details */}
              {isExpanded && debug && (
                <div className="px-3 pb-3 pt-1 border-t border-slate-600/50">
                  <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
                    {/* Baseline info */}
                    <div className="col-span-2 text-slate-400 font-medium mt-1 mb-1">
                      Baseline Period
                    </div>
                    <div className="text-slate-500">Time range:</div>
                    <div className="text-slate-300">
                      {formatTimestamp(debug.baselineStart)} - {formatTimestamp(debug.baselineEnd)}
                    </div>

                    {isCUSUM ? (
                      <>
                        <div className="text-slate-500">Mean:</div>
                        <div className="text-slate-300">{formatValue(debug.baselineMean)}</div>
                        <div className="text-slate-500">Std Dev:</div>
                        <div className="text-slate-300">{formatValue(debug.baselineStddev)}</div>
                      </>
                    ) : (
                      <>
                        <div className="text-slate-500">Median:</div>
                        <div className="text-slate-300">{formatValue(debug.baselineMedian)}</div>
                        <div className="text-slate-500">MAD:</div>
                        <div className="text-slate-300">{formatValue(debug.baselineMAD)}</div>
                      </>
                    )}

                    {/* Detection info */}
                    <div className="col-span-2 text-slate-400 font-medium mt-2 mb-1">
                      Detection
                    </div>
                    <div className="text-slate-500">Threshold:</div>
                    <div className="text-slate-300">{formatValue(debug.threshold)}</div>
                    {isCUSUM && debug.slackParam !== undefined && (
                      <>
                        <div className="text-slate-500">Slack (k):</div>
                        <div className="text-slate-300">{formatValue(debug.slackParam)}</div>
                      </>
                    )}
                    <div className="text-slate-500">Triggered at:</div>
                    <div className="text-slate-300">{formatValue(debug.currentValue)}</div>
                    <div className="text-slate-500">Deviation:</div>
                    <div className={`font-medium ${debug.deviationSigma > 0 ? 'text-red-400' : 'text-blue-400'}`}>
                      {debug.deviationSigma > 0 ? '+' : ''}{formatValue(debug.deviationSigma)}σ
                    </div>

                    {/* CUSUM values mini-chart */}
                    {isCUSUM && debug.cusumValues && debug.cusumValues.length > 0 && (
                      <>
                        <div className="col-span-2 text-slate-400 font-medium mt-2 mb-1">
                          CUSUM Accumulator
                        </div>
                        <div className="col-span-2">
                          <CUSUMSparkline values={debug.cusumValues} threshold={debug.threshold} />
                        </div>
                      </>
                    )}
                  </div>
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

// Simple sparkline visualization for CUSUM values
function CUSUMSparkline({ values, threshold }: { values: number[]; threshold: number }) {
  const height = 40;
  const width = 200;
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
      {/* Zero line */}
      <line x1={padding} y1={zeroY} x2={width - padding} y2={zeroY} stroke="#475569" strokeWidth="1" />
      {/* Threshold lines */}
      <line
        x1={padding}
        y1={thresholdY}
        x2={width - padding}
        y2={thresholdY}
        stroke="#ef4444"
        strokeWidth="1"
        strokeDasharray="2,2"
      />
      <line
        x1={padding}
        y1={negThresholdY}
        x2={width - padding}
        y2={negThresholdY}
        stroke="#ef4444"
        strokeWidth="1"
        strokeDasharray="2,2"
      />
      {/* CUSUM line */}
      <polyline points={points} fill="none" stroke="#8b5cf6" strokeWidth="1.5" />
    </svg>
  );
}
