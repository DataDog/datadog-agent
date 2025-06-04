#!/usr/bin/env node

import { Trino } from 'trino-client';
import { execSync } from 'child_process';

async function testNdmflowExact() {
    try {
        console.log('=== Testing ndmflow with exact specification ===');
        
        // Generate fresh tokens
        console.log('Generating fresh tokens...');
        const ddAuthJWT = execSync('ddauth obo -d us1.staging.dog | grep dd-auth-jwt | cut -d\' \' -f2', { encoding: 'utf8' }).trim();
        const accessToken = execSync('ddtool auth token --datacenter us1.staging.dog apm-trino', { encoding: 'utf8' }).trim();
        
        const clientConfig = {
            server: 'https://trino-gateway.us1.staging.dog',
            catalog: 'eventplatform',
            schema: 'system',
            user: 'jim.wilson',
            extraHeaders: {
                'Authorization': `Bearer ${accessToken}`,
                'X-Trino-Client-Tags': 'org_id=2',
                'X-Trino-Extra-Credential': `orgId=2, clientId=trino-cli, userUuid=976714e4-1b76-11ee-87d8-aa0300114d4e, ddAuthJWT=${ddAuthJWT}`
            }
        };

        const client = Trino.create(clientConfig);
        
        // Test exactly as user specified
        console.log('\n=== Testing with exact track specification ===');
        const exactQuery = `
        SELECT COUNT(*) as count
        FROM TABLE(
            eventplatform.system.track(
                TRACK => 'ndmflow',
                QUERY => '*',
                TIME_RANGE => '1h'
            )
        )`;
        
        console.log('Query:', exactQuery);
        
        const iter = await client.query(exactQuery);
        const rows = [];
        for await (const queryResult of iter) {
            if (queryResult.data) {
                rows.push(...queryResult.data);
            }
        }
        console.log('Count result:', rows);
        
        // If we have data, get sample records
        if (rows[0] && rows[0][0] > 0) {
            console.log(`\n=== Found ${rows[0][0]} records! Getting sample data ===`);
            
            const sampleQuery = `
            SELECT *
            FROM TABLE(
                eventplatform.system.track(
                    TRACK => 'ndmflow',
                    QUERY => '*',
                    TIME_RANGE => '1h'
                )
            )
            LIMIT 5`;
            
            const sampleIter = await client.query(sampleQuery);
            const sampleRows = [];
            for await (const queryResult of sampleIter) {
                if (queryResult.data) {
                    sampleRows.push(...queryResult.data);
                }
            }
            console.log('Sample ndmflow data:');
            console.log(JSON.stringify(sampleRows, null, 2));
            
            // Also try with specific columns
            console.log('\n=== Getting data with specific columns ===');
            const columnsQuery = `
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
            LIMIT 3`;
            
            const columnsIter = await client.query(columnsQuery);
            const columnsRows = [];
            for await (const queryResult of columnsIter) {
                if (queryResult.data) {
                    columnsRows.push(...queryResult.data);
                }
            }
            console.log('NetFlow summary for last hour:');
            columnsRows.forEach((row, index) => {
                console.log(`Record ${index + 1}:`);
                console.log(`  Source IP: ${row[0]}`);
                console.log(`  Destination IP: ${row[1]}`);
                console.log(`  Exporter IP: ${row[2]}`);
                console.log(`  Bytes: ${row[3]}`);
                console.log(`  Packets: ${row[4]}`);
            });
            
        } else {
            console.log('\n=== No data found ===');
            console.log('Possible reasons:');
            console.log('1. No NetFlow data in the last hour');
            console.log('2. Data might be in a different time range');
            console.log('3. Track might be named differently');
            console.log('4. Data might be in different organization');
            
            // Try longer time ranges
            console.log('\n=== Testing longer time ranges ===');
            const timeRanges = ['3h', '6h', '12h', '24h'];
            
            for (const timeRange of timeRanges) {
                try {
                    const testQuery = `
                    SELECT COUNT(*) as count
                    FROM TABLE(
                        eventplatform.system.track(
                            TRACK => 'ndmflow',
                            QUERY => '*',
                            TIME_RANGE => '${timeRange}'
                        )
                    )`;
                    
                    const testIter = await client.query(testQuery);
                    const testRows = [];
                    for await (const queryResult of testIter) {
                        if (queryResult.data) {
                            testRows.push(...queryResult.data);
                        }
                    }
                    console.log(`  ${timeRange}: ${testRows[0] ? testRows[0][0] : 0} records`);
                } catch (error) {
                    console.log(`  ${timeRange}: Error - ${error.message.substring(0, 50)}`);
                }
            }
        }
        
    } catch (error) {
        console.error('Error:', error);
        console.error('Error message:', error.message);
    }
}

testNdmflowExact(); 