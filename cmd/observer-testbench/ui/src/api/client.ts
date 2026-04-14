// API client for the observer test bench

const API_BASE = '/api';

export type SeriesID = string & { readonly __seriesIdBrand: unique symbol };
export type MetricName = string & { readonly __metricNameBrand: unique symbol };

export interface ServerConfig {
  components: Record<string, boolean>;
  /** When true, scenarios load log rows only (no parquet metrics / trace stats). */
  logsOnly?: boolean;
}

export interface EpisodePhase {
  start: string;
  end: string;
}

export interface EpisodeScenario {
  app_name: string;
  description: string;
  long_description: string;
}

export interface EpisodeInfo {
  episode: string;
  cycle: number;
  scenario: EpisodeScenario;
  environment: string;
  execution_id: string;
  success: boolean;
  start_time: string;
  end_time: string;
  warmup?: EpisodePhase;
  baseline?: EpisodePhase;
  disruption?: EpisodePhase;
  cooldown?: EpisodePhase;
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
  episodeInfo?: EpisodeInfo;
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
  category: 'detector' | 'correlator' | 'processing';
  enabled: boolean;
  /** Active hyperparameter values, present when the component has a typed config. */
  config?: Record<string, unknown>;
}

export interface SeriesInfo {
  id: SeriesID;
  namespace: string;
  name: string;
  tags: string[];
  pointCount: number;
  /** True when the series lives in an extractor storage namespace (log-derived metrics). */
  virtual?: boolean;
  /** Telemetry metrics: counter deltas use :sum and cumulative display; gauges use the selected aggregation. */
  metricKind?: 'gauge' | 'counter';
}

export interface Point {
  timestamp: number;
  value: number;
}

export interface AnomalyMarker {
  timestamp: number;
  detectorName: string;
  detectorComponent?: string;
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
  detectorName: string;
  detectorComponent?: string;
  title: string;
  description: string;
  tags: string[];
  timestamp: number;
  debugInfo?: AnomalyDebugInfo;
}

// LogEntry is a raw log entry stored in the testbench.
export interface LogEntry {
  timestampMs: number;
  status: string;   // "error", "warn", "info", "debug", etc.
  content: string;
  tags: string[];
}

// LogsResponse is the paginated response from /api/logs.
export interface LogsResponse {
  logs: LogEntry[];
  total: number;
  limit: number;
  offset: number;
}

export type LogKind = 'all' | 'raw' | 'telemetry';

// LogsSummary is the summary response from /api/logs/summary.
export interface LogsSummary {
  totalCount: number;
  countByLevel: Record<string, number>;
  timeRange: { start: number; end: number };
  histogram: { timestampMs: number; count: number }[];
  tagGroups: Record<string, string[]>;
}

// LogPattern describes a detected log pattern cluster with its count metric series.
export interface LogPattern {
  hash: string;
  patternString: string;  // human-readable, e.g. "GET /api/* 200"
  exampleLog: string;
  count: number;
  seriesIDs: string[];    // compact series IDs for the count timeseries
}

// LogAnomaly is an anomaly emitted directly by a log detector (not via metrics detection).
export interface LogAnomaly {
  source: string;
  detectorName: string;
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
    tags: string[];
  }[];
  firstSeen: number;
  lastUpdated: number;
}

/** Datadog-style incident event (mirrors headless JSON `reports` entries). */
export interface ReportEvent {
  pattern: string;
  title: string;
  message: string;
  tags: string[];
  firstSeen: number;
  lastUpdated: number;
  formattedTime: string;
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

// Generic component data response
export interface ComponentDataResponse {
  enabled: boolean;
  data: unknown;
}

// Replay progress (lock-free, available during load).
export interface ReplayProgress {
  phase: string; // "", "loading", "detecting", "done"
  timestampsDone: number;
  timestampsTotal: number;
  advances: number;
  anomalies: number;
}

// Stats response from correlators
export interface CorrelatorStats {
  [key: string]: Record<string, unknown>;
}

export interface ScoreResult {
  f1: number;
  precision: number;
  recall: number;
  tp: number;
  fp: number;
  fn: number;
  num_predictions: number;
  num_ground_truths: number;
  num_filtered_warmup: number;
  num_filtered_cascading: number;
  num_baseline_fps: number;
  sigma: number;
}

export interface ScoreResponse {
  available: boolean;
  reason?: string;
  score?: ScoreResult;
}

export interface DetectorProcessingStats {
  name: string;
  kind: 'detector' | 'correlator' | 'extractor' | '';
  count: number;
  avg_ns: number;
  median_ns: number;
  p99_ns: number;
  total_ns: number;
}

export interface ReplayStats {
  detector_stats: Record<string, DetectorProcessingStats>;
  input_metrics_count: number;
  input_metrics_cardinality: number;
  input_logs_count: number;
  input_anomalies_count: number;
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

  async getProgress(): Promise<ReplayProgress> {
    return this.fetch('/progress');
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

  async getAnomalies(detector?: string): Promise<Anomaly[]> {
    const params = detector ? `?detector=${encodeURIComponent(detector)}` : '';
    return this.fetch(`/anomalies${params}`);
  }

  async getLogPatterns(): Promise<LogPattern[]> {
    return this.fetch('/log-patterns');
  }

  async getLogs(params?: { kind?: LogKind; level?: string; start?: number; end?: number; limit?: number; offset?: number; tags?: string; pattern?: string }): Promise<LogsResponse> {
    const searchParams = new URLSearchParams();
    if (params?.kind) searchParams.set('kind', params.kind);
    if (params?.level) searchParams.set('level', params.level);
    if (params?.start !== undefined) searchParams.set('start', String(params.start));
    if (params?.end !== undefined) searchParams.set('end', String(params.end));
    if (params?.limit !== undefined) searchParams.set('limit', String(params.limit));
    if (params?.offset !== undefined) searchParams.set('offset', String(params.offset));
    if (params?.tags) searchParams.set('tags', params.tags);
    if (params?.pattern) searchParams.set('pattern', params.pattern);
    const qs = searchParams.toString();
    return this.fetch(`/logs${qs ? '?' + qs : ''}`);
  }

  async getLogsSummary(params?: { kind?: LogKind; level?: string; start?: number; end?: number; tags?: string; pattern?: string }): Promise<LogsSummary> {
    const searchParams = new URLSearchParams();
    if (params?.kind) searchParams.set('kind', params.kind);
    if (params?.level) searchParams.set('level', params.level);
    if (params?.start !== undefined) searchParams.set('start', String(params.start));
    if (params?.end !== undefined) searchParams.set('end', String(params.end));
    if (params?.tags) searchParams.set('tags', params.tags);
    if (params?.pattern) searchParams.set('pattern', params.pattern);
    const qs = searchParams.toString();
    return this.fetch(`/logs/summary${qs ? '?' + qs : ''}`);
  }

  async getLogAnomalies(detector?: string): Promise<LogAnomaly[]> {
    const params = detector ? `?detector=${encodeURIComponent(detector)}` : '';
    return this.fetch(`/log-anomalies${params}`);
  }

  async getCorrelations(): Promise<Correlation[]> {
    return this.fetch('/correlations');
  }

  async getReports(): Promise<ReportEvent[]> {
    return this.fetch('/reports');
  }

  async getComponentData(name: string): Promise<ComponentDataResponse> {
    return this.fetch(`/components/${encodeURIComponent(name)}/data`);
  }

  async getCompressedCorrelations(threshold?: number): Promise<CompressedGroup[]> {
    const params = threshold !== undefined ? `?threshold=${threshold}` : '';
    return this.fetch(`/correlations/compressed${params}`);
  }

  async getStats(): Promise<CorrelatorStats> {
    return this.fetch('/stats');
  }

  async getScore(): Promise<ScoreResponse> {
    return this.fetch('/score');
  }

  async getBenchmarkStats(): Promise<ReplayStats> {
    return this.fetch('/benchmark');
  }

}

export const api = new ApiClient();
