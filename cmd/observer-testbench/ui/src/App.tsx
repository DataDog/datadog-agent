import { useState, useEffect, useRef, useMemo } from 'react';
import { useObserver } from './hooks/useObserver';
import { TSAnalysisView } from './components/TSAnalysisView';
import { CorrelatorView } from './components/CorrelatorView';
import { LogAnomalyView } from './components/LogAnomalyView';

type TabID = 'timeseries' | 'correlators' | 'log-anomalies';

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

function formatTimeRange(range: TimeRange): string {
  const formatTime = (ts: number) =>
    new Date(ts * 1000).toLocaleTimeString([], {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    });
  return `${formatTime(range.start)} - ${formatTime(range.end)}`;
}

function App() {
  const [state, actions] = useObserver();
  const [activeTab, setActiveTab] = useState<TabID>('timeseries');
  const [sidebarWidth, setSidebarWidth] = useState(320);
  const [timeRange, setTimeRange] = useState<TimeRange | null>(null);
  const [smoothLines, setSmoothLines] = useState(true);
  const isResizingRef = useRef(false);

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
                onClick={() => setActiveTab('log-anomalies')}
                className={`px-3 py-1.5 rounded text-sm transition-colors ${
                  activeTab === 'log-anomalies'
                    ? 'bg-purple-600 text-white'
                    : 'text-slate-400 hover:bg-slate-700'
                }`}
              >
                Log Anomalies
                {(state.status?.logAnomalyCount ?? 0) > 0 && (
                  <span className="ml-1.5 text-xs bg-orange-600/80 text-white px-1.5 py-0.5 rounded-full">
                    {state.status!.logAnomalyCount}
                  </span>
                )}
              </button>
            </div>
          </div>
          <div className="flex items-center gap-4">
            {/* Global time span control (available in both tabs) */}
            {activeTimeRange && (
              <div className="flex items-center gap-2 bg-slate-700/50 rounded px-3 py-1.5">
                <span className="text-xs text-slate-400">Time Span:</span>
                <span className="text-sm text-slate-200 font-mono">
                  {formatTimeRange(activeTimeRange)}
                </span>
                <span className="text-xs text-slate-500 ml-1">
                  (middle-drag or cmd+drag to pan)
                </span>
                {timeRange && (
                  <button
                    onClick={() => setTimeRange(null)}
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
                Drag a time-series chart to set time span, middle-drag or cmd+drag to pan
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
          <TSAnalysisView
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
        <div className={`flex-1 flex ${activeTab !== 'log-anomalies' ? 'hidden' : ''}`}>
          <LogAnomalyView
            state={state}
            actions={actions}
            sidebarWidth={sidebarWidth}
          />
        </div>
      </div>
    </div>
  );
}

export default App;
