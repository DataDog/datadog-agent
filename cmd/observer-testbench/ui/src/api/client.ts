// API client for the observer test bench

const API_BASE = '/api';

export interface ServerConfig {
  cusumEnabled: boolean;
  cusumSkipCount: boolean;  // true = filtering out :count metrics
  zscoreEnabled: boolean;
  timeClusterEnabled: boolean;
  leadLagEnabled: boolean;
  surpriseEnabled: boolean;
  graphSketchEnabled: boolean;
  dedupEnabled: boolean;
}

export interface StatusResponse {
  ready: boolean;
  scenario: string | null;
  seriesCount: number;
  anomalyCount: number;
  componentCount: number;
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

// Lead-Lag edge represents temporal causality between sources
export interface LeadLagEdge {
  leader: string;
  follower: string;
  typical_lag: number;  // Seconds
  confidence: number;   // 0-1
  observations: number;
}

export interface LeadLagResponse {
  enabled: boolean;
  edges: LeadLagEdge[];
}

// Surprise edge represents unexpected co-occurrence (high lift)
export interface SurpriseEdge {
  source1: string;
  source2: string;
  lift: number;
  support: number;         // Number of co-occurrences
  source1_count: number;   // Total anomalies from source1
  source2_count: number;   // Total anomalies from source2
  is_surprising: boolean;  // true if lift > MinLift
}

export interface SurpriseResponse {
  enabled: boolean;
  edges: SurpriseEdge[];
}

// GraphSketch edge represents learned co-occurrence patterns
export interface GraphSketchEdge {
  Source1: string;
  Source2: string;
  EdgeKey: string;
  Observations: number;    // Raw count
  Frequency: number;       // Decay-weighted frequency
  FirstSeenUnix: number;
}

export interface GraphSketchResponse {
  enabled: boolean;
  edges: GraphSketchEdge[];
}

// Stats response from correlators
export interface CorrelatorStats {
  leadlag?: {
    enabled: boolean;
    edgeCount: number;
    sourceCount: number;
  };
  surprise?: {
    enabled: boolean;
    edgeCount: number;
    totalWindows: number;
  };
  graphsketch?: {
    enabled: boolean;
    edgeCount: number;
    signalCount: number;
  };
  timecluster?: {
    enabled: boolean;
    clusterCount: number;
  };
}

// Ground truth marker from markers.json (injected anomaly timestamps)
export interface GroundTruthMarker {
  timestamp: number;
  type: string;       // e.g. "baseline_start", "anomaly_start", "anomaly_end"
  description: string;
}

export interface DiagnosisResult {
  status: string;
  result?: string;
  error?: string;
}

export interface EvaluationResult {
  status: string;
  result?: string;
  error?: string;
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

  async getLeadLag(): Promise<LeadLagResponse> {
    return this.fetch('/leadlag');
  }

  async getSurprise(): Promise<SurpriseResponse> {
    return this.fetch('/surprise');
  }

  async getGraphSketch(): Promise<GraphSketchResponse> {
    return this.fetch('/graphsketch');
  }

  async getStats(): Promise<CorrelatorStats> {
    return this.fetch('/stats');
  }

  async getMarkers(): Promise<GroundTruthMarker[]> {
    return this.fetch('/markers');
  }

  async runDiagnosis(): Promise<DiagnosisResult> {
    return this.fetch('/diagnosis/run', { method: 'POST' });
  }

  async runEvaluation(scenario: string): Promise<EvaluationResult> {
    return this.fetch('/evaluation/run', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ scenario }),
    });
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
