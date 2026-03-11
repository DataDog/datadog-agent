import { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import { useObserver } from './hooks/useObserver';
import { MetricsView } from './components/MetricsView';
import { CorrelatorView } from './components/CorrelatorView';
import { LogView } from './components/LogView';
import type { EpisodeInfo, ScoreResult } from './api/client';
import type { PhaseMarker } from './components/ChartWithAnomalyDetails';

type TabID = 'timeseries' | 'correlators' | 'logs';

function ConnectionStatus({ state }: { state: string }) {
  const colors: Record<string, string> = {
    disconnected: 'bg-red-500',
    connected: 'bg-yellow-500',
    loading: 'bg-blue-500',
    ready: 'bg-green-500',
  };

  const labels: Record<string, string> = {
    disconnected: 'Disconnected',
    connected: 'Connected (no scenario)',
    loading: 'Loading...',
    ready: 'Ready',
  };

  return (
    <div className="flex items-center gap-2">
      <div className={`w-2 h-2 rounded-full ${colors[state]} animate-pulse`} />
      <span className="text-sm text-slate-400">{labels[state]}</span>
    </div>
  );
}

interface TimeRange {
  start: number;
  end: number;
}

function formatTimestamp(ts: number): string {
  const d = new Date(ts * 1000);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

function toDatetimeLocal(ts: number): string {
  const d = new Date(ts * 1000);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

function fromDatetimeLocal(value: string): number {
  return new Date(value).getTime() / 1000;
}

function EditableTimestamp({ value, onChange }: { value: number; onChange: (ts: number) => void }) {
  const [editing, setEditing] = useState(false);
  const [inputValue, setInputValue] = useState('');

  const startEditing = () => {
    setInputValue(toDatetimeLocal(value));
    setEditing(true);
  };

  const commit = () => {
    if (inputValue) {
      const ts = fromDatetimeLocal(inputValue);
      if (!isNaN(ts)) onChange(ts);
    }
    setEditing(false);
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') commit();
    if (e.key === 'Escape') setEditing(false);
  };

  if (editing) {
    return (
      <input
        type="datetime-local"
        step="1"
        value={inputValue}
        onChange={e => setInputValue(e.target.value)}
        onBlur={commit}
        onKeyDown={handleKeyDown}
        autoFocus
        className="text-sm font-mono bg-slate-800 border border-purple-500 rounded px-1 py-0.5 text-slate-200 focus:outline-none"
      />
    );
  }

  return (
    <button
      onClick={startEditing}
      title="Click to edit"
      className="text-sm text-slate-200 font-mono hover:text-purple-300 hover:underline cursor-pointer"
    >
      {formatTimestamp(value)}
    </button>
  );
}

const PHASE_STYLES: Record<string, { bg: string; border: string; label: string }> = {
  warmup:     { bg: 'bg-blue-900/40',   border: 'border-blue-500/50',   label: 'Warmup' },
  baseline:   { bg: 'bg-green-900/40',  border: 'border-green-500/50',  label: 'Baseline' },
  disruption: { bg: 'bg-red-900/40',    border: 'border-red-500/50',    label: 'Disruption' },
  cooldown:   { bg: 'bg-yellow-900/40', border: 'border-yellow-500/50', label: 'Cooldown' },
};

function formatTime(isoString: string): string {
  const d = new Date(isoString);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

function EpisodeInfoPanel({ info }: { info: EpisodeInfo }) {
  const [expanded, setExpanded] = useState(false);

  const phases: { key: string; phase: typeof info.baseline }[] = [
    { key: 'warmup', phase: info.warmup },
    { key: 'baseline', phase: info.baseline },
    { key: 'disruption', phase: info.disruption },
    { key: 'cooldown', phase: info.cooldown },
  ].filter(p => p.phase != null);

  return (
    <div className="bg-slate-800/60 border-b border-slate-700 px-4 py-2">
      <div className="flex items-start gap-4 flex-wrap">
        {/* Episode identity */}
        <div className="flex items-center gap-2 shrink-0">
          <span className="text-xs font-mono text-slate-400">Episode</span>
          <span className="text-sm font-semibold text-white">{info.episode}</span>
          <span className="text-xs text-slate-500">#{info.cycle}</span>
          <span className={`text-xs px-1.5 py-0.5 rounded font-medium ${info.success ? 'bg-green-900/60 text-green-400' : 'bg-red-900/60 text-red-400'}`}>
            {info.success ? '✓' : '✗'}
          </span>
        </div>

        {/* App name */}
        <div className="flex items-center gap-1 shrink-0">
          <span className="text-xs text-slate-400">App</span>
          <span className="text-sm text-slate-200">{info.scenario.app_name}</span>
        </div>

        {/* Description (truncated, expandable) */}
        <div className="flex-1 min-w-0">
          <button
            onClick={() => setExpanded(e => !e)}
            className="text-left w-full group"
          >
            {expanded ? (
              <span className="text-xs text-slate-300 leading-relaxed">{info.scenario.long_description || info.scenario.description}</span>
            ) : (
              <span className="text-xs text-slate-400 truncate block group-hover:text-slate-300 transition-colors">
                {info.scenario.description}
                <span className="ml-1 text-slate-500">[+]</span>
              </span>
            )}
          </button>
        </div>

        {/* Phase timeline */}
        {phases.length > 0 && (
          <div className="flex items-center gap-1 shrink-0">
            {phases.map(({ key, phase }) => {
              if (!phase) return null;
              const style = PHASE_STYLES[key] ?? { bg: 'bg-slate-700', border: 'border-slate-500', label: key };
              return (
                <div
                  key={key}
                  className={`flex items-center gap-1 px-2 py-0.5 rounded border text-xs ${style.bg} ${style.border}`}
                >
                  <span className="text-slate-300 font-medium">{style.label}</span>
                  <span className="font-mono text-slate-400">{formatTime(phase.start)}</span>
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}

function f1Color(f1: number): string {
  if (f1 >= 0.7) return 'text-green-400';
  if (f1 >= 0.4) return 'text-yellow-400';
  return 'text-red-400';
}

function ScoreDisplay({ score, sigma, onSigmaChange }: {
  score: ScoreResult;
  sigma: number;
  onSigmaChange: (s: number) => void;
}) {
  return (
    <div className="flex items-center gap-3 bg-slate-700/50 rounded px-3 py-1.5">
      <div className="flex items-center gap-1.5">
        <span className="text-xs text-slate-400">F1</span>
        <span className={`text-sm font-bold font-mono ${f1Color(score.f1)}`}>
          {score.f1.toFixed(3)}
        </span>
      </div>
      <div className="flex items-center gap-1.5">
        <span className="text-xs text-slate-400">P</span>
        <span className="text-sm font-mono text-slate-200">{score.precision.toFixed(3)}</span>
      </div>
      <div className="flex items-center gap-1.5">
        <span className="text-xs text-slate-400">&sigma;</span>
        <input
          type="range"
          min={5}
          max={60}
          step={5}
          value={sigma}
          onChange={e => onSigmaChange(Number(e.target.value))}
          className="w-16 h-1 accent-purple-500"
        />
        <span className="text-xs font-mono text-slate-400 w-6">{sigma}s</span>
      </div>
    </div>
  );
}

function ScoreUnavailable({ reason }: { reason: string }) {
  return (
    <div className="flex items-center gap-2 bg-slate-700/50 rounded px-3 py-1.5">
      <span className="text-xs text-slate-500">Score: {reason}</span>
    </div>
  );
}

function App() {
  const [state, actions] = useObserver();
  const [activeTab, setActiveTab] = useState<TabID>('timeseries');
  const [sidebarWidth, setSidebarWidth] = useState(320);
  const [smoothLines, setSmoothLines] = useState(true);
  const [sigma, setSigma] = useState(30);

  const handleSigmaChange = useCallback((newSigma: number) => {
    setSigma(newSigma);
    actions.fetchScore(newSigma);
  }, [actions]);
  const isResizingRef = useRef(false);

  // Time range with navigation history
  const [timeRangeLive, setTimeRangeLive] = useState<TimeRange | null>(null);
  const [nav, setNav] = useState<{ history: (TimeRange | null)[]; index: number }>({
    history: [null],
    index: 0,
  });
  const commitTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const timeRange = timeRangeLive;
  const canGoBack = nav.index > 0;
  const canGoForward = nav.index < nav.history.length - 1;

  // Live update + debounced history commit (used by panning and zoom)
  const setTimeRange = (range: TimeRange | null) => {
    setTimeRangeLive(range);
    if (commitTimerRef.current) clearTimeout(commitTimerRef.current);
    commitTimerRef.current = setTimeout(() => {
      setNav(prev => {
        const truncated = prev.history.slice(0, prev.index + 1);
        return { history: [...truncated, range], index: truncated.length };
      });
    }, 350);
  };

  // Immediate commit (used by editable timestamps and reset)
  const commitTimeRange = (range: TimeRange | null) => {
    if (commitTimerRef.current) clearTimeout(commitTimerRef.current);
    setTimeRangeLive(range);
    setNav(prev => {
      const truncated = prev.history.slice(0, prev.index + 1);
      return { history: [...truncated, range], index: truncated.length };
    });
  };

  const goBack = () => {
    if (commitTimerRef.current) clearTimeout(commitTimerRef.current);
    setNav(prev => {
      if (prev.index <= 0) return prev;
      const newIndex = prev.index - 1;
      setTimeRangeLive(prev.history[newIndex]);
      return { ...prev, index: newIndex };
    });
  };

  const goForward = () => {
    if (commitTimerRef.current) clearTimeout(commitTimerRef.current);
    setNav(prev => {
      if (prev.index >= prev.history.length - 1) return prev;
      const newIndex = prev.index + 1;
      setTimeRangeLive(prev.history[newIndex]);
      return { ...prev, index: newIndex };
    });
  };

  const series = state.series ?? [];
  const anomalies = state.anomalies ?? [];
  const scenarioTimeRange = useMemo<TimeRange | null>(() => {
    const start = state.status?.scenarioStart;
    const end = state.status?.scenarioEnd;
    if (start == null || end == null || end <= start) return null;
    return { start, end };
  }, [state.status?.scenarioStart, state.status?.scenarioEnd]);

  // Episode info from JSON (optional)
  const episodeInfo = state.status?.episodeInfo ?? null;
  const episodeTimeRange = useMemo<TimeRange | null>(() => {
    if (!episodeInfo?.start_time || !episodeInfo?.end_time) return null;
    const start = new Date(episodeInfo.start_time).getTime() / 1000;
    const end = new Date(episodeInfo.end_time).getTime() / 1000;
    if (isNaN(start) || isNaN(end) || end <= start) return null;
    return { start, end };
  }, [episodeInfo?.start_time, episodeInfo?.end_time]);

  // Sigma window: center on disruption start for scoring visualization
  const sigmaWindowCenter = useMemo(() => {
    if (!episodeInfo?.disruption?.start) return null;
    const ts = new Date(episodeInfo.disruption.start).getTime() / 1000;
    return isNaN(ts) ? null : ts;
  }, [episodeInfo]);

  const sigmaWindow = sigmaWindowCenter != null ? { center: sigmaWindowCenter, sigma } : null;

  // Phase markers derived from episode info — dotted lines on all charts
  const phaseMarkers = useMemo<PhaseMarker[]>(() => {
    if (!episodeInfo) return [];
    const defs = [
      { key: 'warmup',     label: 'Warmup',     phase: episodeInfo.warmup,     color: '#3b82f6' },
      { key: 'baseline',   label: 'Baseline',   phase: episodeInfo.baseline,   color: '#22c55e' },
      { key: 'disruption', label: 'Disruption', phase: episodeInfo.disruption, color: '#ef4444' },
      { key: 'cooldown',   label: 'Cooldown',   phase: episodeInfo.cooldown,   color: '#f59e0b' },
    ];
    const markers: PhaseMarker[] = [];
    for (const { key, label, phase, color } of defs) {
      if (!phase?.start) continue;
      const ts = new Date(phase.start).getTime() / 1000;
      if (!isNaN(ts)) markers.push({ key, label, timestamp: ts, color });
    }
    return markers;
  }, [episodeInfo]);

  // When the active scenario changes, reset zoom (episode range effect will re-apply it)
  const prevScenarioRef = useRef<string | null | undefined>(undefined);
  const appliedEpisodeRef = useRef<string | null>(null);
  useEffect(() => {
    if (state.activeScenario === prevScenarioRef.current) return;
    prevScenarioRef.current = state.activeScenario;
    appliedEpisodeRef.current = null; // Allow episode range to be re-applied
    setTimeRangeLive(null);
    setNav({ history: [null], index: 0 });
  }, [state.activeScenario]);

  // Once episode info arrives for a freshly loaded scenario, apply its time range as default
  useEffect(() => {
    if (!episodeTimeRange || !episodeInfo) return;
    const id = episodeInfo.execution_id || episodeInfo.episode;
    if (appliedEpisodeRef.current === id) return;
    appliedEpisodeRef.current = id;
    commitTimeRange(episodeTimeRange);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [episodeTimeRange, episodeInfo]);

  const activeTimeRange = timeRange ?? scenarioTimeRange;

  // Sidebar resize handlers
  const handleResizeStart = (e: React.MouseEvent) => {
    e.preventDefault();
    isResizingRef.current = true;
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
  };

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (!isResizingRef.current) return;
      const newWidth = Math.max(200, Math.min(600, e.clientX));
      setSidebarWidth(newWidth);
    };

    const handleMouseUp = () => {
      if (isResizingRef.current) {
        isResizingRef.current = false;
        document.body.style.cursor = '';
        document.body.style.userSelect = '';
      }
    };

    window.addEventListener('mousemove', handleMouseMove);
    window.addEventListener('mouseup', handleMouseUp);

    return () => {
      window.removeEventListener('mousemove', handleMouseMove);
      window.removeEventListener('mouseup', handleMouseUp);
    };
  }, []);

  return (
    <div className="h-screen flex flex-col overflow-hidden">
      {/* Header */}
      <header className="bg-slate-800 border-b border-slate-700 px-4 py-3">
        <div className="flex justify-between items-center">
          <div className="flex items-center gap-6">
            <h1 className="text-lg font-semibold text-white">Observer Test Bench</h1>
            {/* Tab bar */}
            <div className="flex gap-1">
              <button
                onClick={() => setActiveTab('timeseries')}
                className={`px-3 py-1.5 rounded text-sm transition-colors ${
                  activeTab === 'timeseries'
                    ? 'bg-purple-600 text-white'
                    : 'text-slate-400 hover:bg-slate-700'
                }`}
              >
                Time Series
              </button>
              <button
                onClick={() => setActiveTab('correlators')}
                className={`px-3 py-1.5 rounded text-sm transition-colors ${
                  activeTab === 'correlators'
                    ? 'bg-purple-600 text-white'
                    : 'text-slate-400 hover:bg-slate-700'
                }`}
              >
                Correlators
              </button>
              <button
                onClick={() => setActiveTab('logs')}
                className={`px-3 py-1.5 rounded text-sm transition-colors ${
                  activeTab === 'logs'
                    ? 'bg-purple-600 text-white'
                    : 'text-slate-400 hover:bg-slate-700'
                }`}
              >
                Logs
              </button>
            </div>
          </div>
          <div className="flex items-center gap-4">
            {/* Score display */}
            {state.scoreResponse?.available && state.scoreResponse.score && (
              <ScoreDisplay
                score={state.scoreResponse.score}
                sigma={sigma}
                onSigmaChange={handleSigmaChange}
              />
            )}
            {state.scoreResponse && !state.scoreResponse.available && state.connectionState === 'ready' && (
              <ScoreUnavailable reason={state.scoreResponse.reason || 'no ground truth available'} />
            )}
            {/* History navigation arrows — always visible when there's history */}
            {(canGoBack || canGoForward) && (
              <div className="flex items-center gap-1">
                <button
                  onClick={goBack}
                  disabled={!canGoBack}
                  title="Go to previous time selection"
                  className={`px-2 py-1 rounded text-sm transition-colors ${
                    canGoBack
                      ? 'text-slate-300 hover:bg-slate-700 hover:text-white'
                      : 'text-slate-600 cursor-default'
                  }`}
                >
                  ←
                </button>
                <button
                  onClick={goForward}
                  disabled={!canGoForward}
                  title="Go to next time selection"
                  className={`px-2 py-1 rounded text-sm transition-colors ${
                    canGoForward
                      ? 'text-slate-300 hover:bg-slate-700 hover:text-white'
                      : 'text-slate-600 cursor-default'
                  }`}
                >
                  →
                </button>
              </div>
            )}
            {/* Global time span control (available in all tabs) */}
            {activeTimeRange && (
              <div className="flex items-center gap-2 bg-slate-700/50 rounded px-3 py-1.5">
                <span className="text-xs text-slate-400">Time Span:</span>
                <EditableTimestamp
                  value={activeTimeRange.start}
                  onChange={start => commitTimeRange({ start, end: activeTimeRange.end })}
                />
                <span className="text-xs text-slate-500">–</span>
                <EditableTimestamp
                  value={activeTimeRange.end}
                  onChange={end => commitTimeRange({ start: activeTimeRange.start, end })}
                />
                <span className="text-xs text-slate-500 ml-1">
                  (middle-drag or cmd+drag to pan)
                </span>
                {/* Two reset buttons: scenario range vs all data */}
                {episodeTimeRange && (
                  <button
                    onClick={() => commitTimeRange(episodeTimeRange)}
                    className="ml-1 text-xs px-2 py-0.5 bg-slate-600 hover:bg-purple-700 rounded text-slate-300"
                    title="Reset to scenario time range (from episode.json)"
                  >
                    Scenario
                  </button>
                )}
                <button
                  onClick={() => commitTimeRange(null)}
                  className="ml-1 text-xs px-2 py-0.5 bg-slate-600 hover:bg-slate-500 rounded text-slate-300"
                  title="Reset to full metrics/logs time range"
                >
                  All Data
                </button>
              </div>
            )}
            {!activeTimeRange && state.connectionState === 'ready' && (
              <span className="text-xs text-slate-500">
                Drag a chart to set time span · middle-drag or cmd+drag to pan
              </span>
            )}

            {/* Time Series-only chart rendering control */}
            {activeTab === 'timeseries' && (
              <label className="flex items-center gap-2 cursor-pointer">
                <span className="text-xs text-slate-400">Smooth</span>
                <button
                  onClick={() => setSmoothLines(!smoothLines)}
                  className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${
                    smoothLines ? 'bg-purple-600' : 'bg-slate-600'
                  }`}
                >
                  <span
                    className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white transition-transform ${
                      smoothLines ? 'translate-x-5' : 'translate-x-1'
                    }`}
                  />
                </button>
              </label>
            )}
            <ConnectionStatus state={state.connectionState} />
            {state.status && (
              <span className="text-sm text-slate-400">
                {series.length} series, {anomalies.length} anomalies
              </span>
            )}
          </div>
        </div>
      </header>

      {/* Episode info panel (shown when episode.json is present) */}
      {episodeInfo && <EpisodeInfoPanel info={episodeInfo} />}

      <div className="flex-1 flex relative min-h-0">
        {/* Resize handle */}
        <div
          className="absolute left-0 top-0 bottom-0 w-1 cursor-col-resize hover:bg-purple-500/50 active:bg-purple-500 z-10"
          style={{ left: sidebarWidth - 2 }}
          onMouseDown={handleResizeStart}
        />

        {/* Tab content - both tabs stay mounted to preserve state */}
        <div className={`flex-1 flex ${activeTab !== 'timeseries' ? 'hidden' : ''}`}>
          <MetricsView
            state={state}
            actions={actions}
            sidebarWidth={sidebarWidth}
            timeRange={activeTimeRange}
            onTimeRangeChange={setTimeRange}
            smoothLines={smoothLines}
            phaseMarkers={phaseMarkers}
          />
        </div>
        <div className={`flex-1 flex ${activeTab !== 'correlators' ? 'hidden' : ''}`}>
          <CorrelatorView
            state={state}
            actions={actions}
            sidebarWidth={sidebarWidth}
            timeRange={activeTimeRange}
            phaseMarkers={phaseMarkers}
            sigmaWindow={sigmaWindow}
          />
        </div>
        <div className={`flex-1 flex ${activeTab !== 'logs' ? 'hidden' : ''}`}>
          <LogView
            state={state}
            actions={actions}
            sidebarWidth={sidebarWidth}
            timeRange={activeTimeRange}
            onTimeRangeChange={setTimeRange}
            phaseMarkers={phaseMarkers}
          />
        </div>
      </div>
    </div>
  );
}

export default App;
