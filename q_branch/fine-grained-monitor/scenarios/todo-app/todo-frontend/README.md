# Static Web Application

A modern static web application served by Node.js/Express with support for API communication and responsive design.

## Features

- **Modern Web Standards**: HTML5, CSS3, ES2022 JavaScript
- **Responsive Design**: Mobile-first approach with responsive layouts
- **API Integration**: Built-in support for backend API communication
- **Production Ready**: Express server with security headers (Helmet) and compression
- **Runtime Configuration**: Environment variables injected at runtime via `/config` endpoint
- **Error Handling**: Comprehensive error handling for network requests
- **Caching Strategy**: Optimized caching for static assets

## Structure

```
src/
├── html/
│   ├── index.html      # Main HTML file
│   ├── app.js          # JavaScript application logic
│   └── style.css       # CSS styles
├── integration-tests/  # Browser-based integration tests
│   ├── basic-ui.test.js      # Basic UI tests
│   ├── form-interaction.test.js  # Form interaction tests
│   ├── run-tests.js    # Test runner script
│   └── package.json    # Test dependencies
├── server.js           # Express server
├── package.json        # Node.js dependencies
├── Dockerfile          # Container definition
└── README.md           # This file
```

## Quick Start

### Using Docker Compose (Recommended)

```bash
# Run with default settings
docker-compose up

# Run with custom environment variables
API_BASE_URL=http://api.example.com:8081/api docker-compose up
```

The application will be available at `http://localhost:8080`

### Using Node.js Directly

```bash
# Install dependencies
cd src
npm install

# Run the server
npm start

# Or with custom environment variables
API_BASE_URL=http://api.example.com:8081/api PORT=8080 npm start
```

### Using Docker

```bash
# Build the image
docker build -t static-app ./src

# Run the container
docker run -p 8080:8080 -e API_BASE_URL=http://api.example.com:8081 static-app
```

## Configuration

### Environment Variables

- `PORT`: Port to serve the application (default: 8080)
- `SERVER_NAME`: Server name identifier (default: localhost)
- `API_BASE_URL`: Base URL for API calls (default: http://localhost:8081/api)

You can configure these by:
1. Editing the `.env` file
2. Setting environment variables in docker-compose.yml
3. Passing them via command line

### Express Server Features

- **Security Headers**: Helmet middleware for security best practices
- **Gzip Compression**: Automatic compression for responses
- **Static File Serving**: Optimized static file serving with caching
- **CORS Support**: Configured for cross-origin API requests
- **Health Check**: `/health` endpoint for container health monitoring
- **Config Endpoint**: `/config` endpoint for runtime configuration

## Development

The application includes a JavaScript class (`App`) that:

- Fetches runtime configuration from `/config` endpoint
- Tests API connections
- Provides utility methods for API requests
- Handles errors and user feedback

## API Integration

The frontend communicates with backend services through the configured `API_BASE_URL`:

- Configuration loaded at runtime from `/config` endpoint
- Error handling for failed requests
- JSON request/response handling
- CORS support for cross-origin requests

## Health Check

The server provides a `/health` endpoint that returns `200 OK` when healthy. This is used by Docker's healthcheck feature.

## Integration Tests

The application includes browser-based integration tests using Puppeteer. These tests verify:

- **UI Functionality**: Page loads, content rendering, and UI interactions
- **Console Errors**: No JavaScript errors in the browser console
- **Form Interactions**: Form submissions and user input handling
- **API Integration**: Backend API communication

### Running Integration Tests

```bash
# Install test dependencies
cd src/integration-tests
npm install

# Run tests (requires the application to be running)
FRONTEND_URL=http://localhost:8080 npm test
```

The tests use Puppeteer to launch a headless browser and perform automated testing of the application's UI and functionality. Each test file exports a `runTests` function that receives a browser instance, frontend URL, and test runner function.
