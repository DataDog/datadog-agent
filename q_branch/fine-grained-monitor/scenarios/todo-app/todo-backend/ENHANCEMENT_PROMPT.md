# Component Enhancement: todo-backend

## Component Overview

- **Name**: todo-backend
- **Type**: rapid-http
- **Class**: server
- **Language**: go
- **Description**: REST API backend service for todo operations

## Objective

Enhance this component to implement the full flow behavior as specified in the service blueprint.
The implementation must include:
1. All required API endpoints with proper routing
2. Comprehensive telemetry emission (logs, metrics, traces)
3. Proper error handling for defined outcomes
4. Database and downstream service calls
5. Datadog tracing integration

**IMPORTANT**: Review `IMPLEMENTATION_GUIDE.md` in this component directory for additional component-specific guidance and requirements.

## Implementation Requirements

### 1. API Endpoints


### 2. Database Operations


#### Operation: SELECT * FROM todos ORDER BY created_at DESC

- **Database**: todo-db
- **Flow**: list_todos_flow

Implementation:
- Connect to database using environment variables (already configured in docker-compose)
- Execute the query: `SELECT * FROM todos ORDER BY created_at DESC`
- Handle database errors appropriately


#### Operation: INSERT INTO todos (title, description, completed) VALUES ($1, $2, $3)

- **Database**: todo-db
- **Flow**: create_todo_flow

Implementation:
- Connect to database using environment variables (already configured in docker-compose)
- Execute the query: `INSERT INTO todos (title, description, completed) VALUES ($1, $2, $3)`
- Handle database errors appropriately


#### Operation: SELECT * FROM todos WHERE id = $1

- **Database**: todo-db
- **Flow**: update_todo_flow

Implementation:
- Connect to database using environment variables (already configured in docker-compose)
- Execute the query: `SELECT * FROM todos WHERE id = $1`
- Handle database errors appropriately


#### Operation: UPDATE todos SET title = $1, description = $2, completed = $3 WHERE id = $4

- **Database**: todo-db
- **Flow**: update_todo_flow

Implementation:
- Connect to database using environment variables (already configured in docker-compose)
- Execute the query: `UPDATE todos SET title = $1, description = $2, completed = $3 WHERE id = $4`
- Handle database errors appropriately


#### Operation: DELETE FROM todos WHERE id = $1

- **Database**: todo-db
- **Flow**: delete_todo_flow

Implementation:
- Connect to database using environment variables (already configured in docker-compose)
- Execute the query: `DELETE FROM todos WHERE id = $1`
- Handle database errors appropriately


#### Operation: SELECT * FROM todos WHERE id = $1

- **Database**: todo-db
- **Flow**: get_todo_flow

Implementation:
- Connect to database using environment variables (already configured in docker-compose)
- Execute the query: `SELECT * FROM todos WHERE id = $1`
- Handle database errors appropriately


#### Operation: UPDATE todos SET completed = NOT completed WHERE id = $1

- **Database**: todo-db
- **Flow**: toggle_todo_flow

Implementation:
- Connect to database using environment variables (already configured in docker-compose)
- Execute the query: `UPDATE todos SET completed = NOT completed WHERE id = $1`
- Handle database errors appropriately


### 3. Downstream Service Calls


#### Call to: todo-db

- **From Flow**: list_todos_flow
- **From Operation**: http.server.request
- **Target Operation**: postgres.query

Implementation:
- Make HTTP request to todo-db using URL from environment variable
- Use appropriate HTTP client with timeout
- Propagate trace context for distributed tracing
- Handle network errors and retries


#### Call to: todo-db

- **From Flow**: create_todo_flow
- **From Operation**: http.server.request
- **Target Operation**: postgres.query

Implementation:
- Make HTTP request to todo-db using URL from environment variable
- Use appropriate HTTP client with timeout
- Propagate trace context for distributed tracing
- Handle network errors and retries


#### Call to: todo-db

- **From Flow**: update_todo_flow
- **From Operation**: http.server.request
- **Target Operation**: postgres.query

Implementation:
- Make HTTP request to todo-db using URL from environment variable
- Use appropriate HTTP client with timeout
- Propagate trace context for distributed tracing
- Handle network errors and retries


#### Call to: todo-db

- **From Flow**: update_todo_flow
- **From Operation**: http.server.request
- **Target Operation**: postgres.query

Implementation:
- Make HTTP request to todo-db using URL from environment variable
- Use appropriate HTTP client with timeout
- Propagate trace context for distributed tracing
- Handle network errors and retries


#### Call to: todo-db

- **From Flow**: delete_todo_flow
- **From Operation**: http.server.request
- **Target Operation**: postgres.query

Implementation:
- Make HTTP request to todo-db using URL from environment variable
- Use appropriate HTTP client with timeout
- Propagate trace context for distributed tracing
- Handle network errors and retries


#### Call to: todo-db

- **From Flow**: get_todo_flow
- **From Operation**: http.server.request
- **Target Operation**: postgres.query

Implementation:
- Make HTTP request to todo-db using URL from environment variable
- Use appropriate HTTP client with timeout
- Propagate trace context for distributed tracing
- Handle network errors and retries


#### Call to: todo-db

- **From Flow**: toggle_todo_flow
- **From Operation**: http.server.request
- **Target Operation**: postgres.query

Implementation:
- Make HTTP request to todo-db using URL from environment variable
- Use appropriate HTTP client with timeout
- Propagate trace context for distributed tracing
- Handle network errors and retries


### 4. Telemetry Implementation

#### 4.1 Structured Logging

Emit the following log messages at appropriate points:


**Log**: [INFO] GET /api/todos - Request from 192.168.{{range_int(1,255)}}.{{range_int(1,255)}}
- **Status**: info
- **Flow**: list_todos_flow
- **Timing**: 0us after operation start


**Log**: [INFO] Fetching all todos from database
- **Status**: info
- **Flow**: list_todos_flow
- **Timing**: 2ms after operation start


**Log**: [INFO] GET /api/todos - 200 OK - {{range_int(10,25)}} items - {{range_float(12,18)}}ms
- **Status**: info
- **Flow**: list_todos_flow
- **Timing**: 15ms after operation start


**Log**: [ERROR] Database connection timeout after 5000ms
- **Status**: error
- **Flow**: list_todos_flow
- **Timing**: 5000ms after operation start


**Log**: [ERROR] GET /api/todos - 500 Internal Server Error - Database unavailable
- **Status**: error
- **Flow**: list_todos_flow
- **Timing**: 5100ms after operation start


**Log**: [INFO] POST /api/todos - Creating new todo
- **Status**: info
- **Flow**: create_todo_flow
- **Timing**: 0us after operation start


**Log**: [INFO] Validating todo data: title="{{random_item("Buy groceries", "Finish homework", "Call dentist", "Pay bills", "Read book", "Go to gym")}}", completed=false
- **Status**: info
- **Flow**: create_todo_flow
- **Timing**: 1ms after operation start


**Log**: [INFO] POST /api/todos - 201 Created - id: {{range_int(1000,9999)}} - {{range_float(20,28)}}ms
- **Status**: info
- **Flow**: create_todo_flow
- **Timing**: 25ms after operation start


**Log**: [ERROR] POST /api/todos - 400 Bad Request - Title is required
- **Status**: error
- **Flow**: create_todo_flow
- **Timing**: 2ms after operation start


**Log**: [INFO] PUT /api/todos/{{range_int(1000,9999)}} - Updating todo
- **Status**: info
- **Flow**: update_todo_flow
- **Timing**: 0us after operation start


**Logging Requirements**:
- Use structured logging (JSON format)
- Include trace_id and span_id in every log
- Include timestamp in ISO 8601 format
- Use appropriate log levels (DEBUG, INFO, WARN, ERROR)
- Support template variables in log messages:
  - `{{random_uuid()}}`: Generate random UUID
  - `{{range_int(min,max)}}`: Random integer in range
  - `{{range_float(min,max)}}`: Random float in range
  - `{{random_item("a", "b", "c")}}`: Pick random item from list

**Example Log Entry (JSON)**:
```json
{
  "timestamp": "2024-01-15T10:30:45.123Z",
  "level": "INFO",
  "service": "component-name",
  "trace_id": "abc123...",
  "span_id": "def456...",
  "message": "Request completed successfully",
  "duration_ms": 45.2
}
```

#### 4.2 Metrics


**Metrics Requirements**:
- Use Datadog statsd client or equivalent
- Emit counts for request volume
- Emit timings for latency (histogram/distribution)
- Add tags for status, error_type, etc.
- Metrics should be emitted even for failed requests

#### 4.3 Distributed Tracing

**Tracing Requirements**:
- Create a new span for each operation
- Set span name to match operation name
- Add tags: service.name, operation.name, http.method, http.status_code
- Propagate trace context to downstream calls (W3C Trace Context or Datadog headers)
- Set span duration to match actual operation time
- Mark spans as error when operation fails

**Example Span Creation (Conceptual)**:
```python
with tracer.trace("operation.name", service="{comp_name}") as span:
    span.set_tag("operation.type", "http.server.request")
    span.set_tag("http.method", "GET")
    
    # Execute operation logic
    result = execute_operation()
    
    span.set_tag("http.status_code", 200)
```

### 5. Error Handling


Implement proper error handling for the defined outcomes:
- Handle error outcomes appropriately when they occur
- For error outcomes, use specified error_type and return appropriate HTTP status
- Log errors appropriately
- Emit error metrics
- Mark tracing spans with error status

**Example Error Handling**:
```python
def execute_operation():
    try:
        # Execute operation logic
        result = perform_work()
        return {"status": "success", "data": result}, 200
    except DatabaseError as e:
        # Database error path
        log_error(f"Database error: {e}")
        return {"error": "database_error", "message": str(e)}, 500
    except NetworkError as e:
        # Network error path
        log_error(f"Network error: {e}")
        return {"error": "network_error", "message": str(e)}, 500
    except Exception as e:
        # Unexpected error path
        log_error(f"Unexpected error: {e}")
        return {"error": "internal_error", "message": "An unexpected error occurred"}, 500
```

### 6. Health Check Endpoint

Implement a `/health` endpoint that returns 200 OK:
- Should always return quickly (< 50ms)
- Return simple JSON: {"status": "healthy", "service": "todo-backend"}
- Used by Docker healthcheck

## Flow Details


### Flow: list_todos_flow

**Description**: List all todos from the frontend UI

**Trigger**:
- Component: todo-frontend
- Method: GET
- Path: /api/todos
- RPS: 10

**Flow Execution**:

**Operation**: http.server.request
- Type: http.server.request
- Component: todo-backend

**Outcome** (status=success):
- Logs:
  - [info] [INFO] GET /api/todos - Request from 192.168.{{range_int(1,255)}}.{{range_int(1,255)}}
  - [info] [INFO] Fetching all todos from database
  - [info] [INFO] GET /api/todos - 200 OK - {{range_int(10,25)}} items - {{range_float(12,18)}}ms
- Child operations:

**Outcome** (status=error):
- Logs:
  - [error] [ERROR] Database connection timeout after 5000ms
  - [error] [ERROR] GET /api/todos - 500 Internal Server Error - Database unavailable


### Flow: create_todo_flow

**Description**: Create a new todo item from the frontend

**Trigger**:
- Component: todo-frontend
- Method: POST
- Path: /api/todos
- RPS: 3

**Flow Execution**:

**Operation**: http.server.request
- Type: http.server.request
- Component: todo-backend

**Outcome** (status=success):
- Logs:
  - [info] [INFO] POST /api/todos - Creating new todo
  - [info] [INFO] Validating todo data: title="{{random_item("Buy groceries", "Finish homework", "Call dentist", "Pay bills", "Read book", "Go to gym")}}", completed=false
  - [info] [INFO] POST /api/todos - 201 Created - id: {{range_int(1000,9999)}} - {{range_float(20,28)}}ms
- Child operations:

**Outcome** (status=error):
- Logs:
  - [error] [ERROR] POST /api/todos - 400 Bad Request - Title is required


### Flow: update_todo_flow

**Description**: Update an existing todo item from the frontend

**Trigger**:
- Component: todo-frontend
- Method: PUT
- Path: /api/todos/{id}
- RPS: 2

**Flow Execution**:

**Operation**: http.server.request
- Type: http.server.request
- Component: todo-backend

**Outcome** (status=success):
- Logs:
  - [info] [INFO] PUT /api/todos/{{range_int(1000,9999)}} - Updating todo
  - [info] [INFO] Todo {{range_int(1000,9999)}} found, applying updates
  - [info] [INFO] PUT /api/todos/{{range_int(1000,9999)}} - 200 OK - {{range_float(24,32)}}ms
- Child operations:

**Outcome** (status=error):
- Logs:
  - [warn] [WARN] PUT /api/todos/9999 - 404 Not Found - Todo does not exist


### Flow: delete_todo_flow

**Description**: Delete a todo item from the frontend

**Trigger**:
- Component: todo-frontend
- Method: DELETE
- Path: /api/todos/{id}
- RPS: 1

**Flow Execution**:

**Operation**: http.server.request
- Type: http.server.request
- Component: todo-backend

**Outcome** (status=success):
- Logs:
  - [info] [INFO] DELETE /api/todos/1234 - Deleting todo
  - [info] [INFO] DELETE /api/todos/1234 - 204 No Content - 19ms
- Child operations:

**Outcome** (status=error):
- Logs:
  - [warn] [WARN] DELETE /api/todos/9999 - 404 Not Found - Todo does not exist


### Flow: get_todo_flow

**Description**: Get a single todo item by ID from the frontend

**Trigger**:
- Component: todo-frontend
- Method: GET
- Path: /api/todos/{id}
- RPS: 5

**Flow Execution**:

**Operation**: http.server.request
- Type: http.server.request
- Component: todo-backend

**Outcome** (status=success):
- Logs:
  - [info] [INFO] GET /api/todos/{{range_int(1000,9999)}} - Fetching todo
  - [info] [INFO] GET /api/todos/{{range_int(1000,9999)}} - 200 OK - {{range_float(10,15)}}ms
- Child operations:

**Outcome** (status=error):
- Logs:
  - [warn] [WARN] GET /api/todos/9999 - 404 Not Found


### Flow: toggle_todo_flow

**Description**: Toggle todo completion status from the frontend

**Trigger**:
- Component: todo-frontend
- Method: PATCH
- Path: /api/todos/{id}/toggle
- RPS: 4

**Flow Execution**:

**Operation**: http.server.request
- Type: http.server.request
- Component: todo-backend

**Outcome** (status=success):
- Logs:
  - [info] [INFO] PATCH /api/todos/1234/toggle - Toggling completion status
  - [info] [INFO] PATCH /api/todos/1234/toggle - 200 OK - 17ms
- Child operations:

**Outcome** (status=error):
- Logs:
  - [warn] [WARN] PATCH /api/todos/9999/toggle - 404 Not Found



## Testing

After implementation:
1. Build the Docker container successfully
2. Container should start and pass health checks
3. Endpoints should respond with appropriate status codes
4. Logs should be emitted in structured JSON format
5. Metrics should be emitted (can verify with statsd mock)
6. Traces should be created (can verify with tracer mock)

## Notes

- Focus on correctness and completeness over optimization
- All environment variables are pre-configured in docker-compose.yml
- Database connections are automatically established
- Tracing context is automatically propagated by instrumentation libraries
- Follow language-specific best practices
- Use appropriate HTTP framework for the language
- Ensure all dependencies are listed in requirements.txt/go.mod/pom.xml

---

**Generated**: {datetime.now().isoformat()}
**Blueprint**: {spec['name']}
**Component**: {comp_name}
