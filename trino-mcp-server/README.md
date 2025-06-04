# Trino MCP Server

A Model Context Protocol (MCP) server that provides Trino/Presto query capabilities for accessing Datadog Event Platform data directly from Cursor IDE.

## Features

- Execute arbitrary Trino/Presto SQL queries
- Pre-built NetFlow data analysis queries
- Support for multiple authentication methods
- Built-in query limits and safety features
- Easy integration with Cursor IDE

## Installation

1. **Clone or download this project**
2. **Install dependencies:**
   ```bash
   cd trino-mcp-server
   npm install
   ```

3. **Build the project:**
   ```bash
   npm run build
   ```

4. **Configure environment variables:**
   ```bash
   # Copy and configure the Datadog-specific environment file
   cp env.datadog.example env.datadog  # if you don't have env.datadog yet
   # Edit env.datadog with your Trino server details and Datadog credentials
   ```

## Configuration

Set up your Trino connection by configuring the `env.datadog` file with your Datadog-specific settings:

```bash
# Required - Trino Server Configuration
TRINO_SERVER=trino-gateway.us1.staging.dog
TRINO_CATALOG=eventplatform
TRINO_SCHEMA=system
TRINO_USER=your-username

# Required - Datadog Authentication
TRINO_AUTH_TYPE=datadog
DD_ORG_ID=2
DD_CLIENT_ID=trino-cli
DD_USER_UUID=your-uuid
DD_DATACENTER=us1.staging.dog

# Token refresh commands (for when tokens expire):
# DD_AUTH_JWT: ddauth obo -d us1.staging.dog | grep dd-auth-jwt | cut -d' ' -f2
# DD_ACCESS_TOKEN: ddtool auth token --datacenter us1.staging.dog apm-trino
```

**Note:** Tests and scripts use `source env.datadog` to load these environment variables.

## Dynamic Token Authentication

The Trino MCP server supports **dynamic token generation** for Datadog authentication, which automatically refreshes expired tokens without manual intervention.

### How Dynamic Tokens Work

When `USE_DYNAMIC_TOKENS=true` is set, the server will:

1. **Check existing tokens** - If `DD_AUTH_JWT` and `DD_ACCESS_TOKEN` are missing or expired
2. **Generate fresh tokens** - Automatically run these commands:
   ```bash
   # Generate fresh JWT token
   ddauth obo -d $DD_DATACENTER | grep dd-auth-jwt | cut -d' ' -f2
   
   # Generate fresh access token  
   ddtool auth token --datacenter $DD_DATACENTER apm-trino
   ```
3. **Use fresh tokens** - Apply the new tokens for authentication
4. **Fallback gracefully** - If token generation fails, fall back to static tokens from environment

### Enabling Dynamic Tokens

**Option 1: Environment Variable**
```bash
# In your env.datadog file
USE_DYNAMIC_TOKENS=true
```

**Option 2: Cursor Configuration**
```json
{
  "mcpServers": {
    "trino-netflow": {
      "command": "node",
      "args": ["/path/to/trino-mcp-server/dist/index.js"],
      "env": {
        "TRINO_SERVER": "trino-gateway.us1.staging.dog",
        "TRINO_CATALOG": "eventplatform",
        "TRINO_SCHEMA": "system",
        "TRINO_USER": "your-username",
        "TRINO_AUTH_TYPE": "datadog",
        "DD_ORG_ID": "2",
        "DD_CLIENT_ID": "trino-cli",
        "DD_USER_UUID": "your-uuid",
        "DD_DATACENTER": "us1.staging.dog",
        "USE_DYNAMIC_TOKENS": "true"
      }
    }
  }
}
```

### Prerequisites for Dynamic Tokens

To use dynamic tokens, you must have these tools installed and configured:

- **`ddauth`** - Datadog authentication CLI tool
- **`ddtool`** - Datadog development tools
- **Valid Datadog credentials** - Must be logged in and have appropriate permissions

### Benefits

- ✅ **No manual token refresh** - Tokens are generated automatically
- ✅ **Always current** - Fresh tokens every query/connection  
- ✅ **Secure** - No static long-lived tokens in configuration
- ✅ **Fallback support** - Works with static tokens as backup

### Troubleshooting Dynamic Tokens

If dynamic token generation fails:
1. **Check tool installation**: Ensure `ddauth` and `ddtool` are in your PATH
2. **Verify authentication**: Run `ddauth login` to re-authenticate  
3. **Check datacenter**: Ensure `DD_DATACENTER` matches your environment
4. **Fallback mode**: Set static tokens in `env.datadog` as backup

## Setting up with Cursor

1. **Add to your Cursor configuration:**
   Edit your Cursor settings and add the MCP server configuration:

   ```json
   {
     "mcpServers": {
       "trino-netflow": {
         "command": "node",
         "args": ["/path/to/trino-mcp-server/dist/index.js"],
         "env": {
           "TRINO_SERVER": "trino-gateway.us1.staging.dog",
           "TRINO_CATALOG": "eventplatform",
           "TRINO_SCHEMA": "system",
           "TRINO_USER": "your-username",
           "TRINO_AUTH_TYPE": "datadog",
           "DD_ORG_ID": "2",
           "DD_CLIENT_ID": "trino-cli",
           "DD_USER_UUID": "your-uuid",
           "DD_DATACENTER": "us1.staging.dog"
         }
       }
     }
   }
   ```

2. **Restart Cursor** to load the MCP server

## Available Tools

### 1. `query_trino`
Execute arbitrary Trino SQL queries.

**Parameters:**
- `query` (required): The SQL query to execute
- `limit` (optional): Maximum rows to return (default: 1000)

**Example:**
```sql
SELECT COUNT(*) FROM eventplatform.system.ndmflow_events WHERE "@timestamp" > current_timestamp - interval '1' hour
```

### 2. `query_netflow_summary`
Get a pre-built summary of NetFlow data by exporter and domain.

**Parameters:**
- `exporter_ip` (optional): Filter by specific exporter IP
- `domain_filter` (optional): Filter by domain pattern (supports wildcards)
- `time_range` (optional): Time range like "1h", "24h", "7d" (default: "1h")
- `limit` (optional): Maximum results (default: 10)

**Example usage in Cursor:**
- "Show me NetFlow summary for the last 24 hours"
- "Get NetFlow data for exporter 192.168.1.1"
- "Show top domains by bytes for exporter 10.0.0.1"

### 3. `get_available_tracks`
List available tracks in the event platform.

## Usage Examples

Once configured in Cursor, you can use natural language to query your data:

1. **"Show me the top 10 domains by traffic in the last hour"**
   - Uses `query_netflow_summary` with default parameters

2. **"Run a custom query to find all flows from 192.168.1.0/24 network"**
   - Uses `query_trino` with a custom SQL query

3. **"What NetFlow exporters do we have data from?"**
   - Uses `query_trino` to query distinct exporter IPs

4. **"Show me flows with more than 1GB of data"**
   - Uses `query_trino` with byte filtering

## Query Examples

Here are some useful Trino queries you can run:

### Basic NetFlow Summary (your original query)
```sql
SELECT exporterip, domain, SUM("@bytes") AS total_bytes
FROM (
    SELECT "@bytes", "@source.ip" AS clientip, "@exporter.ip" as exporterip, "@destination.geoip.as.domain" AS domain
    FROM TABLE(
        eventplatform.system.track(
            TRACK => 'ndmflow',
            QUERY => '@exporter.ip:192.168.128.254 @destination.geoip.as.domain:*',
            COLUMNS => ARRAY['@source.ip', '@exporter.ip', '@destination.geoip.as.domain', '@bytes'],
            OUTPUT_TYPES => ARRAY['varchar', 'varchar', 'varchar', 'int']
        )
    )
)
GROUP BY exporterip, domain
ORDER BY total_bytes DESC
LIMIT 10
```

### Top Talkers by Source IP
```sql
SELECT "@source.ip" as source_ip, SUM("@bytes") as total_bytes, COUNT(*) as flow_count
FROM TABLE(
    eventplatform.system.track(
        TRACK => 'ndmflow',
        QUERY => '*',
        COLUMNS => ARRAY['@source.ip', '@bytes'],
        OUTPUT_TYPES => ARRAY['varchar', 'bigint'],
        TIME_RANGE => '1h'
    )
)
GROUP BY "@source.ip"
ORDER BY total_bytes DESC
LIMIT 20
```

### Protocol Distribution
```sql
SELECT "@ip.protocol" as protocol, COUNT(*) as flow_count, SUM("@bytes") as total_bytes
FROM TABLE(
    eventplatform.system.track(
        TRACK => 'ndmflow',
        QUERY => '*',
        COLUMNS => ARRAY['@ip.protocol', '@bytes'],
        OUTPUT_TYPES => ARRAY['varchar', 'bigint'],
        TIME_RANGE => '1h'  
    )
)
GROUP BY "@ip.protocol"  
ORDER BY flow_count DESC
```

## Development

```bash
# Run in development mode
npm run dev

# Build for production
npm run build

# Start production server
npm start
```

## Troubleshooting

1. **Connection issues**: Verify your Trino server URL and authentication
2. **Permission errors**: Ensure your user has access to the eventplatform catalog
3. **Query timeouts**: Use LIMIT clauses for large datasets
4. **MCP not loading**: Check Cursor's MCP server logs and configuration

## Security Notes

- Store sensitive credentials in environment variables, not in code
- Use least-privilege access for your Trino user
- Be cautious with query limits on large datasets
- Consider implementing query validation for production use

## License

MIT 