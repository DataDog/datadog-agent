import { useState, useEffect, useCallback, useRef } from 'react';
import { api } from '../api/client';
import type {
  StatusResponse, ScenarioInfo, ComponentInfo, SeriesInfo, Anomaly, LogAnomaly, LogEntry, Correlation,
  CompressedGroup, CorrelatorDataResponse, CorrelatorStats
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
  logAnomalies: LogAnomaly[];
  correlations: Correlation[];
  // Generic correlator data keyed by correlator name
  correlatorData: Map<string, CorrelatorDataResponse>;
  compressedGroups: CompressedGroup[];
  correlatorStats: CorrelatorStats | null;
  activeScenario: string | null;
  error: string | null;
}

export interface ObserverActions {
  loadScenario: (name: string) => Promise<void>;
  refresh: () => Promise<void>;
  toggleComponent: (name: string) => Promise<void>;
}

const POLL_INTERVAL = 2000;

export function useObserver(): [ObserverState, ObserverActions] {
  const [connectionState, setConnectionState] = useState<ConnectionState>('disconnected');
  const [status, setStatus] = useState<StatusResponse | null>(null);
  const [scenarios, setScenarios] = useState<ScenarioInfo[]>([]);
  const [components, setComponents] = useState<ComponentInfo[]>([]);
  const [series, setSeries] = useState<SeriesInfo[]>([]);
  const [anomalies, setAnomalies] = useState<Anomaly[]>([]);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [logAnomalies, setLogAnomalies] = useState<LogAnomaly[]>([]);
  const [correlations, setCorrelations] = useState<Correlation[]>([]);
  const [correlatorData, setCorrelatorData] = useState<Map<string, CorrelatorDataResponse>>(new Map());
  const [compressedGroups, setCompressedGroups] = useState<CompressedGroup[]>([]);
  const [correlatorStats, setCorrelatorStats] = useState<CorrelatorStats | null>(null);
  const [activeScenario, setActiveScenario] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Use refs to avoid recreating callbacks on every state change
  const connectionStateRef = useRef(connectionState);
  const activeScenarioRef = useRef(activeScenario);

  useEffect(() => {
    connectionStateRef.current = connectionState;
  }, [connectionState]);

  useEffect(() => {
    activeScenarioRef.current = activeScenario;
  }, [activeScenario]);

  const fetchAll = useCallback(async (): Promise<boolean> => {
    try {
      // First fetch components and basic data
      const [
        statusData, scenariosData, componentsData, seriesData,
        anomaliesData, logsData, logAnomaliesData, correlationsData, compressedGroupsData, statsData
      ] = await Promise.all([
          api.getStatus(),
          api.getScenarios(),
          api.getComponents(),
          api.getSeries(),
          api.getAnomalies(),
          api.getLogs(),
          api.getLogAnomalies(),
          api.getCorrelations(),
          api.getCompressedCorrelations(),
          api.getStats(),
        ]);

      // Discover correlator names from components and fetch their data
      const correlatorNames = componentsData
        .filter((c: ComponentInfo) => c.category === 'correlator')
        .map((c: ComponentInfo) => c.name);

      const correlatorResults = await Promise.all(
        correlatorNames.map(async (name: string) => {
          try {
            const data = await api.getCorrelatorData(name);
            return [name, data] as [string, CorrelatorDataResponse];
          } catch {
            return [name, { enabled: false, data: null }] as [string, CorrelatorDataResponse];
          }
        })
      );

      setStatus(statusData);
      setScenarios(scenariosData);
      setComponents(componentsData);
      setSeries(seriesData);
      setAnomalies(anomaliesData);
      setLogs(logsData);
      setLogAnomalies(logAnomaliesData);
      setCorrelations(correlationsData);
      setCompressedGroups(compressedGroupsData);
      setCorrelatorData(new Map(correlatorResults));
      setCorrelatorStats(statsData);
      setError(null);

      if (statusData.ready && statusData.scenario) {
        setConnectionState('ready');
        setActiveScenario(statusData.scenario);
      } else {
        setConnectionState('connected');
      }
      return true;
    } catch (e) {
      console.error('fetchAll failed:', e);
      return false;
    }
  }, []);

  const poll = useCallback(async () => {
    const currentState = connectionStateRef.current;
    const currentScenario = activeScenarioRef.current;

    try {
      const statusData = await api.getStatus();

      // We just reconnected
      if (currentState === 'disconnected') {
        // If we had an active scenario and it's not loaded, reload it
        if (currentScenario && statusData.scenario !== currentScenario) {
          console.log(`Reconnected - reloading scenario: ${currentScenario}`);
          setConnectionState('loading');
          try {
            await api.loadScenario(currentScenario);
          } catch (e) {
            console.error('Failed to reload scenario:', e);
          }
        }

        // Fetch all data
        await fetchAll();
      } else if (currentState === 'loading') {
        // Check if loading is complete
        if (statusData.ready) {
          await fetchAll();
        }
      } else {
        // Normal polling - just update status and data
        setStatus(statusData);
        if (statusData.ready !== (currentState === 'ready')) {
          await fetchAll();
        }
      }

      setError(null);
    } catch (e) {
      if (currentState !== 'disconnected') {
        console.log('Connection lost');
      }
      setConnectionState('disconnected');
      setError('Unable to connect to observer');
    }
  }, [fetchAll]);

  useEffect(() => {
    // Initial poll
    poll();

    // Set up polling interval
    const interval = setInterval(poll, POLL_INTERVAL);
    return () => clearInterval(interval);
  }, [poll]);

  const loadScenario = useCallback(async (name: string) => {
    setConnectionState('loading');
    setActiveScenario(name);
    setError(null);

    try {
      await api.loadScenario(name);
      await fetchAll();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load scenario');
      setConnectionState('connected');
    }
  }, [fetchAll]);

  const refresh = useCallback(async () => {
    await fetchAll();
  }, [fetchAll]);

  const toggleComponent = useCallback(async (name: string) => {
    try {
      await api.toggleComponent(name);
      await fetchAll();
    } catch (e) {
      console.error('Failed to toggle component:', e);
      setError(e instanceof Error ? e.message : 'Failed to toggle component');
    }
  }, [fetchAll]);

  return [
    {
      connectionState,
      status,
      scenarios,
      components,
      series,
      anomalies,
      logs,
      logAnomalies,
      correlations,
      correlatorData,
      compressedGroups,
      correlatorStats,
      activeScenario,
      error,
    },
    {
      loadScenario,
      refresh,
      toggleComponent,
    },
  ];
}
