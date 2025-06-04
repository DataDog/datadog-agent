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

  private async getTrinoClient(): Promise<any> {
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
        
        // Generate fresh tokens if dynamic tokens enabled
        if (this.config.use_dynamic_tokens || !ddAuthJWT || !ddAccessToken) {
          console.error('Generating fresh tokens...');
          try {
            ddAuthJWT = execSync(`ddauth obo -d ${this.config.dd_datacenter} | grep dd-auth-jwt | cut -d' ' -f2`, { encoding: 'utf8' }).trim();
            ddAccessToken = execSync(`ddtool auth token --datacenter ${this.config.dd_datacenter} apm-trino`, { encoding: 'utf8' }).trim();
          } catch (error) {
            console.error('Failed to generate fresh tokens:', error);
            // Fall back to static tokens if available
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

  private async handleTrinoQuery(args: any) {
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