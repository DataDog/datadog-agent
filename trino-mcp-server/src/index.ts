#!/usr/bin/env node

import { Server } from '@modelcontextprotocol/sdk/server/index.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
  Tool,
} from '@modelcontextprotocol/sdk/types.js';
import { z } from 'zod';
import { Trino, BasicAuth } from 'trino-client';
import { execSync } from 'child_process';

// Configuration schema
const ConfigSchema = z.object({
  server: z.string().default('localhost:8080'),
  catalog: z.string().default('eventplatform'),
  schema: z.string().default('system'),
  user: z.string().default('datadog'),
  password: z.string().optional(),
  auth: z.object({
    type: z.enum(['basic', 'jwt', 'datadog']).optional(),
    token: z.string().optional(),
    username: z.string().optional(),
    password: z.string().optional(),
  }).optional(),
  // Datadog-specific config
  dd_org_id: z.string().optional(),
  dd_client_id: z.string().optional(),
  dd_user_uuid: z.string().optional(),
  dd_datacenter: z.string().optional(),
  dd_auth_jwt: z.string().optional(),
  dd_access_token: z.string().optional(),
  use_dynamic_tokens: z.boolean().optional(),
});

type Config = z.infer<typeof ConfigSchema>;

class TrinoMCPServer {
  private server: Server;
  private trinoClient: any | null = null;
  private config: Config;

  constructor() {
    this.config = this.loadConfig();
    this.server = new Server(
      {
        name: 'trino-mcp-server',
        version: '1.0.0',
        capabilities: {
          tools: {},
        },
      }
    );

    this.setupTools();
    this.setupErrorHandling();
  }

  private loadConfig(): Config {
    const envVars = {
      server: process.env.TRINO_SERVER,
      catalog: process.env.TRINO_CATALOG,
      schema: process.env.TRINO_SCHEMA,
      user: process.env.TRINO_USER,
      password: process.env.TRINO_PASSWORD,
      auth: process.env.TRINO_AUTH_TYPE ? {
        type: process.env.TRINO_AUTH_TYPE as any,
        token: process.env.TRINO_AUTH_TOKEN,
        username: process.env.TRINO_AUTH_USERNAME,
        password: process.env.TRINO_AUTH_PASSWORD,
      } : undefined,
      // Datadog-specific environment variables
      dd_org_id: process.env.DD_ORG_ID,
      dd_client_id: process.env.DD_CLIENT_ID,
      dd_user_uuid: process.env.DD_USER_UUID,
      dd_datacenter: process.env.DD_DATACENTER,
      dd_auth_jwt: process.env.DD_AUTH_JWT,
      dd_access_token: process.env.DD_ACCESS_TOKEN,
      use_dynamic_tokens: process.env.USE_DYNAMIC_TOKENS === 'true',
    };

    // Remove undefined values
    const cleanEnvVars = Object.fromEntries(
      Object.entries(envVars).filter(([_, value]) => value !== undefined)
    );

    return ConfigSchema.parse(cleanEnvVars);
  }

  private async getTrinoClient(forceRefreshTokens = false): Promise<any> {
    // Always create fresh client to get latest tokens
    const clientConfig: any = {
      server: `https://${this.config.server}`,
      catalog: this.config.catalog,
      schema: this.config.schema,
      user: this.config.user,
    };

    if (this.config.auth) {
      if (this.config.auth.type === 'basic') {
        clientConfig.auth = new BasicAuth(
          this.config.auth.username || this.config.user,
          this.config.auth.password || this.config.password
        );
      } else if (this.config.auth.type === 'datadog') {
        // For Datadog auth, we'll use custom headers
        clientConfig.extraHeaders = {};

        let ddAuthJWT = this.config.dd_auth_jwt;
        let ddAccessToken = this.config.dd_access_token;

        // Generate fresh tokens if dynamic tokens enabled, tokens missing, or forced refresh
        if (this.config.use_dynamic_tokens || !ddAuthJWT || !ddAccessToken || forceRefreshTokens) {
          console.error('Generating fresh tokens...');
          try {
            const jwtResult = execSync(`ddauth obo -d ${this.config.dd_datacenter} | grep dd-auth-jwt | cut -d' ' -f2`, { encoding: 'utf8' }).trim();
            const accessTokenResult = execSync(`ddtool auth token --datacenter ${this.config.dd_datacenter} apm-trino`, { encoding: 'utf8' }).trim();

            if (jwtResult && accessTokenResult) {
              ddAuthJWT = jwtResult;
              ddAccessToken = accessTokenResult;
              console.error('Fresh tokens generated successfully');
            } else {
              throw new Error('Token generation returned empty results');
            }
          } catch (error) {
            console.error('Failed to generate fresh tokens:', error);
            // Fall back to static tokens if available
            if (!ddAuthJWT || !ddAccessToken) {
              throw new Error('Token generation failed and no fallback tokens available');
            }
            console.error('Using fallback tokens from configuration');
          }
        }

        // Add access token as Authorization header
        if (ddAccessToken) {
          clientConfig.extraHeaders['Authorization'] = `Bearer ${ddAccessToken}`;
        }

        // Add extra credentials as headers
        if (this.config.dd_org_id) {
          clientConfig.extraHeaders['X-Trino-Extra-Credential'] =
            clientConfig.extraHeaders['X-Trino-Extra-Credential']
              ? `${clientConfig.extraHeaders['X-Trino-Extra-Credential']}, orgId=${this.config.dd_org_id}`
              : `orgId=${this.config.dd_org_id}`;
        }

        if (this.config.dd_client_id) {
          clientConfig.extraHeaders['X-Trino-Extra-Credential'] =
            clientConfig.extraHeaders['X-Trino-Extra-Credential']
              ? `${clientConfig.extraHeaders['X-Trino-Extra-Credential']}, clientId=${this.config.dd_client_id}`
              : `clientId=${this.config.dd_client_id}`;
        }

        if (this.config.dd_user_uuid) {
          clientConfig.extraHeaders['X-Trino-Extra-Credential'] =
            clientConfig.extraHeaders['X-Trino-Extra-Credential']
              ? `${clientConfig.extraHeaders['X-Trino-Extra-Credential']}, userUuid=${this.config.dd_user_uuid}`
              : `userUuid=${this.config.dd_user_uuid}`;
        }

        if (ddAuthJWT) {
          clientConfig.extraHeaders['X-Trino-Extra-Credential'] =
            clientConfig.extraHeaders['X-Trino-Extra-Credential']
              ? `${clientConfig.extraHeaders['X-Trino-Extra-Credential']}, ddAuthJWT=${ddAuthJWT}`
              : `ddAuthJWT=${ddAuthJWT}`;
        }

        // Add client tags
        if (this.config.dd_org_id) {
          clientConfig.extraHeaders['X-Trino-Client-Tags'] = `org_id=${this.config.dd_org_id}`;
        }
      }
    }

    return Trino.create(clientConfig);
  }

  private setupTools() {
    this.server.setRequestHandler(ListToolsRequestSchema, async () => {
      return {
        tools: [
          {
            name: 'query_trino',
            description: 'Execute a Trino/Presto SQL query against the event platform',
            inputSchema: {
              type: 'object',
              properties: {
                query: {
                  type: 'string',
                  description: 'The SQL query to execute',
                },
                limit: {
                  type: 'number',
                  description: 'Maximum number of rows to return (default: 1000)',
                  default: 1000,
                },
              },
              required: ['query'],
            },
          },
          {
            name: 'query_netflow_summary',
            description: 'Get a summary of NetFlow data by exporter',
            inputSchema: {
              type: 'object',
              properties: {
                limit: {
                  type: 'number',
                  description: 'Maximum number of results to return',
                  default: 10,
                },
              },
              required: [],
            },
          },
          {
            name: 'query_netflow_talkers',
            description: 'Get top NetFlow talkers by source and destination IP with byte totals',
            inputSchema: {
              type: 'object',
              properties: {
                limit: {
                  type: 'number',
                  description: 'Maximum number of talkers to return',
                  default: 10,
                },
                group_by: {
                  type: 'string',
                  description: 'Group by: source_ip, dest_ip, or both',
                  enum: ['source_ip', 'dest_ip', 'both'],
                  default: 'both',
                },
              },
              required: [],
            },
          },
          {
            name: 'get_available_tracks',
            description: 'Get list of available tracks in the event platform',
            inputSchema: {
              type: 'object',
              properties: {},
              required: [],
            },
          },
          {
            name: 'query_metrics_summary',
            description: 'Get a summary of metrics data using Datadog metrics query syntax',
            inputSchema: {
              type: 'object',
              properties: {
                metric_name: {
                  type: 'string',
                  description: 'Specific metric name to query (e.g., "system.cpu.user", "nginx.requests")',
                },
                time_range: {
                  type: 'string',
                  description: 'Time range like "1h", "24h", "7d" (default: "1h")',
                  default: '1h',
                },
                limit: {
                  type: 'number',
                  description: 'Maximum number of results to return',
                  default: 10,
                },
              },
              required: [],
            },
          },
          {
            name: 'query_metrics_by_service',
            description: 'Get metrics data grouped by service using Datadog metrics',
            inputSchema: {
              type: 'object',
              properties: {
                service_name: {
                  type: 'string',
                  description: 'Filter by specific service name (e.g., "web-frontend", "api-backend")',
                },
                metric_type: {
                  type: 'string',
                  description: 'Not used with squire.timeseries.metrics - kept for compatibility',
                },
                time_range: {
                  type: 'string',
                  description: 'Time range like "1h", "24h", "7d" (default: "1h")',
                  default: '1h',
                },
                limit: {
                  type: 'number',
                  description: 'Maximum number of results to return',
                  default: 10,
                },
              },
              required: [],
            },
          },
          {
            name: 'query_dbm_metrics',
            description: 'Get Database Monitoring (DBM) metrics using Datadog metrics query syntax',
            inputSchema: {
              type: 'object',
              properties: {
                query_filter: {
                  type: 'string',
                  description: 'Not applicable for metrics queries - kept for compatibility',
                },
                database_type: {
                  type: 'string',
                  description: 'Database type for metrics: postgresql, mysql, oracle, sqlserver (default: postgresql)',
                },
                time_range: {
                  type: 'string',
                  description: 'Time range like "1h", "24h", "7d" (default: "1h")',
                  default: '1h',
                },
                limit: {
                  type: 'number',
                  description: 'Maximum number of results to return',
                  default: 10,
                },
              },
              required: [],
            },
          },
          {
            name: 'query_custom_metrics',
            description: 'Execute custom Datadog metrics queries using the full squire.timeseries.metrics syntax',
            inputSchema: {
              type: 'object',
              properties: {
                metrics_query: {
                  type: 'string',
                  description: 'Datadog metrics query (e.g., "avg:system.cpu.user{host:web01}", "count:nginx.requests{*} by {service}")',
                },
                time_range: {
                  type: 'string',
                  description: 'Time range like "1h", "24h", "7d" (default: "1h")',
                  default: '1h',
                },
                unnest_timeseries: {
                  type: 'boolean',
                  description: 'Whether to unnest timeseries data (default: true)',
                  default: true,
                },
                limit: {
                  type: 'number',
                  description: 'Maximum number of rows to return (default: 100)',
                  default: 100,
                },
              },
              required: ['metrics_query'],
            },
          },
          {
            name: 'query_logs',
            description: 'Query logs from the event platform with filtering and search capabilities',
            inputSchema: {
              type: 'object',
              properties: {
                query: {
                  type: 'string',
                  description: 'Log search query using Datadog log query syntax (e.g., "message:* @source:trino-cli -host:excluded-host")',
                  default: 'message:*',
                },
                columns: {
                  type: 'array',
                  items: { type: 'string' },
                  description: 'Columns to retrieve (e.g., ["message", "timestamp", "env", "@duration"])',
                  default: ['message', 'timestamp', 'env'],
                },
                time_range: {
                  type: 'string',
                  description: 'Time range like "1h", "24h", "7d" (default: "1h")',
                  default: '1h',
                },
                limit: {
                  type: 'number',
                  description: 'Maximum number of results to return',
                  default: 100,
                },
                env_filter: {
                  type: 'array',
                  items: { type: 'string' },
                  description: 'Filter by environment (e.g., ["staging", "prod"])',
                },
              },
              required: [],
            },
          },
          {
            name: 'query_logs_summary',
            description: 'Get a summary of log data grouped by service, environment, or other dimensions',
            inputSchema: {
              type: 'object',
              properties: {
                group_by: {
                  type: 'string',
                  description: 'Field to group by (e.g., "env", "service", "@source")',
                  default: 'env',
                },
                query: {
                  type: 'string',
                  description: 'Log search query filter',
                  default: 'message:*',
                },
                time_range: {
                  type: 'string',
                  description: 'Time range like "1h", "24h", "7d" (default: "1h")',
                  default: '1h',
                },
                limit: {
                  type: 'number',
                  description: 'Maximum number of results to return',
                  default: 10,
                },
              },
              required: [],
            },
          },
        ] satisfies Tool[],
      };
    });

    this.server.setRequestHandler(CallToolRequestSchema, async (request) => {
      const { name, arguments: args } = request.params;

      try {
        switch (name) {
          case 'query_trino':
            return await this.handleTrinoQuery(args);
          case 'query_netflow_summary':
            return await this.handleNetflowSummary(args);
          case 'query_netflow_talkers':
            return await this.handleNetflowTalkers(args);
          case 'get_available_tracks':
            return await this.handleGetTracks(args);
          case 'query_metrics_summary':
            return await this.handleMetricsSummary(args);
          case 'query_metrics_by_service':
            return await this.handleMetricsByService(args);
          case 'query_dbm_metrics':
            return await this.handleDBMMetrics(args);
          case 'query_custom_metrics':
            return await this.handleCustomMetrics(args);
          case 'query_logs':
            return await this.handleLogsQuery(args);
          case 'query_logs_summary':
            return await this.handleLogsSummary(args);
          default:
            throw new Error(`Unknown tool: ${name}`);
        }
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : String(error);
        return {
          content: [
            {
              type: 'text',
              text: `Error executing ${name}: ${errorMessage}`,
            },
          ],
          isError: true,
        };
      }
    });
  }

  private async handleTrinoQuery(args: any, retryWithFreshTokens = true) {
    try {
      const client = await this.getTrinoClient();
      const { query, limit = 1000 } = args;

      const limitedQuery = this.addLimitToQuery(query, limit);

      const iter = await client.query(limitedQuery);

      const rows = [];
      for await (const queryResult of iter) {
        if (queryResult.data) {
          rows.push(...queryResult.data);
        }
      }

      return {
        content: [
          {
            type: 'text',
            text: `Query executed successfully. Returned ${rows.length} rows.\n\n` +
              `Query: ${limitedQuery}\n\n` +
              `Results:\n${JSON.stringify(rows, null, 2)}`,
          },
        ],
      };
    } catch (error: any) {
      // Check if it's a 401 error and we haven't already retried
      if (retryWithFreshTokens && error.message && (
        error.message.includes('401') ||
        error.message.includes('status code 401') ||
        error.message.includes('Unauthorized')
      )) {
        console.error('Received 401 error, retrying with fresh tokens...');
        try {
          // Get a new client with force-refreshed tokens
          const client = await this.getTrinoClient(true);
          const { query, limit = 1000 } = args;
          const limitedQuery = this.addLimitToQuery(query, limit);

          const iter = await client.query(limitedQuery);
          const rows = [];
          for await (const queryResult of iter) {
            if (queryResult.data) {
              rows.push(...queryResult.data);
            }
          }

          return {
            content: [
              {
                type: 'text',
                text: `Query executed successfully (after token refresh). Returned ${rows.length} rows.\n\n` +
                  `Query: ${limitedQuery}\n\n` +
                  `Results:\n${JSON.stringify(rows, null, 2)}`,
              },
            ],
          };
        } catch (retryError) {
          console.error('Retry with fresh tokens also failed:', retryError);
          throw retryError;
        }
      }
      throw error;
    }
  }

  private async handleNetflowSummary(args: any) {
    const { limit = 10 } = args;

    const query = `
SELECT 
  "@exporter.ip" as exporter_ip,
  COUNT(*) as flow_count,
  SUM(CAST("@bytes" AS bigint)) as total_bytes
FROM TABLE(
  eventplatform.system.track(
    TRACK => 'ndmflow',
    COLUMNS => ARRAY['@exporter.ip', '@bytes'],
    OUTPUT_TYPES => ARRAY['varchar', 'varchar']
  )
)
WHERE "@bytes" IS NOT NULL
GROUP BY "@exporter.ip"
ORDER BY total_bytes DESC
LIMIT ${limit}`;

    return await this.handleTrinoQuery({ query });
  }

  private async handleNetflowTalkers(args: any) {
    const { limit = 10, group_by = 'both' } = args;

    let query = '';

    if (group_by === 'source_ip') {
      query = `
SELECT 
  "@source.ip" as source_ip,
  COUNT(*) as flow_count,
  SUM(CAST("@bytes" AS bigint)) as total_bytes
FROM TABLE(
  eventplatform.system.track(
    TRACK => 'ndmflow',
    COLUMNS => ARRAY['@source.ip', '@bytes'],
    OUTPUT_TYPES => ARRAY['varchar', 'varchar']
  )
)
WHERE "@bytes" IS NOT NULL AND "@source.ip" IS NOT NULL
GROUP BY "@source.ip"
ORDER BY total_bytes DESC
LIMIT ${limit}`;
    } else if (group_by === 'dest_ip') {
      query = `
SELECT 
  "@destination.ip" as destination_ip,
  COUNT(*) as flow_count,
  SUM(CAST("@bytes" AS bigint)) as total_bytes
FROM TABLE(
  eventplatform.system.track(
    TRACK => 'ndmflow',
    COLUMNS => ARRAY['@destination.ip', '@bytes'],
    OUTPUT_TYPES => ARRAY['varchar', 'varchar']
  )
)
WHERE "@bytes" IS NOT NULL AND "@destination.ip" IS NOT NULL
GROUP BY "@destination.ip"
ORDER BY total_bytes DESC
LIMIT ${limit}`;
    } else {
      query = `
SELECT 
  "@source.ip" as source_ip,
  "@destination.ip" as destination_ip,
  COUNT(*) as flow_count,
  SUM(CAST("@bytes" AS bigint)) as total_bytes
FROM TABLE(
  eventplatform.system.track(
    TRACK => 'ndmflow',
    COLUMNS => ARRAY['@source.ip', '@destination.ip', '@bytes'],
    OUTPUT_TYPES => ARRAY['varchar', 'varchar', 'varchar']
  )
)
WHERE "@bytes" IS NOT NULL AND "@source.ip" IS NOT NULL AND "@destination.ip" IS NOT NULL
GROUP BY "@source.ip", "@destination.ip"
ORDER BY total_bytes DESC
LIMIT ${limit}`;
    }

    return await this.handleTrinoQuery({ query });
  }

  private async handleGetTracks(args: any) {
    const query = `
SELECT DISTINCT track_name 
FROM eventplatform.system.tracks 
ORDER BY track_name`;

    return await this.handleTrinoQuery({ query, limit: 100 });
  }

  private async handleMetricsSummary(args: any) {
    const { metric_name, time_range = '1h', limit = 10 } = args;

    // Convert time_range to seconds for MIN_TIMESTAMP
    const timeRangeToSeconds = (range: string): number => {
      const num = parseInt(range.slice(0, -1));
      const unit = range.slice(-1);
      switch (unit) {
        case 'h': return num * 3600;
        case 'd': return num * 86400;
        case 'm': return num * 60;
        default: return 3600; // Default 1 hour
      }
    };

    const minTimestamp = -timeRangeToSeconds(time_range);

    // Build metrics query - if no specific metric, get top metrics by volume
    let metricsQuery = metric_name
      ? `${metric_name}{*}`
      : `*{*} by {__name__}`;

    const query = `
SELECT 
  metric,
  scope,
  host,
  service,
  COUNT(*) as data_points,
  AVG(value) as avg_value,
  MIN(value) as min_value,
  MAX(value) as max_value
FROM TABLE(
  squire.timeseries.metrics(
    QUERY => '${metricsQuery}',
    MIN_TIMESTAMP => ${minTimestamp},
    MAX_TIMESTAMP => 0,
    UNNEST_TIMESERIES => true
  )
)
WHERE metric IS NOT NULL
GROUP BY metric, scope, host, service
ORDER BY data_points DESC
LIMIT ${limit}`;

    return await this.handleTrinoQuery({ query });
  }

  private async handleMetricsByService(args: any) {
    const { service_name, metric_type, time_range = '1h', limit = 10 } = args;

    // Convert time_range to seconds
    const timeRangeToSeconds = (range: string): number => {
      const num = parseInt(range.slice(0, -1));
      const unit = range.slice(-1);
      switch (unit) {
        case 'h': return num * 3600;
        case 'd': return num * 86400;
        case 'm': return num * 60;
        default: return 3600;
      }
    };

    const minTimestamp = -timeRangeToSeconds(time_range);

    // Build metrics query with service filter
    let metricsQuery = '*{*}';
    if (service_name) {
      metricsQuery = `*{service:${service_name}}`;
    }

    // Add grouping by service and metric name
    metricsQuery += ' by {service,__name__}';

    const query = `
SELECT 
  service,
  metric,
  COUNT(*) as data_points,
  COUNT(DISTINCT host) as unique_hosts,
  AVG(value) as avg_value,
  MIN(value) as min_value,
  MAX(value) as max_value
FROM TABLE(
  squire.timeseries.metrics(
    QUERY => '${metricsQuery}',
    MIN_TIMESTAMP => ${minTimestamp},
    MAX_TIMESTAMP => 0,
    UNNEST_TIMESERIES => true
  )
)
WHERE service IS NOT NULL AND metric IS NOT NULL
${service_name ? `AND service = '${service_name}'` : ''}
GROUP BY service, metric
ORDER BY data_points DESC
LIMIT ${limit}`;

    return await this.handleTrinoQuery({ query });
  }

  private async handleDBMMetrics(args: any) {
    const { query_filter, database_type, time_range = '1h', limit = 10 } = args;

    // Convert time_range to seconds
    const timeRangeToSeconds = (range: string): number => {
      const num = parseInt(range.slice(0, -1));
      const unit = range.slice(-1);
      switch (unit) {
        case 'h': return num * 3600;
        case 'd': return num * 86400;
        case 'm': return num * 60;
        default: return 3600;
      }
    };

    const minTimestamp = -timeRangeToSeconds(time_range);

    // Build DBM-specific metrics query
    let metricsQuery = 'postgresql.queries.*{*}'; // Default to PostgreSQL queries
    if (database_type) {
      metricsQuery = `${database_type}.queries.*{*}`;
    }

    // Group by service and database
    metricsQuery += ' by {service,db}';

    const query = `
SELECT 
  service,
  db as database_name,
  metric,
  COUNT(*) as data_points,
  AVG(value) as avg_value,
  MAX(value) as max_value
FROM TABLE(
  squire.timeseries.metrics(
    QUERY => '${metricsQuery}',
    MIN_TIMESTAMP => ${minTimestamp},
    MAX_TIMESTAMP => 0,
    UNNEST_TIMESERIES => true
  )
)
WHERE service IS NOT NULL AND db IS NOT NULL
GROUP BY service, db, metric
ORDER BY avg_value DESC
LIMIT ${limit}`;

    return await this.handleTrinoQuery({ query });
  }

  private async handleCustomMetrics(args: any) {
    const { metrics_query, time_range = '1h', unnest_timeseries = true, limit = 100 } = args;

    // Convert time_range to seconds
    const timeRangeToSeconds = (range: string): number => {
      const num = parseInt(range.slice(0, -1));
      const unit = range.slice(-1);
      switch (unit) {
        case 'h': return num * 3600;
        case 'd': return num * 86400;
        case 'm': return num * 60;
        default: return 3600;
      }
    };

    const minTimestamp = -timeRangeToSeconds(time_range);

    const query = `
SELECT 
  metric,
  scope,
  host,
  service,
  value,
  timestamp
FROM TABLE(
  squire.timeseries.metrics(
    QUERY => '${metrics_query}',
    MIN_TIMESTAMP => ${minTimestamp},
    MAX_TIMESTAMP => 0,
    UNNEST_TIMESERIES => ${unnest_timeseries}
  )
)
WHERE metric IS NOT NULL
GROUP BY metric, scope, host, service, value, timestamp
ORDER BY timestamp DESC
LIMIT ${limit}`;

    return await this.handleTrinoQuery({ query });
  }

  private async handleLogsQuery(args: any) {
    const {
      query = 'message:*',
      columns = ['message', 'timestamp', 'env'],
      time_range = '1h',
      limit = 100,
      env_filter
    } = args;

    // Convert time_range to seconds
    const timeRangeToSeconds = (range: string): number => {
      const num = parseInt(range.slice(0, -1));
      const unit = range.slice(-1);
      switch (unit) {
        case 'h': return num * 3600;
        case 'd': return num * 86400;
        case 'm': return num * 60;
        default: return 3600;
      }
    };

    const minTimestamp = -timeRangeToSeconds(time_range);

    // Build columns array and output types array
    const columnsArray = columns.map((col: string) => `'${col}'`).join(', ');
    const outputTypesArray = columns.map(() => "'varchar'").join(', ');

    // Build the base SQL query
    let sqlQuery = `
SELECT ${columns.map((col: string) => `"${col}"`).join(', ')}
FROM TABLE(
  eventplatform.system.track(
    TRACK => 'logs',
    QUERY => '${query}',
    COLUMNS => ARRAY[${columnsArray}],
    OUTPUT_TYPES => ARRAY[${outputTypesArray}],
    MIN_TIMESTAMP => ${minTimestamp},
    MAX_TIMESTAMP => 0
  )
)`;

    // Add environment filter if specified
    if (env_filter && env_filter.length > 0) {
      const envConditions = env_filter.map((env: string) => `'${env}'`).join(', ');
      sqlQuery += `\nWHERE "env" IN (${envConditions})`;
    }

    // Add ordering and limit
    sqlQuery += `\nORDER BY "timestamp" DESC`;

    return await this.handleTrinoQuery({ query: sqlQuery, limit });
  }

  private async handleLogsSummary(args: any) {
    const {
      group_by = 'env',
      query = 'message:*',
      time_range = '1h',
      limit = 10
    } = args;

    // Convert time_range to seconds
    const timeRangeToSeconds = (range: string): number => {
      const num = parseInt(range.slice(0, -1));
      const unit = range.slice(-1);
      switch (unit) {
        case 'h': return num * 3600;
        case 'd': return num * 86400;
        case 'm': return num * 60;
        default: return 3600;
      }
    };

    const minTimestamp = -timeRangeToSeconds(time_range);

    const sqlQuery = `
SELECT 
  "${group_by}" as grouping_field,
  COUNT(*) as log_count,
  COUNT(DISTINCT "timestamp") as unique_timestamps,
  MIN("timestamp") as earliest_log,
  MAX("timestamp") as latest_log
FROM TABLE(
  eventplatform.system.track(
    TRACK => 'logs',
    QUERY => '${query}',
    COLUMNS => ARRAY['${group_by}', 'timestamp'],
    OUTPUT_TYPES => ARRAY['varchar', 'int'],
    MIN_TIMESTAMP => ${minTimestamp},
    MAX_TIMESTAMP => 0
  )
)
WHERE "${group_by}" IS NOT NULL
GROUP BY "${group_by}"
ORDER BY log_count DESC
LIMIT ${limit}`;

    return await this.handleTrinoQuery({ query: sqlQuery });
  }

  private addLimitToQuery(query: string, limit: number): string {
    const upperQuery = query.toUpperCase().trim();

    // If query already has LIMIT, don't add another one
    if (upperQuery.includes('LIMIT')) {
      return query;
    }

    // Add LIMIT to the end of the query
    return `${query.trim()}\nLIMIT ${limit}`;
  }

  private setupErrorHandling() {
    this.server.onerror = (error) => {
      console.error('[MCP Error]', error);
    };

    process.on('SIGINT', async () => {
      if (this.trinoClient) {
        // Close Trino client if needed
      }
      await this.server.close();
      process.exit(0);
    });
  }

  async start() {
    const transport = new StdioServerTransport();
    await this.server.connect(transport);
    console.error('Trino MCP server started');
  }
}

// Start the server
const server = new TrinoMCPServer();
server.start().catch((error) => {
  console.error('Failed to start server:', error);
  process.exit(1);
}); 