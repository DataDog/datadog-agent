import { useState, useEffect, useCallback, useRef } from 'react';
import { api } from '../api/client';
import type { StatusResponse, ScenarioInfo, ComponentInfo, SeriesInfo, Anomaly, Correlation } from '../api/client';

export type ConnectionState = 'disconnected' | 'connected' | 'loading' | 'ready';

export interface ObserverState {
  connectionState: ConnectionState;
  status: StatusResponse | null;
  scenarios: ScenarioInfo[];
  components: ComponentInfo[];
  series: SeriesInfo[];
  anomalies: Anomaly[];
  correlations: Correlation[];
  activeScenario: string | null;
  error: string | null;
}

export interface ObserverActions {
  loadScenario: (name: string) => Promise<void>;
  refresh: () => Promise<void>;
}

const POLL_INTERVAL = 2000;

export function useObserver(): [ObserverState, ObserverActions] {
  const [connectionState, setConnectionState] = useState<ConnectionState>('disconnected');
  const [status, setStatus] = useState<StatusResponse | null>(null);
  const [scenarios, setScenarios] = useState<ScenarioInfo[]>([]);
  const [components, setComponents] = useState<ComponentInfo[]>([]);
  const [series, setSeries] = useState<SeriesInfo[]>([]);
  const [anomalies, setAnomalies] = useState<Anomaly[]>([]);
  const [correlations, setCorrelations] = useState<Correlation[]>([]);
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
      const [statusData, scenariosData, componentsData, seriesData, anomaliesData, correlationsData] =
        await Promise.all([
          api.getStatus(),
          api.getScenarios(),
          api.getComponents(),
          api.getSeries(),
          api.getAnomalies(),
          api.getCorrelations(),
        ]);

      setStatus(statusData);
      setScenarios(scenariosData);
      setComponents(componentsData);
      setSeries(seriesData);
      setAnomalies(anomaliesData);
      setCorrelations(correlationsData);
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

  return [
    {
      connectionState,
      status,
      scenarios,
      components,
      series,
      anomalies,
      correlations,
      activeScenario,
      error,
    },
    {
      loadScenario,
      refresh,
    },
  ];
}
