# Trino MCP Server Tests

This directory contains various test scripts for the Trino MCP server and NetFlow functionality.

## Prerequisites

1. **Environment Setup**: Copy and configure your environment variables:
   ```bash
   cp ../env.example ../env.datadog
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

## Environment Variables

Most tests require these environment variables (see `../env.example`):

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