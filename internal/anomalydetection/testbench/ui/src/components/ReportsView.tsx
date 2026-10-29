import { useMemo, useRef, useEffect, useState, useCallback } from 'react';
import * as d3 from 'd3';
import type { ObserverState, ObserverActions } from '../hooks/useObserver';
import { api } from '../api/client';
import type { ScenarioInfo, ReportEvent } from '../api/client';
import type { TimeRange, PhaseMarker } from './ChartWithAnomalyDetails';
import { MAIN_TAG_FILTER_KEYS } from '../constants';
import { parseTagFilter, extractTagGroups, toggleTagInInput, matchesTagFilter } from '../filters';
import { TagFilterGroups } from './TagFilterGroups';

interface ReportsViewProps {
  state: ObserverState;
  actions: ObserverActions;
  sidebarWidth: number;
  timeRange: TimeRange | null;
  onTimeRangeChange?: (range: TimeRange | null) => void;
  phaseMarkers?: PhaseMarker[];
}

const CHART_HEIGHT = 88;
const CHART_MARGIN = { top: 6, right: 8, bottom: 22, left: 8 };

function reportColor(pattern: string): string {
  const colors = ['#c084fc', '#60a5fa', '#22d3ee', '#2dd4bf', '#4ade80', '#f472b6'];
  let hash = 0;
  for (let i = 0; i < pattern.length; i++) hash = (hash * 31 + pattern.charCodeAt(i)) & 0xffffffff;
  return colors[Math.abs(hash) % colors.length];
}

/** Stable id for a report row — survives reordering when filters or /api/reports data changes. */
function reportStableKey(r: ReportEvent): string {
  return `${r.pattern}\u0001${r.firstSeen}`;
}

/** Interactive timeline: spans + markers per report, brush zoom + pan (same gestures as log view). */
function ReportsTimelineChart({
  reports,
  scenarioStart,
  scenarioEnd,
  timeRange,
  onTimeRangeChange,
  phaseMarkers = [],
  hoveredReportIndex,
  activeReportIndex,
  onReportHover,
  onReportClick,
}: {
  reports: ReportEvent[];
  scenarioStart: number | null;
  scenarioEnd: number | null;
  timeRange?: TimeRange | null;
  onTimeRangeChange?: (range: TimeRange | null) => void;
  phaseMarkers?: PhaseMarker[];
  hoveredReportIndex: number | null;
  activeReportIndex: number | null;
  onReportHover?: (index: number | null) => void;
  onReportClick?: (index: number) => void;
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
  const onReportHoverRef = useRef(onReportHover);
  onReportHoverRef.current = onReportHover;
  const onReportClickRef = useRef(onReportClick);
  onReportClickRef.current = onReportClick;

  const xDomainMs = useMemo((): [number, number] | null => {
    if (timeRange) return [timeRange.start * 1000, timeRange.end * 1000];
    if (scenarioStart != null && scenarioEnd != null && scenarioEnd > scenarioStart) {
      return [scenarioStart * 1000, scenarioEnd * 1000];
    }
    if (reports.length === 0) return null;
    let min = Infinity;
    let max = -Infinity;
    for (const r of reports) {
      min = Math.min(min, r.firstSeen);
      max = Math.max(max, r.lastUpdated);
    }
    if (!isFinite(min) || !isFinite(max) || max <= min) return null;
    const pad = Math.max(1, (max - min) * 0.02) * 1000;
    return [min * 1000 - pad, max * 1000 + pad];
  }, [timeRange, scenarioStart, scenarioEnd, reports]);

  useEffect(() => {
    if (!svgRef.current || !containerRef.current || !xDomainMs) return;
    if (isBrushingRef.current) return;

    const m = CHART_MARGIN;
    const container = containerRef.current;
    const width = container.clientWidth;
    const innerWidth = width - m.left - m.right;
    const innerHeight = CHART_HEIGHT - m.top - m.bottom;

    if (width <= m.left + m.right || innerWidth <= 0 || innerHeight <= 0) {
      d3.select(svgRef.current).selectAll('*').remove();
      return;
    }

    d3.select(svgRef.current).selectAll('*').remove();

    const svg = d3.select(svgRef.current).attr('width', width).attr('height', CHART_HEIGHT);

    svg.append('defs').append('clipPath').attr('id', 'reports-tl-clip').append('rect').attr('width', innerWidth).attr('height', innerHeight);

    const g = svg.append('g').attr('transform', `translate(${m.left},${m.top})`);

    const xScale = d3.scaleTime().domain(xDomainMs).range([0, innerWidth]);
    xScaleRef.current = xScale;

    const mainG = g.append('g').attr('clip-path', 'url(#reports-tl-clip)');

    // Baseline track
    mainG
      .append('rect')
      .attr('x', 0)
      .attr('y', innerHeight - 6)
      .attr('width', innerWidth)
      .attr('height', 4)
      .attr('fill', 'rgba(51, 65, 85, 0.35)')
      .attr('rx', 1);

    reports.forEach((r, i) => {
      const t0 = r.firstSeen * 1000;
      const t1 = Math.max(r.lastUpdated * 1000, t0 + 1);
      const x1 = xScale(t0);
      const x2 = xScale(t1);
      const xLeft = Math.max(0, x1);
      const xRight = Math.min(innerWidth, x2);
      // Skip if the whole segment is outside the visible x range
      if (xRight <= 0 || xLeft >= innerWidth) return;

      const color = reportColor(r.pattern);
      const isActive = activeReportIndex === i;
      const opacity = activeReportIndex != null && !isActive ? 0.3 : 1;
      const ts = isActive ? 6 : 3;
      const barW = Math.max(2, xRight - xLeft);
      const yBar = innerHeight / 2 - 5;

      const spanG = mainG.append('g').style('cursor', 'pointer');
      spanG
        .append('rect')
        .attr('x', xLeft)
        .attr('y', yBar)
        .attr('width', barW)
        .attr('height', 10)
        .attr('fill', color)
        .attr('opacity', opacity * 0.55)
        .attr('rx', 2);

      spanG
        .on('mouseenter', () => onReportHoverRef.current?.(i))
        .on('mouseleave', () => onReportHoverRef.current?.(null))
        .on('click', () => onReportClickRef.current?.(i));

      const markerG = mainG.append('g').style('cursor', 'pointer');
      const x = Math.max(0, Math.min(innerWidth, x1));
      markerG
        .append('polygon')
        .attr('points', `${x - ts},0 ${x + ts},0 ${x},${(ts * 1.7).toFixed(1)}`)
        .attr('fill', color)
        .attr('opacity', opacity);
      if (isActive) {
        markerG
          .append('polygon')
          .attr('points', `${x - ts},0 ${x + ts},0 ${x},${(ts * 1.7).toFixed(1)}`)
          .attr('fill', 'none')
          .attr('stroke', 'white')
          .attr('stroke-width', 1)
          .attr('opacity', 0.65);
      }
      markerG
        .append('line')
        .attr('x1', x)
        .attr('x2', x)
        .attr('y1', ts * 1.7)
        .attr('y2', innerHeight)
        .attr('stroke', color)
        .attr('stroke-width', isActive ? 1.5 : 1)
        .attr('opacity', opacity);
      markerG
        .append('rect')
        .attr('x', x - 8)
        .attr('width', 16)
        .attr('y', 0)
        .attr('height', innerHeight)
        .attr('fill', 'transparent');
      markerG
        .on('mouseenter', () => onReportHoverRef.current?.(i))
        .on('mouseleave', () => onReportHoverRef.current?.(null))
        .on('click', () => onReportClickRef.current?.(i));
    });

    // Highlight row hover: vertical band at firstSeen (like log bucket highlight)
    if (hoveredReportIndex != null && reports[hoveredReportIndex]) {
      const r = reports[hoveredReportIndex];
      const t0 = r.firstSeen * 1000;
      const t1 = Math.max(r.lastUpdated * 1000, t0 + 1);
      const hx1 = Math.max(0, xScale(t0));
      const hx2 = Math.min(innerWidth, xScale(t1));
      if (hx2 > hx1) {
        mainG
          .append('rect')
          .attr('x', hx1)
          .attr('width', hx2 - hx1)
          .attr('y', 0)
          .attr('height', innerHeight)
          .attr('fill', 'rgba(255, 255, 255, 0.1)')
          .attr('stroke', 'rgba(255, 255, 255, 0.35)')
          .attr('stroke-width', 1)
          .attr('pointer-events', 'none');
      }
      const cx = xScale(t0);
      if (cx >= 0 && cx <= innerWidth) {
        mainG
          .append('line')
          .attr('x1', cx)
          .attr('x2', cx)
          .attr('y1', 0)
          .attr('y2', innerHeight)
          .attr('stroke', 'rgba(255, 255, 255, 0.65)')
          .attr('stroke-width', 1)
          .attr('stroke-dasharray', '3,2')
          .attr('pointer-events', 'none');
      }
    }

    phaseMarkers.forEach((marker) => {
      const x = xScale(marker.timestamp * 1000);
      if (x < -20 || x > innerWidth + 20) return;
      mainG
        .append('line')
        .attr('x1', x)
        .attr('x2', x)
        .attr('y1', 0)
        .attr('y2', innerHeight)
        .attr('stroke', marker.color)
        .attr('stroke-width', 1)
        .attr('stroke-dasharray', '4,3')
        .attr('opacity', 0.8)
        .attr('pointer-events', 'none');
      mainG
        .append('text')
        .attr('x', x + 3)
        .attr('y', 10)
        .attr('fill', marker.color)
        .attr('font-size', '9px')
        .attr('font-family', 'monospace')
        .attr('opacity', 0.9)
        .attr('pointer-events', 'none')
        .text(marker.label);
    });

    g.append('g')
      .attr('transform', `translate(0,${innerHeight})`)
      .call(d3.axisBottom(xScale).ticks(6).tickFormat((d) => d3.timeFormat('%H:%M:%S')(d as Date)))
      .attr('color', '#334155')
      .selectAll('text')
      .attr('fill', '#64748b')
      .attr('font-size', '9px');

    g.select('.domain').attr('stroke', '#334155');

    const brush = d3
      .brushX<unknown>()
      .extent([
        [0, 0],
        [innerWidth, innerHeight],
      ])
      .filter((event: MouseEvent) => event.button === 0 && !event.metaKey)
      .on('start', () => {
        isBrushingRef.current = true;
      })
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
    g.select('.brush .selection').attr('fill', 'rgba(168, 85, 247, 0.2)').attr('stroke', '#a855f7').attr('stroke-width', 1);
  }, [
    reports,
    scenarioStart,
    scenarioEnd,
    timeRange,
    xDomainMs,
    hoveredReportIndex,
    activeReportIndex,
    phaseMarkers,
  ]);

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

    const handleAuxClick = (e: MouseEvent) => {
      if (e.button === 1) e.preventDefault();
    };

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

  if (!xDomainMs) return null;

  return (
    <div className="bg-slate-800/60 rounded p-3 mb-4">
      <div className="flex items-center gap-4 text-xs text-slate-400 mb-1.5">
        <span>Reports timeline ({reports.length})</span>
        <span className="text-slate-600 ml-auto">(drag to zoom · middle-drag or cmd+drag to pan)</span>
      </div>
      <div ref={containerRef} className="w-full">
        <svg ref={svgRef} style={{ display: 'block' }} />
      </div>
    </div>
  );
}

function EmptyState({
  connectionState,
  activeScenario,
  error,
}: {
  connectionState: string;
  activeScenario: string | null;
  error: string | null;
}) {
  if (error) {
    return (
      <div className="bg-red-900/50 border border-red-700 rounded-lg p-4 mb-6">
        <div className="text-red-400">{error}</div>
      </div>
    );
  }
  if (connectionState === 'disconnected') {
    return (
      <div className="text-center py-20">
        <div className="text-slate-400 text-lg">Waiting for observer connection...</div>
      </div>
    );
  }
  if (connectionState === 'connected' && !activeScenario) {
    return (
      <div className="text-center py-20">
        <div className="text-slate-400 text-lg">Select a scenario to begin</div>
      </div>
    );
  }
  if (connectionState === 'loading') {
    return (
      <div className="text-center py-20">
        <div className="text-blue-400 text-lg">Loading scenario...</div>
      </div>
    );
  }
  return null;
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
                activeScenario === scenario.name ? 'bg-purple-600 text-white' : 'text-slate-300 hover:bg-slate-700'
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

/** Same compact stat hint as CorrelatorView sidebar. */
function StatsLabel({ stats }: { stats: Record<string, unknown> }) {
  const entries = Object.entries(stats).filter(([k, v]) =>
    typeof v === 'number' && k !== 'enabled' && v > 0
  );
  if (entries.length === 0) return null;
  const preferred = ['clusterCount', 'edgeCount', 'pairCount'];
  const best = preferred.find((k) => stats[k] && (stats[k] as number) > 0);
  const [key, value] = best ? [best, stats[best]] : entries[0];
  const label = key.replace(/Count$/, 's').replace(/([A-Z])/g, ' $1').trim().toLowerCase();
  return (
    <span className="text-xs text-slate-500">{value as number} {label}</span>
  );
}

export function ReportsView({ state, actions, sidebarWidth, timeRange, onTimeRangeChange, phaseMarkers }: ReportsViewProps) {
  const scenarios = state.scenarios ?? [];
  const components = state.components ?? [];
  const correlatorStats = state.correlatorStats;
  const allReports = state.reports ?? [];

  const correlatorComponents = useMemo(
    () => components.filter((c) => c.category === 'correlator'),
    [components]
  );

  const [tagFilterInput, setTagFilterInput] = useState('');
  const [hoveredReportIndex, setHoveredReportIndex] = useState<number | null>(null);
  /** Row selected from timeline click (highlight + scroll); keyed by reportStableKey, not list index. */
  const [timelineFocusedKey, setTimelineFocusedKey] = useState<string | null>(null);
  /** Which list rows show full message body (collapsed by default). */
  const [expandedRowKeys, setExpandedRowKeys] = useState<Set<string>>(() => new Set());
  /** Send state per row key: 'idle' | 'sending' | 'sent' | 'error:<msg>' */
  const [sendState, setSendState] = useState<Map<string, string>>(() => new Map());
  const reportRowRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const initializedScenarioRef = useRef<string | null>(null);

  const handleSendReport = useCallback(async (pattern: string, firstSeen: number, key: string) => {
    setSendState((prev) => new Map(prev).set(key, 'sending'));
    try {
      await api.sendReport(pattern, firstSeen);
      setSendState((prev) => new Map(prev).set(key, 'sent'));
      setTimeout(() => setSendState((prev) => {
        const next = new Map(prev);
        if (next.get(key) === 'sent') next.set(key, 'idle');
        return next;
      }), 3000);
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      setSendState((prev) => new Map(prev).set(key, 'error:' + msg));
    }
  }, []);

  useEffect(() => {
    if (state.activeScenario && initializedScenarioRef.current !== state.activeScenario) {
      initializedScenarioRef.current = state.activeScenario;
      setTagFilterInput('');
      setHoveredReportIndex(null);
      setTimelineFocusedKey(null);
      setExpandedRowKeys(new Set());
      setSendState(new Map());
    }
  }, [state.activeScenario]);

  // Drop focus/expansion/send state for reports that no longer exist (e.g. correlator rerun), not when only order/filter changes.
  useEffect(() => {
    const validKeys = new Set(allReports.map(reportStableKey));
    setExpandedRowKeys((prev) => {
      let changed = false;
      const next = new Set<string>();
      for (const k of prev) {
        if (validKeys.has(k)) next.add(k);
        else changed = true;
      }
      return changed ? next : prev;
    });
    setTimelineFocusedKey((prev) => (prev && validKeys.has(prev) ? prev : null));
    setSendState((prev) => {
      let changed = false;
      const next = new Map<string, string>();
      for (const [k, v] of prev) {
        if (validKeys.has(k)) next.set(k, v);
        else changed = true;
      }
      return changed ? next : prev;
    });
  }, [allReports]);

  const tagGroups = useMemo(() => {
    const all = extractTagGroups(allReports.map((r) => r.tags ?? []));
    return new Map([...all.entries()].filter(([k]) => MAIN_TAG_FILTER_KEYS.has(k)));
  }, [allReports]);

  const filteredReports = useMemo(() => {
    const filter = parseTagFilter(tagFilterInput);
    if (filter.include.size === 0 && filter.exclude.size === 0) return allReports;
    return allReports.filter((r) => matchesTagFilter(r.tags ?? [], filter));
  }, [allReports, tagFilterInput]);

  const sortedReports = useMemo(
    () => [...filteredReports].sort((a, b) => a.firstSeen - b.firstSeen),
    [filteredReports]
  );

  /** Index into sortedReports for chart/list highlight; null if focus key is not in the current filtered list. */
  const timelineFocusedIndex = useMemo(() => {
    if (timelineFocusedKey == null) return null;
    const i = sortedReports.findIndex((r) => reportStableKey(r) === timelineFocusedKey);
    return i >= 0 ? i : null;
  }, [sortedReports, timelineFocusedKey]);

  const visibleReports = useMemo(() => {
    if (!timeRange) return sortedReports;
    return sortedReports.filter(
      (r) => r.lastUpdated >= timeRange.start && r.firstSeen <= timeRange.end
    );
  }, [sortedReports, timeRange]);

  const scenarioStart = state.status?.scenarioStart ?? null;
  const scenarioEnd = state.status?.scenarioEnd ?? null;

  const activeReportIndex = hoveredReportIndex !== null ? hoveredReportIndex : timelineFocusedIndex;

  const toggleRowExpanded = (key: string) => {
    setExpandedRowKeys((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  return (
    <div className="flex-1 flex">
      <aside className="bg-slate-800 border-r border-slate-700 overflow-y-auto" style={{ width: sidebarWidth }}>
        <ScenarioSelector scenarios={scenarios} activeScenario={state.activeScenario} onLoadScenario={actions.loadScenario} />

        <div className="p-4 border-b border-slate-700">
          <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">Correlators</h2>
          <p className="text-[11px] text-slate-500 mb-3 leading-snug">
            Reports are built from correlation output. Turn on at least one correlator (e.g. time cluster), then wait for the run to finish.
          </p>
          <div className="space-y-1">
            {correlatorComponents.map((comp) => {
              const stats = correlatorStats?.[comp.name];
              return (
                <div key={comp.name} className="flex items-center gap-2 px-2 py-1.5">
                  <button
                    type="button"
                    onClick={() => actions.toggleComponent(comp.name)}
                    className={`relative inline-flex h-4 w-7 items-center rounded-full transition-colors flex-shrink-0 ${
                      comp.enabled ? 'bg-purple-600' : 'bg-slate-600'
                    }`}
                  >
                    <span
                      className={`inline-block h-3 w-3 transform rounded-full bg-white transition-transform ${
                        comp.enabled ? 'translate-x-3.5' : 'translate-x-0.5'
                      }`}
                    />
                  </button>
                  <span className="text-sm text-slate-300 flex-1">{comp.displayName}</span>
                  {stats && <StatsLabel stats={stats} />}
                </div>
              );
            })}
          </div>
        </div>

        <div className="p-4 border-b border-slate-700">
          <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2">Tag Filter</h2>
          <div className="relative mb-2">
            <input
              type="text"
              value={tagFilterInput}
              onChange={(e) => setTagFilterInput(e.target.value)}
              placeholder="pattern:… source:…"
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

        <div className="p-4">
          <h2 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2">Summary</h2>
          <div className="text-sm text-slate-300">
            {sortedReports.length} report{sortedReports.length !== 1 ? 's' : ''}
            {sortedReports.length !== allReports.length ? ` (${allReports.length} total)` : ''}
          </div>
        </div>
      </aside>

      <main className="flex-1 flex flex-col min-h-0 p-6 overflow-hidden">
        {state.error && state.connectionState === 'ready' && (
          <div className="bg-red-900/50 border border-red-700 rounded-lg p-4 mb-6 shrink-0">
            <div className="text-red-400">{state.error}</div>
          </div>
        )}

        {state.connectionState !== 'ready' && (
          <div className="overflow-y-auto flex-1 min-h-0">
            <EmptyState connectionState={state.connectionState} activeScenario={state.activeScenario} error={state.error} />
          </div>
        )}

        {state.connectionState === 'ready' && (
          <div className="flex flex-col flex-1 min-h-0 gap-4">
            <div className="shrink-0">
              <ReportsTimelineChart
                reports={sortedReports}
                scenarioStart={scenarioStart}
                scenarioEnd={scenarioEnd}
                timeRange={timeRange}
                onTimeRangeChange={onTimeRangeChange}
                phaseMarkers={phaseMarkers}
                hoveredReportIndex={hoveredReportIndex}
                activeReportIndex={activeReportIndex}
                onReportHover={setHoveredReportIndex}
                onReportClick={(idx) => {
                  const key = reportStableKey(sortedReports[idx]);
                  setTimelineFocusedKey(key);
                  setTimeout(() => {
                    reportRowRefs.current.get(key)?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
                  }, 50);
                }}
              />
            </div>

            {sortedReports.length === 0 ? (
              <div className="text-center py-12 text-slate-500 text-sm shrink-0">No reports match the current filters.</div>
            ) : (
              <div className="flex flex-col flex-1 min-h-0 bg-slate-800 rounded-lg overflow-hidden border border-slate-700/80">
                <div className="px-4 py-2 border-b border-slate-700 text-sm font-medium text-slate-300 shrink-0">
                  Reports
                  {timeRange && (
                    <span className="ml-2 text-xs text-slate-500 font-normal">
                      ({visibleReports.length} in time span)
                    </span>
                  )}
                  <span className="ml-2 text-xs text-slate-500 font-normal">· scroll list · click row to expand</span>
                </div>
                <div className="flex-1 min-h-0 overflow-y-auto divide-y divide-slate-700/80">
                  {sortedReports.map((rep, idx) => {
                    const rowKey = reportStableKey(rep);
                    const inView = !timeRange || (rep.lastUpdated >= timeRange.start && rep.firstSeen <= timeRange.end);
                    const isHi = activeReportIndex === idx;
                    const isExpanded = expandedRowKeys.has(rowKey);
                    const rs = sendState.get(rowKey) ?? 'idle';
                    const isSending = rs === 'sending';
                    const isSent = rs === 'sent';
                    const sendError = rs.startsWith('error:') ? rs.slice(6) : null;
                    return (
                      <div
                        key={rowKey}
                        ref={(el) => {
                          if (el) reportRowRefs.current.set(rowKey, el);
                          else reportRowRefs.current.delete(rowKey);
                        }}
                        className={`transition-colors ${
                          isHi ? 'bg-purple-900/35 ring-1 ring-inset ring-purple-500/40' : 'hover:bg-slate-700/40'
                        } ${!inView ? 'opacity-45' : ''}`}
                        onMouseEnter={() => setHoveredReportIndex(idx)}
                        onMouseLeave={() => setHoveredReportIndex(null)}
                      >
                        <div className="flex items-start">
                          <button
                            type="button"
                            onClick={() => toggleRowExpanded(rowKey)}
                            className="flex-1 min-w-0 text-left px-4 py-2.5 flex items-start gap-2"
                            aria-expanded={isExpanded}
                          >
                            <span className="mt-0.5 text-slate-500 tabular-nums w-4 flex-shrink-0 select-none" aria-hidden>
                              {isExpanded ? '▼' : '▶'}
                            </span>
                            <span
                              className="mt-1 w-2 h-2 rounded-full flex-shrink-0"
                              style={{ backgroundColor: reportColor(rep.pattern) }}
                            />
                            <div className="min-w-0 flex-1">
                              <div className="flex flex-wrap items-baseline gap-2">
                                <span className="text-sm font-medium text-slate-100">{rep.title}</span>
                                <span className="text-xs font-mono text-slate-500">{rep.formattedTime}</span>
                              </div>
                              <div className="text-xs text-slate-500 font-mono mt-0.5 truncate" title={rep.pattern}>
                                {rep.pattern}
                              </div>
                            </div>
                          </button>
                          <button
                            type="button"
                            disabled={isSending || isSent}
                            onClick={(e) => { e.stopPropagation(); handleSendReport(rep.pattern, rep.firstSeen, rowKey); }}
                            title="Send to Datadog backend"
                            className={`flex-shrink-0 self-center mr-3 px-2 py-1 rounded text-[10px] font-mono transition-colors ${
                              isSent
                                ? 'bg-green-700/40 text-green-300 cursor-default'
                                : sendError
                                ? 'bg-red-700/40 text-red-300 hover:bg-red-700/60'
                                : isSending
                                ? 'bg-slate-700 text-slate-400 cursor-wait'
                                : 'bg-slate-700 text-slate-300 hover:bg-purple-700/60 hover:text-purple-200'
                            }`}
                          >
                            {isSending ? '…' : isSent ? '✓ sent' : sendError ? '✗ retry' : 'send'}
                          </button>
                        </div>
                        {isExpanded && (
                          <div className="px-4 pb-3 flex gap-2">
                            <span className="w-4 flex-shrink-0" aria-hidden />
                            <span className="w-2 flex-shrink-0" aria-hidden />
                            <div className="min-w-0 flex-1">
                              <p className="text-sm text-slate-300 whitespace-pre-wrap break-words">{rep.message}</p>
                              {(rep.tags ?? []).length > 0 && (
                                <div className="flex flex-wrap gap-1 mt-2">
                                  {(rep.tags ?? []).map((t) => (
                                    <span key={t} className="text-[10px] px-1.5 py-0.5 rounded bg-slate-700 text-slate-400 font-mono">
                                      {t}
                                    </span>
                                  ))}
                                </div>
                              )}
                              {sendError && (
                                <p className="text-[10px] text-red-400 mt-2 font-mono">Send error: {sendError}</p>
                              )}
                            </div>
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              </div>
            )}
          </div>
        )}
      </main>
    </div>
  );
}
