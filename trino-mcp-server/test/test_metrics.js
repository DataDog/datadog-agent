#!/usr/bin/env node

import { spawn } from 'child_process';
import { execSync } from 'child_process';

async function testMetricsTools() {
    console.log('🧪 Starting Trino MCP Metrics Tests...\n');

    // Generate fresh tokens if dynamic tokens are enabled
    if (process.env.USE_DYNAMIC_TOKENS === 'true') {
        console.log('🔄 Generating fresh authentication tokens...');
        try {
            const ddAuthJWT = execSync(`ddauth obo -d ${process.env.DD_DATACENTER} | grep dd-auth-jwt | cut -d' ' -f2`, { encoding: 'utf8' }).trim();
            const ddAccessToken = execSync(`ddtool auth token --datacenter ${process.env.DD_DATACENTER} apm-trino`, { encoding: 'utf8' }).trim();

            process.env.DD_AUTH_JWT = ddAuthJWT;
            process.env.DD_ACCESS_TOKEN = ddAccessToken;
            console.log('✅ Fresh tokens generated successfully\n');
        } catch (error) {
            console.log('⚠️ Failed to generate fresh tokens, using static tokens from environment\n');
        }
    }

    const tests = [
        {
            name: 'List Available Tools (including metrics)',
            request: {
                jsonrpc: '2.0',
                id: 0,
                method: 'tools/list',
                params: {}
            }
        },
        {
            name: 'Custom Metrics Query - System CPU',
            request: {
                jsonrpc: '2.0',
                id: 1,
                method: 'tools/call',
                params: {
                    name: 'query_custom_metrics',
                    arguments: {
                        metrics_query: 'avg:system.cpu.user{*}',
                        time_range: '1h',
                        limit: 5
                    }
                }
            }
        },
        {
            name: 'Metrics Summary',
            request: {
                jsonrpc: '2.0',
                id: 2,
                method: 'tools/call',
                params: {
                    name: 'query_metrics_summary',
                    arguments: {
                        time_range: '1h',
                        limit: 5
                    }
                }
            }
        },
        {
            name: 'Metrics by Service',
            request: {
                jsonrpc: '2.0',
                id: 3,
                method: 'tools/call',
                params: {
                    name: 'query_metrics_by_service',
                    arguments: {
                        time_range: '1h',
                        limit: 5
                    }
                }
            }
        },
        {
            name: 'Trino Client Metrics (Documentation Example)',
            request: {
                jsonrpc: '2.0',
                id: 4,
                method: 'tools/call',
                params: {
                    name: 'query_custom_metrics',
                    arguments: {
                        metrics_query: 'count:trino.client.query.duration{*} by {query_source,client_experiment}.as_count()',
                        time_range: '1h',
                        unnest_timeseries: false,
                        limit: 10
                    }
                }
            }
        }
    ];

    // Run each test
    for (const test of tests) {
        console.log(`🔍 Testing: ${test.name}`);
        console.log('Request:', JSON.stringify(test.request, null, 2));

        try {
            const result = await sendMCPRequest(test.request);
            console.log('✅ Response:', JSON.stringify(result, null, 2));
        } catch (error) {
            console.error('❌ Error:', error.message);
        }

        console.log('\n' + '='.repeat(60) + '\n');
    }

    console.log('🎉 Metrics testing completed!');
}

function sendMCPRequest(request) {
    return new Promise((resolve, reject) => {
        // Set environment variables for dynamic token generation
        const env = {
            ...process.env,
            TRINO_SERVER: process.env.TRINO_SERVER || 'trino-gateway.us1.staging.dog',
            TRINO_CATALOG: process.env.TRINO_CATALOG || 'eventplatform',
            TRINO_SCHEMA: process.env.TRINO_SCHEMA || 'system',
            TRINO_USER: process.env.TRINO_USER || 'jim.wilson',
            TRINO_AUTH_TYPE: process.env.TRINO_AUTH_TYPE || 'datadog',
            DD_ORG_ID: process.env.DD_ORG_ID || '2',
            DD_CLIENT_ID: process.env.DD_CLIENT_ID || 'trino-cli',
            DD_USER_UUID: process.env.DD_USER_UUID,
            DD_DATACENTER: process.env.DD_DATACENTER || 'us1.staging.dog',
            USE_DYNAMIC_TOKENS: process.env.USE_DYNAMIC_TOKENS || 'true'
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
testMetricsTools().catch(console.error); 