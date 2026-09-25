const http = require('http');

const hostname = '127.0.0.1';
// Let the OS select an available port to avoid conflicts with host services.
const port = 0;

const server = http.createServer((req, res) => {
  res.statusCode = 200;
  res.setHeader('Content-Type', 'text/plain');
  res.end('Hello World');
});

server.listen(port, hostname, () => {
  console.log('Server running');
});
