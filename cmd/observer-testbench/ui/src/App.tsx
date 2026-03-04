import { useState, useEffect, useRef, useMemo } from 'react';
import { useObserver } from './hooks/useObserver';
import { MetricsView } from './components/MetricsView';
import { CorrelatorView } from './components/CorrelatorView';
import { LogView } from './components/LogView';

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

function App() {
  const [state, actions] = useObserver();
  const [activeTab, setActiveTab] = useState<TabID>('timeseries');
  const [sidebarWidth, setSidebarWidth] = useState(320);
  const [smoothLines, setSmoothLines] = useState(true);
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
    <div className="min-h-screen flex flex-col">
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
                {timeRange && (
                  <button
                    onClick={() => commitTimeRange(null)}
                    className="ml-2 text-xs px-2 py-0.5 bg-slate-600 hover:bg-slate-500 rounded text-slate-300"
                    title="Reset time span"
                  >
                    Reset
                  </button>
                )}
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

      <div className="flex-1 flex relative">
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
          />
        </div>
        <div className={`flex-1 flex ${activeTab !== 'correlators' ? 'hidden' : ''}`}>
          <CorrelatorView
            state={state}
            actions={actions}
            sidebarWidth={sidebarWidth}
            timeRange={activeTimeRange}
          />
        </div>
        <div className={`flex-1 flex ${activeTab !== 'logs' ? 'hidden' : ''}`}>
          <LogView
            state={state}
            actions={actions}
            sidebarWidth={sidebarWidth}
            timeRange={activeTimeRange}
            onTimeRangeChange={setTimeRange}
          />
        </div>
      </div>
    </div>
  );
}

export default App;
