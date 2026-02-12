import { useRef, useEffect, useMemo, useState } from 'react';
import * as d3 from 'd3';
import type { Anomaly, CompressedGroup, SeriesID } from '../api/client';

// Reuse the analyzer palette from TimeSeriesChart
const ANALYZER_PALETTE = [
  '#ef4444', // red
  '#3b82f6', // blue
  '#22c55e', // green
  '#f59e0b', // amber
  '#a855f7', // purple
  '#06b6d4', // cyan
];

const analyzerColorCache = new Map<string, string>();
let nextIndex = 0;

function getAnalyzerColor(name: string): string {
  if (!analyzerColorCache.has(name)) {
    analyzerColorCache.set(name, ANALYZER_PALETTE[nextIndex % ANALYZER_PALETTE.length]);
    nextIndex++;
  }
  return analyzerColorCache.get(name)!;
}

// Palette for group annotation bars (distinct from analyzer colors)
const GROUP_PALETTE = [
  '#8b5cf6', // violet
  '#06b6d4', // cyan
  '#f59e0b', // amber
  '#ec4899', // pink
  '#10b981', // emerald
  '#f97316', // orange
];

const DENSE_ROW_HEIGHT = 3;
const ANNOTATION_BAR_HEIGHT = 14;
const ANNOTATION_GAP = 2;
const ANNOTATION_SECTION_GAP = 8;

interface AnomalySwimlaneProps {
  anomalies: Anomaly[];
  compressedGroups: CompressedGroup[];
}

// Assign groups to stacked lanes so overlapping time ranges don't collide.
function assignLanes(groups: CompressedGroup[]): { group: CompressedGroup; lane: number }[] {
  const sorted = [...groups]
    .filter((g) => g.firstSeen != null && g.lastUpdated != null && g.firstSeen > 0)
    .sort((a, b) => (a.firstSeen ?? 0) - (b.firstSeen ?? 0));

  const laneEnds: number[] = []; // tracks the end time of the last bar in each lane
  return sorted.map((group) => {
    const start = group.firstSeen ?? 0;
    // Find the first lane where this group fits without overlap
    let lane = laneEnds.findIndex((end) => start > end);
    if (lane === -1) {
      lane = laneEnds.length;
      laneEnds.push(0);
    }
    laneEnds[lane] = group.lastUpdated ?? 0;
    return { group, lane };
  });
}

export function AnomalySwimlane({
  anomalies,
  compressedGroups,
}: AnomalySwimlaneProps) {
  const svgRef = useRef<SVGSVGElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [tooltip, setTooltip] = useState<{ x: number; y: number; text: string } | null>(null);
  const [containerWidth, setContainerWidth] = useState(0);

  // Compute unique sorted source labels
  const sources = useMemo(() => {
    const set = new Set(anomalies.map((a) => a.source));
    return Array.from(set).sort();
  }, [anomalies]);

  // Assign groups to stacked annotation lanes
  const laneAssignments = useMemo(() => assignLanes(compressedGroups), [compressedGroups]);
  const laneCount = laneAssignments.length > 0
    ? Math.max(...laneAssignments.map((a) => a.lane)) + 1
    : 0;

  // Layout dimensions
  const denseHeight = sources.length * DENSE_ROW_HEIGHT;
  const annotationHeight = laneCount > 0
    ? laneCount * (ANNOTATION_BAR_HEIGHT + ANNOTATION_GAP) - ANNOTATION_GAP
    : 0;
  const totalInnerHeight = denseHeight
    + (annotationHeight > 0 ? ANNOTATION_SECTION_GAP + annotationHeight : 0);

  const margin = { top: 10, right: 20, bottom: 30, left: 12 };

  useEffect(() => {
    if (!svgRef.current || !containerRef.current || anomalies.length === 0 || containerWidth === 0) return;

    const width = containerRef.current.clientWidth || containerWidth;
    const innerWidth = width - margin.left - margin.right;
    const totalHeight = totalInnerHeight + margin.top + margin.bottom;

    d3.select(svgRef.current).selectAll('*').remove();

    const svg = d3
      .select(svgRef.current)
      .attr('width', width)
      .attr('height', totalHeight);

    const g = svg.append('g').attr('transform', `translate(${margin.left},${margin.top})`);

    // Build source -> row y lookup
    const rowY = new Map<string, number>();
    sources.forEach((source, i) => {
      rowY.set(source, i * DENSE_ROW_HEIGHT);
    });

    // Time scale
    const [tMin, tMax] = d3.extent(anomalies, (a) => a.timestamp) as [number, number];
    const xScale = d3.scaleTime()
      .domain([tMin * 1000, tMax * 1000])
      .range([0, innerWidth]);

    // Precompute group coverage: for each group, store members set + time range
    const groupCoverage = compressedGroups
      .filter((g) => g.firstSeen != null && g.lastUpdated != null && g.firstSeen > 0)
      .map((g) => ({
        members: new Set(g.memberSources),
        t0: g.firstSeen!,
        t1: g.lastUpdated!,
      }));

    const isCovered = (sid: string | undefined, ts: number): boolean => {
      if (!sid) return false;
      for (const g of groupCoverage) {
        if (ts >= g.t0 && ts <= g.t1 && g.members.has(sid as SeriesID)) return true;
      }
      return false;
    };

    // Draw anomaly marks (always dense), tagged for hover highlighting
    for (const anomaly of anomalies) {
      const y = rowY.get(anomaly.source);
      if (y == null) continue;
      const x = xScale(anomaly.timestamp * 1000);
      const color = getAnalyzerColor(anomaly.analyzerName);
      const covered = isCovered(anomaly.sourceSeriesId, anomaly.timestamp);

      const baseOpacity = covered ? 0.3 : 0.9;
      g.append('rect')
        .attr('class', 'anomaly-dot')
        .attr('data-sid', anomaly.sourceSeriesId ?? '')
        .attr('data-ts', anomaly.timestamp)
        .attr('data-base-opacity', baseOpacity)
        .attr('x', x - 1.5)
        .attr('y', y)
        .attr('width', 3)
        .attr('height', DENSE_ROW_HEIGHT)
        .attr('fill', color)
        .attr('opacity', baseOpacity)
        .attr('rx', 0.5);
    }

    // Invisible hover targets for dense rows (tooltip on hover)
    for (const [source, y] of rowY) {
      g.append('rect')
        .attr('x', 0)
        .attr('y', y)
        .attr('width', innerWidth)
        .attr('height', DENSE_ROW_HEIGHT)
        .attr('fill', 'transparent')
        .style('cursor', 'default')
        .on('mouseenter', (event: MouseEvent) => {
          const containerRect = containerRef.current?.getBoundingClientRect();
          if (containerRect) {
            setTooltip({
              x: event.clientX - containerRect.left,
              y: y + margin.top + DENSE_ROW_HEIGHT,
              text: source,
            });
          }
        })
        .on('mouseleave', () => setTooltip(null));
    }

    // Separator between dense rows and annotation lane
    if (laneCount > 0) {
      g.append('line')
        .attr('x1', 0)
        .attr('x2', innerWidth)
        .attr('y1', denseHeight + ANNOTATION_SECTION_GAP / 2)
        .attr('y2', denseHeight + ANNOTATION_SECTION_GAP / 2)
        .attr('stroke', '#475569')
        .attr('stroke-width', 0.5)
        .attr('stroke-dasharray', '4,3');
    }

    // Helpers to highlight/reset anomaly dots on group hover
    const highlightGroup = (group: CompressedGroup) => {
      const members = new Set(group.memberSources);
      const t0 = group.firstSeen ?? 0;
      const t1 = group.lastUpdated ?? Infinity;
      g.selectAll('.anomaly-dot').each(function () {
        const el = d3.select(this);
        const sid = el.attr('data-sid');
        const ts = Number(el.attr('data-ts'));
        const hit = members.has(sid as SeriesID) && ts >= t0 && ts <= t1;
        el.attr('opacity', hit ? 1 : 0.1);
      });
    };
    const resetHighlight = () => {
      g.selectAll('.anomaly-dot').each(function () {
        const el = d3.select(this);
        el.attr('opacity', el.attr('data-base-opacity'));
      });
    };

    // Draw annotation lane bars
    const annotationTop = denseHeight + ANNOTATION_SECTION_GAP;
    for (let i = 0; i < laneAssignments.length; i++) {
      const { group, lane } = laneAssignments[i];
      const color = GROUP_PALETTE[i % GROUP_PALETTE.length];
      const barY = annotationTop + lane * (ANNOTATION_BAR_HEIGHT + ANNOTATION_GAP);
      const x1 = xScale((group.firstSeen ?? 0) * 1000);
      const x2 = xScale((group.lastUpdated ?? 0) * 1000);
      // Ensure minimum visible width
      const barWidth = Math.max(x2 - x1, 4);

      g.append('rect')
        .attr('x', x1)
        .attr('y', barY)
        .attr('width', barWidth)
        .attr('height', ANNOTATION_BAR_HEIGHT)
        .attr('fill', color + '33')
        .attr('stroke', color)
        .attr('stroke-width', 1)
        .attr('rx', 3);

      // Label inside the bar (truncated if too narrow)
      const label = group.title || group.groupId;
      const labelWidth = barWidth - 6;
      if (labelWidth > 20) {
        g.append('text')
          .attr('x', x1 + 4)
          .attr('y', barY + ANNOTATION_BAR_HEIGHT / 2)
          .attr('dominant-baseline', 'central')
          .attr('fill', color)
          .attr('font-size', '9px')
          .attr('font-family', 'monospace')
          .attr('pointer-events', 'none')
          .text(label)
          .each(function () {
            // Truncate text to fit bar width
            const node = this as SVGTextElement;
            let text = label;
            while (node.getComputedTextLength() > labelWidth && text.length > 0) {
              text = text.slice(0, -1);
              node.textContent = text + '\u2026';
            }
          });
      }

      // Tooltip + highlight on hover for the bar
      g.append('rect')
        .attr('x', x1)
        .attr('y', barY)
        .attr('width', barWidth)
        .attr('height', ANNOTATION_BAR_HEIGHT)
        .attr('fill', 'transparent')
        .style('cursor', 'default')
        .on('mouseenter', (event: MouseEvent) => {
          highlightGroup(group);
          const containerRect = containerRef.current?.getBoundingClientRect();
          if (containerRect) {
            const span = (group.lastUpdated ?? 0) - (group.firstSeen ?? 0);
            setTooltip({
              x: event.clientX - containerRect.left,
              y: barY + margin.top + ANNOTATION_BAR_HEIGHT,
              text: `${label} (${group.seriesCount} series, ${span}s)`,
            });
          }
        })
        .on('mouseleave', () => {
          resetHighlight();
          setTooltip(null);
        });
    }

    // X axis
    g.append('g')
      .attr('transform', `translate(0,${totalInnerHeight})`)
      .call(
        d3.axisBottom(xScale)
          .ticks(6)
          .tickFormat((d) => d3.timeFormat('%H:%M:%S')(d as Date))
      )
      .attr('color', '#64748b')
      .selectAll('text')
      .attr('fill', '#94a3b8')
      .attr('font-size', '9px');

  }, [anomalies, sources, denseHeight, laneAssignments, laneCount, totalInnerHeight, containerWidth, margin.left, margin.right, margin.top, margin.bottom]);

  // Track container width so the drawing effect re-runs on resize/tab switch
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const observer = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect.width ?? 0;
      if (w > 0) setContainerWidth(w);
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  if (anomalies.length === 0) {
    return (
      <div className="bg-slate-800 rounded-lg p-4">
        <div className="text-sm text-slate-500 text-center py-4">No anomalies to display</div>
      </div>
    );
  }

  // Analyzer legend
  const analyzerNames = Array.from(new Set(anomalies.map((a) => a.analyzerName)));

  return (
    <div className="bg-slate-800 rounded-lg p-4">
      <div className="flex items-center justify-between mb-2">
        <h2 className="text-sm font-semibold text-slate-300">
          Anomaly Swimlane ({anomalies.length} anomalies across {sources.length} sources)
        </h2>
        <div className="flex gap-2">
          {analyzerNames.map((name) => (
            <span
              key={name}
              className="text-[10px] px-1.5 py-0.5 rounded flex items-center gap-1"
              style={{ backgroundColor: getAnalyzerColor(name) + '33', color: getAnalyzerColor(name) }}
            >
              <span className="w-2 h-2 rounded-full" style={{ backgroundColor: getAnalyzerColor(name) }} />
              {name}
            </span>
          ))}
        </div>
      </div>
      <div
        ref={containerRef}
        className="w-full relative"
      >
        <svg ref={svgRef} />
        {tooltip && (
          <div
            className="absolute z-10 px-2 py-1 bg-slate-900 border border-slate-600 rounded text-[10px] text-slate-300 font-mono pointer-events-none whitespace-nowrap"
            style={{ left: tooltip.x, top: tooltip.y + 4 }}
          >
            {tooltip.text}
          </div>
        )}
      </div>
    </div>
  );
}
