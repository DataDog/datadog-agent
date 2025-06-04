# Trino MCP Server Tests

This directory contains various test scripts for the Trino MCP server and NetFlow functionality.

## Prerequisites

1. **Environment Setup**: Copy and configure your environment variables:
   ```bash
   # If you don't have env.datadog yet, create it with your Datadog credentials
   # Edit env.datadog with your credentials
   ```

2. **Build the MCP Server**:
   ```bash
   cd ..
   npm run build
   ```

3. **Install Dependencies**:
   ```bash
   cd ..
   npm install
   ```

## Test Files

### MCP Server Tests

#### `test_mcp_tools.js` - MCP Tools Testing
Tests the MCP server's JSON-RPC interface and available tools.

**What it tests:**
- Available tracks discovery
- NetFlow summary queries
- NetFlow talkers (top source IPs)
- NetFlow talkers (source and destination)

**How to run:**
```bash
cd trino-mcp-server/test
node test_mcp_tools.js
```

**Features:**
- Uses dynamic token generation
- Tests JSON-RPC communication
- Validates tool responses

#### `test_working_format.js` - Working Format Test
Tests specific NetFlow query formats and data processing.

**How to run:**
```bash
cd trino-mcp-server/test
node test_working_format.js
```

#### `test_ndmflow_exact.js` - NDM Flow Exact Test
Tests exact NetFlow data matching and processing.

**How to run:**
```bash
cd trino-mcp-server/test
node test_ndmflow_exact.js
```

#### `test_fresh_tokens.js` - Fresh Token Test
Tests token generation and refresh functionality.

**How to run:**
```bash
cd trino-mcp-server/test
node test_fresh_tokens.js
```

### Direct Trino Tests

#### `test_netflow.js` - Direct NetFlow Query Test
Directly tests Trino NetFlow queries without MCP layer.

**What it tests:**
- Direct Trino client connection
- NetFlow data retrieval
- Authentication with Datadog

**How to run:**
```bash
cd trino-mcp-server/test
# Source environment variables
source ../env.datadog
node test_netflow.js
```

#### `test_trino.js` - Basic Trino Connection Test
Tests basic Trino connectivity and authentication.

**How to run:**
```bash
cd trino-mcp-server/test
# Source environment variables
source ../env.datadog
node test_trino.js
```

### Utility Scripts

#### `list_all_tracks.js` - Track Discovery Utility
Comprehensive discovery script to explore available data tracks in the Event Platform.

**What it does:**
- Lists all tables in eventplatform.system
- Queries the tracks table directly
- Lists all schemas in eventplatform catalog
- Tests common track names for data availability
- Supports dynamic token generation

**How to run:**
```bash
cd trino-mcp-server/test
# With environment variables
source ../env.datadog && USE_DYNAMIC_TOKENS=true node list_all_tracks.js

# Or with inline environment (recommended)
TRINO_SERVER=trino-gateway.us1.staging.dog TRINO_CATALOG=eventplatform TRINO_SCHEMA=system TRINO_USER=jim.wilson TRINO_AUTH_TYPE=datadog DD_ORG_ID=2 DD_CLIENT_ID=trino-cli DD_USER_UUID=your-uuid DD_DATACENTER=us1.staging.dog USE_DYNAMIC_TOKENS=true node list_all_tracks.js
```

**Features:**
- Multiple discovery approaches
- Dynamic token generation 
- Error handling for non-existent tracks
- Debug output for troubleshooting

## Environment Variables

Most tests require these environment variables (configured in `../env.datadog`):

```bash
TRINO_SERVER=trino-gateway.us1.staging.dog
TRINO_CATALOG=eventplatform
TRINO_SCHEMA=system
TRINO_USER=your.username
TRINO_AUTH_TYPE=datadog
DD_ORG_ID=2
DD_CLIENT_ID=trino-cli
DD_USER_UUID=your-uuid
DD_DATACENTER=us1.staging.dog
DD_AUTH_JWT=your-jwt-token
DD_ACCESS_TOKEN=your-access-token
```

## Dynamic Token Authentication

Several test scripts support **dynamic token generation** to automatically refresh Datadog authentication tokens.

### How It Works in Tests

When `USE_DYNAMIC_TOKENS=true` is set, test scripts will:

1. **Check for existing tokens** - Look for `DD_AUTH_JWT` and `DD_ACCESS_TOKEN` in environment
2. **Generate fresh tokens automatically** if tokens are missing or when dynamic mode is enabled:
   ```bash
   # Generate fresh JWT token  
   ddauth obo -d $DD_DATACENTER | grep dd-auth-jwt | cut -d' ' -f2
   
   # Generate fresh access token
   ddtool auth token --datacenter $DD_DATACENTER apm-trino
   ```
3. **Use fresh tokens** for authentication
4. **Fall back to static tokens** if generation fails

### Tests Supporting Dynamic Tokens

- ✅ `test_mcp_tools.js` - Uses dynamic tokens for MCP server testing
- ✅ `list_all_tracks.js` - Supports both static and dynamic token modes
- ❌ Direct Trino tests (`test_trino.js`, `test_netflow.js`) - Use static tokens only

### Usage Examples

**With dynamic tokens (recommended):**
```bash
cd trino-mcp-server/test

# Option 1: Set environment variable
USE_DYNAMIC_TOKENS=true node test_mcp_tools.js

# Option 2: Inline with other environment variables  
source ../env.datadog && USE_DYNAMIC_TOKENS=true node list_all_tracks.js

# Option 3: Full inline setup
TRINO_SERVER=trino-gateway.us1.staging.dog TRINO_CATALOG=eventplatform TRINO_SCHEMA=system TRINO_USER=jim.wilson TRINO_AUTH_TYPE=datadog DD_ORG_ID=2 DD_CLIENT_ID=trino-cli DD_USER_UUID=your-uuid DD_DATACENTER=us1.staging.dog USE_DYNAMIC_TOKENS=true node list_all_tracks.js
```

**With static tokens:**
```bash
cd trino-mcp-server/test

# Load static tokens from env.datadog
source ../env.datadog
node test_trino.js
```

### Prerequisites

To use dynamic tokens in tests, ensure you have:

- **`ddauth`** CLI tool installed and configured
- **`ddtool`** CLI tool installed  
- **Valid Datadog authentication** - Run `ddauth login` if needed
- **Correct datacenter** - Set `DD_DATACENTER` to match your environment

### Benefits of Dynamic Tokens

- ✅ **Never expire** - Fresh tokens generated each run
- ✅ **No manual maintenance** - No need to update `env.datadog` when tokens expire  
- ✅ **More secure** - No long-lived static tokens in files
- ✅ **Reliable testing** - Tests won't fail due to expired tokens

## Running All Tests

To run all tests in sequence:

```bash
#!/bin/bash
cd trino-mcp-server/test

# Source environment for direct Trino tests
source ../env.datadog

echo "Running MCP Server Tests..."
node test_mcp_tools.js
echo -e "\n" + "="*50 + "\n"

echo "Running Working Format Test..."
node test_working_format.js
echo -e "\n" + "="*50 + "\n"

echo "Running Direct NetFlow Test..."
node test_netflow.js
echo -e "\n" + "="*50 + "\n"

echo "Running Basic Trino Test..."
node test_trino.js
echo -e "\n" + "="*50 + "\n"

echo "Running Track Discovery Utility..."
USE_DYNAMIC_TOKENS=true node list_all_tracks.js
```

## Troubleshooting

### Common Issues

1. **"Process exited with code 1"**: Check that the MCP server is built (`npm run build`)
2. **"Authentication failed"**: Verify your tokens in `env.datadog` are current
3. **"Connection refused"**: Ensure you have access to the Trino staging environment
4. **"No data found"**: NetFlow data may not be available in the queried time range

### Debug Mode

For verbose output, set `DEBUG=1`:
```bash
DEBUG=1 node test_mcp_tools.js
```

### Token Refresh

If authentication fails, you may need to refresh your tokens:
```bash
# Get new tokens from your authentication source
# Update env.datadog with new DD_AUTH_JWT and DD_ACCESS_TOKEN
```

## Development

When adding new tests:
1. Follow the naming convention: `test_[feature].js`
2. Add documentation to this README
3. Include error handling and clear output messages
4. Test both success and failure scenarios 