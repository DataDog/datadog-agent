import { useState, useEffect, useMemo, useRef } from 'react';
import * as d3 from 'd3';
import { api } from '../api/client';
import type { DetectorProcessingStats, ScenarioInfo } from '../api/client';
import type { ObserverState, ObserverActions } from '../hooks/useObserver';
import { TagFilterGroups } from './TagFilterGroups';
import { parseTagFilter, toggleTagInInput, matchesTagFilter } from '../filters';

// ── Chart constants ────────────────────────────────────────────────────────────

// The "total" bar uses a fully independent secondary x-scale (its values can be
// orders of magnitude larger than avg/p50/p99). A visual separator + second axis
// makes the two scales unambiguous.
const SEP_H = 10; // extra vertical gap between per-call rows and the total row
const MARGIN = { top: 44, right: 95, bottom: 28, left: 235 };
const BAR_H = 15;
const BAR_GAP = 3;
const GROUP_PAD = 16;
const GROUP_INNER_H = 4 * BAR_H + 3 * BAR_GAP + SEP_H; // 91 px
const GROUP_H = GROUP_INNER_H + GROUP_PAD;               // 107 px

// Per-call stats (share the primary top x-axis, domain in ns)
const PER_CALL_STATS: { key: 'avg' | 'p50' | 'p99'; label: string; color: string }[] = [
  { key: 'avg', label: 'avg', color: '#60a5fa' },  // blue-400
  { key: 'p50', label: 'p50', color: '#4ade80' },  // green-400
  { key: 'p99', label: 'p99', color: '#f87171' },  // red-400
];

const TOTAL_COLOR = '#fb923c'; // orange-400

// Format µs-range values (used on the primary axis and per-call labels)
function fmtUs(ns: number): string {
  const us = ns / 1000;
  if (us < 10) return us.toFixed(2) + 'µs';
  if (us < 100) return us.toFixed(1) + 'µs';
  return us.toFixed(0) + 'µs';
}

// Format potentially large totals with auto-scaling µs → ms → s
function fmtTotal(ns: number): string {
  const us = ns / 1000;
  if (us < 1_000) return us.toFixed(1) + 'µs';
  const ms = us / 1_000;
  if (ms < 1_000) return ms.toFixed(1) + 'ms';
  return (ms / 1_000).toFixed(3) + 's';
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

type StatKey = 'avg' | 'p50' | 'p99' | 'total';

function BenchmarkBarChart({ stats, highlighted, onHover }: BenchmarkBarChartProps) {
  const svgRef = useRef<SVGSVGElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [containerWidth, setContainerWidth] = useState(0);
  const [activeStat, setActiveStat] = useState<StatKey | null>(null);
  const onHoverRef = useRef(onHover);
  onHoverRef.current = onHover;
  const onStatClickRef = useRef<(key: StatKey) => void>(() => {});
  onStatClickRef.current = (key: StatKey) =>
    setActiveStat((prev) => (prev === key ? null : key));

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

    // Legend — clickable items (click to isolate a stat, click again to reset)
    const legendG = svg.append('g').attr('transform', `translate(${MARGIN.left}, 16)`);

    const legendItems: { key: StatKey; label: string; color: string; x: number; baseOpacity: number }[] = [
      { key: 'avg',   label: 'avg',   color: '#60a5fa', x: 0,   baseOpacity: 1 },
      { key: 'p50',   label: 'p50',   color: '#4ade80', x: 72,  baseOpacity: 1 },
      { key: 'p99',   label: 'p99',   color: '#f87171', x: 144, baseOpacity: 1 },
      { key: 'total', label: 'total', color: TOTAL_COLOR, x: 235, baseOpacity: 0.85 },
    ];

    // Pipe separator between per-call and total
    legendG.append('text').attr('x', 220).attr('y', 9).attr('fill', '#475569').attr('font-size', '10px').text('|');

    legendItems.forEach(({ key, label, color, x, baseOpacity }) => {
      const isSelected = activeStat === key;
      const isDimmed = activeStat !== null && !isSelected;
      const itemOpacity = isDimmed ? 0.25 : baseOpacity;

      const itemG = legendG.append('g')
        .attr('transform', `translate(${x}, 0)`)
        .style('cursor', 'pointer')
        .attr('opacity', itemOpacity)
        .on('click', () => onStatClickRef.current(key));

      itemG.append('rect')
        .attr('x', 0).attr('y', 0).attr('width', 10).attr('height', 10)
        .attr('fill', color).attr('rx', 2);

      itemG.append('text')
        .attr('x', 14).attr('y', 9)
        .attr('fill', color).attr('font-size', '10px').attr('font-family', 'monospace')
        .text(label);

      // Underline when selected
      if (isSelected) {
        const labelWidth = label.length * 7;
        itemG.append('line')
          .attr('x1', 14).attr('x2', 14 + labelWidth)
          .attr('y1', 13).attr('y2', 13)
          .attr('stroke', color).attr('stroke-width', 1.5);
      }
    });

    // "own scale" note
    legendG.append('text').attr('x', 283).attr('y', 9).attr('fill', '#475569').attr('font-size', '9px')
      .attr('opacity', activeStat === null || activeStat === 'total' ? 0.7 : 0.2)
      .text('(own scale →)');

    const g = svg.append('g').attr('transform', `translate(${MARGIN.left},${MARGIN.top})`);

    // Primary scale: per-call stats (avg / p50 / p99)
    const maxPerCallNs = d3.max(stats, (s) => s.p99_ns) ?? 1;
    const xScale = d3.scaleLinear().domain([0, maxPerCallNs * 1.12]).range([0, innerWidth]);

    // Secondary scale: total — fully independent domain
    const maxTotalNs = d3.max(stats, (s) => s.total_ns) ?? 1;
    const xScaleTotal = d3.scaleLinear().domain([0, maxTotalNs * 1.12]).range([0, innerWidth]);

    const perCallAxisOpacity = activeStat === 'total' ? 0.15 : 1;
    const totalAxisOpacity   = activeStat !== null && activeStat !== 'total' ? 0.15 : 1;

    // Top axis — per-call µs scale (suppress zero to avoid collision with "avg" label)
    g.append('g')
      .attr('transform', 'translate(0,-8)')
      .attr('opacity', perCallAxisOpacity)
      .call(d3.axisTop(xScale).ticks(5).tickFormat((d) => (+d === 0 ? '' : fmtUs(+d))))
      .call((ax) => ax.select('.domain').attr('stroke', '#334155'))
      .call((ax) => ax.selectAll('text').attr('fill', '#64748b').attr('font-size', '10px'))
      .call((ax) => ax.selectAll('.tick line').attr('stroke', '#334155'));

    // Bottom axis — total scale (auto µs/ms/s), orange tint
    g.append('g')
      .attr('transform', `translate(0,${innerHeight + 8})`)
      .attr('opacity', totalAxisOpacity)
      .call(d3.axisBottom(xScaleTotal).ticks(5).tickFormat((d) => (+d === 0 ? '' : fmtTotal(+d))))
      .call((ax) => ax.select('.domain').attr('stroke', '#334155'))
      .call((ax) => ax.selectAll('text').attr('fill', TOTAL_COLOR).attr('font-size', '10px').attr('opacity', '0.7'))
      .call((ax) => ax.selectAll('.tick line').attr('stroke', '#334155'));

    // Primary grid lines (faint)
    xScale.ticks(5).forEach((t) => {
      g.append('line')
        .attr('x1', xScale(t)).attr('x2', xScale(t))
        .attr('y1', 0).attr('y2', innerHeight)
        .attr('stroke', '#1e293b').attr('stroke-width', 1);
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

      // Transparent hit zone spanning full row (including margins)
      groupG.append('rect')
        .attr('x', -MARGIN.left).attr('y', -2)
        .attr('width', MARGIN.left + innerWidth + MARGIN.right)
        .attr('height', GROUP_INNER_H + 4)
        .attr('fill', 'transparent');

      // ── Per-call bars (avg / p50 / p99) — primary scale ──────────────────
      const perCallVals: Record<string, number> = { avg: stat.avg_ns, p50: stat.median_ns, p99: stat.p99_ns };

      PER_CALL_STATS.forEach(({ key, color }, si) => {
        const barY = si * (BAR_H + BAR_GAP);
        const val = perCallVals[key];
        const barW = Math.max(0, xScale(val));
        const statOpacity = activeStat === null || activeStat === key ? 1 : 0.07;

        groupG.append('rect')
          .attr('x', 0).attr('y', barY).attr('width', barW).attr('height', BAR_H)
          .attr('fill', color).attr('rx', 2).attr('opacity', statOpacity);

        groupG.append('text')
          .attr('x', -10).attr('y', barY + BAR_H / 2)
          .attr('text-anchor', 'end').attr('dominant-baseline', 'middle')
          .attr('fill', color).attr('font-size', '9px').attr('font-family', 'monospace')
          .attr('opacity', statOpacity)
          .text(key);

        groupG.append('text')
          .attr('x', barW + 6).attr('y', barY + BAR_H / 2)
          .attr('dominant-baseline', 'middle')
          .attr('fill', color).attr('font-size', '10px').attr('font-family', 'monospace')
          .attr('opacity', statOpacity)
          .text(fmtUs(val));
      });

      // ── Dashed separator line before the total row ────────────────────────
      const sepY = 3 * (BAR_H + BAR_GAP) + SEP_H / 2;
      groupG.append('line')
        .attr('x1', -10).attr('x2', innerWidth)
        .attr('y1', sepY).attr('y2', sepY)
        .attr('stroke', '#334155').attr('stroke-width', 0.5)
        .attr('stroke-dasharray', '3,3');

      // ── Total bar — secondary scale ───────────────────────────────────────
      const totalBarY = 3 * (BAR_H + BAR_GAP) + SEP_H;
      const totalW = Math.max(0, xScaleTotal(stat.total_ns));
      const totalStatOpacity = activeStat === null || activeStat === 'total' ? 1 : 0.07;

      groupG.append('rect')
        .attr('x', 0).attr('y', totalBarY).attr('width', totalW).attr('height', BAR_H)
        .attr('fill', TOTAL_COLOR).attr('opacity', totalStatOpacity * 0.75).attr('rx', 2);

      groupG.append('text')
        .attr('x', -10).attr('y', totalBarY + BAR_H / 2)
        .attr('text-anchor', 'end').attr('dominant-baseline', 'middle')
        .attr('fill', TOTAL_COLOR).attr('font-size', '9px').attr('font-family', 'monospace')
        .attr('opacity', totalStatOpacity * 0.85).text('tot');

      groupG.append('text')
        .attr('x', totalW + 6).attr('y', totalBarY + BAR_H / 2)
        .attr('dominant-baseline', 'middle')
        .attr('fill', TOTAL_COLOR).attr('font-size', '10px').attr('font-family', 'monospace')
        .attr('opacity', totalStatOpacity * 0.85).text(fmtTotal(stat.total_ns));

      // ── Detector name + call count ────────────────────────────────────────
      const nameX = -60;
      const displayName = stat.name.length > 20 ? stat.name.slice(0, 19) + '…' : stat.name;

      groupG.append('text')
        .attr('x', nameX).attr('y', GROUP_INNER_H / 2 - 7)
        .attr('text-anchor', 'end').attr('dominant-baseline', 'middle')
        .attr('fill', isActive ? '#e2e8f0' : '#475569')
        .attr('font-size', '11px').attr('font-family', 'monospace')
        .text(displayName);

      groupG.append('text')
        .attr('x', nameX).attr('y', GROUP_INNER_H / 2 + 9)
        .attr('text-anchor', 'end').attr('dominant-baseline', 'middle')
        .attr('fill', '#475569').attr('font-size', '9px')
        .text(`${stat.count.toLocaleString()} calls`);
    });
  }, [stats, highlighted, containerWidth, activeStat]);

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
