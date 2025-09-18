#!/usr/bin/env node

import { Trino } from 'trino-client';

async function debugNetflowData() {
    try {
        console.log('=== NetFlow Debug Session ===');
        
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

        const client = Trino.create(clientConfig);

        // Test 1: Try to list available tracks/tables
        console.log('\n=== Test 1: Listing available tracks ===');
        try {
            const tracksQuery = `
            SELECT table_name 
            FROM information_schema.tables 
            WHERE table_schema = 'system' 
            AND table_name LIKE '%flow%'
            OR table_name LIKE '%ndm%'
            ORDER BY table_name`;
            
            const tracksIter = await client.query(tracksQuery);
            const trackRows = [];
            for await (const queryResult of tracksIter) {
                if (queryResult.data) {
                    trackRows.push(...queryResult.data);
                }
            }
            console.log('Available flow/ndm tables:', trackRows);
        } catch (error) {
            console.log('Error listing tables:', error.message);
        }

        // Test 2: Try different track names
        console.log('\n=== Test 2: Trying different track names ===');
        const trackNames = ['ndmflow', 'netflow', 'flows', 'ndm', 'flow'];
        
        for (const trackName of trackNames) {
            try {
                console.log(`Trying track: ${trackName}`);
                const testQuery = `
                SELECT COUNT(*) as count
                FROM TABLE(
                    eventplatform.system.track(
                        TRACK => '${trackName}',
                        QUERY => '*',
                        TIME_RANGE => '1h'
                    )
                )`;
                
                const iter = await client.query(testQuery);
                const rows = [];
                for await (const queryResult of iter) {
                    if (queryResult.data) {
                        rows.push(...queryResult.data);
                    }
                }
                console.log(`  ${trackName}: ${rows[0] ? rows[0][0] : 'No data'} records`);
            } catch (error) {
                console.log(`  ${trackName}: Error - ${error.message.substring(0, 100)}`);
            }
        }

        // Test 3: Try different time ranges for ndmflow
        console.log('\n=== Test 3: Trying different time ranges for ndmflow ===');
        const timeRanges = ['1h', '3h', '6h', '12h', '24h'];
        
        for (const timeRange of timeRanges) {
            try {
                console.log(`Trying time range: ${timeRange}`);
                const testQuery = `
                SELECT COUNT(*) as count
                FROM TABLE(
                    eventplatform.system.track(
                        TRACK => 'ndmflow',
                        QUERY => '*',
                        TIME_RANGE => '${timeRange}'
                    )
                )`;
                
                const iter = await client.query(testQuery);
                const rows = [];
                for await (const queryResult of iter) {
                    if (queryResult.data) {
                        rows.push(...queryResult.data);
                    }
                }
                console.log(`  ${timeRange}: ${rows[0] ? rows[0][0] : 'No data'} records`);
            } catch (error) {
                console.log(`  ${timeRange}: Error - ${error.message.substring(0, 100)}`);
            }
        }

        // Test 4: Try to get any data without specifying columns
        console.log('\n=== Test 4: Getting any ndmflow data without column specification ===');
        try {
            const simpleQuery = `
            SELECT *
            FROM TABLE(
                eventplatform.system.track(
                    TRACK => 'ndmflow',
                    QUERY => '*',
                    TIME_RANGE => '24h'
                )
            )
            LIMIT 1`;
            
            console.log('Query:', simpleQuery);
            const iter = await client.query(simpleQuery);
            const rows = [];
            for await (const queryResult of iter) {
                if (queryResult.data) {
                    rows.push(...queryResult.data);
                }
            }
            console.log('Simple query results:', rows);
        } catch (error) {
            console.log('Simple query error:', error.message);
        }

        // Test 5: Try direct table access if it exists
        console.log('\n=== Test 5: Trying direct table access ===');
        const directTables = ['ndmflow', 'netflow', 'flows'];
        
        for (const tableName of directTables) {
            try {
                console.log(`Trying direct access to: ${tableName}`);
                const directQuery = `SELECT COUNT(*) FROM eventplatform.system.${tableName}`;
                
                const iter = await client.query(directQuery);
                const rows = [];
                for await (const queryResult of iter) {
                    if (queryResult.data) {
                        rows.push(...queryResult.data);
                    }
                }
                console.log(`  ${tableName}: ${rows[0] ? rows[0][0] : 'No data'} records`);
            } catch (error) {
                console.log(`  ${tableName}: Error - ${error.message.substring(0, 100)}`);
            }
        }

        console.log('\n=== Debug session complete ===');
        
    } catch (error) {
        console.error('Debug session error:', error);
    }
}

debugNetflowData(); 