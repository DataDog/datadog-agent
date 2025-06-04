#!/usr/bin/env node

import { Trino } from 'trino-client';

async function testNetflowQuery() {
    try {
        console.log('Testing NetFlow query...');
        
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
        
        // Try to query one NetFlow record using the track function
        console.log('Querying NetFlow data...');
        const netflowQuery = `
        SELECT 
            "@source.ip" AS source_ip,
            "@destination.ip" AS dest_ip,
            "@exporter.ip" AS exporter_ip,
            "@bytes" AS bytes,
            "@packets" AS packets
        FROM TABLE(
            eventplatform.system.track(
                TRACK => 'ndmflow',
                QUERY => '*',
                COLUMNS => ARRAY['@source.ip', '@destination.ip', '@exporter.ip', '@bytes', '@packets'],
                OUTPUT_TYPES => ARRAY['varchar', 'varchar', 'varchar', 'bigint', 'bigint'],
                TIME_RANGE => '1h'
            )
        )
        LIMIT 1`;
        
        console.log('Query:', netflowQuery);
        
        const iter = await client.query(netflowQuery);
        
        const rows = [];
        for await (const queryResult of iter) {
            if (queryResult.data) {
                rows.push(...queryResult.data);
            }
        }
        
        console.log('NetFlow query results:', rows);
        
        if (rows.length > 0) {
            console.log('Success! Found NetFlow data:');
            console.log('Source IP:', rows[0][0]);
            console.log('Destination IP:', rows[0][1]);
            console.log('Exporter IP:', rows[0][2]);
            console.log('Bytes:', rows[0][3]);
            console.log('Packets:', rows[0][4]);
        } else {
            console.log('No NetFlow data found in the last hour');
        }
        
    } catch (error) {
        console.error('Error:', error);
        console.error('Error details:', {
            message: error.message,
            stack: error.stack?.substring(0, 500) + '...'
        });
    }
}

testNetflowQuery(); 