# Rapid-HTTP Go Backend - todo-backend

High-performance HTTP backend service for todo-app.

## Description

REST API backend service for todo operations

## Configuration

The service is configured through the docker-compose.yml file with the following settings:
- Port: `8081`
- Gin Mode: `release` (production) or `debug` (development)

## API Endpoints

### Health Check
```
GET /health
```
Returns service health status.

### Todo API
```
GET    /api/todos       - List all todos
POST   /api/todos       - Create a new todo
GET    /api/todos/:id   - Get a specific todo
PUT    /api/todos/:id   - Update a todo
DELETE /api/todos/:id   - Delete a todo
GET    /api/todos/search - Search todos
```

## Connection Information

The service is accessible at:
- Internal (from Docker network): `http://todo-backend:8081`
- External (from host machine): `http://localhost:8081`

## Docker Compose

The service is defined in the main docker-compose.yml file with:
- Multi-stage build for optimized image size
- Health checks
- Volume mounting for development (optional)
- Automatic restart policy

## Usage

### Development Mode

For development with hot reload, run:
```bash
cd src
go run main.go
```

### Building and Running the Service

```bash
# Build and start in one command (recommended)
docker-compose up --build -d todo-backend

# Or run in foreground to see logs
docker-compose up --build todo-backend
```

### Testing the API

```bash
# Health check
curl http://localhost:8081/health

# List todos
curl http://localhost:8081/api/todos

# Create a todo
curl -X POST http://localhost:8081/api/todos \
  -H "Content-Type: application/json" \
  -d '{"title":"My Task","completed":false}'
```

### View Logs

```bash
docker-compose logs -f todo-backend
```

## Environment Variables

- `PORT`: Server port (default: 8081)
- `GIN_MODE`: Gin framework mode - `release` for production, `debug` for development

## System Context

{{system_context}}

This backend service integrates with:
{{service_requirements}}

## Development

### Project Structure

```
src/
├── main.go          # Main application entry point
├── go.mod           # Go module dependencies
├── go.sum           # Dependency checksums
└── Dockerfile       # Multi-stage build configuration
```

### Adding New Endpoints

To add new API endpoints, edit `src/main.go` and add route handlers:

```go
api.GET("/your-endpoint", yourHandler)
```

### Adding Dependencies

```bash
cd src
go get github.com/your/dependency
go mod tidy
```

## Performance Considerations

For production use, consider:
- Using `GIN_MODE=release` to disable debug logging
- Implementing proper logging and metrics
- Adding rate limiting middleware
- Configuring CORS if needed
- Using connection pooling for databases
- Implementing caching strategies
- Setting appropriate timeouts

## Building Blocks

This component is designed as a reusable building block that can:
- Run standalone or as part of a larger system
- Be easily extended with additional endpoints
- Integrate with databases (PostgreSQL, Elasticsearch, etc.)
- Scale horizontally behind a load balancer
