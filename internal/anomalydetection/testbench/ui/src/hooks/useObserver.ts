import { useState, useEffect, useCallback, useRef } from 'react';
import { api } from '../api/client';
import type {
  StatusResponse, ScenarioInfo, ComponentInfo, SeriesInfo, Anomaly, LogAnomaly, LogEntry, LogsSummary, Correlation,
  CompressedGroup, ComponentDataResponse, CorrelatorStats, ReplayProgress, ReportEvent
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
  reports: ReportEvent[];
  componentData: Map<string, ComponentDataResponse>;
  compressedGroups: CompressedGroup[];
  correlatorStats: CorrelatorStats | null;
  scenarioDataVersion: number;
  activeScenario: string | null;
  error: string | null;
  loadProgress: ReplayProgress | null;
  pendingComponents: Set<string>;
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
  const [reports, setReports] = useState<ReportEvent[]>([]);
  const [componentData, setComponentData] = useState<Map<string, ComponentDataResponse>>(new Map());
  const [compressedGroups, setCompressedGroups] = useState<CompressedGroup[]>([]);
  const [correlatorStats, setCorrelatorStats] = useState<CorrelatorStats | null>(null);
  const [scenarioDataVersion, setScenarioDataVersion] = useState(0);
  const [activeScenario, setActiveScenario] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loadProgress, setLoadProgress] = useState<ReplayProgress | null>(null);

  const [pendingComponents, setPendingComponents] = useState<Set<string>>(new Set());
  const pendingComponentsRef = useRef(new Set<string>());
  const metadataReadyRef = useRef(false);
  const scenarioRef = useRef<string | null>(null);
  const generationRef = useRef(0);
  const fetchingRef = useRef<{ generation: number; promise: Promise<boolean> } | null>(null);
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

  const fetchScenarioData = useCallback(() => {
    const generation = generationRef.current;
    if (fetchingRef.current?.generation === generation) return fetchingRef.current.promise;
    const promise = (async () => {
      try {
        const [
          componentsData, anomaliesData, logsSummaryData, logAnomaliesData,
          correlationsData, reportsData, compressedGroupsData, statsData, seriesData
        ] = await Promise.all([
          api.getComponents(),
          api.getAnomalies(),
          api.getLogsSummary(),
          api.getLogAnomalies(),
          api.getCorrelations(),
          api.getReports(),
          api.getCompressedCorrelations(),
          api.getStats(),
          api.getSeries(),
        ]);

        if (generation !== generationRef.current) return false;
        const componentNames = (componentsData ?? []).map((c: ComponentInfo) => c.name);
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

        if (generation !== generationRef.current) return false;
        setComponents(componentsData ?? []);
        setSeries(seriesData ?? []);
        setAnomalies(anomaliesData ?? []);
        setLogs([]);
        setLogsSummary(logsSummaryData);
        setLogAnomalies(logAnomaliesData ?? []);
        setCorrelations(correlationsData ?? []);
        setReports(reportsData ?? []);
        setCompressedGroups(compressedGroupsData ?? []);
        setComponentData(new Map(componentDataResults));
        setCorrelatorStats(statsData);
        setScenarioDataVersion((v) => v + 1);
        clearRetryTimer();
        metadataReadyRef.current = true;
        setConnectionState('ready');
        setLoadProgress(null);
        setError(null);
        return true;
      } catch (e) {
        if (generation !== generationRef.current) return false;
        console.error('fetchScenarioData failed:', e);
        setError(e instanceof Error ? e.message : 'Failed to refresh scenario data');
        scheduleScenarioRetry();
        return false;
      } finally {
        if (fetchingRef.current?.generation === generation) fetchingRef.current = null;
      }
    })();
    fetchingRef.current = { generation, promise };
    return promise;
  }, [clearRetryTimer, scheduleScenarioRetry]);

  // Stable ref so SSE listener always calls the latest fetchScenarioData without re-subscribing.
  const fetchScenarioDataRef = useRef(fetchScenarioData);
  fetchScenarioDataRef.current = fetchScenarioData;

  const adoptStatus = useCallback((statusData: StatusResponse) => {
    if (scenarioRef.current !== statusData.scenario || (!statusData.ready && metadataReadyRef.current)) {
      generationRef.current += 1;
      metadataReadyRef.current = false;
      scenarioRef.current = statusData.scenario;
    }
    setStatus(statusData);
    setActiveScenario(statusData.scenario || null);
    setError(null);
    if (statusData.ready && statusData.scenario) {
      setConnectionState(metadataReadyRef.current ? 'ready' : 'loading');
      setLoadProgress(null);
    } else {
      setConnectionState(statusData.scenario ? 'loading' : 'connected');
    }
  }, []);

  // Reconcile after mutations, sharing metadata requests with SSE updates.
  const reconcile = useCallback(async () => {
    try {
      const statusData = await api.getStatus();
      adoptStatus(statusData);
      if (statusData.ready && statusData.scenario) await fetchScenarioData();
    } catch (e) {
      console.error('reconcile failed:', e);
      setError(e instanceof Error ? e.message : 'Failed to reconcile observer state');
      scheduleScenarioRetry();
    }
  }, [adoptStatus, fetchScenarioData, scheduleScenarioRetry]);

  // SSE connection — single persistent stream replaces all polling.
  useEffect(() => {
    const es = new EventSource('/api/events');

    es.addEventListener('status', (e: MessageEvent) => {
      const statusData: StatusResponse = JSON.parse(e.data);
      adoptStatus(statusData);
      if (statusData.ready && statusData.scenario) fetchScenarioDataRef.current();
    });

    es.addEventListener('progress', (e: MessageEvent) => {
      const progress: ReplayProgress = JSON.parse(e.data);
      setLoadProgress(progress);
    });

    es.onopen = () => {
      setError(null);
    };

    es.onerror = () => {
      generationRef.current += 1;
      metadataReadyRef.current = false;
      clearRetryTimer();
      setConnectionState('disconnected');
      setError('Connection lost. Reconnecting...');
      // EventSource auto-reconnects; on reconnect the hub replays latest status.
    };

    // Fetch scenarios once (rarely change, not worth pushing via SSE).
    api.getScenarios().then(data => setScenarios(data ?? [])).catch(() => {});

    return () => {
      clearRetryTimer();
      es.close();
    };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const loadScenario = useCallback(async (name: string) => {
    generationRef.current += 1;
    scenarioRef.current = name;
    metadataReadyRef.current = false;
    clearRetryTimer();
    setConnectionState('loading');
    setActiveScenario(name);
    setSeries([]);
    setAnomalies([]);
    setLogAnomalies([]);
    setLogs([]);
    setLogsSummary(null);
    setReports([]);
    setCorrelations([]);
    setCompressedGroups([]);
    setComponentData(new Map());
    setScenarioDataVersion(0);
    setLoadProgress(null);
    setError(null);

    try {
      await api.loadScenario(name);
      // SSE status event normally handles the transition to 'ready'.
      // Fall back to direct fetch in case the SSE event was missed.
      await reconcile();
    } catch (e) {
      const message = e instanceof Error ? e.message : 'Failed to load scenario';
      await reconcile();
      setError(message);
    }
  }, [reconcile, clearRetryTimer]);

  const requestedScenario = useRef(new URLSearchParams(window.location.search).get('scenario') || import.meta.env.VITE_SCENARIO);
  useEffect(() => {
    const requested = requestedScenario.current;
    if (requested && (connectionState === 'connected' || connectionState === 'ready') && scenarios.some(s => s.name === requested)) {
      requestedScenario.current = null;
      if (activeScenario !== requested) void loadScenario(requested);
    }
  }, [connectionState, scenarios, activeScenario, loadScenario]);

  const refresh = useCallback(async () => {
    await fetchScenarioData();
  }, [fetchScenarioData]);

  const toggleComponent = useCallback(async (name: string) => {
    if (pendingComponentsRef.current.size > 0) return;
    pendingComponentsRef.current.add(name);
    setPendingComponents(new Set(pendingComponentsRef.current));
    try {
      await api.toggleComponent(name);
      // SSE status event normally triggers refresh.
      // Fall back to direct fetch in case the SSE event was missed.
      await reconcile();
    } catch (e) {
      console.error('Failed to toggle component:', e);
      setError(e instanceof Error ? e.message : 'Failed to toggle component');
    } finally {
      pendingComponentsRef.current.delete(name);
      setPendingComponents(new Set(pendingComponentsRef.current));
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
      reports,
      componentData,
      compressedGroups,
      correlatorStats,
      scenarioDataVersion,
      activeScenario,
      error,
      loadProgress,
      pendingComponents,
    },
    {
      loadScenario,
      refresh,
      toggleComponent,
    },
  ];
}
