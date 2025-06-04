#!/usr/bin/env node

import { Trino } from 'trino-client';

async function testTrinoConnection() {
    try {
        console.log('Testing Trino connection...');
        
        const clientConfig = {
            server: `https://${process.env.TRINO_SERVER}`,
            catalog: process.env.TRINO_CATALOG,
            schema: process.env.TRINO_SCHEMA,
            user: process.env.TRINO_USER,
            extraHeaders: {}
        };

        // Add Datadog authentication headers
        if (process.env.DD_ACCESS_TOKEN) {
            clientConfig.extraHeaders['Authorization'] = `Bearer ${process.env.DD_ACCESS_TOKEN}`;
        }

        if (process.env.DD_ORG_ID) {
            clientConfig.extraHeaders['X-Trino-Extra-Credential'] = `orgId=${process.env.DD_ORG_ID}`;
        }

        if (process.env.DD_CLIENT_ID) {
            const existingCreds = clientConfig.extraHeaders['X-Trino-Extra-Credential'];
            clientConfig.extraHeaders['X-Trino-Extra-Credential'] = 
                existingCreds ? `${existingCreds}, clientId=${process.env.DD_CLIENT_ID}` : `clientId=${process.env.DD_CLIENT_ID}`;
        }

        if (process.env.DD_USER_UUID) {
            const existingCreds = clientConfig.extraHeaders['X-Trino-Extra-Credential'];
            clientConfig.extraHeaders['X-Trino-Extra-Credential'] = 
                existingCreds ? `${existingCreds}, userUuid=${process.env.DD_USER_UUID}` : `userUuid=${process.env.DD_USER_UUID}`;
        }

        if (process.env.DD_AUTH_JWT) {
            const existingCreds = clientConfig.extraHeaders['X-Trino-Extra-Credential'];
            clientConfig.extraHeaders['X-Trino-Extra-Credential'] = 
                existingCreds ? `${existingCreds}, ddAuthJWT=${process.env.DD_AUTH_JWT}` : `ddAuthJWT=${process.env.DD_AUTH_JWT}`;
        }

        if (process.env.DD_ORG_ID) {
            clientConfig.extraHeaders['X-Trino-Client-Tags'] = `org_id=${process.env.DD_ORG_ID}`;
        }

        console.log('Client config:', {
            server: clientConfig.server,
            catalog: clientConfig.catalog,
            schema: clientConfig.schema,
            user: clientConfig.user,
            hasAuthHeaders: Object.keys(clientConfig.extraHeaders).length > 0
        });

        const client = Trino.create(clientConfig);
        
        // Test with a simple query first
        console.log('Executing test query...');
        const testQuery = 'SELECT 1 as test_col';
        const iter = await client.query(testQuery);
        
        const rows = [];
        for await (const queryResult of iter) {
            if (queryResult.data) {
                rows.push(...queryResult.data);
            }
        }
        
        console.log('Test query successful!');
        console.log('Results:', rows);
        
        // Now try to list available tracks
        console.log('\\nTrying to list available tracks...');
        const tracksQuery = 'SELECT DISTINCT track_name FROM eventplatform.system.tracks ORDER BY track_name LIMIT 10';
        const tracksIter = await client.query(tracksQuery);
        
        const trackRows = [];
        for await (const queryResult of tracksIter) {
            if (queryResult.data) {
                trackRows.push(...queryResult.data);
            }
        }
        
        console.log('Available tracks:');
        console.log(trackRows);
        
    } catch (error) {
        console.error('Error:', error);
        console.error('Error details:', {
            message: error.message,
            stack: error.stack,
            cause: error.cause
        });
    }
}

testTrinoConnection(); 