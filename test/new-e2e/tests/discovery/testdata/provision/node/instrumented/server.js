const tracer = require('dd-trace').init();

const { createServer } = require('http');

const hostname = '0.0.0.0';
const port = process.env.PORT || 8000 ;

const server = createServer((req, res) => {
  res.statusCode = 200;
  res.setHeader('Content-Type', 'text/plain');
  res.end('Hello World');
});

server.listen(port, hostname, () => {
  console.log(`Server running at http://${hostname}:${port}/`);
});

