# Implementation Guide: todo-backend

## Quick Start

1. Review the full enhancement prompt in `ENHANCEMENT_PROMPT.md`
2. Implement the required endpoints and logic

## Component Type Guidance

This component uses the **rapid-http** (server) type. The following guidance from the component type template applies:

You are tasked with generating a Go HTTP backend service using the Gin framework.

Component: todo-backend (rapid-http/server)
System: todo-app
Description: REST API backend service for todo operations

Generate a complete HTTP backend service that includes:
1. RESTful API endpoints for required operations
2. Proper request/response handling with JSON
3. Health check endpoint at /health
4. Datadog APM tracing via Orchestrion (compile-time auto-instrumentation)
5. Basic error handling
6. CORS middleware for cross-origin requests

## Phase 1 BUILD Focus

For Phase 1, focus on **connectivity and distributed tracing**:
- Wire services together with HTTP calls
- Use placeholder/hardcoded data for responses
- Orchestrion automatically instruments Gin and HTTP clients at compile time
- No manual tracer initialization needed

Example service-to-service call:
```go
func callDownstream(c *gin.Context) {
    // Call another service
    downstreamURL := getEnv("DOWNSTREAM_SERVICE_URL", "http://other-service:8081")
    
    // Create HTTP client - Orchestrion automatically instruments it
    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Get(downstreamURL + "/api/endpoint")
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer resp.Body.Close()
    
    var result map[string]any
    json.NewDecoder(resp.Body).Decode(&result)
    c.JSON(resp.StatusCode, result)
}
```

The service uses Orchestrion for automatic APM instrumentation:
- Orchestrion automatically instruments Gin router at compile time
- HTTP clients created with http.Client are automatically traced
- No manual middleware registration or tracer initialization needed
- Configuration is read from DD_SERVICE, DD_ENV, DD_VERSION environment variables

See: https://docs.datadoghq.com/tracing/trace_collection/automatic_instrumentation/dd_libraries/go/?tab=compiletimeinstrumentation

Code Structure:
- main.go with Gin router setup and all API endpoints
- Domain type definitions (structs) at the top
- Proper HTTP status codes and error responses
- Clean separation of handler functions
- Health check that returns service status

Generate the complete implementation with all files needed to build and run the service.

---

## Checklist

### Endpoints

### Database Operations
- [ ] SELECT * FROM todos ORDER BY created_at DESC (to todo-db)
- [ ] INSERT INTO todos (title, description, completed) VALUES ($1, $2, $3) (to todo-db)
- [ ] SELECT * FROM todos WHERE id = $1 (to todo-db)
- [ ] UPDATE todos SET title = $1, description = $2, completed = $3 WHERE id = $4 (to todo-db)
- [ ] DELETE FROM todos WHERE id = $1 (to todo-db)
- [ ] SELECT * FROM todos WHERE id = $1 (to todo-db)
- [ ] UPDATE todos SET completed = NOT completed WHERE id = $1 (to todo-db)

### Downstream Calls
- [ ] Call todo-db (postgres.query) from list_todos_flow/http.server.request
- [ ] Call todo-db (postgres.query) from create_todo_flow/http.server.request
- [ ] Call todo-db (postgres.query) from update_todo_flow/http.server.request
- [ ] Call todo-db (postgres.query) from update_todo_flow/http.server.request
- [ ] Call todo-db (postgres.query) from delete_todo_flow/http.server.request
- [ ] Call todo-db (postgres.query) from get_todo_flow/http.server.request
- [ ] Call todo-db (postgres.query) from toggle_todo_flow/http.server.request

### Telemetry
- [ ] Structured JSON logging with trace correlation
- [ ] Metrics emission (counts, latencies)
- [ ] Distributed tracing spans
- [ ] Health check endpoint

### Testing
- [ ] Docker build succeeds
- [ ] Container starts and passes health check
- [ ] Endpoints return expected responses
- [ ] Logs are emitted
- [ ] No crashes or errors in steady state

### Performance
- [ ] Application responds with reasonable latency

## Key Files to Modify


- `src/main.go`: Main application entry point and routing
- `src/handlers/`: HTTP request handlers
- `src/services/`: Business logic and downstream calls
- `src/telemetry/`: Telemetry utilities (logging, metrics, tracing)
- `src/Dockerfile`: If dependencies change

## Resources

- **Blueprint**: Full service blueprint specification
- **Enhancement Prompt**: Detailed implementation requirements
- **Environment Variables**: Pre-configured in docker-compose.yml
- **Component Templates**: Base scaffolding in component directory

## Support

For issues with the enhancement process, consult:
1. The full enhancement prompt
2. Flow definitions in the blueprint
3. Component type documentation
4. Example implementations in component-types/
