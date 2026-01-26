# Component Enhancement: payment-service

## Component Overview

- **Name**: payment-service
- **Type**: rapid-http
- **Class**: server
- **Language**: go
- **Description**: Go-based payment processor

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


#### Endpoint: POST /pay

- **Flow**: process_payment
- **Description**: 
- **Expected RPS**: 50

Implementation:
- Create route handler for POST /pay
- Extract any path parameters (e.g., {id})
- Parse request body if applicable
- Execute flow logic (see flow details below)
- Return appropriate response with status codes


#### Endpoint: POST /internal/generate-report

- **Flow**: compliance_report
- **Description**: 
- **Expected RPS**: 1

Implementation:
- Create route handler for POST /internal/generate-report
- Extract any path parameters (e.g., {id})
- Parse request body if applicable
- Execute flow logic (see flow details below)
- Return appropriate response with status codes


### 2. Database Operations


#### Operation: check_fraud

- **Database**: fraud-db
- **Flow**: process_payment

Implementation:
- Connect to database using environment variables (already configured in docker-compose)
- Execute the query: `check_fraud`
- Handle database errors appropriately


### 3. Downstream Service Calls


#### Call to: fraud-db

- **From Flow**: process_payment
- **From Operation**: handle_transaction
- **Target Operation**: redis.command

Implementation:
- Make HTTP request to fraud-db using URL from environment variable
- Use appropriate HTTP client with timeout
- Propagate trace context for distributed tracing
- Handle network errors and retries


### 4. Telemetry Implementation

#### 4.1 Structured Logging


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
- Return simple JSON: {"status": "healthy", "service": "payment-service"}
- Used by Docker healthcheck

## Flow Details


### Flow: process_payment

**Description**: N/A

**Trigger**:
- Component: payment-service
- Method: POST
- Path: /pay
- RPS: 50

**Flow Execution**:

**Operation**: handle_transaction
- Type: 
- Component: payment-service

**Outcome** (status=success):
- Child operations:


### Flow: compliance_report

**Description**: N/A

**Trigger**:
- Component: payment-service
- Method: POST
- Path: /internal/generate-report
- RPS: 1

**Flow Execution**:

**Operation**: generate_pdf
- Type: 
- Component: payment-service

**Outcome** (status=success):



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
