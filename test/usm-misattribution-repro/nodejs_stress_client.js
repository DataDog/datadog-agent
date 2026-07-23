#!/usr/bin/env node
/**
 * Node.js stress test client for misattribution reproduction.
 * Creates rapid TLS connections to multiple servers.
 */

const https = require('https');
const tls = require('tls');

// Parse arguments
const args = {
    server1: 'localhost:8443',
    server2: 'localhost:9443',
    duration: 60,
    concurrency: 50,
    skipClose: 0.1,
    rate: 10, // ms between requests
};

for (let i = 2; i < process.argv.length; i++) {
    const arg = process.argv[i];
    if (arg === '-server1') args.server1 = process.argv[++i];
    else if (arg === '-server2') args.server2 = process.argv[++i];
    else if (arg === '-duration') args.duration = parseInt(process.argv[++i], 10);
    else if (arg === '-concurrency') args.concurrency = parseInt(process.argv[++i], 10);
    else if (arg === '-skip-close') args.skipClose = parseFloat(process.argv[++i]);
    else if (arg === '-rate') args.rate = parseInt(process.argv[++i], 10);
}

const [host1, port1] = args.server1.split(':');
const [host2, port2] = args.server2.split(':');

const stats = {
    server1Requests: 0,
    server2Requests: 0,
    errors: 0,
    skippedClose: 0,
};

console.log('Starting stress test:');
console.log(`  Server 1: ${host1}:${port1}`);
console.log(`  Server 2: ${host2}:${port2}`);
console.log(`  Duration: ${args.duration}s`);
console.log(`  Concurrency: ${args.concurrency}`);
console.log(`  Skip Close rate: ${(args.skipClose * 100).toFixed(1)}%`);
console.log(`  TLS context: NEW per request (Node.js behavior)`);

const paths1 = ['/', '/health', `/server${port1}/identify`, '/api/v1/data'];
const paths2 = ['/', '/health', `/server${port2}/identify`, '/api/v1/users'];

function makeRequest(host, port, path, callback) {
    // Create NEW agent per request to force new TLS connections
    // This maximizes memory churn
    const agent = new https.Agent({
        rejectUnauthorized: false,
        keepAlive: false,
    });

    const options = {
        hostname: host,
        port: parseInt(port, 10),
        path: path,
        method: 'GET',
        agent: agent,
        rejectUnauthorized: false,
    };

    const req = https.request(options, (res) => {
        let data = '';
        res.on('data', (chunk) => { data += chunk; });
        res.on('end', () => {
            if (port === port1) stats.server1Requests++;
            else stats.server2Requests++;

            // Randomly skip destroy to simulate leaked connections
            if (Math.random() < args.skipClose) {
                stats.skippedClose++;
                // Don't destroy! Let GC handle it
            } else {
                res.destroy();
                agent.destroy();
            }
            callback();
        });
    });

    req.on('error', (e) => {
        stats.errors++;
        callback();
    });

    req.end();
}

function worker() {
    const endTime = Date.now() + (args.duration * 1000);

    function loop() {
        if (Date.now() >= endTime) return;

        // Randomly pick a server
        const useServer1 = Math.random() < 0.5;
        const host = useServer1 ? host1 : host2;
        const port = useServer1 ? port1 : port2;
        const paths = useServer1 ? paths1 : paths2;
        const path = paths[Math.floor(Math.random() * paths.length)];

        makeRequest(host, port, path, () => {
            setTimeout(loop, args.rate);
        });

        // Occasionally force GC if available
        if (Math.random() < 0.05 && global.gc) {
            global.gc();
        }
    }

    loop();
}

// Progress reporter
const progressInterval = setInterval(() => {
    console.log(`Progress: server${port1}=${stats.server1Requests}, server${port2}=${stats.server2Requests}, errors=${stats.errors}, skipped_close=${stats.skippedClose}`);
}, 5000);

// Start workers
for (let i = 0; i < args.concurrency; i++) {
    worker();
}

// End after duration
setTimeout(() => {
    clearInterval(progressInterval);
    console.log('\nTest complete:');
    console.log(`  Server 1 (${port1}) requests: ${stats.server1Requests}`);
    console.log(`  Server 2 (${port2}) requests: ${stats.server2Requests}`);
    console.log(`  Errors: ${stats.errors}`);
    console.log(`  Skipped Close: ${stats.skippedClose}`);
    process.exit(0);
}, args.duration * 1000 + 1000);