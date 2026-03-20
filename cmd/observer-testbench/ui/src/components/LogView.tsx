import { useState, useMemo, useRef, useEffect, Fragment } from 'react';
import * as d3 from 'd3';
import { api } from '../api/client';
import type { ScenarioInfo, LogAnomaly, LogEntry, LogsSummary, LogPattern, SeriesData } from '../api/client';
import type { ObserverState, ObserverActions } from '../hooks/useObserver';
import type { PhaseMarker } from './ChartWithAnomalyDetails';
import { MAIN_TAG_FILTER_KEYS } from '../constants';
import { parseTagFilter, toggleTagInInput, matchesTagFilter } from '../filters';
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
  onJumpToSeries?: (groupKey: string) => void;
  requestedPatternFilter?: string | null;
  onRequestedPatternFilterConsumed?: () => void;
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
  activeAnomalyIndex,
  timeRange,
  onTimeRangeChange,
  onAnomalyClick,
  onAnomalyHover,
  phaseMarkers = [],
  logsSummary,
  totalLogCount,
}: {
  logs: LogEntry[];
  anomalies: LogAnomaly[];
  scenarioStart: number | null;
  scenarioEnd: number | null;
  hoveredTimestamp?: number | null;
  activeAnomalyIndex?: number | null;
  timeRange?: TimeRange | null;
  onTimeRangeChange?: (range: TimeRange | null) => void;
  onAnomalyClick?: (index: number) => void;
  onAnomalyHover?: (index: number | null) => void;
  phaseMarkers?: PhaseMarker[];
  logsSummary?: LogsSummary | null;
  totalLogCount?: number;
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
  const onAnomalyClickRef = useRef(onAnomalyClick);
  onAnomalyClickRef.current = onAnomalyClick;
  const onAnomalyHoverRef = useRef(onAnomalyHover);
  onAnomalyHoverRef.current = onAnomalyHover;

  const buckets = useMemo(() => {
    // When summary histogram is available, use pre-computed server-side data
    if (logsSummary?.histogram && logsSummary.histogram.length > 0) {
      const hist = logsSummary.histogram;
      // Compute bucket size from consecutive timestamps (assume uniform)
      const bucketSizeMs = hist.length > 1 ? hist[1].timestampMs - hist[0].timestampMs : 1000;
      return hist.map((h) => ({
        total: h.count,
        error: 0,
        warn: 0,
        info: 0,
        debug: h.count, // Without per-level histogram data, attribute all to debug (neutral color)
        startTs: h.timestampMs / 1000,
        endTs: (h.timestampMs + bucketSizeMs) / 1000,
      }));
    }

    // Fallback: compute from provided logs array (used when summary is not yet available)
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
  }, [logs, logsSummary, scenarioStart, scenarioEnd, timeRange]);

  const detectors = useMemo(
    () => Array.from(new Set(anomalies.map((a) => a.detectorName))),
    [anomalies]
  );
  const usesSummaryHistogram = Boolean(logsSummary?.histogram && logsSummary.histogram.length > 0);

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

    if (width <= m.left + m.right || innerWidth <= 0 || innerHeight <= 0) {
      d3.select(svgRef.current).selectAll('*').remove();
      return;
    }

    d3.select(svgRef.current).selectAll('*').remove();

    const svg = d3.select(svgRef.current).attr('width', width).attr('height', LOG_CHART_HEIGHT);

    svg.append('defs').append('clipPath')
      .attr('id', 'log-rate-clip')
      .append('rect').attr('width', innerWidth).attr('height', innerHeight);

    const g = svg.append('g').attr('transform', `translate(${m.left},${m.top})`);

    const fallbackStart = buckets[0]?.startTs;
    const fallbackEnd = buckets[buckets.length - 1]?.endTs;
    const xDomain: [number, number] = timeRange
      ? [timeRange.start * 1000, timeRange.end * 1000]
      : scenarioStart != null && scenarioEnd != null && scenarioEnd > scenarioStart
        ? [scenarioStart * 1000, scenarioEnd * 1000]
        : [fallbackStart! * 1000, fallbackEnd! * 1000];
    const xScale = d3.scaleTime().domain(xDomain).range([0, innerWidth]);
    xScaleRef.current = xScale;

    const maxTotal = Math.max(1, ...buckets.map((b) => b.total));
    const barsG = g.append('g').attr('clip-path', 'url(#log-rate-clip)');

    const STATUS_LAYERS = [
      { key: 'debug' as const, color: 'rgba(100, 116, 139, 0.6)' },
      { key: 'info'  as const, color: 'rgba(59, 130, 246, 0.65)' },
      { key: 'warn'  as const, color: 'rgba(245, 158, 11, 0.8)'  },
      { key: 'error' as const, color: 'rgba(239, 68, 68, 0.85)'  },
    ] as const;

    // Stacked bars per bucket when per-level data exists; otherwise render a
    // single-color volume bar so the chart does not imply a level breakdown.
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
      if (usesSummaryHistogram) {
        barsG.append('rect')
          .attr('x', x).attr('width', bw)
          .attr('y', innerHeight - totalH).attr('height', totalH)
          .attr('fill', 'rgba(45, 212, 191, 0.65)');
        return;
      }

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
      const isActive = activeAnomalyIndex === i;
      const opacity = activeAnomalyIndex != null && !isActive ? 0.35 : 1;
      const ts = isActive ? 6 : 3;
      const markerG = barsG.append('g')
        .style('cursor', 'pointer')
        .on('click', () => onAnomalyClickRef.current?.(i))
        .on('mouseenter', () => onAnomalyHoverRef.current?.(i))
        .on('mouseleave', () => onAnomalyHoverRef.current?.(null));
      markerG.append('polygon')
        .attr('points', `${x - ts},0 ${x + ts},0 ${x},${(ts * 1.7).toFixed(1)}`)
        .attr('fill', color)
        .attr('opacity', opacity);
      if (isActive) {
        markerG.append('polygon')
          .attr('points', `${x - ts},0 ${x + ts},0 ${x},${(ts * 1.7).toFixed(1)}`)
          .attr('fill', 'none')
          .attr('stroke', 'white')
          .attr('stroke-width', 1)
          .attr('opacity', 0.6);
      }
      markerG.append('line')
        .attr('x1', x).attr('x2', x)
        .attr('y1', ts * 1.7).attr('y2', innerHeight)
        .attr('stroke', color)
        .attr('stroke-width', isActive ? 1.5 : 1)
        .attr('opacity', opacity);
      // Wider invisible hit area for easier clicking
      markerG.append('rect')
        .attr('x', x - 8).attr('width', 16)
        .attr('y', 0).attr('height', innerHeight)
        .attr('fill', 'transparent');
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
  }, [buckets, anomalies, scenarioStart, scenarioEnd, timeRange, hoveredTimestamp, activeAnomalyIndex, phaseMarkers, usesSummaryHistogram]);

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
        <span>Log rate ({totalLogCount ?? logs.length} log{(totalLogCount ?? logs.length) !== 1 ? 's' : ''} total)</span>
        {usesSummaryHistogram ? (
          <span className="flex items-center gap-1">
            <span className="inline-block w-2 h-2 rounded-sm bg-teal-400/70" />
            total volume
          </span>
        ) : (
          <span className="flex items-center gap-2">
            <span className="flex items-center gap-1"><span className="inline-block w-2 h-2 rounded-sm bg-red-500/80" />error</span>
            <span className="flex items-center gap-1"><span className="inline-block w-2 h-2 rounded-sm bg-amber-500/75" />warn</span>
            <span className="flex items-center gap-1"><span className="inline-block w-2 h-2 rounded-sm bg-blue-500/65" />info</span>
            <span className="flex items-center gap-1"><span className="inline-block w-2 h-2 rounded-sm bg-slate-500/60" />debug</span>
          </span>
        )}
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

const PATTERN_CHART_HEIGHT = 60;
const PATTERN_CHART_MARGIN = { top: 4, right: 8, bottom: 18, left: 8 };

const PATTERN_EVOLUTION_HEIGHT = 100;
const PATTERN_EVOLUTION_MARGIN = { top: 4, right: 8, bottom: 20, left: 36 };
const MAX_EVOLUTION_PATTERNS = 10;
const PATTERN_EVOLUTION_COLORS = [
  '#2dd4bf', // teal-400
  '#60a5fa', // blue-400
  '#4ade80', // green-400
  '#facc15', // yellow-400
  '#f87171', // red-400
  '#c084fc', // purple-400
  '#fb923c', // orange-400
  '#22d3ee', // cyan-400
  '#a78bfa', // violet-400
  '#f472b6', // pink-400
];

function PatternCountChart({
  seriesIDs,
  timeRange,
  scenarioStart,
  scenarioEnd,
  phaseMarkers = [],
}: {
  seriesIDs: string[];
  timeRange?: TimeRange | null;
  scenarioStart: number | null;
  scenarioEnd: number | null;
  phaseMarkers?: PhaseMarker[];
}) {
  const svgRef = useRef<SVGSVGElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [allSeries, setAllSeries] = useState<SeriesData[]>([]);

  useEffect(() => {
    const ids = seriesIDs ?? [];
    if (ids.length === 0) return;
    let cancelled = false;
    Promise.all(ids.map((id) => api.getSeriesDataByID(id))).then((results) => {
      if (!cancelled) setAllSeries(results);
    }).catch(console.error);
    return () => { cancelled = true; };
  }, [seriesIDs]);

  useEffect(() => {
    if (!svgRef.current || !containerRef.current || allSeries.length === 0) return;

    const m = PATTERN_CHART_MARGIN;
    const container = containerRef.current;
    const width = container.clientWidth;
    const innerWidth = width - m.left - m.right;
    const innerHeight = PATTERN_CHART_HEIGHT - m.top - m.bottom;
    if (innerWidth <= 0 || innerHeight <= 0) return;

    const displayStart = timeRange?.start ?? scenarioStart;
    const displayEnd = timeRange?.end ?? scenarioEnd;
    if (!displayStart || !displayEnd || displayEnd <= displayStart) return;

    const bucketCount = 60;
    const bucketSize = (displayEnd - displayStart) / bucketCount;
    const counts = new Array(bucketCount).fill(0);

    for (const series of allSeries) {
      for (const pt of series.points) {
        const t = pt.timestamp;
        if (t < displayStart || t > displayEnd) continue;
        const idx = Math.min(Math.floor((t - displayStart) / bucketSize), bucketCount - 1);
        counts[idx] += pt.value;
      }
    }

    const maxCount = Math.max(1, ...counts);

    d3.select(svgRef.current).selectAll('*').remove();
    const svg = d3.select(svgRef.current).attr('width', width).attr('height', PATTERN_CHART_HEIGHT);
    svg.append('defs').append('clipPath').attr('id', 'ptn-clip')
      .append('rect').attr('width', innerWidth).attr('height', innerHeight);
    const g = svg.append('g').attr('transform', `translate(${m.left},${m.top})`);
    const xScale = d3.scaleLinear().domain([displayStart, displayEnd]).range([0, innerWidth]);
    const barsG = g.append('g').attr('clip-path', 'url(#ptn-clip)');

    counts.forEach((count, i) => {
      const startT = displayStart + i * bucketSize;
      const endT = startT + bucketSize;
      const x = xScale(startT);
      const bw = Math.max(0, xScale(endT) - x - 1);
      if (bw <= 0) return;
      if (count === 0) {
        barsG.append('rect').attr('x', x).attr('width', bw)
          .attr('y', innerHeight - 2).attr('height', 2)
          .attr('fill', 'rgba(51, 65, 85, 0.25)');
        return;
      }
      const h = Math.max(3, (count / maxCount) * innerHeight);
      barsG.append('rect').attr('x', x).attr('width', bw)
        .attr('y', innerHeight - h).attr('height', h)
        .attr('fill', 'rgba(45, 212, 191, 0.7)');
    });

    phaseMarkers.forEach((marker) => {
      const x = xScale(marker.timestamp);
      if (x < -20 || x > innerWidth + 20) return;
      barsG.append('line')
        .attr('x1', x).attr('x2', x).attr('y1', 0).attr('y2', innerHeight)
        .attr('stroke', marker.color).attr('stroke-width', 1)
        .attr('stroke-dasharray', '4,3').attr('opacity', 0.8);
    });

    g.append('g').attr('transform', `translate(0,${innerHeight})`)
      .call(d3.axisBottom(xScale).ticks(5)
        .tickFormat((d) => {
          const date = new Date((d as number) * 1000);
          return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
        }))
      .attr('color', '#334155')
      .selectAll('text').attr('fill', '#64748b').attr('font-size', '9px');
    g.select('.domain').attr('stroke', '#334155');
  }, [allSeries, timeRange, scenarioStart, scenarioEnd, phaseMarkers]);

  if ((seriesIDs ?? []).length === 0) {
    return (
      <div className="text-xs text-slate-500 italic py-2">
        No count series found for this pattern.
      </div>
    );
  }

  return (
    <div ref={containerRef} className="w-full mt-2">
      <svg ref={svgRef} style={{ display: 'block' }} />
    </div>
  );
}

// Chart showing the count evolution of every pattern stacked over time.
// Up to MAX_EVOLUTION_PATTERNS patterns are shown (top by count).
function PatternEvolutionChart({
  patterns,
  timeRange,
  scenarioStart,
  scenarioEnd,
  phaseMarkers = [],
  selectedPatternHash,
}: {
  patterns: LogPattern[];
  timeRange?: TimeRange | null;
  scenarioStart: number | null;
  scenarioEnd: number | null;
  phaseMarkers?: PhaseMarker[];
  selectedPatternHash?: string | null;
}) {
  const svgRef = useRef<SVGSVGElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [seriesDataMap, setSeriesDataMap] = useState<Map<string, SeriesData>>(new Map());

  // Only show top N patterns by count
  const chartPatterns = useMemo(
    () => patterns.slice(0, MAX_EVOLUTION_PATTERNS),
    [patterns]
  );

  // Collect unique series IDs across the displayed patterns
  const allSeriesIDs = useMemo(() => {
    const ids = new Set<string>();
    for (const p of chartPatterns) {
      for (const id of (p.seriesIDs ?? [])) ids.add(id);
    }
    return Array.from(ids);
  }, [chartPatterns]);

  useEffect(() => {
    if (allSeriesIDs.length === 0) return;
    let cancelled = false;
    Promise.all(
      allSeriesIDs.map((id) =>
        api.getSeriesDataByID(id).then((s) => [id, s] as [string, SeriesData])
      )
    ).then((results) => {
      if (!cancelled) setSeriesDataMap(new Map(results));
    }).catch(console.error);
    return () => { cancelled = true; };
  }, [allSeriesIDs]);

  useEffect(() => {
    if (!svgRef.current || !containerRef.current) return;
    if (seriesDataMap.size === 0 && allSeriesIDs.length > 0) return;
    if (chartPatterns.length === 0) return;

    const m = PATTERN_EVOLUTION_MARGIN;
    const container = containerRef.current;
    const width = container.clientWidth;
    const innerWidth = width - m.left - m.right;
    const innerHeight = PATTERN_EVOLUTION_HEIGHT - m.top - m.bottom;
    if (innerWidth <= 0 || innerHeight <= 0) return;

    const displayStart = timeRange?.start ?? scenarioStart;
    const displayEnd = timeRange?.end ?? scenarioEnd;
    if (!displayStart || !displayEnd || displayEnd <= displayStart) return;

    const bucketCount = 60;
    const bucketSize = (displayEnd - displayStart) / bucketCount;

    // Compute per-pattern bucketed counts
    const patternBuckets = chartPatterns.map((p) => {
      const counts = new Array<number>(bucketCount).fill(0);
      for (const id of (p.seriesIDs ?? [])) {
        const series = seriesDataMap.get(id);
        if (!series) continue;
        for (const pt of series.points) {
          const t = pt.timestamp;
          if (t < displayStart || t > displayEnd) continue;
          const idx = Math.min(Math.floor((t - displayStart) / bucketSize), bucketCount - 1);
          counts[idx] += pt.value;
        }
      }
      return counts;
    });

    const stackedBuckets = Array.from({ length: bucketCount }, (_, i) => ({
      startT: displayStart + i * bucketSize,
      endT: displayStart + (i + 1) * bucketSize,
      total: patternBuckets.reduce((s, c) => s + c[i], 0),
      segments: patternBuckets.map((c) => c[i]),
    }));

    const maxTotal = Math.max(1, ...stackedBuckets.map((b) => b.total));

    d3.select(svgRef.current).selectAll('*').remove();
    const svg = d3.select(svgRef.current).attr('width', width).attr('height', PATTERN_EVOLUTION_HEIGHT);
    svg.append('defs').append('clipPath').attr('id', 'ptn-evo-clip')
      .append('rect').attr('width', innerWidth).attr('height', innerHeight);
    const g = svg.append('g').attr('transform', `translate(${m.left},${m.top})`);
    const xScale = d3.scaleLinear().domain([displayStart, displayEnd]).range([0, innerWidth]);
    const barsG = g.append('g').attr('clip-path', 'url(#ptn-evo-clip)');

    stackedBuckets.forEach((b) => {
      const x = xScale(b.startT);
      const bw = Math.max(0, xScale(b.endT) - x - 1);
      if (bw <= 0) return;

      if (b.total === 0) {
        barsG.append('rect').attr('x', x).attr('width', bw)
          .attr('y', innerHeight - 2).attr('height', 2)
          .attr('fill', 'rgba(51, 65, 85, 0.25)');
        return;
      }

      const totalH = Math.max(3, (b.total / maxTotal) * innerHeight);
      let yBottom = innerHeight;
      b.segments.forEach((count, i) => {
        if (count === 0) return;
        const isSelected = selectedPatternHash === chartPatterns[i].hash;
        const opacity = selectedPatternHash && !isSelected ? 0.25 : 0.85;
        const segH = (count / b.total) * totalH;
        const hex = PATTERN_EVOLUTION_COLORS[i % PATTERN_EVOLUTION_COLORS.length];
        barsG.append('rect').attr('x', x).attr('width', bw)
          .attr('y', yBottom - segH).attr('height', segH)
          .attr('fill', hex).attr('opacity', opacity);
        yBottom -= segH;
      });
    });

    phaseMarkers.forEach((marker) => {
      const x = xScale(marker.timestamp);
      if (x < -20 || x > innerWidth + 20) return;
      barsG.append('line')
        .attr('x1', x).attr('x2', x).attr('y1', 0).attr('y2', innerHeight)
        .attr('stroke', marker.color).attr('stroke-width', 1)
        .attr('stroke-dasharray', '4,3').attr('opacity', 0.8);
    });

    // Y axis
    const yScale = d3.scaleLinear().domain([0, maxTotal]).range([innerHeight, 0]);
    const yAxis = g.append('g')
      .call(d3.axisLeft(yScale).ticks(3).tickFormat(d3.format('.0~s') as (v: d3.NumberValue) => string));
    yAxis.attr('color', '#334155');
    yAxis.selectAll('text').attr('fill', '#64748b').attr('font-size', '9px');
    yAxis.select('.domain').attr('stroke', '#334155');

    // X axis
    const xAxis = g.append('g').attr('transform', `translate(0,${innerHeight})`)
      .call(d3.axisBottom(xScale).ticks(5)
        .tickFormat((d) => {
          const date = new Date((d as number) * 1000);
          return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
        }));
    xAxis.attr('color', '#334155');
    xAxis.selectAll('text').attr('fill', '#64748b').attr('font-size', '9px');
    xAxis.select('.domain').attr('stroke', '#334155');
  }, [seriesDataMap, chartPatterns, allSeriesIDs, timeRange, scenarioStart, scenarioEnd, phaseMarkers, selectedPatternHash]);

  if (chartPatterns.length === 0) return null;

  return (
    <div className="mb-3 bg-slate-900/40 rounded border border-slate-700/40 p-2">
      <div className="text-[10px] text-slate-500 uppercase tracking-wide mb-1.5 flex items-center justify-between">
        <span>Patterns over time</span>
        {patterns.length > MAX_EVOLUTION_PATTERNS && (
          <span className="normal-case text-slate-600">top {MAX_EVOLUTION_PATTERNS} of {patterns.length}</span>
        )}
      </div>
      <div ref={containerRef} className="w-full">
        <svg ref={svgRef} style={{ display: 'block' }} />
      </div>
      {/* Legend */}
      <div className="flex flex-wrap gap-x-3 gap-y-0.5 mt-1.5">
        {chartPatterns.map((p, i) => {
          const isSelected = selectedPatternHash === p.hash;
          return (
            <div
              key={p.hash}
              className={`flex items-center gap-1 text-[10px] transition-opacity ${
                selectedPatternHash && !isSelected ? 'opacity-40' : 'opacity-100'
              }`}
            >
              <span
                className="w-2 h-2 rounded-sm flex-shrink-0"
                style={{ backgroundColor: PATTERN_EVOLUTION_COLORS[i % PATTERN_EVOLUTION_COLORS.length] }}
              />
              <span className="text-slate-400 truncate max-w-[140px]" title={p.patternString}>
                {p.patternString}
              </span>
            </div>
          );
        })}
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

const LOG_PAGE_SIZE = 50;

export function LogView({ state, actions, sidebarWidth, timeRange, onTimeRangeChange, phaseMarkers, onJumpToSeries, requestedPatternFilter, onRequestedPatternFilterConsumed }: LogViewProps) {
  const scenarios = state.scenarios ?? [];
  const allLogAnomalies = state.logAnomalies ?? [];

  const [tagFilterInput, setTagFilterInput] = useState('');
  const [expandedLogIndex, setExpandedLogIndex] = useState<number | null>(null);
  const [expandedAnomalyIndex, setExpandedAnomalyIndex] = useState<number | null>(null);
  const [hoveredLogTimestamp, setHoveredLogTimestamp] = useState<number | null>(null);
  const [hoveredAnomalyIndex, setHoveredAnomalyIndex] = useState<number | null>(null);
  const [userDisabledDetectors, setUserDisabledDetectors] = useState<Set<string>>(new Set());
  const [anomaliesExpanded, setAnomaliesExpanded] = useState(true);
  const [logsExpanded, setLogsExpanded] = useState(true);
  const [logPage, setLogPage] = useState(1);
  const [telemetryLogsExpanded, setTelemetryLogsExpanded] = useState(true);
  const [telemetryLogPage, setTelemetryLogPage] = useState(1);
  const [expandedTelemetryLogIndex, setExpandedTelemetryLogIndex] = useState<number | null>(null);
  const anomalyRowRefs = useRef<Map<number, HTMLDivElement>>(new Map());
  const initializedScenarioRef = useRef<string | null>(null);

  // Log patterns state
  const [logPatterns, setLogPatterns] = useState<LogPattern[]>([]);
  const [patternsExpanded, setPatternsExpanded] = useState(true);
  const [patternSearch, setPatternSearch] = useState('');
  const [patternSortBy, setPatternSortBy] = useState<'count' | 'pattern'>('count');
  const [selectedPatternHash, setSelectedPatternHash] = useState<string | null>(null);
  const [activePatternFilter, setActivePatternFilter] = useState<string | null>(null);
  const patternRowRefs = useRef<Map<string, HTMLTableRowElement>>(new Map());
  // Derive unique log detector names from all anomalies
  const logDetectors = useMemo(
    () => Array.from(new Set(allLogAnomalies.map((a) => a.detectorName))).sort(),
    [allLogAnomalies]
  );

  const enabledLogDetectors = useMemo(
    () => new Set(logDetectors.filter((d) => !userDisabledDetectors.has(d))),
    [logDetectors, userDisabledDetectors]
  );

  // Combined hover+expanded index drives chart highlighting
  const activeAnomalyIndex = hoveredAnomalyIndex !== null ? hoveredAnomalyIndex : expandedAnomalyIndex;

  const [rawLogsPage, setRawLogsPage] = useState<LogEntry[]>([]);
  const [rawLogsTotal, setRawLogsTotal] = useState(0);
  const [telemetryLogsPage, setTelemetryLogsPage] = useState<LogEntry[]>([]);
  const [telemetryLogsTotal, setTelemetryLogsTotal] = useState(0);
  const [logsSummary, setLogsSummary] = useState<LogsSummary | null>(state.logsSummary);

  // Apply a pattern filter requested from an external tab (e.g. MetricsView "↗ View in logs" button).
  // Also expand the pattern section, open the detail row, and scroll it into view.
  useEffect(() => {
    if (!requestedPatternFilter) return;
    setActivePatternFilter(requestedPatternFilter);
    setSelectedPatternHash(requestedPatternFilter);
    setPatternsExpanded(true);
    setLogPage(1);
    onRequestedPatternFilterConsumed?.();
    // Scroll the row into view after the next paint.
    requestAnimationFrame(() => {
      patternRowRefs.current.get(requestedPatternFilter)?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    });
  }, [requestedPatternFilter, onRequestedPatternFilterConsumed]);

  // Fetch log patterns when scenario is ready
  useEffect(() => {
    if (state.connectionState !== 'ready') return;
    let cancelled = false;
    api.getLogPatterns().then((patterns) => {
      if (!cancelled) setLogPatterns(patterns);
    }).catch(console.error);
    return () => { cancelled = true; };
  }, [state.connectionState, state.activeScenario, state.scenarioDataVersion]);

  // Fetch raw and telemetry pages independently so pagination matches the UI panes.
  useEffect(() => {
    if (state.connectionState !== 'ready') return;

    let cancelled = false;
    const baseParams: { start?: number; end?: number; tags?: string; pattern?: string } = {};
    if (timeRange) {
      baseParams.start = Math.floor(timeRange.start * 1000);
      baseParams.end = Math.floor(timeRange.end * 1000);
    }
    const trimmedTagFilter = tagFilterInput.trim();
    if (trimmedTagFilter) {
      baseParams.tags = trimmedTagFilter;
    }
    if (activePatternFilter) {
      baseParams.pattern = activePatternFilter;
    }

    api.getLogs({
      ...baseParams,
      kind: 'raw',
      limit: LOG_PAGE_SIZE,
      offset: (logPage - 1) * LOG_PAGE_SIZE,
    }).then((resp) => {
      if (!cancelled) {
        setRawLogsPage(resp.logs);
        setRawLogsTotal(resp.total);
      }
    }).catch((err) => {
      console.error('Failed to fetch raw logs page:', err);
    });

    api.getLogs({
      ...baseParams,
      kind: 'telemetry',
      limit: LOG_PAGE_SIZE,
      offset: (telemetryLogPage - 1) * LOG_PAGE_SIZE,
    }).then((resp) => {
      if (!cancelled) {
        setTelemetryLogsPage(resp.logs);
        setTelemetryLogsTotal(resp.total);
      }
    }).catch((err) => {
      console.error('Failed to fetch telemetry logs page:', err);
    });

    api.getLogsSummary({
      ...baseParams,
      kind: 'all',
    }).then((resp) => {
      if (!cancelled) {
        setLogsSummary(resp);
      }
    }).catch((err) => {
      console.error('Failed to fetch logs summary:', err);
    });

    return () => { cancelled = true; };
  }, [logPage, telemetryLogPage, timeRange, tagFilterInput, activePatternFilter, state.connectionState, state.activeScenario, state.scenarioDataVersion]);

  // Reset state when scenario changes
  useEffect(() => {
    if (state.activeScenario && initializedScenarioRef.current !== state.activeScenario) {
      initializedScenarioRef.current = state.activeScenario;
      setTagFilterInput('');
      setExpandedLogIndex(null);
      setExpandedAnomalyIndex(null);
      setHoveredAnomalyIndex(null);
      setUserDisabledDetectors(new Set());
      setLogPage(1);
      setTelemetryLogPage(1);
      setExpandedTelemetryLogIndex(null);
      setRawLogsPage([]);
      setRawLogsTotal(0);
      setTelemetryLogsPage([]);
      setTelemetryLogsTotal(0);
      setLogsSummary(null);
      setLogPatterns([]);
      setSelectedPatternHash(null);
      setActivePatternFilter(null);
      setPatternSearch('');
      anomalyRowRefs.current.clear();
    }
  }, [state.activeScenario]);

  const logTagGroups = useMemo(() => {
    const all = new Map<string, string[]>(
      Object.entries(logsSummary?.tagGroups ?? {}).map(([key, values]) => [key, [...values]])
    );
    return new Map([...all.entries()].filter(([k]) => MAIN_TAG_FILTER_KEYS.has(k)));
  }, [logsSummary]);

  const regularLogs = useMemo(
    () => [...rawLogsPage].sort((a, b) => a.timestampMs - b.timestampMs),
    [rawLogsPage]
  );

  const telemetryLogs = useMemo(
    () => [...telemetryLogsPage].sort((a, b) => a.timestampMs - b.timestampMs),
    [telemetryLogsPage]
  );

  const sortedAnomalies = useMemo(() => {
    const filter = parseTagFilter(tagFilterInput);
    let anomalies = (filter.include.size === 0 && filter.exclude.size === 0)
      ? allLogAnomalies
      : allLogAnomalies.filter((a) => matchesTagFilter(a.tags ?? [], filter));
    if (enabledLogDetectors.size < logDetectors.length) {
      anomalies = anomalies.filter((a) => enabledLogDetectors.has(a.detectorName));
    }
    return [...anomalies].sort((a, b) => a.timestamp - b.timestamp);
  }, [allLogAnomalies, tagFilterInput, enabledLogDetectors, logDetectors.length]);

  const scenarioStart = state.status?.scenarioStart ?? null;
  const scenarioEnd = state.status?.scenarioEnd ?? null;

  const filteredPatterns = useMemo(() => {
    const search = patternSearch.trim().toLowerCase();
    let patterns = search
      ? logPatterns.filter((p) => p.patternString.toLowerCase().includes(search))
      : logPatterns;
    if (patternSortBy === 'pattern') {
      patterns = [...patterns].sort((a, b) => a.patternString.localeCompare(b.patternString));
    }
    return patterns;
  }, [logPatterns, patternSearch, patternSortBy]);

  const visibleAnomalies = useMemo(() => {
    if (!timeRange) return sortedAnomalies;
    return sortedAnomalies.filter((a) => a.timestamp >= timeRange.start && a.timestamp <= timeRange.end);
  }, [sortedAnomalies, timeRange]);

  return (
    <div className="flex-1 flex">
      {/* Sidebar */}
      <aside
        className="bg-slate-800 border-r border-slate-700 overflow-y-auto"
        style={{ width: sidebarWidth }}
      >
        <ScenarioSelector
          scenarios={scenarios}
          activeScenario={state.activeScenario}
          onLoadScenario={actions.loadScenario}
        />

        {/* Log Detectors */}
        {logDetectors.length > 0 && (
          <div className="p-4 border-b border-slate-700">
            <div className="flex items-center justify-between mb-2">
              <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider">
                Log Detectors
              </h2>
              <div className="flex gap-1">
                <button
                  onClick={() => setUserDisabledDetectors(new Set())}
                  className="text-[10px] px-1.5 py-0.5 rounded bg-slate-700 text-slate-400 hover:text-slate-200 transition-colors"
                >
                  All
                </button>
                <button
                  onClick={() => setUserDisabledDetectors(new Set(logDetectors))}
                  className="text-[10px] px-1.5 py-0.5 rounded bg-slate-700 text-slate-400 hover:text-slate-200 transition-colors"
                >
                  None
                </button>
              </div>
            </div>
            <div className="space-y-0.5">
              {logDetectors.map((name) => {
                const count = allLogAnomalies.filter((a) => a.detectorName === name).length;
                const isEnabled = !userDisabledDetectors.has(name);
                return (
                  <label
                    key={name}
                    className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-slate-700 cursor-pointer"
                  >
                    <input
                      type="checkbox"
                      checked={isEnabled}
                      onChange={() =>
                        setUserDisabledDetectors((prev) => {
                          const next = new Set(prev);
                          if (next.has(name)) next.delete(name);
                          else next.add(name);
                          return next;
                        })
                      }
                      className="rounded border-slate-600 bg-slate-700 text-purple-600 focus:ring-purple-500 flex-shrink-0"
                    />
                    <span
                      className="text-xs px-1.5 py-0.5 rounded font-medium flex-1 truncate"
                      style={{ backgroundColor: `${detectorLineColor(name)}22`, color: detectorLineColor(name) }}
                    >
                      {name}
                    </span>
                    <span className="text-xs text-slate-500 tabular-nums flex-shrink-0">{count}</span>
                  </label>
                );
              })}
            </div>
          </div>
        )}

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
                setTelemetryLogPage(1);
              }}
              placeholder="host:web-1 service:api"
              className="w-full bg-slate-700 text-slate-200 text-xs rounded px-2 py-1.5 placeholder-slate-500 focus:outline-none focus:ring-1 focus:ring-teal-500 font-mono pr-6"
            />
            {tagFilterInput && (
              <button
                onClick={() => { setTagFilterInput(''); setLogPage(1); setTelemetryLogPage(1); }}
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
              setTelemetryLogPage(1);
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
              {logsSummary?.totalCount ?? rawLogsTotal + telemetryLogsTotal} log{(logsSummary?.totalCount ?? rawLogsTotal + telemetryLogsTotal) !== 1 ? 's' : ''} total
            </div>
            {logsSummary?.countByLevel && Object.keys(logsSummary.countByLevel).length > 0 && (
              <div className="flex flex-wrap gap-1.5">
                {Object.entries(logsSummary.countByLevel).map(([level, count]) => (
                  <span key={level} className={`text-xs px-1.5 py-0.5 rounded font-medium ${levelBadgeColor(level)}`}>
                    {level}: {count}
                  </span>
                ))}
              </div>
            )}
            <div className="text-sm text-slate-300">
              {sortedAnomalies.length}{sortedAnomalies.length !== allLogAnomalies.length ? ` of ${allLogAnomalies.length}` : ''} anomal{allLogAnomalies.length !== 1 ? 'ies' : 'y'} shown
            </div>
            {logPatterns.length > 0 && (
              <div className="flex items-center gap-1.5 text-xs text-slate-400 pt-1 border-t border-slate-700/50 mt-1">
                <span className="w-1.5 h-1.5 rounded-full bg-teal-500/70 flex-shrink-0" />
                {logPatterns.length} pattern{logPatterns.length !== 1 ? 's' : ''}
                {activePatternFilter && (
                  <button
                    onClick={() => setActivePatternFilter(null)}
                    className="ml-auto text-teal-400 hover:text-teal-300 text-[10px]"
                    title="Clear pattern filter"
                  >
                    ✕ clear filter
                  </button>
                )}
              </div>
            )}
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
              logs={[]}
              anomalies={visibleAnomalies}
              scenarioStart={scenarioStart ?? null}
              scenarioEnd={scenarioEnd ?? null}
              hoveredTimestamp={hoveredLogTimestamp}
              activeAnomalyIndex={activeAnomalyIndex}
              timeRange={timeRange}
              onTimeRangeChange={onTimeRangeChange}
              onAnomalyClick={(idx) => {
                setExpandedAnomalyIndex(idx);
                setHoveredAnomalyIndex(idx);
                setTimeout(() => {
                  anomalyRowRefs.current.get(idx)?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
                }, 50);
              }}
              onAnomalyHover={(idx) => setHoveredAnomalyIndex(idx)}
              phaseMarkers={phaseMarkers}
              logsSummary={logsSummary}
              totalLogCount={logsSummary?.totalCount ?? rawLogsTotal + telemetryLogsTotal}
            />

            {/* Anomaly Details panel — compact rows styled like metrics view */}
            {sortedAnomalies.length > 0 && (
              <div className="mb-6 bg-slate-800 rounded-lg overflow-hidden">
                <button
                  onClick={() => setAnomaliesExpanded(!anomaliesExpanded)}
                  className="flex items-center gap-2 w-full px-4 py-2.5 text-sm font-medium text-slate-300 hover:text-white hover:bg-slate-700/50 transition-colors border-b border-slate-700"
                >
                  <span className="text-slate-500">{anomaliesExpanded ? '▼' : '▶'}</span>
                  Log Anomalies
                  <span className="ml-1 text-xs text-slate-500 font-normal">
                    ({visibleAnomalies.length}{visibleAnomalies.length !== sortedAnomalies.length ? ` of ${sortedAnomalies.length} in view` : ''})
                  </span>
                </button>

                {anomaliesExpanded && (
                  <div className="px-4 py-2 space-y-0">
                    {visibleAnomalies.length === 0 ? (
                      <div className="py-4 text-center text-xs text-slate-500">
                        No anomalies in the current time range
                      </div>
                    ) : (
                      visibleAnomalies.map((anomaly, idx) => {
                        const isExpanded = expandedAnomalyIndex === idx;
                        const isActive = activeAnomalyIndex === idx;
                        const headerTags = (anomaly.tags ?? []).filter(
                          (tag) => MAIN_TAG_FILTER_KEYS.has(tag.slice(0, tag.indexOf(':')))
                        );
                        return (
                          <div
                            key={`${anomaly.detectorName}-${anomaly.timestamp}-${idx}`}
                            ref={(el) => {
                              if (el) anomalyRowRefs.current.set(idx, el);
                              else anomalyRowRefs.current.delete(idx);
                            }}
                            className={`text-xs rounded border-b border-slate-700/50 last:border-b-0 transition-colors ${
                              isActive ? 'bg-slate-700/40 ring-1 ring-inset ring-slate-500/60' : ''
                            }`}
                            onMouseEnter={() => setHoveredAnomalyIndex(idx)}
                            onMouseLeave={() => setHoveredAnomalyIndex(null)}
                          >
                            <button
                              onClick={() => setExpandedAnomalyIndex(isExpanded ? null : idx)}
                              className="w-full text-left flex items-center gap-2 py-1.5 px-1 hover:bg-slate-700/50 rounded"
                            >
                              <span className="flex-shrink-0 text-slate-500 font-mono w-16 text-right">
                                {formatTimestamp(anomaly.timestamp)}
                              </span>
                              <span
                                className="flex-shrink-0 text-[10px] px-1.5 py-0.5 rounded font-medium"
                                style={{ backgroundColor: `${detectorLineColor(anomaly.detectorName)}22`, color: detectorLineColor(anomaly.detectorName) }}
                              >
                                {anomaly.detectorName}
                              </span>
                              {anomaly.score !== undefined && (
                                <span className={`flex-shrink-0 text-[10px] px-1.5 py-0.5 rounded font-mono ${scoreColor(anomaly.score)}`}>
                                  {anomaly.score.toFixed(2)}
                                </span>
                              )}
                              <span className="text-slate-300 flex-1 truncate">{anomaly.title}</span>
                              {headerTags.length > 0 && (
                                <span className="flex gap-1 flex-shrink-0">
                                  {headerTags.map((tag) => (
                                    <span key={tag} className="px-1 py-0.5 rounded text-[9px] bg-slate-700/80 text-slate-400 font-mono">
                                      {tag}
                                    </span>
                                  ))}
                                </span>
                              )}
                              <span className="text-slate-500 flex-shrink-0">{isExpanded ? '▼' : '▶'}</span>
                            </button>

                            {isExpanded && (
                              <div className="ml-1 mb-2 p-2 bg-slate-900/50 rounded border border-slate-700/50">
                                <pre className="text-xs text-slate-300 whitespace-pre-wrap break-all font-mono leading-relaxed max-h-32 overflow-y-auto">
                                  {anomaly.description}
                                </pre>
                                {anomaly.tags && anomaly.tags.length > 0 && (
                                  <div className="mt-2 pt-2 border-t border-slate-700/50">
                                    <div className="text-[10px] text-slate-500 mb-1">Tags:</div>
                                    <div className="flex gap-1 flex-wrap">
                                      {anomaly.tags.map((tag) => (
                                        <span key={tag} className="px-1.5 py-0.5 rounded text-[10px] bg-slate-700/80 text-slate-400 font-mono">
                                          {tag}
                                        </span>
                                      ))}
                                    </div>
                                  </div>
                                )}
                                <div className="mt-1.5 text-[10px] text-slate-500">
                                  Source: <span className="text-slate-400">{anomaly.source}</span>
                                </div>
                              </div>
                            )}
                          </div>
                        );
                      })
                    )}
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
                Raw Logs (page {logPage} of {Math.max(1, Math.ceil(rawLogsTotal / LOG_PAGE_SIZE))}, {rawLogsTotal} total)
              </button>

              {logsExpanded && (
                rawLogsTotal === 0 && rawLogsPage.length === 0 ? (
                  <div className="text-center py-8 text-slate-500 text-sm">
                    No log entries. Load a scenario with log files or the demo scenario.
                  </div>
                ) : regularLogs.length === 0 ? (
                  <div className="text-center py-8 text-slate-500 text-sm">
                    No logs match the selected filters.
                  </div>
                ) : (
                  <>
                    <div className="overflow-y-auto max-h-[480px] space-y-0.5 pr-1">
                      {regularLogs.map((entry, idx) => (
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
                    {/* Pagination controls */}
                    <div className="flex items-center justify-between mt-2">
                      <button
                        onClick={() => setLogPage((p) => Math.max(1, p - 1))}
                        disabled={logPage <= 1}
                        className="px-3 py-1.5 text-xs text-slate-400 hover:text-slate-200 bg-slate-700/40 hover:bg-slate-700/70 rounded transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
                      >
                        Previous
                      </button>
                      <span className="text-xs text-slate-500">
                        {rawLogsTotal === 0 ? 0 : (logPage - 1) * LOG_PAGE_SIZE + 1}&ndash;{Math.min(logPage * LOG_PAGE_SIZE, rawLogsTotal)} of {rawLogsTotal}
                      </span>
                      <button
                        onClick={() => setLogPage((p) => p + 1)}
                        disabled={logPage * LOG_PAGE_SIZE >= rawLogsTotal}
                        className="px-3 py-1.5 text-xs text-slate-400 hover:text-slate-200 bg-slate-700/40 hover:bg-slate-700/70 rounded transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
                      >
                        Next
                      </button>
                    </div>
                  </>
                )
              )}
            </div>

            {/* Telemetry log entries — shown before patterns */}
            {(telemetryLogsTotal > 0 || telemetryLogs.length > 0) && (
              <div>
                <div className="flex items-center gap-3 mb-3">
                  <div className="flex-1 border-t border-purple-800/50" />
                  <button
                    onClick={() => setTelemetryLogsExpanded(!telemetryLogsExpanded)}
                    className="flex items-center gap-1.5 text-xs text-purple-400 font-medium hover:text-purple-300 transition-colors"
                  >
                    <span className="text-purple-600">{telemetryLogsExpanded ? '▼' : '▶'}</span>
                    <span className="inline-flex items-center justify-center w-4 h-4 rounded-full bg-purple-600 text-white text-[9px] font-bold">T</span>
                    Telemetry Logs (page {telemetryLogPage} of {Math.max(1, Math.ceil(telemetryLogsTotal / LOG_PAGE_SIZE))}, {telemetryLogsTotal} total)
                  </button>
                  <div className="flex-1 border-t border-purple-800/50" />
                </div>

                {telemetryLogsExpanded && (
                  telemetryLogsTotal === 0 && telemetryLogs.length === 0 ? (
                    <div className="text-center py-8 text-slate-500 text-sm">
                      No telemetry logs match the current filters.
                    </div>
                  ) : (
                    <>
                      <div className="overflow-y-auto max-h-[480px] space-y-0.5 pr-1">
                        {telemetryLogs.map((entry, idx) => (
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
                      <div className="flex items-center justify-between mt-2">
                        <button
                          onClick={() => setTelemetryLogPage((p) => Math.max(1, p - 1))}
                          disabled={telemetryLogPage <= 1}
                          className="px-3 py-1.5 text-xs text-purple-300 hover:text-purple-100 bg-purple-900/20 hover:bg-purple-900/40 rounded transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
                        >
                          Previous
                        </button>
                        <span className="text-xs text-slate-500">
                          {telemetryLogsTotal === 0 ? 0 : (telemetryLogPage - 1) * LOG_PAGE_SIZE + 1}&ndash;{Math.min(telemetryLogPage * LOG_PAGE_SIZE, telemetryLogsTotal)} of {telemetryLogsTotal}
                        </span>
                        <button
                          onClick={() => setTelemetryLogPage((p) => p + 1)}
                          disabled={telemetryLogPage * LOG_PAGE_SIZE >= telemetryLogsTotal}
                          className="px-3 py-1.5 text-xs text-purple-300 hover:text-purple-100 bg-purple-900/20 hover:bg-purple-900/40 rounded transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
                        >
                          Next
                        </button>
                      </div>
                    </>
                  )
                )}
              </div>
            )}
            {/* Log Patterns section — below raw and telemetry logs */}
            <div className="mt-6">
              <button
                onClick={() => setPatternsExpanded(!patternsExpanded)}
                className="flex items-center gap-2 text-sm font-medium text-slate-300 hover:text-white mb-3 transition-colors"
              >
                <span className="text-slate-500">{patternsExpanded ? '▼' : '▶'}</span>
                Log Patterns
                {logPatterns.length > 0 && (
                  <span className="text-xs px-1.5 py-0.5 rounded bg-teal-900/40 text-teal-400 font-mono">
                    {logPatterns.length}
                  </span>
                )}
              </button>

              {patternsExpanded && (
                logPatterns.length === 0 ? (
                  <div className="text-center py-6 text-slate-500 text-sm">
                    No patterns detected. Load a scenario with logs.
                  </div>
                ) : (
                  <>
                    {/* Pattern count evolution chart */}
                    <PatternEvolutionChart
                      patterns={logPatterns}
                      timeRange={timeRange}
                      scenarioStart={scenarioStart}
                      scenarioEnd={scenarioEnd}
                      phaseMarkers={phaseMarkers}
                      selectedPatternHash={selectedPatternHash}
                    />

                    {/* Search + sort bar */}
                    <div className="flex items-center gap-2 mb-3">
                      <div className="relative flex-1">
                        <input
                          type="text"
                          value={patternSearch}
                          onChange={(e) => setPatternSearch(e.target.value)}
                          placeholder="Search patterns… (e.g. GET /api *)"
                          className="w-full bg-slate-800 border border-slate-700 text-slate-200 text-xs rounded px-3 py-1.5 placeholder-slate-500 focus:outline-none focus:ring-1 focus:ring-teal-500 font-mono"
                        />
                        {patternSearch && (
                          <button
                            onClick={() => setPatternSearch('')}
                            className="absolute right-2 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-300 text-sm leading-none"
                          >
                            ×
                          </button>
                        )}
                      </div>
                      <select
                        value={patternSortBy}
                        onChange={(e) => setPatternSortBy(e.target.value as 'count' | 'pattern')}
                        className="bg-slate-800 border border-slate-700 text-slate-300 text-xs rounded px-2 py-1.5 focus:outline-none focus:ring-1 focus:ring-teal-500"
                      >
                        <option value="count">Sort: count ↓</option>
                        <option value="pattern">Sort: pattern A–Z</option>
                      </select>
                    </div>

                    {/* Scrollable pattern table with sticky header */}
                    <div className="rounded border border-slate-700/60 overflow-hidden">
                      <div className="overflow-y-auto max-h-[400px]">
                        <table className="w-full text-xs border-collapse">
                          <thead className="sticky top-0 z-10">
                            <tr className="border-b border-slate-700 bg-slate-800">
                              <th className="text-left py-2 px-3 text-slate-400 font-medium">Pattern</th>
                              <th className="text-right py-2 px-3 text-slate-400 font-medium w-20">Count</th>
                              <th className="text-left py-2 px-3 text-slate-400 font-medium hidden md:table-cell">Example</th>
                            </tr>
                          </thead>
                          <tbody>
                            {filteredPatterns.length === 0 ? (
                              <tr>
                                <td colSpan={3} className="py-4 text-center text-slate-500">
                                  No patterns match "{patternSearch}"
                                </td>
                              </tr>
                            ) : (
                              filteredPatterns.map((p) => {
                                const isSelected = selectedPatternHash === p.hash;
                                const isFiltered = activePatternFilter === p.hash;
                                return (
                                  <Fragment key={p.hash}>
                                    {/* Pattern row */}
                                    <tr
                                      ref={(el) => {
                                        if (el) patternRowRefs.current.set(p.hash, el);
                                        else patternRowRefs.current.delete(p.hash);
                                      }}
                                      onClick={() => setSelectedPatternHash(isSelected ? null : p.hash)}
                                      className={`border-b border-slate-800/60 cursor-pointer hover:bg-slate-700/30 transition-colors ${
                                        isSelected
                                          ? 'bg-teal-900/20 border-teal-700/30'
                                          : isFiltered
                                            ? 'bg-teal-900/10'
                                            : ''
                                      }`}
                                    >
                                      <td className="py-2 px-3 font-mono text-teal-300">
                                        <div className="flex items-center gap-1.5">
                                          <span className="text-slate-600 text-[10px]">{isSelected ? '▼' : '▶'}</span>
                                          <span title={p.patternString} className="truncate max-w-xs">{p.patternString}</span>
                                          {isFiltered && (
                                            <span className="flex-shrink-0 text-[9px] px-1 py-0.5 rounded bg-teal-600/30 text-teal-400">filtered</span>
                                          )}
                                        </div>
                                      </td>
                                      <td className="py-2 px-3 text-right text-slate-300 tabular-nums">
                                        {p.count.toLocaleString()}
                                      </td>
                                      <td className="py-2 px-3 text-slate-500 hidden md:table-cell">
                                        <span title={p.exampleLog} className="block truncate max-w-xs">{p.exampleLog}</span>
                                      </td>
                                    </tr>

                                    {/* Inline detail row — shown when selected */}
                                    {isSelected && (
                                      <tr className="border-b border-teal-700/20">
                                        <td colSpan={3} className="px-4 py-3 bg-slate-800/70">
                                          <div className="flex items-center gap-3 mb-2">
                                            <div className="flex-1 min-w-0">
                                              <div className="text-[10px] text-slate-500 uppercase tracking-wide mb-1">Pattern</div>
                                              <pre className="text-xs text-slate-200 font-mono bg-slate-900/60 rounded px-2 py-1 whitespace-pre-wrap break-all">
                                                {p.patternString}
                                              </pre>
                                            </div>
                                            <div className="flex-shrink-0 text-right">
                                              <div className="text-[10px] text-slate-500 uppercase tracking-wide mb-1">ID</div>
                                              <code className="text-xs text-slate-400 font-mono bg-slate-900/60 rounded px-2 py-1">{p.hash}</code>
                                            </div>
                                          </div>
                                          <div className="flex items-start justify-between gap-3 mb-2">
                                            {p.exampleLog && (
                                              <div className="flex-1 min-w-0">
                                                <div className="text-[10px] text-slate-500 uppercase tracking-wide mb-1">Example</div>
                                                <pre className="text-xs text-slate-400 font-mono bg-slate-900/60 rounded px-2 py-1 whitespace-pre-wrap break-all max-h-12 overflow-y-auto">
                                                  {p.exampleLog}
                                                </pre>
                                              </div>
                                            )}
                                            <div className="flex flex-col gap-1.5 flex-shrink-0">
                                              <button
                                                onClick={(e) => {
                                                  e.stopPropagation();
                                                  if (isFiltered) {
                                                    setActivePatternFilter(null);
                                                  } else {
                                                    setActivePatternFilter(p.hash);
                                                    setLogPage(1);
                                                  }
                                                }}
                                                className={`text-xs px-2.5 py-1.5 rounded transition-colors ${
                                                  isFiltered
                                                    ? 'bg-teal-600 text-white hover:bg-teal-700'
                                                    : 'bg-slate-700 text-slate-300 hover:bg-slate-600'
                                                }`}
                                              >
                                                {isFiltered ? '✕ Clear filter' : '↓ Filter logs'}
                                              </button>
                                              {onJumpToSeries && (p.seriesIDs ?? []).length > 0 && (
                                                <button
                                                  onClick={(e) => {
                                                    e.stopPropagation();
                                                    onJumpToSeries(`parquet/_virtual.log.log_pattern_extractor.${p.hash}.count`);
                                                  }}
                                                  className="text-xs px-2.5 py-1.5 rounded bg-slate-700 text-slate-300 hover:bg-slate-600 transition-colors"
                                                  title="Open this series in the Time Series tab"
                                                >
                                                  ↗ View in metrics
                                                </button>
                                              )}
                                            </div>
                                          </div>
                                          <div className="text-[10px] text-slate-500 uppercase tracking-wide mb-1">
                                            Count over time
                                            {(p.seriesIDs ?? []).length > 1 && (
                                              <span className="ml-1 normal-case text-slate-600">({p.seriesIDs.length} tag combinations)</span>
                                            )}
                                          </div>
                                          <PatternCountChart
                                            seriesIDs={p.seriesIDs ?? []}
                                            timeRange={timeRange}
                                            scenarioStart={scenarioStart}
                                            scenarioEnd={scenarioEnd}
                                            phaseMarkers={phaseMarkers}
                                          />
                                        </td>
                                      </tr>
                                    )}
                                  </Fragment>
                                );
                              })
                            )}
                          </tbody>
                        </table>
                      </div>
                    </div>
                  </>
                )
              )}
            </div>
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
