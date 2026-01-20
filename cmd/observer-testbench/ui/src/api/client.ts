// API client for the observer test bench

const API_BASE = '/api';

export interface StatusResponse {
  ready: boolean;
  scenario: string | null;
  seriesCount: number;
  anomalyCount: number;
  componentCount: number;
}

export interface ScenarioInfo {
  name: string;
  path: string;
  hasParquet: boolean;
  hasLogs: boolean;
  hasEvents: boolean;
}

export interface ComponentInfo {
  name: string;
  type: 'log_processor' | 'ts_analysis' | 'anomaly_processor';
  description?: string;
}

export interface SeriesInfo {
  namespace: string;
  name: string;
  tags: string[];
  pointCount: number;
}

export interface Point {
  timestamp: number;
  value: number;
}

export interface AnomalyMarker {
  timestamp: number;
  analyzerName: string;
  title: string;
}

export interface SeriesData {
  namespace: string;
  name: string;
  tags: string[];
  points: Point[];
  anomalies: AnomalyMarker[];
}

export interface AnomalyDebugInfo {
  baselineStart: number;
  baselineEnd: number;
  baselineMean?: number;
  baselineMedian?: number;
  baselineStddev?: number;
  baselineMAD?: number;
  threshold: number;
  slackParam?: number;
  currentValue: number;
  deviationSigma: number;
  cusumValues?: number[];
}

export interface Anomaly {
  source: string;
  analyzerName: string;
  title: string;
  description: string;
  tags: string[];
  timestamp: number;
  debugInfo?: AnomalyDebugInfo;
}

export interface Correlation {
  pattern: string;
  title: string;
  signals: string[];
  anomalies: {
    source: string;
    title: string;
    description: string;
    timestamp: number;
  }[];
  firstSeen: number;
  lastUpdated: number;
}

class ApiClient {
  private async fetch<T>(path: string, options?: RequestInit): Promise<T> {
    const response = await fetch(`${API_BASE}${path}`, options);
    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: response.statusText }));
      throw new Error(error.error || 'API request failed');
    }
    return response.json();
  }

  async getStatus(): Promise<StatusResponse> {
    return this.fetch('/status');
  }

  async getScenarios(): Promise<ScenarioInfo[]> {
    return this.fetch('/scenarios');
  }

  async loadScenario(name: string): Promise<{ status: string; scenario: string }> {
    return this.fetch(`/scenarios/${encodeURIComponent(name)}/load`, {
      method: 'POST',
    });
  }

  async getComponents(): Promise<ComponentInfo[]> {
    return this.fetch('/components');
  }

  async getSeries(): Promise<SeriesInfo[]> {
    return this.fetch('/series');
  }

  async getSeriesData(namespace: string, name: string): Promise<SeriesData> {
    return this.fetch(`/series/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`);
  }

  async getAnomalies(analyzer?: string): Promise<Anomaly[]> {
    const params = analyzer ? `?analyzer=${encodeURIComponent(analyzer)}` : '';
    return this.fetch(`/anomalies${params}`);
  }

  async getCorrelations(): Promise<Correlation[]> {
    return this.fetch('/correlations');
  }
}

export const api = new ApiClient();
