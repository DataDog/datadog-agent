import { useMemo } from 'react';
import type { StatusResponse, BaselineDetectorStatus } from '../api/client';

const noDetectors: BaselineDetectorStatus[] = [];

function formatTime(timestamp: number): string {
  return new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(new Date(timestamp * 1000));
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
export function BaselineTimelineWidget({ status }: { status: StatusResponse | null }) {
  const baseline = status?.baseline;
  const detectors = baseline?.detectors ?? noDetectors;
  const timeline = useMemo(() => {
    if (!baseline?.enabled || detectors.length === 0 || !baseline.started) return null;
    const scenarioStart = status?.scenarioStart ?? baseline.startSec;
    const scenarioEnd = status?.scenarioEnd ?? Math.max(...detectors.map(d => d.baselineEndSec)) + 1;
    return { start: Math.min(scenarioStart, baseline.startSec), end: Math.max(scenarioEnd, ...detectors.map(d => d.baselineEndSec + 1)) };
  }, [baseline, detectors, status?.scenarioEnd, status?.scenarioStart]);

  if (!baseline?.enabled) return null;
  if (!timeline) return <div className="rounded-lg border border-slate-700 bg-slate-800 px-4 py-6 text-center text-sm text-slate-500">Waiting for the first analysis advance.</div>;

  const midpoint = timeline.start + (timeline.end - timeline.start) / 2;
  return (
    <section className="rounded-lg border border-slate-700 bg-slate-800 overflow-hidden">
      <div className="px-4 py-3 border-b border-slate-700 flex flex-wrap justify-between gap-3">
        <div><h2 className="text-sm font-medium text-slate-200">Detector baselines</h2><p className="mt-0.5 text-xs text-slate-500">Full scenario timeline · qualification window: {Math.round(baseline.durationSec / 60)}m</p></div>
        <div className="flex flex-wrap gap-2 text-xs text-slate-300"><Legend className="border-slate-600 bg-slate-700/50" label="Before analysis" /><Legend className="border-blue-400 bg-blue-500/20" label="Warmup" /><Legend className="border-emerald-400 bg-emerald-500/20" label="Baseline" /><Legend className="border-purple-400 bg-purple-500/20" label="Detection" /></div>
      </div>
      <div className="p-4">
        <div className="ml-40 flex justify-between pb-2 text-xs text-slate-500 tabular-nums"><span>{formatTime(timeline.start)}</span><span>{formatTime(midpoint)}</span><span>{formatTime(timeline.end)}</span></div>
        <div className="space-y-3">
          {detectors.map(detector => <DetectorLane key={detector.name} detector={detector} analysisStart={baseline.startSec!} start={timeline.start} end={timeline.end} />)}
        </div>
      </div>
    </section>
  );
}

function DetectorLane({ detector, analysisStart, start, end }: { detector: BaselineDetectorStatus; analysisStart: number; start: number; end: number }) {
  const before = percent(analysisStart, start, end);
  const warmup = percent(detector.warmupEndSec, start, end) - before;
  const baseline = percent(detector.baselineEndSec, start, end) - before - warmup;
  const detection = 100 - before - warmup - baseline;
  return <div className="grid grid-cols-[9rem_minmax(28rem,1fr)] items-stretch gap-4"><div className="py-2"><div className="text-sm font-medium text-slate-200">{detector.name}</div><div className="mt-1 text-xs text-slate-500">{detector.completed ? `${detector.mutedCount} muted` : 'baseline active'}</div></div><div className="flex min-w-0 gap-1" aria-label={`${detector.name} baseline timeline`}><PhaseBox className="border-slate-600 bg-slate-700/50" width={before} title="Waiting" detail={formatDuration(analysisStart - start)} /><PhaseBox className="border-blue-400 bg-blue-500/20" width={warmup} title="Warmup" detail={formatDuration(detector.warmupEndSec - analysisStart)} /><PhaseBox className="border-emerald-400 bg-emerald-500/20" width={baseline} title="Baseline" detail={formatDuration(detector.baselineEndSec - detector.warmupEndSec)} /><PhaseBox className="border-purple-400 bg-purple-500/20" width={detection} title="Detection" detail={formatDuration(end - detector.baselineEndSec)} /></div></div>;
}

function PhaseBox({ className, width, title, detail }: { className: string; width: number; title: string; detail: string }) { return <div className={`min-w-0 rounded border px-3 py-2 ${className}`} style={{ width: `${Math.max(0, width)}%` }}><div className="truncate text-xs font-medium text-slate-100">{title}</div><div className="truncate mt-0.5 text-[11px] text-slate-300">{detail}</div></div>; }
function Legend({ className, label }: { className: string; label: string }) { return <span className={`rounded border px-2 py-1 ${className}`}>{label}</span>; }
