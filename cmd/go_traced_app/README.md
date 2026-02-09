# Go Traced Application

A simple Go application that demonstrates sending traces, CPU profiling, and memory profiling data to a locally running Datadog trace-agent.

## Features

- **Distributed Tracing**: Sends traces with multiple spans simulating a web application
- **Runtime Metrics**: Automatically collects and sends CPU, memory, and goroutine metrics
- **CPU Profiling**: Generates CPU profiles every 10 seconds
- **Memory Profiling**: Generates heap and goroutine profiles

## Prerequisites

1. Build and run the Datadog trace-agent locally:
   ```bash
   cd /Users/olivier.gaca/go/src/github.com/DataDog/datadog-agent
   dda inv trace-agent.build
   ./bin/trace-agent/trace-agent --config bin/trace-agent/dist/datadog.yaml
   ```

2. Ensure the trace-agent is configured with a valid API key in `bin/trace-agent/dist/datadog.yaml`:
   ```yaml
   api_key: your_api_key_here
   ```

## Building the Application

```bash
cd cmd/GoTracedApp
go mod download
go build -o go-traced-app
```

## Running the Application

### Basic Usage

```bash
./go-traced-app
```

### Running Multiple Instances as Different Services

You can run multiple instances with different service names:

```bash
# Terminal 1 - Run as "frontend-service"
./go-traced-app -service frontend-service

# Terminal 2 - Run as "backend-service"
./go-traced-app -service backend-service

# Terminal 3 - Run as "api-gateway"
./go-traced-app -service api-gateway
```

### Command-Line Flags

- `-service <name>` - Service name for traces and profiling (default: "go-traced-app")
- `-agent <address>` - Datadog agent address (default: "localhost:8126")
- `-env <environment>` - Environment name (default: "dev")

### Examples

```bash
# Custom service name
./go-traced-app -service my-custom-service

# Different agent address
./go-traced-app -agent 10.0.0.5:8126

# Production environment
./go-traced-app -service payment-api -env production

# All options combined
./go-traced-app -service checkout-service -agent localhost:8126 -env staging
```

The application will:
- Start sending traces to the specified agent endpoint
- Generate a new trace every 2 seconds
- Each trace includes multiple spans simulating:
  - A web request
  - A database query
  - A cache lookup
  - Business logic processing
- Upload CPU and memory profiles every 10 seconds

## Output

You should see output like:
```
Go Traced Application
=====================
Go Traced App started
Sending traces to localhost:8126
Press Ctrl+C to stop
Sent trace with multiple spans
Sent trace with multiple spans
...
```

## Verifying Data in Datadog

If your trace-agent is properly configured with an API key and connected to Datadog:

1. **Traces**: Go to APM → Traces in Datadog and filter by service `go-traced-app`
2. **Profiling**: Go to APM → Profiling and look for the `go-traced-app` service
3. **Runtime Metrics**: In the trace view, you'll see runtime metrics like CPU, memory usage, and goroutine count

## Configuration Options

You can customize the application by modifying [main.go](main.go):

- Change the trace agent address: `tracer.WithAgentAddr("your-host:port")`
- Modify the service name: `tracer.WithService("your-service-name")`
- Adjust the environment: `tracer.WithEnv("production")`
- Change the trace generation frequency: Modify the ticker duration in `runDemo()`
- Add more spans: Create additional child spans in `processRequest()`

## Troubleshooting

**No traces appearing:**
- Verify the trace-agent is running on `localhost:8126`
- Check the trace-agent logs for errors
- Ensure the API key is valid in the trace-agent config

**Profiling data not appearing:**
- Profiling requires the trace-agent to forward data to Datadog
- Check that profiling is enabled in your Datadog account
- Wait at least 10 seconds for the first profile upload

**Connection refused errors:**
- Make sure the trace-agent is running before starting this application
- Verify the port 8126 is not blocked by a firewall
