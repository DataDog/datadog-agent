import { useEffect, useMemo, useRef, useState } from 'react';
import { TimeSeriesChart, getSeriesVariantColor } from './TimeSeriesChart';
import type { SeriesVariant } from './TimeSeriesChart';
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
  seriesVariants?: SeriesVariant[];
}

function buildSeriesIDSet(seriesVariants?: SeriesVariant[]): Set<string> {
  const set = new Set<string>();
  (seriesVariants ?? []).forEach((s) => {
    if (s.seriesId) set.add(s.seriesId);
  });
  return set;
}

function seriesIDSignature(seriesVariants?: SeriesVariant[]): string {
  const ids = Array.from(buildSeriesIDSet(seriesVariants)).sort();
  return ids.join('|');
}

function buildSeriesVariantColorMap(seriesVariants?: SeriesVariant[]): Map<string, string> {
  const map = new Map<string, string>();
  if (!seriesVariants || seriesVariants.length === 0) return map;

  const ordered = [...seriesVariants].sort((a, b) => {
    const labelCmp = a.label.localeCompare(b.label);
    if (labelCmp !== 0) return labelCmp;
    return (a.seriesId ?? '').localeCompare(b.seriesId ?? '');
  });

  ordered.forEach((series, idx) => {
    if (!series.seriesId) return;
    map.set(series.seriesId, getSeriesVariantColor(idx));
  });
  return map;
}

function getAnomalyId(anomaly: {
  analyzerName: string;
  analyzerComponent?: string;
  sourceSeriesId?: string;
  timestamp: number;
  title: string;
}): string {
  const analyzerId = anomaly.analyzerComponent ?? anomaly.analyzerName;
  return `${analyzerId}:${anomaly.sourceSeriesId ?? 'unknown'}:${anomaly.timestamp}:${anomaly.title}`;
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
  seriesVariants,
}: ChartWithAnomalyDetailsProps) {
  const [expandedIndex, setExpandedIndex] = useState<number | null>(null);
  const [hoveredAnomalyId, setHoveredAnomalyId] = useState<string | null>(null);
  const [visibleSeriesIds, setVisibleSeriesIds] = useState<Set<string>>(() => buildSeriesIDSet(seriesVariants));
  const anomalyRowRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const seriesVariantsSig = useMemo(() => seriesIDSignature(seriesVariants), [seriesVariants]);
  const seriesVariantColorByID = useMemo(() => buildSeriesVariantColorMap(seriesVariants), [seriesVariants]);

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

  const formatFieldName = (key: string) => {
    // Convert camelCase to Title Case: "baselineMean" -> "Baseline Mean"
    return key
      .replace(/([A-Z])/g, ' $1')
      .replace(/^./, (s) => s.toUpperCase())
      .trim();
  };

  useEffect(() => {
    setVisibleSeriesIds(buildSeriesIDSet(seriesVariants));
  }, [seriesVariantsSig]);

  // Filter anomalies by enabled analyzers and visible series variants.
  const filteredAnomalies = useMemo(
    () =>
      anomalies.filter((a) => {
        if (!enabledAnalyzers.has(a.analyzerComponent ?? a.analyzerName)) return false;
        if (!seriesVariants || seriesVariants.length === 0) return true;
        if (!a.sourceSeriesId) return true;
        return visibleSeriesIds.has(a.sourceSeriesId);
      }),
    [anomalies, enabledAnalyzers, seriesVariants, visibleSeriesIds]
  );

  const filteredAnomalyIds = useMemo(
    () => filteredAnomalies.map((a) => getAnomalyId(a)),
    [filteredAnomalies]
  );
  const anomalySeriesIDByAnomalyID = useMemo(() => {
    const map = new Map<string, string | undefined>();
    filteredAnomalies.forEach((a) => {
      map.set(getAnomalyId(a), a.sourceSeriesId);
    });
    return map;
  }, [filteredAnomalies]);

  const expandedAnomalyId = expandedIndex !== null ? filteredAnomalyIds[expandedIndex] ?? null : null;
  const activeAnomalyId = hoveredAnomalyId ?? expandedAnomalyId;
  const activeSeriesId = useMemo(() => {
    if (!activeAnomalyId) return null;
    return anomalySeriesIDByAnomalyID.get(activeAnomalyId) ?? null;
  }, [activeAnomalyId, anomalySeriesIDByAnomalyID]);

  useEffect(() => {
    if (expandedIndex !== null && expandedIndex >= filteredAnomalies.length) {
      setExpandedIndex(null);
    }
  }, [expandedIndex, filteredAnomalies.length]);

  const handleToggleSeriesVisibility = (seriesId: string) => {
    setVisibleSeriesIds((prev) => {
      const next = new Set(prev);
      if (next.has(seriesId)) {
        next.delete(seriesId);
      } else {
        next.add(seriesId);
      }
      return next;
    });
  };

  const handleMarkerClick = (markerId: string) => {
    const idx = filteredAnomalyIds.findIndex((id) => id === markerId);
    if (idx === -1) return;
    setExpandedIndex(idx);
    setHoveredAnomalyId(markerId);
    anomalyRowRefs.current.get(markerId)?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
  };

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
        seriesVariants={seriesVariants}
        visibleSeriesIds={seriesVariants && seriesVariants.length > 0 ? visibleSeriesIds : undefined}
        onToggleSeriesVisibility={handleToggleSeriesVisibility}
        highlightedSeriesId={activeSeriesId}
        highlightedMarkerId={activeAnomalyId}
        onMarkerHover={setHoveredAnomalyId}
        onMarkerClick={handleMarkerClick}
      />

      {/* Anomaly details - compact list below chart */}
      {filteredAnomalies.length > 0 && (
        <div className="border-t border-slate-700 px-4 py-2">
          <div className="text-xs text-slate-500 mb-1">
            {filteredAnomalies.length} anomal{filteredAnomalies.length === 1 ? 'y' : 'ies'} detected
          </div>
          <div className="space-y-0">
            {filteredAnomalies.map((anomaly, idx) => {
              const isExpanded = expandedIndex === idx;
              const debug = anomaly.debugInfo;
              const anomalyId = getAnomalyId(anomaly);
              const isLinked = activeAnomalyId === anomalyId;
              const seriesColor = anomaly.sourceSeriesId
                ? (seriesVariantColorByID.get(anomaly.sourceSeriesId) ?? '#64748b')
                : '#64748b';

              return (
                <div
                  key={`${anomaly.analyzerName}-${anomaly.timestamp}-${idx}`}
                  className={`text-xs rounded border-b border-slate-700/50 last:border-b-0 ${isLinked ? 'bg-slate-700/40 ring-1 ring-slate-500/70' : ''}`}
                  ref={(el) => {
                    if (el) {
                      anomalyRowRefs.current.set(anomalyId, el);
                    } else {
                      anomalyRowRefs.current.delete(anomalyId);
                    }
                  }}
                  onMouseEnter={() => setHoveredAnomalyId(anomalyId)}
                  onMouseLeave={() => setHoveredAnomalyId(null)}
                >
                  {/* Compact header */}
                  <button
                    onClick={() => {
                      setExpandedIndex(isExpanded ? null : idx);
                      setHoveredAnomalyId(anomalyId);
                    }}
                    className="w-full text-left flex items-center gap-2 py-1 hover:bg-slate-700/50 rounded px-1"
                  >
                    <span
                      className={`w-2 h-2 rounded-full flex-shrink-0 ${isLinked ? 'ring-2 ring-slate-200' : ''}`}
                      style={{ backgroundColor: seriesColor }}
                    />
                    <span className="px-1.5 py-0.5 rounded text-[10px] bg-slate-700 text-slate-300">
                      {anomaly.analyzerName}
                    </span>
                    <span className="text-slate-400">
                      {formatTimestamp(anomaly.timestamp)}
                    </span>
                    <span className="text-slate-300 flex-1 truncate">
                      {debug ? `${formatValue(debug.deviationSigma)}σ from baseline` : anomaly.title}
                    </span>
                    <span className="text-slate-500">{isExpanded ? '▼' : '▶'}</span>
                  </button>

                  {/* Expanded details - rendered generically */}
                  {isExpanded && debug && (
                    <div className="ml-1 mt-1 mb-2 p-2 bg-slate-900/50 rounded border border-slate-700/50">
                      <div className="grid grid-cols-2 gap-x-4 gap-y-1">
                        {Object.entries(debug).map(([key, value]) => {
                          // Skip array fields in the grid (rendered separately below)
                          if (Array.isArray(value)) return null;
                          // Skip zero/null values
                          if (value === 0 && key !== 'deviationSigma') return null;
                          return (
                            <div key={key} className="contents">
                              <div className="text-slate-500">{formatFieldName(key)}:</div>
                              <div className={
                                key === 'deviationSigma'
                                  ? (typeof value === 'number' && value > 0 ? 'text-red-400' : 'text-blue-400')
                                  : 'text-slate-300'
                              }>
                                {key === 'deviationSigma'
                                  ? `${typeof value === 'number' && value > 0 ? '+' : ''}${formatValue(value as number)}σ`
                                  : formatValue(value as number)}
                              </div>
                            </div>
                          );
                        })}
                      </div>

                      {/* CUSUM sparkline - gated on field existence, not algorithm name */}
                      {debug.cusumValues && debug.cusumValues.length > 1 && (
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
