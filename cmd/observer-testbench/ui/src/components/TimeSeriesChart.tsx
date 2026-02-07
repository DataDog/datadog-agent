import { useRef, useEffect, useMemo, useState } from 'react';
import * as d3 from 'd3';
import type { Point, AnomalyMarker } from '../api/client';
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

// Line colors for split series - distinct, vibrant colors
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

function getLineColor(index: number) {
  return LINE_COLORS[index % LINE_COLORS.length];
}

// Represents a single series line when splitting by tag
export interface SplitSeries {
  label: string;  // The tag value (e.g., "host:web1")
  points: Point[];
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
  splitSeries?: SplitSeries[];  // When provided, renders multiple lines instead of single points array
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
  splitSeries,
}: TimeSeriesChartProps) {
  const svgRef = useRef<SVGSVGElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const isBrushingRef = useRef(false);

  // Panning state refs
  const isPanningRef = useRef(false);
  const panStartXRef = useRef(0);
  const panStartRangeRef = useRef<TimeRange | null>(null);
  const xScaleRef = useRef<d3.ScaleTime<number, number> | null>(null);

  // Filter anomalies by enabled analyzers
  const filteredAnomalies = useMemo(
    () => (anomalies ?? []).filter((a) => enabledAnalyzers.has(a.analyzerName)),
    [anomalies, enabledAnalyzers]
  );

  // Filter points by time range
  const displayPoints = useMemo(() => {
    if (!timeRange) return points;
    return points.filter((p) => p.timestamp >= timeRange.start && p.timestamp <= timeRange.end);
  }, [points, timeRange]);

  // Filter split series points by time range
  const displaySplitSeries = useMemo(() => {
    if (!splitSeries) return undefined;
    if (!timeRange) return splitSeries;
    return splitSeries.map((s) => ({
      ...s,
      points: s.points.filter((p) => p.timestamp >= timeRange.start && p.timestamp <= timeRange.end),
    }));
  }, [splitSeries, timeRange]);

  // Stable callback ref for brush
  const onTimeRangeChangeRef = useRef(onTimeRangeChange);
  onTimeRangeChangeRef.current = onTimeRangeChange;

  useEffect(() => {
    const hasSplitData = displaySplitSeries && displaySplitSeries.length > 0 && displaySplitSeries.some(s => s.points.length > 0);
    const hasMainData = points.length > 0;

    if (!svgRef.current || !containerRef.current || (!hasMainData && !hasSplitData)) return;

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
    const useSplitData = hasSplitData;
    const pointsToRender = displayPoints.length > 0 ? displayPoints : points;

    // Calculate extents - when split, combine all series for proper scaling
    let xExtent: [number, number];
    let yExtent: [number, number];

    if (useSplitData && displaySplitSeries) {
      const allPoints = displaySplitSeries.flatMap(s => s.points);
      xExtent = d3.extent(allPoints, (d) => d.timestamp * 1000) as [number, number];
      yExtent = d3.extent(allPoints, (d) => d.value) as [number, number];
    } else {
      xExtent = d3.extent(pointsToRender, (d) => d.timestamp * 1000) as [number, number];
      yExtent = d3.extent(pointsToRender, (d) => d.value) as [number, number];
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

    // Draw anomaly markers - simple circles at data points
    anomaliesByTimestamp.forEach((anomaliesAtTime, timestamp) => {
      const x = xScale(timestamp * 1000);
      const dataPoint = points.find((p) => p.timestamp === timestamp);

      if (dataPoint) {
        const baseY = yScale(dataPoint.value);
        const numAnomalies = anomaliesAtTime.length;

        anomaliesAtTime.forEach((anomaly, idx) => {
          const color = getAnalyzerColor(anomaly.analyzerName);
          const xOffset = numAnomalies > 1 ? (idx - (numAnomalies - 1) / 2) * 8 : 0;

          g.append('circle')
            .attr('cx', x + xOffset)
            .attr('cy', baseY)
            .attr('r', 4)
            .attr('fill', color.stroke)
            .attr('stroke', '#1e293b')
            .attr('stroke-width', 1.5);
        });
      }
    });

    // Line generator
    const line = d3
      .line<Point>()
      .x((d) => xScale(d.timestamp * 1000))
      .y((d) => yScale(d.value))
      .curve(smoothLines ? d3.curveMonotoneX : d3.curveLinear);

    // Draw the line(s)
    if (useSplitData && displaySplitSeries) {
      // Draw multiple lines for split series
      displaySplitSeries.forEach((series, idx) => {
        if (series.points.length === 0) return;
        const color = getLineColor(idx);
        g.append('path')
          .datum(series.points)
          .attr('fill', 'none')
          .attr('stroke', color)
          .attr('stroke-width', 1.5)
          .attr('d', line);
      });

      // Draw legend
      const legendX = innerWidth - 10;
      const legendY = 5;
      const legendSpacing = 14;

      displaySplitSeries.forEach((series, idx) => {
        if (series.points.length === 0) return;
        const color = getLineColor(idx);
        const y = legendY + idx * legendSpacing;

        // Color line
        g.append('line')
          .attr('x1', legendX - 35)
          .attr('x2', legendX - 20)
          .attr('y1', y)
          .attr('y2', y)
          .attr('stroke', color)
          .attr('stroke-width', 2);

        // Label
        g.append('text')
          .attr('x', legendX - 38)
          .attr('y', y + 3)
          .attr('text-anchor', 'end')
          .attr('fill', '#94a3b8')
          .attr('font-size', '9px')
          .text(series.label.length > 20 ? series.label.slice(0, 20) + '…' : series.label);
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

  }, [points, displayPoints, filteredAnomalies, correlationRanges, height, timeRange, smoothLines, displaySplitSeries]);

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

  // Handle middle-click panning
  useEffect(() => {
    const svg = svgRef.current;
    if (!svg) return;

    const handleMouseDown = (e: MouseEvent) => {
      // Middle mouse button is button 1
      if (e.button !== 1) return;
      if (!timeRange) return; // Can only pan when zoomed

      e.preventDefault();
      isPanningRef.current = true;
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
      if (e.button !== 1) return;
      if (isPanningRef.current) {
        isPanningRef.current = false;
        panStartRangeRef.current = null;
        svg.style.cursor = '';
      }
    };

    // Also handle mouse leaving the window
    const handleMouseLeave = () => {
      if (isPanningRef.current) {
        isPanningRef.current = false;
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

  const [showCorrelationLegend, setShowCorrelationLegend] = useState(false);

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
    </div>
  );
}
