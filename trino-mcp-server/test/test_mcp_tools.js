import { spawn } from 'child_process';

// Test MCP server tools
async function testMCPTools() {
    console.log('Testing MCP server NetFlow tools...\n');

    const tests = [
        {
            name: 'Get Available Tracks',
            request: {
                jsonrpc: '2.0',
                id: 1,
                method: 'tools/call',
                params: {
                    name: 'get_available_tracks',
                    arguments: {}
                }
            }
        },
        {
            name: 'NetFlow Summary',
            request: {
                jsonrpc: '2.0',
                id: 2,
                method: 'tools/call',
                params: {
                    name: 'query_netflow_summary',
                    arguments: { limit: 5 }
                }
            }
        },
        {
            name: 'NetFlow Talkers - Top Source IPs',
            request: {
                jsonrpc: '2.0',
                id: 3,
                method: 'tools/call',
                params: {
                    name: 'query_netflow_talkers',
                    arguments: {
                        limit: 5,
                        group_by: 'source_ip'
                    }
                }
            }
        },
        {
            name: 'NetFlow Talkers - Both Source and Destination',
            request: {
                jsonrpc: '2.0',
                id: 4,
                method: 'tools/call',
                params: {
                    name: 'query_netflow_talkers',
                    arguments: {
                        limit: 3,
                        group_by: 'both'
                    }
                }
            }
        }
    ];

    // First, list available tools
    const listToolsRequest = {
        jsonrpc: '2.0',
        id: 0,
        method: 'tools/list',
        params: {}
    };

    console.log('Listing available tools...');
    const result = await sendMCPRequest(listToolsRequest);
    console.log('Available tools:', JSON.stringify(result, null, 2));
    console.log('\n' + '='.repeat(50) + '\n');

    // Run each test
    for (const test of tests) {
        console.log(`Testing: ${test.name}`);
        console.log('Request:', JSON.stringify(test.request, null, 2));

        try {
            const result = await sendMCPRequest(test.request);
            console.log('Response:', JSON.stringify(result, null, 2));
        } catch (error) {
            console.error('Error:', error.message);
        }

        console.log('\n' + '='.repeat(50) + '\n');
    }
}

function sendMCPRequest(request) {
    return new Promise((resolve, reject) => {
        // Set environment variables for dynamic token generation
        const env = {
            ...process.env,
            TRINO_SERVER: 'trino-gateway.us1.staging.dog',
            TRINO_CATALOG: 'eventplatform',
            TRINO_SCHEMA: 'system',
            TRINO_USER: 'jim.wilson',
            TRINO_AUTH_TYPE: 'datadog',
            DD_ORG_ID: '2',
            DD_CLIENT_ID: 'trino-cli',
            DD_USER_UUID: '976714e4-1b76-11ee-87d8-aa0300114d4e',
            DD_DATACENTER: 'us1.staging.dog',
            USE_DYNAMIC_TOKENS: 'true'
        };

        const child = spawn('node', ['../dist/index.js'], {
            stdio: ['pipe', 'pipe', 'pipe'],
            env: env
        });

        let output = '';
        let errorOutput = '';

        child.stdout.on('data', (data) => {
            output += data.toString();
        });

        child.stderr.on('data', (data) => {
            errorOutput += data.toString();
        });

        child.on('close', (code) => {
            if (code !== 0) {
                reject(new Error(`Process exited with code ${code}. Error: ${errorOutput}`));
                return;
            }

            try {
                // Parse JSON-RPC response
                const lines = output.trim().split('\n');
                const lastLine = lines[lines.length - 1];
                const response = JSON.parse(lastLine);
                resolve(response);
            } catch (error) {
                reject(new Error(`Failed to parse response: ${error.message}. Output: ${output}`));
            }
        });

        // Send the request
        child.stdin.write(JSON.stringify(request) + '\n');
        child.stdin.end();
    });
}

// Run the tests
testMCPTools().catch(console.error); 