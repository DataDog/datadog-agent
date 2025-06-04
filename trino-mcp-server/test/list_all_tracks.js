#!/usr/bin/env node

import { Trino } from 'trino-client';
import { execSync } from 'child_process';

async function listAllTracks() {
    try {
        console.log('=== Listing All Available Tracks ===');

        const clientConfig = {
            server: `https://${process.env.TRINO_SERVER}`,
            catalog: process.env.TRINO_CATALOG,
            schema: process.env.TRINO_SCHEMA,
            user: process.env.TRINO_USER,
            extraHeaders: {}
        };

        // Dynamic token generation
        let ddAuthJWT = process.env.DD_AUTH_JWT;
        let ddAccessToken = process.env.DD_ACCESS_TOKEN;

        // Generate fresh tokens if dynamic tokens enabled or tokens are missing
        if (process.env.USE_DYNAMIC_TOKENS === 'true' || !ddAuthJWT || !ddAccessToken) {
            console.log('Generating fresh tokens...');
            try {
                ddAuthJWT = execSync(`ddauth obo -d ${process.env.DD_DATACENTER} | grep dd-auth-jwt | cut -d' ' -f2`, { encoding: 'utf8' }).trim();
                ddAccessToken = execSync(`ddtool auth token --datacenter ${process.env.DD_DATACENTER} apm-trino`, { encoding: 'utf8' }).trim();
                console.log('Fresh tokens generated successfully');
            } catch (error) {
                console.error('Failed to generate fresh tokens:', error.message);
                console.log('Falling back to static tokens if available...');
            }
        }

        // Add Datadog authentication headers
        if (ddAccessToken) {
            clientConfig.extraHeaders['Authorization'] = `Bearer ${ddAccessToken}`;
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

        if (ddAuthJWT) {
            const existingCreds = clientConfig.extraHeaders['X-Trino-Extra-Credential'];
            clientConfig.extraHeaders['X-Trino-Extra-Credential'] =
                existingCreds ? `${existingCreds}, ddAuthJWT=${ddAuthJWT}` : `ddAuthJWT=${ddAuthJWT}`;
        }

        if (process.env.DD_ORG_ID) {
            clientConfig.extraHeaders['X-Trino-Client-Tags'] = `org_id=${process.env.DD_ORG_ID}`;
        }

        const client = Trino.create(clientConfig);

        // Try to list all tables in eventplatform.system
        console.log('\n=== All tables in eventplatform.system ===');
        try {
            const allTablesQuery = `
            SELECT table_name, table_type 
            FROM information_schema.tables 
            WHERE table_schema = 'system' 
            ORDER BY table_name`;

            const iter = await client.query(allTablesQuery);
            const rows = [];
            for await (const queryResult of iter) {
                if (queryResult.data) {
                    rows.push(...queryResult.data);
                }
            }
            console.log('Available tables:');
            rows.forEach(row => {
                console.log(`  ${row[0]} (${row[1]})`);
            });
        } catch (error) {
            console.log('Error listing tables:', error.message);
        }

        // Try to use the tracks table function to get available tracks
        console.log('\n=== Trying to get tracks from tracks table ===');
        try {
            const tracksQuery = `SELECT * FROM eventplatform.system.tracks LIMIT 10`;

            const iter = await client.query(tracksQuery);
            const rows = [];
            for await (const queryResult of iter) {
                if (queryResult.data) {
                    rows.push(...queryResult.data);
                }
            }
            console.log('Tracks table results:');
            console.log(rows);
        } catch (error) {
            console.log('Error accessing tracks table:', error.message);
        }

        // List all schemas
        console.log('\n=== All schemas in eventplatform ===');
        try {
            const schemasQuery = `
            SELECT schema_name 
            FROM information_schema.schemata 
            WHERE catalog_name = 'eventplatform'
            ORDER BY schema_name`;

            const iter = await client.query(schemasQuery);
            const rows = [];
            for await (const queryResult of iter) {
                if (queryResult.data) {
                    rows.push(...queryResult.data);
                }
            }
            console.log('Available schemas:');
            rows.forEach(row => {
                console.log(`  ${row[0]}`);
            });
        } catch (error) {
            console.log('Error listing schemas:', error.message);
        }

        // Try some common track names that might exist
        console.log('\n=== Testing specific track names with sample queries ===');
        const possibleTracks = [
            'ndmflow',  // Move this first since we know it works
            'logs', 'traces', 'metrics', 'rum', 'synthetics', 'security',
            'network', 'apm', 'infrastructure', 'events', 'audit',
            'ndm-flow', 'ndm_flow', 'network-flow', 'network_flow'
        ];

        for (const trackName of possibleTracks) {
            try {
                console.log(`  Testing track: ${trackName}`);
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
                if (rows[0] && rows[0][0] > 0) {
                    console.log(`  ✓ ${trackName}: ${rows[0][0]} records`);
                } else {
                    console.log(`  - ${trackName}: 0 records`);
                }
            } catch (error) {
                console.log(`  ✗ ${trackName}: ${error.message}`);
            }
        }

        console.log('\n=== Complete ===');

    } catch (error) {
        console.error('Error:', error);
    }
}

listAllTracks(); 