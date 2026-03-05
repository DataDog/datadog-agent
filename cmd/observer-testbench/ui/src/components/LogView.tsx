import { useState, useMemo, useRef, useEffect } from 'react';
import * as d3 from 'd3';
import type { ScenarioInfo, LogAnomaly, LogEntry } from '../api/client';
import type { ObserverState, ObserverActions } from '../hooks/useObserver';
import type { PhaseMarker } from './ChartWithAnomalyDetails';
import { MAIN_TAG_FILTER_KEYS } from '../constants';
import { parseTagFilter, extractTagGroups, toggleTagInInput, matchesTagFilter } from '../filters';
import { TagFilterGroups } from './TagFilterGroups';

interface TimeRange {
  start: number;
  end: number;
}

interface LogViewProps {
  state: ObserverState;
  actions: ObserverActions;
  sidebarWidth: number;
  timeRange?: TimeRange | null;
  onTimeRangeChange?: (range: TimeRange | null) => void;
  phaseMarkers?: PhaseMarker[];
}

const LOG_CHART_HEIGHT = 80;
const LOG_CHART_MARGIN = { top: 6, right: 8, bottom: 22, left: 8 };
const LOG_CHART_TARGET_BUCKETS = 60;
const LOG_CHART_MIN_BUCKET_SIZE_S = 1;

function formatTimestamp(ts: number): string {
  return new Date(ts * 1000).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });
}

function formatTimestampMs(ts: number): string {
  return new Date(ts).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });
}

function levelBadgeColor(status: string): string {
  switch (status.toLowerCase()) {
    case 'error':
      return 'text-red-400 bg-red-900/40';
    case 'warn':
    case 'warning':
      return 'text-amber-400 bg-amber-900/40';
    case 'info':
      return 'text-blue-400 bg-blue-900/40';
    case 'debug':
    case 'trace':
      return 'text-slate-400 bg-slate-700/40';
    default:
      return 'text-slate-400 bg-slate-700/40';
  }
}

function scoreColor(score: number): string {
  if (score >= 0.9) return 'text-red-400 bg-red-900/40';
  if (score >= 0.7) return 'text-orange-400 bg-orange-900/40';
  if (score >= 0.5) return 'text-yellow-400 bg-yellow-900/40';
  return 'text-slate-400 bg-slate-700/40';
}

function detectorBadgeColor(name: string): string {
  const colors = [
    'text-purple-400 bg-purple-900/40',
    'text-blue-400 bg-blue-900/40',
    'text-cyan-400 bg-cyan-900/40',
    'text-teal-400 bg-teal-900/40',
    'text-green-400 bg-green-900/40',
  ];
  let hash = 0;
  for (let i = 0; i < name.length; i++) hash = (hash * 31 + name.charCodeAt(i)) & 0xffffffff;
  return colors[Math.abs(hash) % colors.length];
}

function detectorLineColor(name: string): string {
  const colors = ['#c084fc', '#60a5fa', '#22d3ee', '#2dd4bf', '#4ade80'];
  let hash = 0;
  for (let i = 0; i < name.length; i++) hash = (hash * 31 + name.charCodeAt(i)) & 0xffffffff;
  return colors[Math.abs(hash) % colors.length];
}

// Combined log rate + anomaly timeline chart — interactive (drag to zoom, middle/cmd+drag to pan)
function LogRateChart({
  logs,
  anomalies,
  scenarioStart,
  scenarioEnd,
  hoveredTimestamp,
  hoveredAnomalyIndex,
  timeRange,
  onTimeRangeChange,
  phaseMarkers = [],
}: {
  logs: LogEntry[];
  anomalies: LogAnomaly[];
  scenarioStart: number | null;
  scenarioEnd: number | null;
  hoveredTimestamp?: number | null;
  hoveredAnomalyIndex?: number | null;
  timeRange?: TimeRange | null;
  onTimeRangeChange?: (range: TimeRange | null) => void;
  phaseMarkers?: PhaseMarker[];
}) {
  const svgRef = useRef<SVGSVGElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const isBrushingRef = useRef(false);
  const isPanningRef = useRef(false);
  const panStartXRef = useRef(0);
  const panStartRangeRef = useRef<TimeRange | null>(null);
  const panTriggerRef = useRef<'middle' | 'meta-left' | null>(null);
  const xScaleRef = useRef<d3.ScaleTime<number, number> | null>(null);
  const onTimeRangeChangeRef = useRef(onTimeRangeChange);
  onTimeRangeChangeRef.current = onTimeRangeChange;

  const buckets = useMemo(() => {
    let displayStart = timeRange?.start ?? scenarioStart;
    let displayEnd = timeRange?.end ?? scenarioEnd;
    // Fall back to log timestamp bounds when no other bounds are available
    if (!displayStart || !displayEnd || displayEnd <= displayStart) {
      for (const l of logs) {
        const ts = l.timestampMs / 1000;
        if (!ts) continue;
        if (!displayStart || ts < displayStart) displayStart = ts;
        if (!displayEnd || ts > displayEnd) displayEnd = ts;
      }
    }
    if (!displayStart || !displayEnd || displayEnd <= displayStart) return [];
    const bucketSize = Math.max(LOG_CHART_MIN_BUCKET_SIZE_S, (displayEnd - displayStart) / LOG_CHART_TARGET_BUCKETS);
    const bucketCount = Math.ceil((displayEnd - displayStart) / bucketSize);
    const data = Array.from({ length: bucketCount }, () => ({ total: 0, error: 0, warn: 0, info: 0, debug: 0 }));
    for (const l of logs) {
      const ts = l.timestampMs / 1000;
      if (ts < displayStart || ts > displayEnd) continue;
      const idx = Math.min(Math.floor((ts - displayStart) / bucketSize), bucketCount - 1);
      if (idx >= 0) {
        data[idx].total++;
        const s = l.status.toLowerCase();
        if (s === 'error') data[idx].error++;
        else if (s === 'warn' || s === 'warning') data[idx].warn++;
        else if (s === 'info') data[idx].info++;
        else data[idx].debug++;
      }
    }
    return data.map((d, i) => ({
      ...d,
      startTs: displayStart + i * bucketSize,
      endTs: displayStart + (i + 1) * bucketSize,
    }));
  }, [logs, scenarioStart, scenarioEnd, timeRange]);

  const detectors = useMemo(
    () => Array.from(new Set(anomalies.map((a) => a.detectorName))),
    [anomalies]
  );

  // D3 chart drawing
  useEffect(() => {
    if (!svgRef.current || !containerRef.current) return;
    if (buckets.length === 0) return;
    if (isBrushingRef.current) return;

    const m = LOG_CHART_MARGIN;
    const container = containerRef.current;
    const width = container.clientWidth;
    const innerWidth = width - m.left - m.right;
    const innerHeight = LOG_CHART_HEIGHT - m.top - m.bottom;

    d3.select(svgRef.current).selectAll('*').remove();

    const svg = d3.select(svgRef.current).attr('width', width).attr('height', LOG_CHART_HEIGHT);

    svg.append('defs').append('clipPath')
      .attr('id', 'log-rate-clip')
      .append('rect').attr('width', innerWidth).attr('height', innerHeight);

    const g = svg.append('g').attr('transform', `translate(${m.left},${m.top})`);

    const xDomain: [number, number] = timeRange
      ? [timeRange.start * 1000, timeRange.end * 1000]
      : [scenarioStart * 1000, scenarioEnd * 1000];
    const xScale = d3.scaleTime().domain(xDomain).range([0, innerWidth]);
    xScaleRef.current = xScale;

    const maxTotal = Math.max(1, ...buckets.map((b) => b.total));
    const barsG = g.append('g').attr('clip-path', 'url(#log-rate-clip)');

    // Status layers — bottom to top: debug, info, warn, error
    const STATUS_LAYERS = [
      { key: 'debug' as const, color: 'rgba(100, 116, 139, 0.6)' },
      { key: 'info'  as const, color: 'rgba(59, 130, 246, 0.65)' },
      { key: 'warn'  as const, color: 'rgba(245, 158, 11, 0.8)'  },
      { key: 'error' as const, color: 'rgba(239, 68, 68, 0.85)'  },
    ] as const;

    // Stacked bars per bucket
    buckets.forEach((b) => {
      const x = xScale(b.startTs * 1000);
      const bw = Math.max(0, xScale(b.endTs * 1000) - x - 1);
      if (bw <= 0) return;

      if (b.total === 0) {
        barsG.append('rect')
          .attr('x', x).attr('width', bw)
          .attr('y', innerHeight - 2).attr('height', 2)
          .attr('fill', 'rgba(51, 65, 85, 0.25)');
        return;
      }

      const totalH = Math.max(4, (b.total / maxTotal) * innerHeight);
      let yBottom = innerHeight;
      for (const { key, color } of STATUS_LAYERS) {
        const count = b[key];
        if (count === 0) continue;
        const segH = (count / b.total) * totalH;
        barsG.append('rect')
          .attr('x', x).attr('width', bw)
          .attr('y', yBottom - segH).attr('height', segH)
          .attr('fill', color);
        yBottom -= segH;
      }
    });

    // Anomaly markers (triangle + vertical line per anomaly)
    anomalies.forEach((a, i) => {
      const x = xScale(a.timestamp * 1000);
      if (x < 0 || x > innerWidth) return;
      const color = detectorLineColor(a.detectorName);
      const isHovered = hoveredAnomalyIndex === i;
      const opacity = hoveredAnomalyIndex != null && !isHovered ? 0.35 : 1;
      const ts = isHovered ? 5 : 3;
      barsG.append('polygon')
        .attr('points', `${x - ts},0 ${x + ts},0 ${x},${(ts * 1.7).toFixed(1)}`)
        .attr('fill', color)
        .attr('opacity', opacity);
      barsG.append('line')
        .attr('x1', x).attr('x2', x)
        .attr('y1', ts * 1.7).attr('y2', innerHeight)
        .attr('stroke', color)
        .attr('stroke-width', isHovered ? 1.5 : 1)
        .attr('opacity', opacity);
    });

    // Hover effects — drawn on top of bars
    if (hoveredTimestamp != null) {
      const hts = hoveredTimestamp / 1000;
      const hb = buckets.find((b) => hts >= b.startTs && hts < b.endTs);
      if (hb) {
        const hx = xScale(hb.startTs * 1000);
        const hw = Math.max(0, xScale(hb.endTs * 1000) - hx);
        barsG.append('rect')
          .attr('x', hx).attr('width', hw)
          .attr('y', 0).attr('height', innerHeight)
          .attr('fill', 'rgba(255, 255, 255, 0.12)')
          .attr('stroke', 'rgba(255, 255, 255, 0.45)')
          .attr('stroke-width', 1);
      }
      // Exact timestamp cursor line
      const x = xScale(hoveredTimestamp);
      if (x >= 0 && x <= innerWidth) {
        barsG.append('line')
          .attr('x1', x).attr('x2', x)
          .attr('y1', 0).attr('y2', innerHeight)
          .attr('stroke', 'rgba(255, 255, 255, 0.7)')
          .attr('stroke-width', 1)
          .attr('stroke-dasharray', '3,2');
      }
    }

    // Phase marker lines (dotted vertical lines for episode phases)
    phaseMarkers.forEach((marker) => {
      const x = xScale(marker.timestamp * 1000);
      if (x < -20 || x > innerWidth + 20) return;
      barsG.append('line')
        .attr('x1', x).attr('x2', x)
        .attr('y1', 0).attr('y2', innerHeight)
        .attr('stroke', marker.color)
        .attr('stroke-width', 1)
        .attr('stroke-dasharray', '4,3')
        .attr('opacity', 0.8)
        .attr('pointer-events', 'none');
      barsG.append('text')
        .attr('x', x + 3)
        .attr('y', 10)
        .attr('fill', marker.color)
        .attr('font-size', '9px')
        .attr('font-family', 'monospace')
        .attr('opacity', 0.9)
        .attr('pointer-events', 'none')
        .text(marker.label);
    });

    // X axis
    g.append('g')
      .attr('transform', `translate(0,${innerHeight})`)
      .call(
        d3.axisBottom(xScale)
          .ticks(6)
          .tickFormat((d) => d3.timeFormat('%H:%M:%S')(d as Date))
      )
      .attr('color', '#334155')
      .selectAll('text')
      .attr('fill', '#64748b')
      .attr('font-size', '9px');

    g.select('.domain').attr('stroke', '#334155');

    // Brush for zoom
    const brush = d3
      .brushX<unknown>()
      .extent([[0, 0], [innerWidth, innerHeight]])
      .filter((event: MouseEvent) => event.button === 0 && !event.metaKey)
      .on('start', () => { isBrushingRef.current = true; })
      .on('end', (event: d3.D3BrushEvent<unknown>) => {
        isBrushingRef.current = false;
        if (!event.selection) return;
        const [x0, x1] = event.selection as [number, number];
        const start = Math.floor(xScale.invert(x0).getTime() / 1000);
        const end = Math.floor(xScale.invert(x1).getTime() / 1000);
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        g.select('.brush').call(brush.move as any, null);
        if (onTimeRangeChangeRef.current && end > start) {
          onTimeRangeChangeRef.current({ start, end });
        }
      });

    g.append('g').attr('class', 'brush').call(brush);
    g.select('.brush .selection')
      .attr('fill', 'rgba(45, 212, 191, 0.2)')
      .attr('stroke', '#2dd4bf')
      .attr('stroke-width', 1);
  }, [buckets, anomalies, scenarioStart, scenarioEnd, timeRange, hoveredTimestamp, hoveredAnomalyIndex, phaseMarkers]);

  // Panning (middle-click or cmd+left-drag)
  useEffect(() => {
    const svg = svgRef.current;
    if (!svg) return;

    const handleMouseDown = (e: MouseEvent) => {
      const isMiddle = e.button === 1;
      const isMetaLeft = e.button === 0 && e.metaKey;
      if (!isMiddle && !isMetaLeft) return;
      if (!timeRange) return;
      e.preventDefault();
      isPanningRef.current = true;
      panTriggerRef.current = isMiddle ? 'middle' : 'meta-left';
      panStartXRef.current = e.clientX;
      panStartRangeRef.current = { ...timeRange };
      svg.style.cursor = 'grabbing';
    };

    const handleMouseMove = (e: MouseEvent) => {
      if (!isPanningRef.current || !panStartRangeRef.current || !xScaleRef.current) return;
      const dx = e.clientX - panStartXRef.current;
      const scale = xScaleRef.current;
      const domain = scale.domain();
      const r = scale.range();
      const pixelsPerMs = (r[1] - r[0]) / (domain[1].getTime() - domain[0].getTime());
      const dtSec = (-dx / pixelsPerMs) / 1000;
      onTimeRangeChangeRef.current?.({
        start: panStartRangeRef.current.start + dtSec,
        end: panStartRangeRef.current.end + dtSec,
      });
    };

    const handleMouseUp = (e: MouseEvent) => {
      if (!isPanningRef.current) return;
      const trigger = panTriggerRef.current;
      if (trigger === 'middle' && e.button !== 1) return;
      if (trigger === 'meta-left' && e.button !== 0) return;
      isPanningRef.current = false;
      panTriggerRef.current = null;
      panStartRangeRef.current = null;
      svg.style.cursor = '';
    };

    const handleBlur = () => {
      if (isPanningRef.current) {
        isPanningRef.current = false;
        panTriggerRef.current = null;
        panStartRangeRef.current = null;
        svg.style.cursor = '';
      }
    };

    const handleAuxClick = (e: MouseEvent) => { if (e.button === 1) e.preventDefault(); };

    svg.addEventListener('mousedown', handleMouseDown);
    window.addEventListener('mousemove', handleMouseMove);
    window.addEventListener('mouseup', handleMouseUp);
    window.addEventListener('blur', handleBlur);
    svg.addEventListener('auxclick', handleAuxClick);

    return () => {
      svg.removeEventListener('mousedown', handleMouseDown);
      window.removeEventListener('mousemove', handleMouseMove);
      window.removeEventListener('mouseup', handleMouseUp);
      window.removeEventListener('blur', handleBlur);
      svg.removeEventListener('auxclick', handleAuxClick);
    };
  }, [timeRange]);

  const hasBounds = (scenarioStart && scenarioEnd && scenarioEnd > scenarioStart)
    || (timeRange && timeRange.end > timeRange.start)
    || buckets.length > 0;
  if (!hasBounds) return null;

  return (
    <div className="bg-slate-800/60 rounded p-3 mb-4">
      <div className="flex items-center gap-4 text-xs text-slate-400 mb-1.5">
        <span>Log rate ({logs.length} log{logs.length !== 1 ? 's' : ''} total)</span>
        <span className="flex items-center gap-2">
          <span className="flex items-center gap-1"><span className="inline-block w-2 h-2 rounded-sm bg-red-500/80" />error</span>
          <span className="flex items-center gap-1"><span className="inline-block w-2 h-2 rounded-sm bg-amber-500/75" />warn</span>
          <span className="flex items-center gap-1"><span className="inline-block w-2 h-2 rounded-sm bg-blue-500/65" />info</span>
          <span className="flex items-center gap-1"><span className="inline-block w-2 h-2 rounded-sm bg-slate-500/60" />debug</span>
        </span>
        {detectors.map((name) => (
          <span key={name} className="flex items-center gap-1">
            <span className="inline-block w-2 h-2 rounded-sm" style={{ backgroundColor: detectorLineColor(name) }} />
            <span style={{ color: detectorLineColor(name) }}>{name}</span>
          </span>
        ))}
        <span className="text-xs text-slate-600 ml-auto">(drag to zoom · middle-drag or cmd+drag to pan)</span>
      </div>
      <div ref={containerRef} className="w-full">
        <svg ref={svgRef} style={{ display: 'block' }} />
      </div>
    </div>
  );
}

interface LogEntryRowProps {
  entry: LogEntry;
  isExpanded: boolean;
  onToggle: () => void;
  isTelemetry?: boolean;
  onHoverEnter?: () => void;
  onHoverLeave?: () => void;
}

function LogEntryRow({ entry, isExpanded, onToggle, isTelemetry = false, onHoverEnter, onHoverLeave }: LogEntryRowProps) {
  const contentPreview = entry.content.length > 120 && !isExpanded
    ? entry.content.slice(0, 120) + '…'
    : entry.content;

  return (
    <div className="bg-slate-700/30 rounded overflow-hidden" onMouseEnter={onHoverEnter} onMouseLeave={onHoverLeave}>
      <button
        onClick={onToggle}
        className="w-full text-left px-3 py-2 hover:bg-slate-700/50 transition-colors"
      >
        <div className="flex items-start gap-2">
          <span className="flex-shrink-0 text-xs text-slate-500 font-mono pt-0.5 w-20 text-right">
            {formatTimestampMs(entry.timestampMs)}
          </span>
          <span className={`flex-shrink-0 text-xs px-1.5 py-0.5 rounded font-medium uppercase ${levelBadgeColor(entry.status)}`}>
            {entry.status}
          </span>
          {isTelemetry && (
            <span className="flex-shrink-0 inline-flex items-center justify-center w-4 h-4 rounded-full bg-purple-600 text-white text-[9px] font-bold mt-0.5" title="Telemetry log">T</span>
          )}
          <span className="text-xs text-slate-300 font-mono leading-relaxed break-all flex-1">
            {contentPreview}
          </span>
          {entry.tags && entry.tags.length > 0 && (() => {
            const headerTags = entry.tags.filter((tag) =>
              MAIN_TAG_FILTER_KEYS.has(tag.slice(0, tag.indexOf(':')))
            );
            return headerTags.length > 0 ? (
              <div className="flex gap-1 flex-wrap flex-shrink-0">
                {headerTags.map((tag) => (
                  <span key={tag} className="px-1 py-0.5 rounded text-[9px] bg-slate-600/50 text-slate-400 font-mono">
                    {tag}
                  </span>
                ))}
              </div>
            ) : null;
          })()}
          {entry.content.length > 120 && (
            <span className="text-slate-500 flex-shrink-0 text-xs">{isExpanded ? '▼' : '▶'}</span>
          )}
        </div>
      </button>

      {isExpanded && entry.tags && entry.tags.length > 0 && (
        <div className="px-3 pb-2 border-t border-slate-600/30">
          <div className="flex gap-1 flex-wrap mt-1.5">
            {entry.tags.map((tag) => (
              <span
                key={tag}
                className="text-xs px-1.5 py-0.5 rounded bg-slate-600/50 text-slate-400 font-mono"
              >
                {tag}
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

interface LogAnomalyCardProps {
  anomaly: LogAnomaly;
  isExpanded: boolean;
  onToggle: () => void;
  onHoverEnter?: () => void;
  onHoverLeave?: () => void;
}

function LogAnomalyCard({ anomaly, isExpanded, onToggle, onHoverEnter, onHoverLeave }: LogAnomalyCardProps) {
  return (
    <div className="bg-slate-700/50 rounded overflow-hidden" onMouseEnter={onHoverEnter} onMouseLeave={onHoverLeave}>
      <button
        onClick={onToggle}
        className="w-full text-left px-4 py-3 hover:bg-slate-700/70 transition-colors"
      >
        <div className="flex items-start gap-3">
          <div className="flex-shrink-0 text-xs text-slate-500 font-mono pt-0.5 w-20 text-right">
            {formatTimestamp(anomaly.timestamp)}
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 flex-wrap mb-1">
              <span className={`text-xs px-1.5 py-0.5 rounded font-medium ${detectorBadgeColor(anomaly.detectorName)}`}>
                {anomaly.detectorName}
              </span>
              {anomaly.score !== undefined && (
                <span className={`text-xs px-1.5 py-0.5 rounded font-mono ${scoreColor(anomaly.score)}`}>
                  score: {anomaly.score.toFixed(2)}
                </span>
              )}
            </div>
            <div className="text-sm text-slate-200 font-medium leading-snug">
              {anomaly.title}
            </div>
            {!isExpanded && (
              <div className="text-xs text-slate-400 mt-0.5 truncate">
                {anomaly.description}
              </div>
            )}
          </div>
          {anomaly.tags && anomaly.tags.length > 0 && (() => {
            const headerTags = anomaly.tags.filter((tag) =>
              MAIN_TAG_FILTER_KEYS.has(tag.slice(0, tag.indexOf(':')))
            );
            return headerTags.length > 0 ? (
              <div className="flex gap-1 flex-wrap flex-shrink-0">
                {headerTags.map((tag) => (
                  <span key={tag} className="px-1 py-0.5 rounded text-[9px] bg-slate-600/50 text-slate-400 font-mono">
                    {tag}
                  </span>
                ))}
              </div>
            ) : null;
          })()}
          <span className="text-slate-500 flex-shrink-0 text-xs">{isExpanded ? '▼' : '▶'}</span>
        </div>
      </button>

      {isExpanded && (
        <div className="px-4 pb-3 border-t border-slate-600/50">
          <div className="mt-2 mb-2">
            <div className="text-xs text-slate-400 font-medium mb-1">Log Content</div>
            <pre className="text-xs text-slate-300 bg-slate-800/60 rounded p-2 whitespace-pre-wrap break-all font-mono leading-relaxed max-h-40 overflow-y-auto">
              {anomaly.description}
            </pre>
          </div>
          {anomaly.tags && anomaly.tags.length > 0 && (
            <div className="mt-2">
              <div className="text-xs text-slate-400 font-medium mb-1">Tags</div>
              <div className="flex gap-1 flex-wrap">
                {anomaly.tags.map((tag) => (
                  <span
                    key={tag}
                    className="text-xs px-1.5 py-0.5 rounded bg-slate-600/50 text-slate-400 font-mono"
                  >
                    {tag}
                  </span>
                ))}
              </div>
            </div>
          )}
          <div className="mt-2 text-xs text-slate-500">
            Source: <span className="text-slate-400">{anomaly.source}</span>
          </div>
        </div>
      )}
    </div>
  );
}

const LOG_PAGE_SIZE = 50;

/** Synthesize a `status:<value>` tag from the entry's status field so it can be filtered like any other tag. */
function getEffectiveTags(tags: string[], status: string): string[] {
  const statusTag = `status:${status.toLowerCase()}`;
  return tags.includes(statusTag) ? tags : [statusTag, ...tags];
}

export function LogView({ state, actions, sidebarWidth, timeRange, onTimeRangeChange, phaseMarkers }: LogViewProps) {
  const scenarios = state.scenarios ?? [];
  const allLogs = state.logs ?? [];
  const allLogAnomalies = state.logAnomalies ?? [];

  const [tagFilterInput, setTagFilterInput] = useState('');
  const [expandedLogIndex, setExpandedLogIndex] = useState<number | null>(null);
  const [expandedAnomalyIndex, setExpandedAnomalyIndex] = useState<number | null>(null);
  const [hoveredLogTimestamp, setHoveredLogTimestamp] = useState<number | null>(null);
  const [hoveredAnomalyIndex, setHoveredAnomalyIndex] = useState<number | null>(null);
  const [anomaliesExpanded, setAnomaliesExpanded] = useState(true);
  const [logsExpanded, setLogsExpanded] = useState(true);
  const [logPage, setLogPage] = useState(1);
  const [telemetryLogsExpanded, setTelemetryLogsExpanded] = useState(true);
  const [telemetryLogPage, setTelemetryLogPage] = useState(1);
  const [expandedTelemetryLogIndex, setExpandedTelemetryLogIndex] = useState<number | null>(null);
  const initializedScenarioRef = useRef<string | null>(null);

  // Reset state when scenario changes
  useEffect(() => {
    if (state.activeScenario && initializedScenarioRef.current !== state.activeScenario) {
      initializedScenarioRef.current = state.activeScenario;
      setTagFilterInput('');
      setExpandedLogIndex(null);
      setExpandedAnomalyIndex(null);
      setLogPage(1);
      setTelemetryLogPage(1);
      setExpandedTelemetryLogIndex(null);
    }
  }, [state.activeScenario]);

  const logTagGroups = useMemo(() => {
    const all = extractTagGroups(allLogs.map((l) => getEffectiveTags(l.tags ?? [], l.status)));
    return new Map([...all.entries()].filter(([k]) => MAIN_TAG_FILTER_KEYS.has(k)));
  }, [allLogs]);

  const filteredLogs = useMemo(() => {
    const filter = parseTagFilter(tagFilterInput);
    return allLogs
      .filter((l) => {
        if (filter.include.size === 0 && filter.exclude.size === 0) return true;
        return matchesTagFilter(getEffectiveTags(l.tags ?? [], l.status), filter);
      })
      .sort((a, b) => a.timestampMs - b.timestampMs);
  }, [allLogs, tagFilterInput]);

  const regularLogs = useMemo(
    () => filteredLogs.filter((l) => !(l.tags ?? []).includes('telemetry:true')),
    [filteredLogs]
  );

  const telemetryLogs = useMemo(
    () => filteredLogs.filter((l) => (l.tags ?? []).includes('telemetry:true')),
    [filteredLogs]
  );

  const sortedAnomalies = useMemo(() => {
    const filter = parseTagFilter(tagFilterInput);
    const anomalies = (filter.include.size === 0 && filter.exclude.size === 0)
      ? allLogAnomalies
      : allLogAnomalies.filter((a) => matchesTagFilter(a.tags ?? [], filter));
    return [...anomalies].sort((a, b) => a.timestamp - b.timestamp);
  }, [allLogAnomalies, tagFilterInput]);

  const scenarioStart = state.status?.scenarioStart ?? null;
  const scenarioEnd = state.status?.scenarioEnd ?? null;

  const visibleRegularLogs = useMemo(() => {
    if (!timeRange) return regularLogs;
    return regularLogs.filter((l) => l.timestampMs / 1000 >= timeRange.start && l.timestampMs / 1000 <= timeRange.end);
  }, [regularLogs, timeRange]);

  const visibleTelemetryLogs = useMemo(() => {
    if (!timeRange) return telemetryLogs;
    return telemetryLogs.filter((l) => l.timestampMs / 1000 >= timeRange.start && l.timestampMs / 1000 <= timeRange.end);
  }, [telemetryLogs, timeRange]);

  const visibleAnomalies = useMemo(() => {
    if (!timeRange) return sortedAnomalies;
    return sortedAnomalies.filter((a) => a.timestamp >= timeRange.start && a.timestamp <= timeRange.end);
  }, [sortedAnomalies, timeRange]);

  return (
    <div className="flex-1 flex">
      {/* Sidebar */}
      <aside
        className="bg-slate-800 border-r border-slate-700 flex flex-col"
        style={{ width: sidebarWidth }}
      >
        <ScenarioSelector
          scenarios={scenarios}
          activeScenario={state.activeScenario}
          onLoadScenario={actions.loadScenario}
        />

        {/* Tag filter */}
        <div className="p-4 border-b border-slate-700">
          <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2">
            Tag Filter
          </h2>
          <div className="relative mb-2">
            <input
              type="text"
              value={tagFilterInput}
              onChange={(e) => {
                setTagFilterInput(e.target.value);
                setExpandedLogIndex(null);
                setLogPage(1);
              }}
              placeholder="host:web-1 service:api"
              className="w-full bg-slate-700 text-slate-200 text-xs rounded px-2 py-1.5 placeholder-slate-500 focus:outline-none focus:ring-1 focus:ring-teal-500 font-mono pr-6"
            />
            {tagFilterInput && (
              <button
                onClick={() => { setTagFilterInput(''); setLogPage(1); }}
                className="absolute right-1.5 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-300"
              >
                ×
              </button>
            )}
          </div>
          <TagFilterGroups
            tagGroups={logTagGroups}
            tagFilterInput={tagFilterInput}
            onToggleTag={(tag) => {
              setTagFilterInput(toggleTagInInput(tagFilterInput, tag));
              setExpandedLogIndex(null);
              setLogPage(1);
            }}
            accentColor="teal"
            statusAware
          />
        </div>

        {/* Summary */}
        <div className="p-4">
          <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2">
            Summary
          </h2>
          <div className="space-y-1.5">
            <div className="text-sm text-slate-300">
              {allLogs.filter((l) => !(l.tags ?? []).includes('telemetry:true')).length} log{allLogs.filter((l) => !(l.tags ?? []).includes('telemetry:true')).length !== 1 ? 's' : ''} total
            </div>
            {allLogs.some((l) => (l.tags ?? []).includes('telemetry:true')) && (
              <div className="text-sm text-purple-400 flex items-center gap-1.5">
                <span className="inline-flex items-center justify-center w-3.5 h-3.5 rounded-full bg-purple-600 text-white text-[8px] font-bold">T</span>
                {allLogs.filter((l) => (l.tags ?? []).includes('telemetry:true')).length} telemetry log{allLogs.filter((l) => (l.tags ?? []).includes('telemetry:true')).length !== 1 ? 's' : ''}
              </div>
            )}
            <div className="text-sm text-slate-300">
              {allLogAnomalies.length} anomal{allLogAnomalies.length !== 1 ? 'ies' : 'y'} detected
            </div>
          </div>
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 p-6 overflow-y-auto">
        {state.error && (
          <div className="bg-red-900/50 border border-red-700 rounded-lg p-4 mb-6">
            <div className="text-red-400">{state.error}</div>
          </div>
        )}

        {state.connectionState === 'disconnected' && (
          <div className="text-center py-20">
            <div className="text-slate-400 text-lg">Waiting for observer connection...</div>
          </div>
        )}

        {state.connectionState === 'connected' && !state.activeScenario && (
          <div className="text-center py-20">
            <div className="text-slate-400 text-lg">Select a scenario to begin</div>
          </div>
        )}

        {state.connectionState === 'loading' && (
          <div className="text-center py-20">
            <div className="text-blue-400 text-lg">Loading scenario...</div>
          </div>
        )}

        {state.connectionState === 'ready' && (
          <div>
            {/* Log rate + anomaly timeline */}
            <LogRateChart
              logs={allLogs.filter((l) => !(l.tags ?? []).includes('telemetry:true'))}
              anomalies={sortedAnomalies}
              scenarioStart={scenarioStart ?? null}
              scenarioEnd={scenarioEnd ?? null}
              hoveredTimestamp={hoveredLogTimestamp}
              hoveredAnomalyIndex={hoveredAnomalyIndex}
              timeRange={timeRange}
              onTimeRangeChange={onTimeRangeChange}
              phaseMarkers={phaseMarkers}
            />

            {/* Detected Anomalies collapsible section */}
            {allLogAnomalies.length > 0 && (
              <div className="mb-6">
                <button
                  onClick={() => setAnomaliesExpanded(!anomaliesExpanded)}
                  className="flex items-center gap-2 text-sm font-medium text-slate-300 hover:text-white mb-3 transition-colors"
                >
                  <span className="text-slate-500">{anomaliesExpanded ? '▼' : '▶'}</span>
                  Detected Anomalies ({visibleAnomalies.length}{visibleAnomalies.length !== allLogAnomalies.length ? ` of ${allLogAnomalies.length}` : ''})
                </button>
                {anomaliesExpanded && (
                  <div className="space-y-1.5">
                    {visibleAnomalies.map((anomaly, idx) => (
                      <LogAnomalyCard
                        key={`${anomaly.detectorName}-${anomaly.timestamp}-${idx}`}
                        anomaly={anomaly}
                        isExpanded={expandedAnomalyIndex === idx}
                        onToggle={() =>
                          setExpandedAnomalyIndex(expandedAnomalyIndex === idx ? null : idx)
                        }
                        onHoverEnter={() => setHoveredAnomalyIndex(idx)}
                        onHoverLeave={() => setHoveredAnomalyIndex(null)}
                      />
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* Raw log entries (regular only) */}
            <div>
              <button
                onClick={() => setLogsExpanded(!logsExpanded)}
                className="flex items-center gap-2 text-sm font-medium text-slate-300 hover:text-white mb-3 transition-colors"
              >
                <span className="text-slate-500">{logsExpanded ? '▼' : '▶'}</span>
                Raw Logs ({visibleRegularLogs.length}{visibleRegularLogs.length !== allLogs.length - telemetryLogs.length ? ` of ${allLogs.length - telemetryLogs.length}` : ''})
              </button>

              {logsExpanded && (
                allLogs.filter((l) => !(l.tags ?? []).includes('telemetry:true')).length === 0 ? (
                  <div className="text-center py-8 text-slate-500 text-sm">
                    No log entries. Load a scenario with log files or the demo scenario.
                  </div>
                ) : visibleRegularLogs.length === 0 ? (
                  <div className="text-center py-8 text-slate-500 text-sm">
                    No logs match the selected filters.
                  </div>
                ) : (
                  <>
                    <div className="overflow-y-auto max-h-[480px] space-y-0.5 pr-1">
                      {visibleRegularLogs.slice(0, logPage * LOG_PAGE_SIZE).map((entry, idx) => (
                        <LogEntryRow
                          key={`${entry.timestampMs}-${idx}`}
                          entry={entry}
                          isExpanded={expandedLogIndex === idx}
                          onToggle={() => setExpandedLogIndex(expandedLogIndex === idx ? null : idx)}
                          onHoverEnter={() => setHoveredLogTimestamp(entry.timestampMs)}
                          onHoverLeave={() => setHoveredLogTimestamp(null)}
                        />
                      ))}
                    </div>
                    {visibleRegularLogs.length > logPage * LOG_PAGE_SIZE && (
                      <button
                        onClick={() => setLogPage((p) => p + 1)}
                        className="mt-2 w-full py-1.5 text-xs text-slate-400 hover:text-slate-200 bg-slate-700/40 hover:bg-slate-700/70 rounded transition-colors"
                      >
                        Show more ({visibleRegularLogs.length - logPage * LOG_PAGE_SIZE} remaining)
                      </button>
                    )}
                  </>
                )
              )}
            </div>

            {/* Telemetry log entries */}
            {(allLogs.some((l) => (l.tags ?? []).includes('telemetry:true')) || telemetryLogs.length > 0) && (
              <div>
                <div className="flex items-center gap-3 mb-3">
                  <div className="flex-1 border-t border-purple-800/50" />
                  <button
                    onClick={() => setTelemetryLogsExpanded(!telemetryLogsExpanded)}
                    className="flex items-center gap-1.5 text-xs text-purple-400 font-medium hover:text-purple-300 transition-colors"
                  >
                    <span className="text-purple-600">{telemetryLogsExpanded ? '▼' : '▶'}</span>
                    <span className="inline-flex items-center justify-center w-4 h-4 rounded-full bg-purple-600 text-white text-[9px] font-bold">T</span>
                    Telemetry Logs ({visibleTelemetryLogs.length}{visibleTelemetryLogs.length !== telemetryLogs.length ? ` of ${telemetryLogs.length}` : ''})
                  </button>
                  <div className="flex-1 border-t border-purple-800/50" />
                </div>

                {telemetryLogsExpanded && (
                  visibleTelemetryLogs.length === 0 ? (
                    <div className="text-center py-8 text-slate-500 text-sm">
                      No telemetry logs match the current filters.
                    </div>
                  ) : (
                    <>
                      <div className="overflow-y-auto max-h-[480px] space-y-0.5 pr-1">
                        {visibleTelemetryLogs.slice(0, telemetryLogPage * LOG_PAGE_SIZE).map((entry, idx) => (
                          <LogEntryRow
                            key={`telem-${entry.timestampMs}-${idx}`}
                            entry={entry}
                            isExpanded={expandedTelemetryLogIndex === idx}
                            onToggle={() => setExpandedTelemetryLogIndex(expandedTelemetryLogIndex === idx ? null : idx)}
                            isTelemetry
                            onHoverEnter={() => setHoveredLogTimestamp(entry.timestampMs)}
                            onHoverLeave={() => setHoveredLogTimestamp(null)}
                          />
                        ))}
                      </div>
                      {visibleTelemetryLogs.length > telemetryLogPage * LOG_PAGE_SIZE && (
                        <button
                          onClick={() => setTelemetryLogPage((p) => p + 1)}
                          className="mt-2 w-full py-1.5 text-xs text-purple-400 hover:text-purple-200 bg-purple-900/20 hover:bg-purple-900/40 rounded transition-colors"
                        >
                          Show more ({visibleTelemetryLogs.length - telemetryLogPage * LOG_PAGE_SIZE} remaining)
                        </button>
                      )}
                    </>
                  )
                )}
              </div>
            )}
          </div>
        )}
      </main>
    </div>
  );
}

function ScenarioSelector({
  scenarios,
  activeScenario,
  onLoadScenario,
}: {
  scenarios: ScenarioInfo[];
  activeScenario: string | null;
  onLoadScenario: (name: string) => Promise<void>;
}) {
  return (
    <div className="p-4 border-b border-slate-700">
      <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">
        Scenarios
      </h2>
      <div className="space-y-1">
        {scenarios.length === 0 ? (
          <div className="text-sm text-slate-500">No scenarios found</div>
        ) : (
          scenarios.map((scenario) => (
            <button
              key={scenario.name}
              onClick={() => onLoadScenario(scenario.name)}
              className={`w-full text-left px-3 py-2 rounded text-sm transition-colors ${
                activeScenario === scenario.name
                  ? 'bg-purple-600 text-white'
                  : 'text-slate-300 hover:bg-slate-700'
              }`}
            >
              <div className="font-medium">{scenario.name}</div>
              <div className="text-xs text-slate-400 mt-0.5">
                {[
                  scenario.hasParquet && 'parquet',
                  scenario.hasLogs && 'logs',
                  scenario.hasEvents && 'events',
                ]
                  .filter(Boolean)
                  .join(', ') || 'empty'}
              </div>
            </button>
          ))
        )}
      </div>
    </div>
  );
}
