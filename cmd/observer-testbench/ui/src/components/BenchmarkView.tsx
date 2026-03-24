import { useState, useEffect, useMemo, useRef } from 'react';
import * as d3 from 'd3';
import { api } from '../api/client';
import type { DetectorProcessingStats, ScenarioInfo } from '../api/client';
import type { ObserverState, ObserverActions } from '../hooks/useObserver';
import { TagFilterGroups } from './TagFilterGroups';
import { parseTagFilter, toggleTagInInput, matchesTagFilter } from '../filters';

// ── Chart constants ────────────────────────────────────────────────────────────

const MARGIN = { top: 44, right: 95, bottom: 20, left: 235 };
const BAR_H = 15;
const BAR_GAP = 3;
const GROUP_PAD = 16;
const GROUP_INNER_H = 3 * BAR_H + 2 * BAR_GAP; // 51 px
const GROUP_H = GROUP_INNER_H + GROUP_PAD;       // 67 px

const STATS: { key: 'avg' | 'p50' | 'p99'; label: string; color: string }[] = [
  { key: 'avg', label: 'avg', color: '#60a5fa' },  // blue-400
  { key: 'p50', label: 'p50', color: '#4ade80' },  // green-400
  { key: 'p99', label: 'p99', color: '#f87171' },  // red-400
];

function fmtUs(ns: number): string {
  const us = ns / 1000;
  if (us < 10) return us.toFixed(2) + 'µs';
  if (us < 100) return us.toFixed(1) + 'µs';
  return us.toFixed(0) + 'µs';
}

// ── Scenario selector (same as other views) ───────────────────────────────────

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
      <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">Scenarios</h2>
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

// ── Bar chart ─────────────────────────────────────────────────────────────────

interface BenchmarkBarChartProps {
  stats: DetectorProcessingStats[];
  highlighted: Set<string> | null; // null = all at full opacity
  onHover: (name: string | null) => void;
}

function BenchmarkBarChart({ stats, highlighted, onHover }: BenchmarkBarChartProps) {
  const svgRef = useRef<SVGSVGElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [containerWidth, setContainerWidth] = useState(0);
  const onHoverRef = useRef(onHover);
  onHoverRef.current = onHover;

  // Track container width via ResizeObserver
  useEffect(() => {
    if (!containerRef.current) return;
    const ro = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect.width;
      if (w) setContainerWidth(Math.floor(w));
    });
    ro.observe(containerRef.current);
    return () => ro.disconnect();
  }, []);

  useEffect(() => {
    if (!svgRef.current || !containerRef.current || stats.length === 0) {
      d3.select(svgRef.current).selectAll('*').remove();
      return;
    }

    const width = containerWidth || containerRef.current.clientWidth;
    const innerWidth = width - MARGIN.left - MARGIN.right;
    if (innerWidth <= 0) return;

    const innerHeight = stats.length * GROUP_H - GROUP_PAD;
    const totalHeight = innerHeight + MARGIN.top + MARGIN.bottom;

    d3.select(svgRef.current).selectAll('*').remove();
    const svg = d3.select(svgRef.current).attr('width', width).attr('height', totalHeight);

    // Legend (above the chart area, inside the left+chart space)
    const legendG = svg.append('g').attr('transform', `translate(${MARGIN.left}, 16)`);
    STATS.forEach(({ label, color }, i) => {
      const lx = i * 72;
      legendG.append('rect').attr('x', lx).attr('y', 0).attr('width', 10).attr('height', 10).attr('fill', color).attr('rx', 2);
      legendG
        .append('text')
        .attr('x', lx + 14)
        .attr('y', 9)
        .attr('fill', color)
        .attr('font-size', '10px')
        .attr('font-family', 'monospace')
        .text(label);
    });

    const g = svg.append('g').attr('transform', `translate(${MARGIN.left},${MARGIN.top})`);

    const maxNs = d3.max(stats, (s) => s.p99_ns) ?? 1;
    const xScale = d3.scaleLinear().domain([0, maxNs * 1.12]).range([0, innerWidth]);

    // Top axis — suppress the zero tick label to avoid overlap with stat key labels
    g.append('g')
      .attr('transform', 'translate(0,-8)')
      .call(
        d3
          .axisTop(xScale)
          .ticks(5)
          .tickFormat((d) => (+d === 0 ? '' : fmtUs(+d)))
      )
      .call((ax) => ax.select('.domain').attr('stroke', '#334155'))
      .call((ax) => ax.selectAll('text').attr('fill', '#64748b').attr('font-size', '10px'))
      .call((ax) => ax.selectAll('.tick line').attr('stroke', '#334155'));

    // Grid lines
    xScale.ticks(5).forEach((t) => {
      g.append('line')
        .attr('x1', xScale(t))
        .attr('x2', xScale(t))
        .attr('y1', 0)
        .attr('y2', innerHeight)
        .attr('stroke', '#1e293b')
        .attr('stroke-width', 1);
    });

    // Draw each detector group
    stats.forEach((stat, gi) => {
      const groupY = gi * GROUP_H;
      const isActive = highlighted === null || highlighted.has(stat.name);
      const opacity = isActive ? 1 : 0.15;

      const groupG = g
        .append('g')
        .attr('transform', `translate(0,${groupY})`)
        .attr('opacity', opacity)
        .style('cursor', 'default')
        .on('mouseenter', () => onHoverRef.current(stat.name))
        .on('mouseleave', () => onHoverRef.current(null));

      // Transparent hit zone spanning full row (including left margin)
      groupG
        .append('rect')
        .attr('x', -MARGIN.left)
        .attr('y', -2)
        .attr('width', MARGIN.left + innerWidth + MARGIN.right)
        .attr('height', GROUP_INNER_H + 4)
        .attr('fill', 'transparent');

      const vals: Record<string, number> = {
        avg: stat.avg_ns,
        p50: stat.median_ns,
        p99: stat.p99_ns,
      };

      STATS.forEach(({ key, color }, si) => {
        const barY = si * (BAR_H + BAR_GAP);
        const val = vals[key];
        const barW = Math.max(0, xScale(val));

        // Bar
        groupG
          .append('rect')
          .attr('x', 0)
          .attr('y', barY)
          .attr('width', barW)
          .attr('height', BAR_H)
          .attr('fill', color)
          .attr('rx', 2);

        // Stat key label — in the margin just before x=0
        groupG
          .append('text')
          .attr('x', -10)
          .attr('y', barY + BAR_H / 2)
          .attr('text-anchor', 'end')
          .attr('dominant-baseline', 'middle')
          .attr('fill', color)
          .attr('font-size', '9px')
          .attr('font-family', 'monospace')
          .text(key);

        // Value label at the end of the bar
        groupG
          .append('text')
          .attr('x', barW + 6)
          .attr('y', barY + BAR_H / 2)
          .attr('dominant-baseline', 'middle')
          .attr('fill', color)
          .attr('font-size', '10px')
          .attr('font-family', 'monospace')
          .text(fmtUs(val));
      });

      // Detector name — right-aligned in the wide left column
      const nameX = -60;
      const displayName =
        stat.name.length > 20 ? stat.name.slice(0, 19) + '…' : stat.name;
      groupG
        .append('text')
        .attr('x', nameX)
        .attr('y', GROUP_INNER_H / 2 - 7)
        .attr('text-anchor', 'end')
        .attr('dominant-baseline', 'middle')
        .attr('fill', isActive ? '#e2e8f0' : '#475569')
        .attr('font-size', '11px')
        .attr('font-family', 'monospace')
        .text(displayName);

      groupG
        .append('text')
        .attr('x', nameX)
        .attr('y', GROUP_INNER_H / 2 + 9)
        .attr('text-anchor', 'end')
        .attr('dominant-baseline', 'middle')
        .attr('fill', '#475569')
        .attr('font-size', '9px')
        .text(`${stat.count.toLocaleString()} calls`);
    });
  }, [stats, highlighted, containerWidth]);

  return (
    <div ref={containerRef} className="w-full">
      <svg ref={svgRef} style={{ display: 'block' }} />
    </div>
  );
}

// ── Main view ─────────────────────────────────────────────────────────────────

interface BenchmarkViewProps {
  state: ObserverState;
  actions: ObserverActions;
  sidebarWidth: number;
}

export function BenchmarkView({ state, actions, sidebarWidth }: BenchmarkViewProps) {
  const [rawStats, setRawStats] = useState<Record<string, DetectorProcessingStats> | null>(null);
  const [tagFilterInput, setTagFilterInput] = useState('');
  const [hoveredDetector, setHoveredDetector] = useState<string | null>(null);

  // Fetch when a scenario finishes loading
  useEffect(() => {
    if (state.connectionState !== 'ready') return;
    api
      .getBenchmarkStats()
      .then(setRawStats)
      .catch(() => setRawStats(null));
  }, [state.activeScenario, state.connectionState]);

  // Sort by p99 descending so the slowest detectors are at the top
  const stats = useMemo(() => {
    if (!rawStats) return [];
    return Object.values(rawStats).sort((a, b) => b.p99_ns - a.p99_ns);
  }, [rawStats]);

  // Tag groups: one entry per detector using the `detector:<name>` convention
  const tagGroups = useMemo(
    () => new Map([['detector', stats.map((s) => `detector:${s.name}`)]]),
    [stats]
  );

  // Highlighted set: hover takes priority over tag filter
  const highlightedSet = useMemo((): Set<string> | null => {
    if (hoveredDetector) return new Set([hoveredDetector]);

    const filter = parseTagFilter(tagFilterInput);
    if (filter.include.size === 0 && filter.exclude.size === 0) return null;

    const included = new Set<string>();
    for (const s of stats) {
      if (matchesTagFilter([`detector:${s.name}`], filter)) {
        included.add(s.name);
      }
    }
    return included.size > 0 ? included : null;
  }, [hoveredDetector, tagFilterInput, stats]);

  return (
    <div className="flex-1 flex">
      {/* Sidebar */}
      <aside
        className="bg-slate-800 border-r border-slate-700 overflow-y-auto flex-shrink-0"
        style={{ width: sidebarWidth }}
      >
        <ScenarioSelector
          scenarios={state.scenarios ?? []}
          activeScenario={state.activeScenario}
          onLoadScenario={actions.loadScenario}
        />

        {stats.length > 0 && (
          <div className="p-4 border-b border-slate-700">
            <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2">
              Detector Filter
            </h2>
            <div className="relative mb-2">
              <input
                type="text"
                value={tagFilterInput}
                onChange={(e) => setTagFilterInput(e.target.value)}
                placeholder="detector:…"
                className="w-full bg-slate-700 text-slate-200 text-xs rounded px-2 py-1.5 placeholder-slate-500 focus:outline-none focus:ring-1 focus:ring-purple-500 font-mono pr-6"
              />
              {tagFilterInput && (
                <button
                  onClick={() => setTagFilterInput('')}
                  className="absolute right-1.5 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-300"
                >
                  ×
                </button>
              )}
            </div>
            <TagFilterGroups
              tagGroups={tagGroups}
              tagFilterInput={tagFilterInput}
              onToggleTag={(tag) => setTagFilterInput(toggleTagInInput(tagFilterInput, tag))}
            />
          </div>
        )}

        {stats.length > 0 && (
          <div className="p-4">
            <div className="text-xs text-slate-500 space-y-1">
              <div>{stats.length} detector{stats.length !== 1 ? 's' : ''} measured</div>
              <div className="text-slate-600">sorted by p99 descending</div>
            </div>
          </div>
        )}
      </aside>

      {/* Main area */}
      <main className="flex-1 flex flex-col min-h-0 p-6 overflow-auto">
        {/* Empty states */}
        {state.connectionState === 'disconnected' && (
          <div className="text-center py-20">
            <div className="text-slate-400 text-lg">Waiting for observer connection…</div>
          </div>
        )}
        {state.connectionState === 'connected' && !state.activeScenario && (
          <div className="text-center py-20">
            <div className="text-slate-400 text-lg">Select a scenario to begin</div>
          </div>
        )}
        {state.connectionState === 'loading' && (
          <div className="text-center py-20">
            <div className="text-blue-400 text-lg">Loading scenario…</div>
          </div>
        )}
        {state.connectionState === 'ready' && stats.length === 0 && (
          <div className="text-center py-20">
            <div className="text-slate-400 text-lg">No benchmark data available</div>
            <div className="text-slate-500 text-sm mt-2">
              Load a scenario to see per-detector processing times.
            </div>
          </div>
        )}

        {/* Chart widget */}
        {state.connectionState === 'ready' && stats.length > 0 && (
          <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-4">
            <div className="flex items-baseline gap-3 mb-3">
              <h2 className="text-sm font-semibold text-slate-200">
                Detector Processing Times
              </h2>
              <span className="text-xs text-slate-500">
                per processing call · hover a row or click a tag to highlight
              </span>
            </div>
            <BenchmarkBarChart
              stats={stats}
              highlighted={highlightedSet}
              onHover={setHoveredDetector}
            />
          </div>
        )}
      </main>
    </div>
  );
}
