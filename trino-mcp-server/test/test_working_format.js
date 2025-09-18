#!/usr/bin/env node

import { Trino } from 'trino-client';
import { execSync } from 'child_process';

async function testWorkingFormat() {
    try {
        console.log('=== Testing with exact working CLI format ===');
        
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
        
        // Test with exact working format (no TIME_RANGE, with COLUMNS and OUTPUT_TYPES)
        console.log('\n=== Testing exact working query format ===');
        const workingQuery = `
        SELECT * FROM TABLE(
            eventplatform.system.track(
                TRACK => 'ndmflow', 
                COLUMNS => ARRAY['id', 'timestamp', '@exporter.ip', '@bytes'], 
                OUTPUT_TYPES => ARRAY['varchar', 'int', 'varchar', 'int']
            )
        ) LIMIT 5`;
        
        console.log('Query:', workingQuery);
        
        const iter = await client.query(workingQuery);
        const rows = [];
        for await (const queryResult of iter) {
            if (queryResult.data) {
                rows.push(...queryResult.data);
            }
        }
        
        console.log('Working format results:');
        console.log('Number of rows:', rows.length);
        if (rows.length > 0) {
            console.log('Sample data:');
            rows.forEach((row, index) => {
                console.log(`Row ${index + 1}: ID=${row[0]}, Timestamp=${row[1]}, Exporter=${row[2]}, Bytes=${row[3]}`);
            });
            
            // Now try a NetFlow summary for the last hour
            console.log('\n=== NetFlow Summary Query ===');
            const summaryQuery = `
            SELECT 
                "@exporter.ip" AS exporter_ip,
                COUNT(*) AS flow_count,
                SUM("@bytes") AS total_bytes,
                AVG("@bytes") AS avg_bytes,
                MIN("timestamp") AS earliest_time,
                MAX("timestamp") AS latest_time
            FROM TABLE(
                eventplatform.system.track(
                    TRACK => 'ndmflow',
                    COLUMNS => ARRAY['@exporter.ip', '@bytes', 'timestamp'],
                    OUTPUT_TYPES => ARRAY['varchar', 'int', 'int']
                )
            )
            WHERE "timestamp" >= ${Date.now() - 3600000}  -- Last hour
            GROUP BY "@exporter.ip"
            ORDER BY total_bytes DESC
            LIMIT 10`;
            
            const summaryIter = await client.query(summaryQuery);
            const summaryRows = [];
            for await (const queryResult of summaryIter) {
                if (queryResult.data) {
                    summaryRows.push(...queryResult.data);
                }
            }
            
            console.log('NetFlow Summary (last hour):');
            if (summaryRows.length > 0) {
                summaryRows.forEach((row, index) => {
                    console.log(`${index + 1}. Exporter: ${row[0]}`);
                    console.log(`   Flow Count: ${row[1]}`);
                    console.log(`   Total Bytes: ${row[2]}`);
                    console.log(`   Avg Bytes: ${row[3]}`);
                    console.log(`   Time Range: ${new Date(row[4])} to ${new Date(row[5])}`);
                    console.log('');
                });
            } else {
                console.log('No flows found in the last hour');
                
                // Try without time filter to see all available data
                console.log('\n=== All available data (no time filter) ===');
                const allDataQuery = `
                SELECT 
                    "@exporter.ip" AS exporter_ip,
                    COUNT(*) AS flow_count,
                    SUM("@bytes") AS total_bytes,
                    MIN("timestamp") AS earliest_time,
                    MAX("timestamp") AS latest_time
                FROM TABLE(
                    eventplatform.system.track(
                        TRACK => 'ndmflow',
                        COLUMNS => ARRAY['@exporter.ip', '@bytes', 'timestamp'],
                        OUTPUT_TYPES => ARRAY['varchar', 'int', 'int']
                    )
                )
                GROUP BY "@exporter.ip"
                ORDER BY total_bytes DESC
                LIMIT 5`;
                
                const allDataIter = await client.query(allDataQuery);
                const allDataRows = [];
                for await (const queryResult of allDataIter) {
                    if (queryResult.data) {
                        allDataRows.push(...queryResult.data);
                    }
                }
                
                console.log('All available NetFlow data:');
                allDataRows.forEach((row, index) => {
                    console.log(`${index + 1}. Exporter: ${row[0]}`);
                    console.log(`   Flow Count: ${row[1]}`);
                    console.log(`   Total Bytes: ${row[2]}`);
                    console.log(`   Time Range: ${new Date(row[3])} to ${new Date(row[4])}`);
                    console.log('');
                });
            }
            
        } else {
            console.log('No data returned');
        }
        
    } catch (error) {
        console.error('Error:', error);
        console.error('Error message:', error.message);
    }
}

testWorkingFormat(); 