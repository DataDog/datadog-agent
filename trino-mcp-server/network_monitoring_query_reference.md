# Network Device Monitoring Query Reference

<!-- 
KEYWORDS: netflow, snmp, network monitoring, trino queries, mcp server, device metrics, 
bandwidth utilization, cpu usage, memory usage, interface monitoring, datadog agent monitoring,
network device analysis, flow analysis, device_ip queries, network troubleshooting
-->

## ⚠️ **IMPORTANT: Keep This File Updated**
**For AI Assistant**: When successfully executing new types of network monitoring queries that aren't documented here, add them to this file immediately. This ensures future queries benefit from tested patterns and prevents rediscovering working syntax.

**Update whenever you discover:**
- New working query patterns
- Additional SNMP metrics that work
- Different time window strategies  
- New NetFlow field combinations
- Alternative aggregation approaches
- Workarounds for query limitations

## 🎯 **How to Use This File**
**For Future Queries**: Reference this file by name or mention "network monitoring reference" to ensure these tested patterns are used.

**Purpose**: Contains all working NetFlow and SNMP query patterns discovered through MCP server testing.

**Context**: Created for Datadog Agent workspace with MCP Trino server configured with `TRINO_CATALOG: eventplatform`.

### 🚀 **How to Ensure This File Gets Referenced in Future Conversations**

#### **Option A: Direct Reference** ⭐ Most Reliable
```
"Check my network_monitoring_query_reference.md file and help me analyze device 10.100.70.150"
"Using the network_monitoring_query_reference.md file, help me get NetFlow data for device X"
```

#### **Option B: Keyword Trigger**
```
"I need to do network monitoring analysis using our established Trino query patterns"
"Help me with NetFlow SNMP queries using the reference patterns we created"
"Using our network monitoring reference, show me interface utilization for device X"
```

#### **Option C: File in Context**
- Keep this file open in Cursor when asking questions - it will be automatically visible in context
- This is the most seamless approach for ongoing work

#### **Option D: Context Keywords**
Use any of these phrases to trigger file discovery:
- "network monitoring reference"
- "NetFlow SNMP reference we created"  
- "established query patterns"
- "proven Trino patterns"
- "MCP server queries we tested"

### 💡 **Why This Works**
- **Semantic Search**: Keywords help locate relevant files automatically
- **Context Awareness**: Specific terms trigger searches for related content
- **File Structure**: Clear organization makes extracting the right queries easy
- **Tested Patterns**: All queries are validated and ready to use

**Remember**: When you mention network monitoring, NetFlow, SNMP, or device analysis in the future, referencing this file ensures you get working queries instead of having to rediscover syntax!

---

## MCP Server Configuration Status
✅ **Working Configuration**: The MCP server configured with `TRINO_CATALOG: eventplatform` successfully accesses both:
- `eventplatform` catalog (for NetFlow data)
- `squire` catalog (for SNMP/metrics data)

**Key Finding**: No configuration changes needed - cross-catalog queries work automatically.

---

## NetFlow Queries

### Core NetFlow Data Structure
- **Track**: `ndmflow`
- **Key Fields**: 
  - `@source.ip`, `@destination.ip`, `@device.ip` (⚠️ Important: use `@device.ip` not `device.ip`)
  - `@bytes`, `@packets`, `@exporter.ip`, `@timestamp`

### 1. Device-Specific NetFlow Summary
```sql
-- Get flow statistics for a specific device
SELECT COUNT(*) as total_flows, 
       SUM(CAST("@bytes" AS bigint)) as total_bytes, 
       SUM(CAST("@packets" AS bigint)) as total_packets, 
       COUNT(DISTINCT "@source.ip") as unique_sources, 
       COUNT(DISTINCT "@destination.ip") as unique_destinations 
FROM TABLE(eventplatform.system.track(
    TRACK => 'ndmflow', 
    COLUMNS => ARRAY['@source.ip', '@destination.ip', '@bytes', '@packets', '@device.ip'], 
    OUTPUT_TYPES => ARRAY['varchar', 'varchar', 'varchar', 'varchar', 'varchar']
)) 
WHERE "@device.ip" = '10.100.70.140'
```

### 2. Top Traffic Sources by Device
```sql
-- Get top traffic sources for a device (by bytes)
SELECT "@source.ip", 
       COUNT(*) as flow_count, 
       SUM(CAST("@bytes" AS bigint)) as total_bytes 
FROM TABLE(eventplatform.system.track(
    TRACK => 'ndmflow', 
    COLUMNS => ARRAY['@source.ip', '@bytes', '@device.ip'], 
    OUTPUT_TYPES => ARRAY['varchar', 'varchar', 'varchar']
)) 
WHERE "@device.ip" = '10.100.70.140' 
GROUP BY "@source.ip" 
ORDER BY total_bytes DESC 
LIMIT 10
```

### 3. Recent NetFlow Records for Device
```sql
-- Get recent flow records for a device
SELECT "@source.ip", "@destination.ip", "@bytes", "@packets", "@exporter.ip", "@device.ip", "@timestamp" 
FROM TABLE(eventplatform.system.track(
    TRACK => 'ndmflow', 
    COLUMNS => ARRAY['@source.ip', '@destination.ip', '@bytes', '@packets', '@exporter.ip', '@device.ip', '@timestamp'], 
    OUTPUT_TYPES => ARRAY['varchar', 'varchar', 'varchar', 'varchar', 'varchar', 'varchar', 'varchar']
)) 
WHERE "@device.ip" = '10.100.70.140' 
ORDER BY "@timestamp" DESC 
LIMIT 20
```

### 4. NetFlow Exporter Summary
```sql
-- Get summary by exporter (shows all devices generating NetFlow)
SELECT "@exporter.ip" as exporter_ip,
       COUNT(*) as flow_count,
       SUM(CAST("@bytes" AS bigint)) as total_bytes
FROM TABLE(eventplatform.system.track(
    TRACK => 'ndmflow',
    COLUMNS => ARRAY['@exporter.ip', '@bytes'],
    OUTPUT_TYPES => ARRAY['varchar', 'varchar']
))
WHERE "@bytes" IS NOT NULL
GROUP BY "@exporter.ip"
ORDER BY total_bytes DESC
LIMIT 20
```

---

## SNMP Metrics Queries

### Core Metrics Query Structure
- **Function**: `squire.timeseries.metrics()`
- **Key Parameters**: 
  - `QUERY => 'metric_name{tags}'`
  - `MIN_TIMESTAMP => -seconds` (negative for past)
  - `MAX_TIMESTAMP => 0` (0 for now)
  - `UNNEST_TIMESERIES => true/false`

### 1. CPU Usage for Device
```sql
-- Get CPU usage over time
select * from TABLE(squire.timeseries.metrics(
    QUERY => 'avg:snmp.cpu.usage{device_ip:10.100.70.140}', 
    MIN_TIMESTAMP => -3600, MAX_TIMESTAMP => 0, 
    UNNEST_TIMESERIES => true
))
```

### 2. Memory Usage for Device
```sql
-- Get memory usage over time
select * from TABLE(squire.timeseries.metrics(
    QUERY => 'avg:snmp.memory.usage{device_ip:10.100.70.140}', 
    MIN_TIMESTAMP => -1800, MAX_TIMESTAMP => 0, 
    UNNEST_TIMESERIES => true
))
```

### 3. Interface Bandwidth Utilization (Inbound)
```sql
-- Get inbound bandwidth utilization per interface (time series)
select * from TABLE(squire.timeseries.metrics(
    QUERY => 'avg:snmp.ifBandwidthInUsage.rate{device_ip:10.100.70.140} by {interface}', 
    MIN_TIMESTAMP => -1800, MAX_TIMESTAMP => 0, 
    UNNEST_TIMESERIES => true
))
```

### 4. Interface Bandwidth Utilization (Outbound)
```sql
-- Get outbound bandwidth utilization per interface (time series)
select * from TABLE(squire.timeseries.metrics(
    QUERY => 'avg:snmp.ifBandwidthOutUsage.rate{device_ip:10.100.70.140} by {interface}', 
    MIN_TIMESTAMP => -1800, MAX_TIMESTAMP => 0, 
    UNNEST_TIMESERIES => true
))
```

### 5. Interface Summary (Max Utilization)
```sql
-- Get max bandwidth utilization summary per interface (aggregated view)
select * from TABLE(squire.timeseries.metrics(
    QUERY => 'max:snmp.ifBandwidthInUsage.rate{device_ip:10.100.70.140} by {interface}', 
    MIN_TIMESTAMP => -900, MAX_TIMESTAMP => 0, 
    UNNEST_TIMESERIES => false
))
```

Query: `max:snmp.ifBandwidthOutUsage.rate{device_ip:10.100.70.140} by {interface}`

---

## Log Queries

### Core Log Data Structure
- **Track**: `logs`
- **Key Fields**: 
  - `message`, `timestamp`, `env`, `@source`
  - `@duration`, `@level`, `host`, `service`

### 1. Basic Log Search
```sql
-- Get recent logs with specific filters
SELECT message, timestamp, env, "@duration" 
FROM TABLE(
  eventplatform.system.track(
    TRACK => 'logs', 
    QUERY => 'message:* @source:trino-cli -host:excluded-host',
    COLUMNS => ARRAY['message','timestamp', 'env', '@duration'],
    OUTPUT_TYPES => ARRAY['varchar', 'int', 'varchar', 'double'], 
    MIN_TIMESTAMP => -3600, 
    MAX_TIMESTAMP => 0
  )
) 
WHERE env IN ('staging', 'prod')
ORDER BY timestamp DESC
LIMIT 20
```

### 2. Error Log Analysis
```sql
-- Find error logs across services
SELECT message, timestamp, env, "@source"
FROM TABLE(
  eventplatform.system.track(
    TRACK => 'logs',
    QUERY => 'status:error OR message:*exception* OR message:*error*',
    COLUMNS => ARRAY['message', 'timestamp', 'env', '@source'],
    OUTPUT_TYPES => ARRAY['varchar', 'int', 'varchar', 'varchar'],
    MIN_TIMESTAMP => -3600,
    MAX_TIMESTAMP => 0
  )
)
WHERE env IN ('staging', 'prod')
ORDER BY timestamp DESC
LIMIT 50
```

### 3. Service-Specific Log Volume
```sql
-- Get log counts by service and environment
SELECT "@source" as service, COUNT(*) as log_count, env
FROM TABLE(
  eventplatform.system.track(
    TRACK => 'logs',
    QUERY => 'message:*',
    COLUMNS => ARRAY['@source', 'env'],
    OUTPUT_TYPES => ARRAY['varchar', 'varchar'],
    MIN_TIMESTAMP => -3600,
    MAX_TIMESTAMP => 0
  )
)
WHERE "@source" IS NOT NULL
GROUP BY "@source", env
ORDER BY log_count DESC
LIMIT 15
```

### 4. Performance Analysis
```sql
-- Analyze log durations by service
SELECT "@source", env, 
       AVG(CAST("@duration" AS double)) as avg_duration,
       MAX(CAST("@duration" AS double)) as max_duration,
       COUNT(*) as log_count
FROM TABLE(
  eventplatform.system.track(
    TRACK => 'logs',
    QUERY => '@duration:>0',
    COLUMNS => ARRAY['@source', 'env', '@duration'],
    OUTPUT_TYPES => ARRAY['varchar', 'varchar', 'double'],
    MIN_TIMESTAMP => -3600,
    MAX_TIMESTAMP => 0
  )
)
WHERE "@duration" IS NOT NULL AND "@source" IS NOT NULL
GROUP BY "@source", env
ORDER BY avg_duration DESC
LIMIT 10
```

### MCP Server Functions for Logs
- **`query_logs`**: Flexible log search with custom columns and filters
- **`query_logs_summary`**: Aggregated log data grouped by dimensions

### Log Query Syntax Examples
- **Basic search**: `message:*` (all logs)
- **Service filter**: `@source:trino-cli`
- **Exclude hosts**: `-host:excluded-hostname`
- **Error filtering**: `status:error OR message:*exception*`
- **Duration filter**: `@duration:>1000` (logs with duration > 1000ms)
- **Environment**: Use WHERE clause: `env IN ('staging', 'prod')`

---

## Common Parameters and Time Windows

### Time Windows (MIN_TIMESTAMP values)
- `-300` = 5 minutes
- `-900` = 15 minutes
- `-1800` = 30 minutes
- `-3600` = 1 hour
- `-7200` = 2 hours
- `-86400` = 24 hours

### UNNEST_TIMESERIES Options
- `true` = Returns individual time points (good for trending)
- `false` = Returns aggregated arrays (good for summaries)

### Available SNMP Metrics (Common)
- `snmp.cpu.usage` - CPU utilization percentage
- `snmp.memory.usage` - Memory utilization percentage
- `snmp.ifBandwidthInUsage.rate` - Interface inbound bandwidth utilization
- `snmp.ifBandwidthOutUsage.rate` - Interface outbound bandwidth utilization

---

## MCP Tool Functions to Use

### NetFlow Functions
- `mcp_trino-netflow_query_trino` - Direct SQL queries (recommended)
- `mcp_trino-netflow_query_netflow_summary` - Basic NetFlow summary
- `mcp_trino-netflow_query_netflow_talkers` - Top talkers

### Metrics Functions
- `mcp_trino-netflow_query_trino` - Direct SQL queries (recommended for metrics)
- ⚠️ Avoid helper functions like `query_metrics_summary` - they have issues

---

## Quick Start Templates

### Device NetFlow Summary
```sql
SELECT COUNT(*) as flows, SUM(CAST("@bytes" AS bigint)) as bytes, COUNT(DISTINCT "@source.ip") as sources
FROM TABLE(eventplatform.system.track(TRACK => 'ndmflow', COLUMNS => ARRAY['@source.ip', '@bytes', '@device.ip'], OUTPUT_TYPES => ARRAY['varchar', 'varchar', 'varchar']))
WHERE "@device.ip" = 'YOUR_DEVICE_IP'
```

### Device SNMP Summary
```sql
select * from TABLE(squire.timeseries.metrics(QUERY => 'avg:snmp.cpu.usage{device_ip:YOUR_DEVICE_IP}', MIN_TIMESTAMP => -3600, MAX_TIMESTAMP => 0, UNNEST_TIMESERIES => true))
```

### Interface Bandwidth Summary
```sql
select * from TABLE(squire.timeseries.metrics(QUERY => 'max:snmp.ifBandwidthInUsage.rate{device_ip:YOUR_DEVICE_IP} by {interface}', MIN_TIMESTAMP => -900, MAX_TIMESTAMP => 0, UNNEST_TIMESERIES => false))
```

---

## Troubleshooting Notes

1. **Always use `@device.ip`** not `device.ip` for NetFlow queries
2. **Use direct `query_trino` function** instead of helper functions for reliability
3. **MCP server auto-refreshes tokens** - no manual intervention needed
4. **Cross-catalog access works** - don't modify server configuration
5. **Time parameters are in seconds** and negative for past time
6. **Cast bytes/packets to bigint** for proper aggregation in NetFlow queries

---

## Example Device Analysis Workflow

1. **Get NetFlow Summary**: Check overall traffic patterns
2. **Identify Top Sources**: Find highest bandwidth consumers
3. **Check SNMP Metrics**: Verify CPU/memory health
4. **Analyze Interface Utilization**: Identify bottlenecks
5. **Review Time Trends**: Look for patterns over time

Replace `YOUR_DEVICE_IP` with actual device IP (e.g., `10.100.70.140`) in queries above. 