// API client for the observer test bench

const API_BASE = '/api';

export type SeriesID = string & { readonly __seriesIdBrand: unique symbol };
export type MetricName = string & { readonly __metricNameBrand: unique symbol };

export interface ServerConfig {
  components: Record<string, boolean>;
  cusumSkipCount: boolean;  // true = filtering out :count metrics
}

export interface StatusResponse {
  ready: boolean;
  scenario: string | null;
  seriesCount: number;
  anomalyCount: number;
  logAnomalyCount: number;
  componentCount: number;
  correlatorsProcessing: boolean;
  scenarioStart?: number;
  scenarioEnd?: number;
  serverConfig: ServerConfig;
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
  displayName: string;
  category: 'analyzer' | 'correlator' | 'processing';
  enabled: boolean;
}

export interface SeriesInfo {
  id: SeriesID;
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
  analyzerComponent?: string;
  sourceSeriesId?: SeriesID;
  title: string;
}

export interface SeriesData {
  id: SeriesID;
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
  source: MetricName;
  sourceSeriesId?: SeriesID;
  analyzerName: string;
  analyzerComponent?: string;
  title: string;
  description: string;
  tags: string[];
  timestamp: number;
  debugInfo?: AnomalyDebugInfo;
}

// LogAnomaly is an anomaly emitted directly by a log processor (not via TS analysis).
export interface LogAnomaly {
  source: string;
  processorName: string;
  title: string;
  description: string;
  tags: string[];
  timestamp: number;
  score?: number;
}

export interface Correlation {
  pattern: string;
  title: string;
  memberSeriesIds: SeriesID[];
  metricNames: MetricName[];
  anomalies: {
    source: MetricName;
    title: string;
    description: string;
    timestamp: number;
  }[];
  firstSeen: number;
  lastUpdated: number;
}

// Lead-Lag edge represents temporal causality between sources
export interface LeadLagEdge {
  leader: SeriesID;
  follower: SeriesID;
  typical_lag: number;  // Seconds
  confidence: number;   // 0-1
  observations: number;
}

// Surprise edge represents unexpected co-occurrence (high lift)
export interface SurpriseEdge {
  source1: SeriesID;
  source2: SeriesID;
  lift: number;
  support: number;         // Number of co-occurrences
  source1_count: number;   // Total anomalies from source1
  source2_count: number;   // Total anomalies from source2
  is_surprising: boolean;  // true if lift > MinLift
}

// GraphSketch edge represents learned co-occurrence patterns
export interface GraphSketchEdge {
  Source1: SeriesID;
  Source2: SeriesID;
  EdgeKey: string;
  Observations: number;    // Raw count
  Frequency: number;       // Decay-weighted frequency
  FirstSeenUnix: number;
}

// Compressed group description from trie-based metric compression
export interface MetricPattern {
  pattern: string;
  matched: number;
  universe: number;
  precision: number;
}

export interface CompressedGroup {
  correlator: string;
  groupId: string;
  title: string;
  commonTags: Record<string, string>;
  patterns: MetricPattern[];
  memberSources: SeriesID[];
  seriesCount: number;
  precision: number;
  firstSeen?: number;
  lastUpdated?: number;
}

// Generic correlator data response
export interface CorrelatorDataResponse {
  enabled: boolean;
  data: unknown;
}

// Stats response from correlators
export interface CorrelatorStats {
  [key: string]: Record<string, unknown>;
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

  async toggleComponent(name: string): Promise<StatusResponse> {
    return this.fetch(`/components/${encodeURIComponent(name)}/toggle`, {
      method: 'POST',
    });
  }

  async getSeries(): Promise<SeriesInfo[]> {
    return this.fetch('/series');
  }

  async getSeriesData(namespace: string, name: string): Promise<SeriesData> {
    return this.fetch(`/series/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`);
  }

  async getSeriesDataByID(id: string): Promise<SeriesData> {
    return this.fetch(`/series/id/${encodeURIComponent(id)}`);
  }

  async getAnomalies(analyzer?: string): Promise<Anomaly[]> {
    const params = analyzer ? `?analyzer=${encodeURIComponent(analyzer)}` : '';
    return this.fetch(`/anomalies${params}`);
  }

  async getLogAnomalies(processor?: string): Promise<LogAnomaly[]> {
    const params = processor ? `?processor=${encodeURIComponent(processor)}` : '';
    return this.fetch(`/log-anomalies${params}`);
  }

  async getCorrelations(): Promise<Correlation[]> {
    return this.fetch('/correlations');
  }

  async getCorrelatorData(name: string): Promise<CorrelatorDataResponse> {
    return this.fetch(`/correlators/${encodeURIComponent(name)}`);
  }

  // Legacy endpoints (thin wrappers for backward compat)
  async getLeadLag(): Promise<{ enabled: boolean; edges: LeadLagEdge[] }> {
    return this.fetch('/leadlag');
  }

  async getSurprise(): Promise<{ enabled: boolean; edges: SurpriseEdge[] }> {
    return this.fetch('/surprise');
  }

  async getGraphSketch(): Promise<{ enabled: boolean; edges: GraphSketchEdge[] }> {
    return this.fetch('/graphsketch');
  }

  async getCompressedCorrelations(threshold?: number): Promise<CompressedGroup[]> {
    const params = threshold !== undefined ? `?threshold=${threshold}` : '';
    return this.fetch(`/correlations/compressed${params}`);
  }

  async getStats(): Promise<CorrelatorStats> {
    return this.fetch('/stats');
  }

  async updateConfig(config: {
    cusumSkipCount?: boolean;
    dedupEnabled?: boolean;
  }): Promise<StatusResponse> {
    return this.fetch('/config', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(config),
    });
  }
}

export const api = new ApiClient();
