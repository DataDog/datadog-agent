import { useMemo, useState } from 'react';
import type { StatusResponse, BaselineDetectorStatus } from '../api/client';
import type { PhaseMarker } from './ChartWithAnomalyDetails';

const noDetectors: BaselineDetectorStatus[] = [];

function formatTime(timestamp: number): string {
  return new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(new Date(timestamp * 1000));
}

function formatTimestamp(timestamp: number): string {
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric', month: 'short', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  }).format(new Date(timestamp * 1000));
}

function formatDuration(seconds: number): string {
  const rounded = Math.max(0, Math.round(seconds));
  if (rounded < 60) return `${rounded}s`;
  const minutes = Math.floor(rounded / 60);
  const remainder = rounded % 60;
  return remainder === 0 ? `${minutes}m` : `${minutes}m ${remainder}s`;
}

function percent(timestamp: number, start: number, end: number): number {
  return Math.max(0, Math.min(100, ((timestamp - start) / Math.max(1, end - start)) * 100));
}

// BaselineTimelineWidget deliberately uses the complete scenario span rather
// than the selected chart range: the period before first analysis is useful
// context when warmup does not begin at scenario start.
export function BaselineTimelineWidget({ status, phaseMarkers = [] }: { status: StatusResponse | null; phaseMarkers?: PhaseMarker[] }) {
  const baseline = status?.baseline;
  const detectors = baseline?.detectors ?? noDetectors;
  const [hoverPercent, setHoverPercent] = useState<number | null>(null);
  const timeline = useMemo(() => {
    if (!baseline?.enabled || detectors.length === 0 || !baseline.started) return null;
    const scenarioStart = status?.scenarioStart ?? baseline.startSec;
    const knownEnds = detectors.flatMap(d => d.baselineEndSec === undefined ? [] : [d.baselineEndSec + 1]);
    const scenarioEnd = status?.scenarioEnd ?? Math.max(baseline.startSec + 1, ...knownEnds);
    return { start: Math.min(scenarioStart, baseline.startSec), end: Math.max(scenarioEnd, ...knownEnds) };
  }, [baseline, detectors, status?.scenarioEnd, status?.scenarioStart]);

  if (!baseline?.enabled) return null;
  if (!timeline) return <div className="rounded-lg border border-slate-700 bg-slate-800 px-4 py-6 text-center text-sm text-slate-500">Waiting for the first analysis advance.</div>;

  const midpoint = timeline.start + (timeline.end - timeline.start) / 2;
  const hoverTimestamp = hoverPercent === null ? null : timeline.start + (hoverPercent / 100) * (timeline.end - timeline.start);
  const visibleMarkers = phaseMarkers.filter(marker => marker.timestamp >= timeline.start && marker.timestamp <= timeline.end);
  return (
    <section className="rounded-lg border border-slate-700 bg-slate-800 overflow-hidden">
      <div className="px-4 py-3 border-b border-slate-700 flex flex-wrap justify-between gap-3">
        <div><h2 className="text-sm font-medium text-slate-200">Detector baselines</h2><p className="mt-0.5 text-xs text-slate-500">Full scenario timeline · qualification window: {Math.round(baseline.durationSec / 60)}m</p></div>
        <div className="flex flex-wrap gap-2 text-xs text-slate-300"><Legend className="border-slate-600 bg-slate-700/50" label="Before analysis" /><Legend className="border-blue-400 bg-blue-500/20" label="Warmup" /><Legend className="border-emerald-400 bg-emerald-500/20" label="Baseline" /><Legend className="border-purple-400 bg-purple-500/20" label="Detection" /></div>
      </div>
      <div className="p-4">
        <div className="relative ml-40 h-3 text-[9px] font-mono">
          {visibleMarkers.map(marker => {
            const left = percent(marker.timestamp, timeline.start, timeline.end);
            return <span key={marker.key} className="absolute left-1 whitespace-nowrap" style={{ left: `${left}%`, color: marker.color }}>{marker.label}</span>;
          })}
        </div>
        <div className="ml-40 flex justify-between pb-2 text-xs text-slate-500 tabular-nums"><span>{formatTime(timeline.start)}</span><span>{formatTime(midpoint)}</span><span>{formatTime(timeline.end)}</span></div>
        <div className="relative">
          <div className="space-y-3">
            {detectors.map(detector => <DetectorLane key={detector.name} detector={detector} analysisStart={baseline.startSec!} analyzedThrough={baseline.analyzedThroughSec ?? timeline.end} start={timeline.start} end={timeline.end} />)}
          </div>
          <div className="absolute inset-y-0 left-40 right-0 pointer-events-none">
            {visibleMarkers.map(marker => {
              const left = percent(marker.timestamp, timeline.start, timeline.end);
              return <div key={marker.key} className="absolute inset-y-0" style={{ left: `${left}%` }}>
                <div className="h-full border-l border-dashed opacity-80" style={{ borderColor: marker.color }} />
              </div>;
            })}
          </div>
          <div
            className="absolute inset-y-0 left-40 right-0 cursor-crosshair"
            onMouseMove={(event) => {
              const rect = event.currentTarget.getBoundingClientRect();
              setHoverPercent(Math.max(0, Math.min(100, ((event.clientX - rect.left) / rect.width) * 100)));
            }}
            onMouseLeave={() => setHoverPercent(null)}
          >
            {hoverPercent !== null && hoverTimestamp !== null && <>
              <div className="absolute inset-y-0 border-l border-dashed border-slate-300/60" style={{ left: `${hoverPercent}%` }} />
              <div className="absolute top-1 z-10 -translate-x-1/2 rounded bg-slate-950 px-2 py-1 text-[10px] font-mono text-slate-100 shadow" style={{ left: `${hoverPercent}%` }}>
                {formatTimestamp(hoverTimestamp)}
              </div>
            </>}
          </div>
        </div>
      </div>
    </section>
  );
}

function DetectorLane({ detector, analysisStart, analyzedThrough, start, end }: { detector: BaselineDetectorStatus; analysisStart: number; analyzedThrough: number; start: number; end: number }) {
  const before = percent(analysisStart, start, end);
  const observedEnd = Math.max(analysisStart, Math.min(analyzedThrough, end));
  if (!detector.ready || detector.warmupEndSec === undefined || detector.baselineEndSec === undefined) {
    return <div className="grid grid-cols-[9rem_minmax(28rem,1fr)] items-stretch gap-4"><div className="py-2"><div className="text-sm font-medium text-slate-200">{detector.name}</div><div className="mt-1 text-xs text-slate-500">waiting for readiness</div></div><div className="flex min-w-0 gap-1" aria-label={`${detector.name} baseline timeline`}><PhaseBox className="border-slate-600 bg-slate-700/50" width={before} title="Before analysis" detail={formatDuration(analysisStart - start)} /><PhaseBox className="border-blue-400 bg-blue-500/20" width={100 - before} title="Warmup" detail={`${formatDuration(observedEnd - analysisStart)} · waiting`} /></div></div>;
  }
  const warmup = percent(detector.warmupEndSec, start, end) - before;
  const qualificationEnd = detector.completed ? detector.baselineEndSec : Math.min(detector.baselineEndSec, observedEnd);
  const baseline = percent(qualificationEnd, start, end) - before - warmup;
  const detection = detector.completed ? 100 - before - warmup - baseline : 0;
  return <div className="grid grid-cols-[9rem_minmax(28rem,1fr)] items-stretch gap-4"><div className="py-2"><div className="text-sm font-medium text-slate-200">{detector.name}</div><div className="mt-1 text-xs text-slate-500">{detector.completed ? `${detector.mutedCount} muted` : 'baseline active'}</div></div><div className="flex min-w-0 gap-1" aria-label={`${detector.name} baseline timeline`}><PhaseBox className="border-slate-600 bg-slate-700/50" width={before} title="Before analysis" detail={formatDuration(analysisStart - start)} /><PhaseBox className="border-blue-400 bg-blue-500/20" width={warmup} title="Warmup" detail={formatDuration(detector.warmupEndSec - analysisStart)} /><PhaseBox className="border-emerald-400 bg-emerald-500/20" width={baseline} title="Baseline" detail={formatDuration(qualificationEnd - detector.warmupEndSec)} />{detector.completed && <PhaseBox className="border-purple-400 bg-purple-500/20" width={detection} title="Detection" detail={formatDuration(end - detector.baselineEndSec)} />}</div></div>;
}

function PhaseBox({ className, width, title, detail }: { className: string; width: number; title: string; detail: string }) { return <div className={`min-w-0 rounded border px-3 py-2 ${className}`} style={{ width: `${Math.max(0, width)}%` }}><div className="truncate text-xs font-medium text-slate-100">{title}</div><div className="truncate mt-0.5 text-[11px] text-slate-300">{detail}</div></div>; }
function Legend({ className, label }: { className: string; label: string }) { return <span className={`rounded border px-2 py-1 ${className}`}>{label}</span>; }
