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
          return (
            <div
              key={`${anomaly.source}-${anomaly.detectorName}-${anomaly.timestamp}`}
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
                      className="text-xs px-1.5 py-0.5 rounded bg-blue-900/50 text-blue-400"
                    >
                      {anomaly.detectorName}
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

                    {debug.baselineMean !== undefined && (
                      <>
                        <div className="text-slate-500">Mean:</div>
                        <div className="text-slate-300">{formatValue(debug.baselineMean)}</div>
                        <div className="text-slate-500">Std Dev:</div>
                        <div className="text-slate-300">{formatValue(debug.baselineStddev)}</div>
                      </>
                    )}
                    {debug.baselineMedian !== undefined && (
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
                    <div className="text-slate-500">Triggered at:</div>
                    <div className="text-slate-300">{formatValue(debug.currentValue)}</div>
                    <div className="text-slate-500">Deviation:</div>
                    <div className={`font-medium ${debug.deviationSigma > 0 ? 'text-red-400' : 'text-blue-400'}`}>
                      {debug.deviationSigma > 0 ? '+' : ''}{formatValue(debug.deviationSigma)}σ
                    </div>
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
