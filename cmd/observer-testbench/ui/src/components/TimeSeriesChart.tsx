import { useRef, useEffect, useMemo, useState } from 'react';
import * as d3 from 'd3';
import type { Point, AnomalyMarker, SeriesID } from '../api/client';
import type { CorrelationRange, TimeRange } from './ChartWithAnomalyDetails';

// Analyzer color palette - colors are assigned by stable index
const ANALYZER_PALETTE: { fill: string; stroke: string }[] = [
  { fill: 'rgba(239, 68, 68, 0.2)', stroke: '#ef4444' },    // red
  { fill: 'rgba(59, 130, 246, 0.2)', stroke: '#3b82f6' },    // blue
  { fill: 'rgba(34, 197, 94, 0.2)', stroke: '#22c55e' },     // green
  { fill: 'rgba(251, 191, 36, 0.2)', stroke: '#f59e0b' },    // amber
  { fill: 'rgba(168, 85, 247, 0.2)', stroke: '#a855f7' },    // purple
  { fill: 'rgba(6, 182, 212, 0.2)', stroke: '#06b6d4' },     // cyan
];

// Build a stable name-to-color mapping from palette
const analyzerColorCache = new Map<string, { fill: string; stroke: string }>();
let nextAnalyzerColorIndex = 0;

function getAnalyzerColorStable(analyzerName: string) {
  if (!analyzerColorCache.has(analyzerName)) {
    analyzerColorCache.set(analyzerName, ANALYZER_PALETTE[nextAnalyzerColorIndex % ANALYZER_PALETTE.length]);
    nextAnalyzerColorIndex++;
  }
  return analyzerColorCache.get(analyzerName)!;
}

// Correlation range colors - distinct from analyzer colors
const CORRELATION_COLORS = [
  { fill: 'rgba(16, 185, 129, 0.15)', stroke: '#10b981' },   // emerald
  { fill: 'rgba(245, 158, 11, 0.15)', stroke: '#f59e0b' },   // amber
  { fill: 'rgba(236, 72, 153, 0.15)', stroke: '#ec4899' },   // pink
  { fill: 'rgba(6, 182, 212, 0.15)', stroke: '#06b6d4' },    // cyan
  { fill: 'rgba(139, 92, 246, 0.15)', stroke: '#8b5cf6' },   // violet
];

// Line colors for per-series variants - distinct, vibrant colors
const LINE_COLORS = [
  '#8b5cf6', // violet (default/primary)
  '#f59e0b', // amber
  '#10b981', // emerald
  '#ec4899', // pink
  '#06b6d4', // cyan
  '#ef4444', // red
  '#84cc16', // lime
  '#f97316', // orange
  '#6366f1', // indigo
  '#14b8a6', // teal
];

function getAnalyzerColor(analyzerName: string) {
  return getAnalyzerColorStable(analyzerName);
}

function getCorrelationColor(index: number) {
  return CORRELATION_COLORS[index % CORRELATION_COLORS.length];
}

export function getSeriesVariantColor(index: number) {
  return LINE_COLORS[index % LINE_COLORS.length];
}

function getAnomalyMarkerId(anomaly: AnomalyMarker): string {
  const analyzerId = anomaly.analyzerComponent ?? anomaly.analyzerName;
  return `${analyzerId}:${anomaly.sourceSeriesId ?? 'unknown'}:${anomaly.timestamp}:${anomaly.title}`;
}

// Represents one concrete tagged series variant drawn as a separate line.
export interface SeriesVariant {
  label: string;  // The tag value (e.g., "host:web1")
  points: Point[];
  seriesId?: SeriesID;
}

interface TimeSeriesChartProps {
  name: string;
  points: Point[];
  anomalies: AnomalyMarker[];
  correlationRanges?: CorrelationRange[];
  enabledAnalyzers: Set<string>;
  timeRange?: TimeRange | null;
  onTimeRangeChange?: (range: TimeRange | null) => void;
  height?: number;
  smoothLines?: boolean;
  seriesVariants?: SeriesVariant[];  // When provided, renders multiple lines instead of single points array
  visibleSeriesIds?: Set<SeriesID>;
  onToggleSeriesVisibility?: (seriesId: SeriesID) => void;
  highlightedSeriesId?: SeriesID | null;
  highlightedMarkerId?: string | null;
  onMarkerHover?: (markerId: string | null) => void;
  onMarkerClick?: (markerId: string) => void;
}

export function TimeSeriesChart({
  name,
  points,
  anomalies,
  correlationRanges = [],
  enabledAnalyzers,
  timeRange,
  onTimeRangeChange,
  height = 200,
  smoothLines = true,
  seriesVariants,
  visibleSeriesIds,
  onToggleSeriesVisibility,
  highlightedSeriesId = null,
  highlightedMarkerId = null,
  onMarkerHover,
  onMarkerClick,
}: TimeSeriesChartProps) {
  const [showCorrelationLegend, setShowCorrelationLegend] = useState(false);
  const [showSeriesLegend, setShowSeriesLegend] = useState(false);
  const [hoveredLegendSeriesId, setHoveredLegendSeriesId] = useState<SeriesID | null>(null);
  const svgRef = useRef<SVGSVGElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const isBrushingRef = useRef(false);

  // Panning state refs
  const isPanningRef = useRef(false);
  const panStartXRef = useRef(0);
  const panStartRangeRef = useRef<TimeRange | null>(null);
  const panTriggerRef = useRef<'middle' | 'meta-left' | null>(null);
  const xScaleRef = useRef<d3.ScaleTime<number, number> | null>(null);

  const isSeriesVisible = (seriesId?: SeriesID): boolean => {
    if (!seriesId || !visibleSeriesIds) return true;
    return visibleSeriesIds.has(seriesId);
  };

  // Filter anomalies by enabled analyzers (and visible series variants when present)
  const filteredAnomalies = useMemo(
    () =>
      (anomalies ?? []).filter((a): a is AnomalyMarker => {
        if (!a) return false;
        const analyzerID = a.analyzerComponent ?? a.analyzerName;
        return !!analyzerID && enabledAnalyzers.has(analyzerID) && isSeriesVisible(a.sourceSeriesId);
      }),
    [anomalies, enabledAnalyzers, visibleSeriesIds]
  );

  // Filter points by time range
  const displayPoints = useMemo(() => {
    if (!timeRange) return points;
    return points.filter((p) => p.timestamp >= timeRange.start && p.timestamp <= timeRange.end);
  }, [points, timeRange]);

  // Filter variant series points by time range
  const displaySeriesVariants = useMemo(() => {
    if (!seriesVariants) return undefined;
    if (!timeRange) return seriesVariants;
    return seriesVariants.map((s) => ({
      ...s,
      points: s.points.filter((p) => p.timestamp >= timeRange.start && p.timestamp <= timeRange.end),
    }));
  }, [seriesVariants, timeRange]);

  // Keep a stable variant order so legend positions/colors don't shift when toggling visibility.
  const orderedSeriesVariants = useMemo(() => {
    if (!displaySeriesVariants) return undefined;
    return [...displaySeriesVariants].sort((a, b) => {
      const labelCmp = a.label.localeCompare(b.label);
      if (labelCmp !== 0) return labelCmp;
      return (a.seriesId ?? '').localeCompare(b.seriesId ?? '');
    });
  }, [displaySeriesVariants]);

  const visibleOrderedSeriesVariants = useMemo(() => {
    if (!orderedSeriesVariants) return undefined;
    return orderedSeriesVariants.filter((s) => isSeriesVisible(s.seriesId));
  }, [orderedSeriesVariants, visibleSeriesIds]);
  const activeHighlightedSeriesId = hoveredLegendSeriesId ?? highlightedSeriesId;

  const setAllSeriesVisibility = (visible: boolean) => {
    if (!orderedSeriesVariants || !onToggleSeriesVisibility) return;
    orderedSeriesVariants.forEach((series) => {
      if (!series.seriesId) return;
      const currentlyVisible = isSeriesVisible(series.seriesId);
      if (currentlyVisible !== visible) {
        onToggleSeriesVisibility(series.seriesId);
      }
    });
  };

  // Stable callback ref for brush
  const onTimeRangeChangeRef = useRef(onTimeRangeChange);
  onTimeRangeChangeRef.current = onTimeRangeChange;

  useEffect(() => {
    const variantMode = !!(orderedSeriesVariants && orderedSeriesVariants.length > 0);
    if (!svgRef.current || !containerRef.current) return;

    // Skip redraw if user is currently brushing to prevent visual disruption
    // (Panning should update live, so we don't skip for that)
    if (isBrushingRef.current) return;

    const container = containerRef.current;
    const width = container.clientWidth;
    const margin = { top: 20, right: 20, bottom: 30, left: 50 };
    const innerWidth = width - margin.left - margin.right;
    const innerHeight = height - margin.top - margin.bottom;

    // Clear previous content
    d3.select(svgRef.current).selectAll('*').remove();

    const svg = d3
      .select(svgRef.current)
      .attr('width', width)
      .attr('height', height);

    const g = svg.append('g').attr('transform', `translate(${margin.left},${margin.top})`);

    // Determine which data to use for scales and rendering
    const useVariantData = variantMode;
    const pointsToRender = displayPoints.length > 0 ? displayPoints : points;
    const allVariantPoints = orderedSeriesVariants?.flatMap((s) => s.points) ?? [];
    const visibleVariantPoints = visibleOrderedSeriesVariants?.flatMap((s) => s.points) ?? [];

    // Calculate extents - when using series variants, combine all visible variants for scaling
    let xExtent: [number, number];
    let yExtent: [number, number];

    if (useVariantData) {
      const scalePoints = visibleVariantPoints.length > 0
        ? visibleVariantPoints
        : allVariantPoints.length > 0
          ? allVariantPoints
          : pointsToRender;
      xExtent = d3.extent(scalePoints, (d) => d.timestamp * 1000) as [number, number];
      yExtent = d3.extent(scalePoints, (d) => d.value) as [number, number];
    } else {
      xExtent = d3.extent(pointsToRender, (d) => d.timestamp * 1000) as [number, number];
      yExtent = d3.extent(pointsToRender, (d) => d.value) as [number, number];
    }

    if (xExtent?.[0] === undefined || xExtent?.[1] === undefined || yExtent?.[0] === undefined || yExtent?.[1] === undefined) {
      return;
    }

    // Add padding to y extent
    const yPadding = (yExtent[1] - yExtent[0]) * 0.1 || 1;

    const xScale = d3.scaleTime().domain(xExtent).range([0, innerWidth]);
    xScaleRef.current = xScale;

    const yScale = d3
      .scaleLinear()
      .domain([yExtent[0] - yPadding, yExtent[1] + yPadding])
      .range([innerHeight, 0]);

    // Draw correlation ranges as subtle background shading
    correlationRanges.forEach((range) => {
      const color = getCorrelationColor(range.id);
      const x1 = xScale(range.start * 1000);
      const x2 = xScale(range.end * 1000);
      const rectWidth = Math.max(x2 - x1, 4); // Minimum 4px width for visibility

      g.append('rect')
        .attr('x', x1)
        .attr('y', 0)
        .attr('width', rectWidth)
        .attr('height', innerHeight)
        .attr('fill', color.fill)
        .attr('stroke', color.stroke)
        .attr('stroke-width', 1)
        .attr('stroke-dasharray', '4,2')
        .attr('opacity', 0.5);
    });

    // Group anomalies by timestamp to handle overlaps
    const anomaliesByTimestamp = new Map<number, typeof filteredAnomalies>();
    filteredAnomalies.forEach((anomaly) => {
      const existing = anomaliesByTimestamp.get(anomaly.timestamp) || [];
      existing.push(anomaly);
      anomaliesByTimestamp.set(anomaly.timestamp, existing);
    });

    type MarkerRenderDatum = {
      markerId: string;
      x: number;
      y: number;
      color: { fill: string; stroke: string };
      selected: boolean;
      sourceSeriesId?: SeriesID;
    };

    const markerData: MarkerRenderDatum[] = [];

  // Draw anomaly markers - simple circles at data points
    anomaliesByTimestamp.forEach((anomaliesAtTime, timestamp) => {
      const x = xScale(timestamp * 1000);

      const numAnomalies = anomaliesAtTime.length;

      anomaliesAtTime.forEach((anomaly, idx) => {
        let dataPoint: Point | undefined;
        if (useVariantData && visibleOrderedSeriesVariants && anomaly.sourceSeriesId) {
          const match = visibleOrderedSeriesVariants.find((s) => s.seriesId === anomaly.sourceSeriesId);
          dataPoint = match?.points.find((p) => p.timestamp === timestamp);
        }
        if (!dataPoint && !useVariantData) {
          dataPoint = points.find((p) => p.timestamp === timestamp);
        }
        if (!dataPoint) return;

        const baseY = yScale(dataPoint.value);
        const color = getAnalyzerColor(anomaly.analyzerName);
        const xOffset = numAnomalies > 1 ? (idx - (numAnomalies - 1) / 2) * 8 : 0;
        const markerId = getAnomalyMarkerId(anomaly);
        markerData.push({
          markerId,
          x: x + xOffset,
          y: baseY,
          color,
          selected: markerId === highlightedMarkerId,
          sourceSeriesId: anomaly.sourceSeriesId,
        });
      });
    });

    const markerSelection = g
      .append('g')
      .attr('class', 'anomaly-markers')
      .selectAll('circle')
      .data(markerData)
      .enter()
      .append('circle')
      .attr('cx', (d) => d.x)
      .attr('cy', (d) => d.y)
      .attr('r', (d) => (d.selected ? 6 : 4))
      .attr('fill', (d) => d.color.stroke)
      .attr('stroke', (d) => (d.selected ? '#f8fafc' : '#1e293b'))
      .attr('stroke-width', (d) => (d.selected ? 2.5 : 1.5))
      .attr('opacity', (d) => {
        let opacity = 1;
        if (activeHighlightedSeriesId && d.sourceSeriesId && d.sourceSeriesId !== activeHighlightedSeriesId) {
          opacity = 0.25;
        }
        if (highlightedMarkerId && !d.selected) {
          opacity = Math.min(opacity, 0.45);
        }
        return opacity;
      })
      .style('cursor', 'pointer');

    markerSelection
      .on('mouseenter', (_event, d) => {
        if (onMarkerHover) onMarkerHover(d.markerId);
      })
      .on('mouseleave', () => {
        if (onMarkerHover) onMarkerHover(null);
      })
      .on('click', (_event, d) => {
        if (onMarkerClick) onMarkerClick(d.markerId);
      });

    // Line generator
    const line = d3
      .line<Point>()
      .x((d) => xScale(d.timestamp * 1000))
      .y((d) => yScale(d.value))
      .curve(smoothLines ? d3.curveMonotoneX : d3.curveLinear);

    // Draw the line(s)
    if (useVariantData && visibleOrderedSeriesVariants) {
      // Draw multiple lines for per-series variants
      visibleOrderedSeriesVariants.forEach((series) => {
        if (series.points.length === 0) return;
        const colorIdx = orderedSeriesVariants?.findIndex((s) => s.seriesId === series.seriesId && s.label === series.label) ?? 0;
        const color = getSeriesVariantColor(Math.max(colorIdx, 0));
        const isHighlighted = !activeHighlightedSeriesId || !series.seriesId || series.seriesId === activeHighlightedSeriesId;
        g.append('path')
          .datum(series.points)
          .attr('fill', 'none')
          .attr('stroke', color)
          .attr('stroke-width', isHighlighted ? 1.9 : 1.2)
          .attr('opacity', isHighlighted ? 1 : 0.2)
          .attr('d', line);
      });

    } else {
      // Draw single line
      g.append('path')
        .datum(pointsToRender)
        .attr('fill', 'none')
        .attr('stroke', '#8b5cf6')
        .attr('stroke-width', 1.5)
        .attr('d', line);
    }

    // X axis
    g.append('g')
      .attr('transform', `translate(0,${innerHeight})`)
      .call(
        d3
          .axisBottom(xScale)
          .ticks(6)
          .tickFormat((d) => d3.timeFormat('%H:%M:%S')(d as Date))
      )
      .attr('color', '#64748b')
      .selectAll('text')
      .attr('fill', '#94a3b8');

    // Y axis
    g.append('g')
      .call(d3.axisLeft(yScale).ticks(5))
      .attr('color', '#64748b')
      .selectAll('text')
      .attr('fill', '#94a3b8');

    // Grid lines
    g.append('g')
      .attr('class', 'grid')
      .attr('opacity', 0.1)
      .call(d3.axisLeft(yScale).ticks(5).tickSize(-innerWidth).tickFormat(() => ''));

    // Add brush for time range selection
    const brush = d3
      .brushX<unknown>()
      .extent([
        [0, 0],
        [innerWidth, innerHeight],
      ])
      // Keep cmd+left-drag free for panning (treated like middle-drag).
      .filter((event: MouseEvent) => event.button === 0 && !event.metaKey)
      .on('start', () => {
        isBrushingRef.current = true;
      })
      .on('end', (event: d3.D3BrushEvent<unknown>) => {
        isBrushingRef.current = false;

        if (!event.selection) return;
        const [x0, x1] = event.selection as [number, number];

        // Convert pixel positions back to timestamps (in seconds)
        const start = Math.floor(xScale.invert(x0).getTime() / 1000);
        const end = Math.floor(xScale.invert(x1).getTime() / 1000);

        // Clear the brush selection visually
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        g.select('.brush').call(brush.move as any, null);

        // Call the callback to set global time range
        if (onTimeRangeChangeRef.current && end > start) {
          onTimeRangeChangeRef.current({ start, end });
        }
      });

    // Append brush to chart
    g.append('g')
      .attr('class', 'brush')
      .call(brush)
      .selectAll('rect')
      .attr('rx', 2)
      .attr('ry', 2);

    // Style the brush selection
    g.select('.brush .selection')
      .attr('fill', 'rgba(139, 92, 246, 0.3)')
      .attr('stroke', '#8b5cf6')
      .attr('stroke-width', 1);

  }, [
    points,
    displayPoints,
    filteredAnomalies,
    correlationRanges,
    height,
    timeRange,
    smoothLines,
    orderedSeriesVariants,
    visibleOrderedSeriesVariants,
    highlightedMarkerId,
    activeHighlightedSeriesId,
    onMarkerHover,
    onMarkerClick,
  ]);

  // Handle resize
  useEffect(() => {
    const handleResize = () => {
      // Trigger re-render by updating a dummy state
      if (svgRef.current && containerRef.current) {
        const width = containerRef.current.clientWidth;
        d3.select(svgRef.current).attr('width', width);
      }
    };

    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  // Handle middle-click or cmd+left-click panning
  useEffect(() => {
    const svg = svgRef.current;
    if (!svg) return;

    const isMetaLeftPan = (e: MouseEvent) => e.button === 0 && e.metaKey;

    const handleMouseDown = (e: MouseEvent) => {
      const middlePan = e.button === 1;
      const metaLeftPan = isMetaLeftPan(e);
      if (!middlePan && !metaLeftPan) return;
      if (!timeRange) return; // Can only pan when zoomed

      e.preventDefault();
      isPanningRef.current = true;
      panTriggerRef.current = middlePan ? 'middle' : 'meta-left';
      panStartXRef.current = e.clientX;
      panStartRangeRef.current = { ...timeRange };
      svg.style.cursor = 'grabbing';
    };

    const handleMouseMove = (e: MouseEvent) => {
      if (!isPanningRef.current || !panStartRangeRef.current || !xScaleRef.current) return;

      const dx = e.clientX - panStartXRef.current;
      const xScale = xScaleRef.current;

      // Convert pixel delta to time delta
      // Negative dx (dragging left) should move time forward (increase start/end)
      const domain = xScale.domain();
      const range = xScale.range();
      const pixelsPerMs = (range[1] - range[0]) / (domain[1].getTime() - domain[0].getTime());
      const timeDeltaMs = -dx / pixelsPerMs;
      const timeDeltaSec = timeDeltaMs / 1000;

      const newStart = panStartRangeRef.current.start + timeDeltaSec;
      const newEnd = panStartRangeRef.current.end + timeDeltaSec;

      // Update the time range
      if (onTimeRangeChangeRef.current) {
        onTimeRangeChangeRef.current({ start: newStart, end: newEnd });
      }
    };

    const handleMouseUp = (e: MouseEvent) => {
      if (!isPanningRef.current) return;
      const trigger = panTriggerRef.current;
      const releasedMiddle = trigger === 'middle' && e.button === 1;
      const releasedMetaLeft = trigger === 'meta-left' && e.button === 0;
      if (!releasedMiddle && !releasedMetaLeft) return;
      if (isPanningRef.current) {
        isPanningRef.current = false;
        panTriggerRef.current = null;
        panStartRangeRef.current = null;
        svg.style.cursor = '';
      }
    };

    // Also handle mouse leaving the window
    const handleMouseLeave = () => {
      if (isPanningRef.current) {
        isPanningRef.current = false;
        panTriggerRef.current = null;
        panStartRangeRef.current = null;
        svg.style.cursor = '';
      }
    };

    svg.addEventListener('mousedown', handleMouseDown);
    window.addEventListener('mousemove', handleMouseMove);
    window.addEventListener('mouseup', handleMouseUp);
    window.addEventListener('blur', handleMouseLeave);

    // Prevent context menu on middle click
    const handleContextMenu = (e: MouseEvent) => {
      if (e.button === 1) e.preventDefault();
    };
    svg.addEventListener('auxclick', handleContextMenu);

    return () => {
      svg.removeEventListener('mousedown', handleMouseDown);
      window.removeEventListener('mousemove', handleMouseMove);
      window.removeEventListener('mouseup', handleMouseUp);
      window.removeEventListener('blur', handleMouseLeave);
      svg.removeEventListener('auxclick', handleContextMenu);
    };
  }, [timeRange]);

  if (points.length === 0) {
    return (
      <div className="bg-slate-800 rounded-lg p-4">
        <div className="text-sm text-slate-400 mb-2 font-mono">{name}</div>
        <div className="text-slate-500 text-center py-8">No data</div>
      </div>
    );
  }

  return (
    <div className="bg-slate-800 rounded-lg p-4">
      <div className="flex justify-between items-center mb-2 gap-2">
        <div className="text-sm text-slate-300 font-mono truncate">{name}</div>
        <div className="flex gap-2 items-center flex-shrink-0">
          {/* Detector legend - only show if there are anomalies */}
          {filteredAnomalies.length > 0 && Array.from(new Set(filteredAnomalies.map((a) => a.analyzerName))).map((analyzer) => {
            const color = getAnalyzerColor(analyzer);
            const displayName = analyzer;
            return (
              <span
                key={analyzer}
                className="text-[10px] px-1.5 py-0.5 rounded flex items-center gap-1"
                style={{ backgroundColor: color.fill, color: color.stroke }}
              >
                <span className="w-2 h-2 rounded-full" style={{ backgroundColor: color.stroke }} />
                {displayName}
              </span>
            );
          })}
          {/* Correlation count - clickable to expand */}
          {correlationRanges.length > 0 && (
            <button
              onClick={() => setShowCorrelationLegend(!showCorrelationLegend)}
              className="text-[10px] px-1.5 py-0.5 rounded bg-slate-700 text-slate-400 hover:bg-slate-600 flex items-center gap-1"
            >
              {correlationRanges.length} correlation{correlationRanges.length !== 1 ? 's' : ''}
              <span className="text-[8px]">{showCorrelationLegend ? '▲' : '▼'}</span>
            </button>
          )}
        </div>
      </div>
      {/* Expandable correlation legend */}
      {showCorrelationLegend && correlationRanges.length > 0 && (
        <div className="mb-2 p-2 bg-slate-900/50 rounded text-[10px] flex flex-wrap gap-2">
          {correlationRanges.map((range, i) => {
            const color = getCorrelationColor(range.id);
            return (
              <span
                key={i}
                className="flex items-center gap-1 px-1.5 py-0.5 rounded"
                style={{ backgroundColor: color.fill, border: `1px dashed ${color.stroke}` }}
              >
                <span
                  className="w-3 h-3 rounded-sm flex-shrink-0"
                  style={{ backgroundColor: color.fill, border: `1px dashed ${color.stroke}` }}
                />
                <span style={{ color: color.stroke }}>{range.title}</span>
              </span>
            );
          })}
        </div>
      )}
      <div ref={containerRef} className="w-full">
        <svg ref={svgRef} />
      </div>
      {orderedSeriesVariants && orderedSeriesVariants.length > 1 && (
        <div className="mt-2 flex items-center gap-1">
          <button
            onClick={() => setShowSeriesLegend(!showSeriesLegend)}
            className="text-[10px] px-1.5 py-0.5 rounded bg-slate-700/80 text-slate-400 hover:bg-slate-600 flex items-center gap-1"
          >
            {orderedSeriesVariants.length} series
            <span className="text-[8px]">{showSeriesLegend ? '▲' : '▼'}</span>
          </button>
          <button
            onClick={() => setAllSeriesVisibility(true)}
            className="text-[10px] px-1.5 py-0.5 rounded bg-slate-700/60 text-slate-400 hover:bg-slate-600"
            title="Show all series"
          >
            All
          </button>
          <button
            onClick={() => setAllSeriesVisibility(false)}
            className="text-[10px] px-1.5 py-0.5 rounded bg-slate-700/60 text-slate-400 hover:bg-slate-600"
            title="Hide all series"
          >
            None
          </button>
        </div>
      )}
      {showSeriesLegend && orderedSeriesVariants && orderedSeriesVariants.length > 1 && (
        <div className="mt-2 p-2 bg-slate-900/50 rounded text-[10px] flex flex-wrap gap-0">
          {orderedSeriesVariants.map((series, idx) => (
            <button
              key={`${series.seriesId ?? series.label}-${idx}`}
              type="button"
              className={`text-left flex items-center gap-1.5 rounded border border-slate-700/50 bg-slate-800/40 px-2 py-1 transition-colors ${
                series.seriesId ? 'hover:bg-slate-800/70' : 'cursor-not-allowed opacity-60'
              }`}
              disabled={!series.seriesId}
              onClick={() => {
                if (series.seriesId && onToggleSeriesVisibility) {
                  onToggleSeriesVisibility(series.seriesId);
                }
              }}
              onMouseEnter={() => setHoveredLegendSeriesId(series.seriesId ?? null)}
              onMouseLeave={() => setHoveredLegendSeriesId(null)}
            >
              <span
                className="w-3 h-0.5 rounded-full"
                style={{
                  backgroundColor: getSeriesVariantColor(idx),
                  opacity: isSeriesVisible(series.seriesId) ? 1 : 0.35,
                }}
              />
              <span
                className={`${isSeriesVisible(series.seriesId) ? 'text-slate-300' : 'text-slate-600 line-through'}`}
              >
                {series.label}
              </span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
