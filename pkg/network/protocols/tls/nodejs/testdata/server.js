const https = require('https');
const fs = require('fs');
const url = require('url');

// Read the SSL certificate and private key
const options = {
    key: fs.readFileSync(`${process.env.CERTS_DIR}/srv.key`),
    cert: fs.readFileSync(`${process.env.CERTS_DIR}/srv.crt`)
};

// Create the HTTPS echo server
const server = https.createServer(options, (req, res) => {
    const parsedUrl = url.parse(req.url, true);
    const pathname = parsedUrl.pathname;
    let statusCode = 200;
    console.log(`Request received: ${req.method} ${pathname}`)
    if (pathname.startsWith('/status/')) {
        const newStatusCode = parseInt(pathname.split('/')[2]);
        if (!isNaN(newStatusCode)) {
            statusCode = newStatusCode;
        }
    }

    let body = '';

    req.on('data', (chunk) => {
        body += chunk;
    });

    req.on('end', () => {
        let headers = {}
        if (req.headers['content-type'] !== undefined) {
            headers['Content-Type'] = req.headers['content-type']
        }
        res.writeHead(statusCode, headers);
        res.end(body);
    });
});

const port = 4141
// Start the server
server.listen(port, () => {
    console.log(`Server running at https://${process.env.ADDR}:${port}/`);
});
