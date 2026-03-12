import { useState, useEffect, useCallback, useRef } from 'react';
import { api } from '../api/client';
import type {
  StatusResponse, ScenarioInfo, ComponentInfo, SeriesInfo, Anomaly, LogAnomaly, LogEntry, LogsSummary, Correlation,
  CompressedGroup, ComponentDataResponse, CorrelatorStats, ReplayProgress
} from '../api/client';

export type ConnectionState = 'disconnected' | 'connected' | 'loading' | 'ready';

export interface ObserverState {
  connectionState: ConnectionState;
  status: StatusResponse | null;
  scenarios: ScenarioInfo[];
  components: ComponentInfo[];
  series: SeriesInfo[];
  anomalies: Anomaly[];
  logs: LogEntry[];
  logsSummary: LogsSummary | null;
  logAnomalies: LogAnomaly[];
  correlations: Correlation[];
  componentData: Map<string, ComponentDataResponse>;
  compressedGroups: CompressedGroup[];
  correlatorStats: CorrelatorStats | null;
  scenarioDataVersion: number;
  activeScenario: string | null;
  error: string | null;
  loadProgress: ReplayProgress | null;
}

export interface ObserverActions {
  loadScenario: (name: string) => Promise<void>;
  refresh: () => Promise<void>;
  toggleComponent: (name: string) => Promise<void>;
}

export function useObserver(): [ObserverState, ObserverActions] {
  const [connectionState, setConnectionState] = useState<ConnectionState>('disconnected');
  const [status, setStatus] = useState<StatusResponse | null>(null);
  const [scenarios, setScenarios] = useState<ScenarioInfo[]>([]);
  const [components, setComponents] = useState<ComponentInfo[]>([]);
  const [series, setSeries] = useState<SeriesInfo[]>([]);
  const [anomalies, setAnomalies] = useState<Anomaly[]>([]);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [logsSummary, setLogsSummary] = useState<LogsSummary | null>(null);
  const [logAnomalies, setLogAnomalies] = useState<LogAnomaly[]>([]);
  const [correlations, setCorrelations] = useState<Correlation[]>([]);
  const [componentData, setComponentData] = useState<Map<string, ComponentDataResponse>>(new Map());
  const [compressedGroups, setCompressedGroups] = useState<CompressedGroup[]>([]);
  const [correlatorStats, setCorrelatorStats] = useState<CorrelatorStats | null>(null);
  const [scenarioDataVersion, setScenarioDataVersion] = useState(0);
  const [activeScenario, setActiveScenario] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loadProgress, setLoadProgress] = useState<ReplayProgress | null>(null);

  const fetchingRef = useRef(false);
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const clearRetryTimer = useCallback(() => {
    if (retryTimerRef.current) {
      clearTimeout(retryTimerRef.current);
      retryTimerRef.current = null;
    }
  }, []);

  const scheduleScenarioRetry = useCallback((delayMs = 3000) => {
    if (retryTimerRef.current) return;
    retryTimerRef.current = setTimeout(() => {
      retryTimerRef.current = null;
      fetchScenarioDataRef.current();
    }, delayMs);
  }, []);

  const fetchScenarioData = useCallback(async () => {
    if (fetchingRef.current) return null;
    fetchingRef.current = true;
    try {
      const [
        componentsData, anomaliesData, logsSummaryData, logAnomaliesData,
        correlationsData, compressedGroupsData, statsData, seriesData
      ] = await Promise.all([
        api.getComponents(),
        api.getAnomalies(),
        api.getLogsSummary(),
        api.getLogAnomalies(),
        api.getCorrelations(),
        api.getCompressedCorrelations(),
        api.getStats(),
        api.getSeries(),
      ]);

      const componentNames = componentsData.map((c: ComponentInfo) => c.name);
      const componentDataResults = await Promise.all(
        componentNames.map(async (name: string) => {
          try {
            const data = await api.getComponentData(name);
            return [name, data] as [string, ComponentDataResponse];
          } catch {
            return [name, { enabled: false, data: null }] as [string, ComponentDataResponse];
          }
        })
      );

      setComponents(componentsData);
      setSeries(seriesData);
      setAnomalies(anomaliesData);
      setLogs([]);
      setLogsSummary(logsSummaryData);
      setLogAnomalies(logAnomaliesData);
      setCorrelations(correlationsData);
      setCompressedGroups(compressedGroupsData);
      setComponentData(new Map(componentDataResults));
      setCorrelatorStats(statsData);
      setScenarioDataVersion((v) => v + 1);
      clearRetryTimer();
      setError(null);
      return true;
    } catch (e) {
      console.error('fetchScenarioData failed:', e);
      setError(e instanceof Error ? e.message : 'Failed to refresh scenario data');
      scheduleScenarioRetry();
      return false;
    } finally {
      fetchingRef.current = false;
    }
  }, [clearRetryTimer, scheduleScenarioRetry]);

  // Stable ref so SSE listener always calls the latest fetchScenarioData without re-subscribing.
  const fetchScenarioDataRef = useRef(fetchScenarioData);
  fetchScenarioDataRef.current = fetchScenarioData;

  // Reconciliation: after a mutating action, if no SSE status arrives within
  // this timeout, fall back to a direct fetch so the UI never stays stale.
  const reconcile = useCallback(async () => {
    try {
      const statusData = await api.getStatus();
      setStatus(statusData);
      setError(null);
      if (statusData.ready && statusData.scenario) {
        setConnectionState('ready');
        setActiveScenario(statusData.scenario);
        setLoadProgress(null);
        const ok = await fetchScenarioData();
        if (ok === false) {
          scheduleScenarioRetry();
        }
      } else if (statusData.scenario) {
        setConnectionState('loading');
      } else {
        setConnectionState('connected');
      }
    } catch (e) {
      console.error('reconcile failed:', e);
      setError(e instanceof Error ? e.message : 'Failed to reconcile observer state');
      scheduleScenarioRetry();
    }
  }, [fetchScenarioData, scheduleScenarioRetry]);

  // SSE connection — single persistent stream replaces all polling.
  useEffect(() => {
    const es = new EventSource('/api/events');

    es.addEventListener('status', (e: MessageEvent) => {
      const statusData: StatusResponse = JSON.parse(e.data);
      setStatus(statusData);
      setError(null);

      if (statusData.ready && statusData.scenario) {
        setConnectionState('ready');
        setActiveScenario(statusData.scenario);
        setLoadProgress(null);
        fetchScenarioDataRef.current();
      } else if (statusData.scenario) {
        setConnectionState('loading');
        setActiveScenario(statusData.scenario);
      } else {
        setConnectionState('connected');
      }
    });

    es.addEventListener('progress', (e: MessageEvent) => {
      const progress: ReplayProgress = JSON.parse(e.data);
      setLoadProgress(progress);
    });

    es.onopen = () => {
      setError(null);
    };

    es.onerror = () => {
      setConnectionState('disconnected');
      setError('Connection lost. Reconnecting...');
      // EventSource auto-reconnects; on reconnect the hub replays latest status.
    };

    // Fetch scenarios once (rarely change, not worth pushing via SSE).
    api.getScenarios().then(setScenarios).catch(() => {});

    return () => {
      clearRetryTimer();
      es.close();
    };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const loadScenario = useCallback(async (name: string) => {
    setConnectionState('loading');
    setActiveScenario(name);
    setSeries([]);
    setLoadProgress(null);
    setError(null);

    try {
      await api.loadScenario(name);
      // SSE status event normally handles the transition to 'ready'.
      // Fall back to direct fetch in case the SSE event was missed.
      await reconcile();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load scenario');
      setConnectionState('connected');
    }
  }, [reconcile]);

  const refresh = useCallback(async () => {
    await fetchScenarioData();
  }, [fetchScenarioData]);

  const toggleComponent = useCallback(async (name: string) => {
    try {
      await api.toggleComponent(name);
      // SSE status event normally triggers refresh.
      // Fall back to direct fetch in case the SSE event was missed.
      await reconcile();
    } catch (e) {
      console.error('Failed to toggle component:', e);
      setError(e instanceof Error ? e.message : 'Failed to toggle component');
    }
  }, [reconcile]);

  return [
    {
      connectionState,
      status,
      scenarios,
      components,
      series,
      anomalies,
      logs,
      logsSummary,
      logAnomalies,
      correlations,
      componentData,
      compressedGroups,
      correlatorStats,
      scenarioDataVersion,
      activeScenario,
      error,
      loadProgress,
    },
    {
      loadScenario,
      refresh,
      toggleComponent,
    },
  ];
}
