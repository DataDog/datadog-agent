# Load Generator

Load Generator is a Python-based service using Locust to generate realistic HTTP traffic patterns for testing GenSim applications. It simulates user behavior and integrates with Datadog APM for distributed tracing.

## Overview

Load Generator with sophisticated causal experiment support:
- **Phase-based experiments**: Separate baseline and intervention phases with automatic transition
- **Dynamic user scaling**: Automatically adjusts user count at phase boundaries
- **ON/OFF state machine**: Realistic user burstiness with active/idle periods
- **Gamma distributions**: Natural timing variability for think times and session lengths
- **Task dispatcher**: Configurable request mix per phase
- **Datadog APM**: Traces, logs, and metrics from load generator
- **Locust web UI**: Real-time monitoring and manual control

## Architecture

- **Framework**: Locust (Python load testing framework)
- **Language**: Python 3.11
- **Tracing**: Datadog APM via ddtrace
- **Port**: 8089 (Locust web UI)

## Features

### Dynamic Phase-Based Experiments

The load generator supports automatic phase transitions for causal experiments:

1. **Baseline Phase**: Runs for `LOAD_WARMUP_DURATION` seconds with baseline configuration
2. **Automatic Transition**: A background greenlet monitors elapsed time
3. **Intervention Phase**: Automatically scales to intervention user count and task weights
4. **Detailed Logging**: Phase transitions are logged with full before/after metrics

Example timeline:
```
t=0s:    Start with 10 baseline users
t=300s:  Transition detected → scale to 50 intervention users
t=900s:  Test completes
```

The transition happens smoothly using Locust's `runner.start()` which spawns/despawns users as needed.

### User Behavior Simulation

The load generator simulates realistic user interactions:

1. **Session Start**: Health check
2. **List Todos**: View existing todos
3. **Create Todo**: Add new todos
4. **Update Todo**: Modify existing todos
5. **Complete Todo**: Mark todos as complete
6. **Delete Todo**: Remove todos

### Configurable Load Patterns

- **User Count**: Number of concurrent users
- **Spawn Rate**: Rate at which users are spawned
- **Run Time**: Duration of load test (0 = indefinite)
- **Wait Time**: Time between tasks (1-3 seconds by default)

## Configuration

### Environment Variables

#### Basic Locust Configuration
- `LOCUST_HOST`: Target application URL (default: `http://todo-backend:8081`)
- `LOCUST_RUN_TIME`: Test duration in seconds (default: `0` = indefinite)
- `LOCUST_AUTOSTART`: Auto-start load test (default: `true`)
- `DD_SERVICE`: Service name for traces (default: `load-generator`)
- `DD_ENV`: Datadog environment (default: `development`)
- `DD_VERSION`: Service version (default: `1.0.0`)

#### Phase-Based Experiment Configuration
- `LOAD_WARMUP_DURATION`: Seconds in baseline phase before switching to intervention (default: `0`)
- `LOAD_BASELINE_USERS`: Number of concurrent users during baseline phase (default: `10`)
- `LOAD_BASELINE_SPAWN_RATE`: Users spawned per second during baseline (default: `2`)
- `LOAD_BASELINE_TASK_WEIGHTS`: JSON dict of task weights for baseline
- `LOAD_INTERVENTION_USERS`: Number of concurrent users during intervention phase (default: `10`)
- `LOAD_INTERVENTION_SPAWN_RATE`: Users spawned per second during intervention (default: `2`)
- `LOAD_INTERVENTION_TASK_WEIGHTS`: JSON dict of task weights for intervention

**Note**: User count dynamically adjusts at phase transition. The load generator monitors elapsed time and automatically scales from baseline to intervention user count.

### Locust Configuration

The `locustfile.py` defines:
- User behavior tasks
- Wait times between tasks
- Request patterns
- Error handling

## Usage

### Starting the Service

```bash
docker-compose up load-generator
```

Or run directly:

```bash
pip install -r requirements.txt
locust -f locustfile.py --host=http://localhost:8081
```

### Accessing Locust Web UI

Once running, access the Locust web UI at:
```
http://localhost:8089
```

### Running Load Tests

#### Via Web UI

1. Open `http://localhost:8089`
2. Set number of users and spawn rate
3. Click "Start Swarming"
4. Monitor statistics in real-time

#### Via Command Line (Headless)

```bash
locust -f locustfile.py \
  --host=http://todo-backend:8081 \
  --users 100 \
  --spawn-rate 10 \
  --run-time 300s \
  --headless
```

### Environment-Based Configuration

Set via docker-compose:

```yaml
load-generator:
  environment:
    - LOCUST_HOST=http://todo-backend:8081
    - LOCUST_USERS=50
    - LOCUST_SPAWN_RATE=5
    - LOCUST_RUN_TIME=600
```

## Load Patterns

### Default Behavior

- Users wait 1-3 seconds between tasks (realistic user behavior)
- Tasks are executed in random order
- Each user performs multiple operations
- Created todos are tracked for later operations

### Task Distribution

The load generator performs:
- **40%**: List todos
- **30%**: Create new todos
- **15%**: Update todos
- **10%**: Complete todos
- **5%**: Delete todos

## Datadog APM Integration

### Distributed Tracing

Load Generator automatically:
- Creates spans for each HTTP request
- Propagates trace context to downstream services
- Exports traces to Datadog Agent

### Trace Configuration

```python
# Datadog tracer configured automatically via ddtrace
# Environment variables:
#   DD_AGENT_HOST=dd-agent
#   DD_TRACE_AGENT_PORT=8126
```

### Trace Attributes

Each span includes:
- HTTP method
- HTTP status code
- Request path
- Response time
- Service name: `load-generator`

## Monitoring

### Locust Statistics

The Locust web UI provides:
- **Requests per second**: Total RPS
- **Response times**: Min, max, median, p95, p99
- **Failure rate**: Percentage of failed requests
- **User count**: Current number of active users

### Metrics Available

- Total requests
- Requests per second
- Response time percentiles
- Number of failures
- Average response time

## Troubleshooting

### Load Generator Not Starting

1. **Check target URL**: Verify `LOCUST_HOST` is correct
2. **Check connectivity**: Ensure target service is accessible
3. **Check port**: Verify port 8089 is available
4. **Check logs**: Review container logs for errors

### No Traffic Generated

1. **Check users count**: Ensure `LOCUST_USERS` > 0
2. **Check autostart**: Verify `LOCUST_AUTOSTART=true`
3. **Check target**: Verify target service is responding

### Tracing Not Working

1. **Check Datadog Agent**: Verify dd-agent is running and accessible
2. **Check endpoint**: Verify `DD_AGENT_HOST=dd-agent` and `DD_TRACE_AGENT_PORT=8126`
3. **Check network**: Ensure network connectivity to the Datadog Agent

### High Error Rates

1. **Check target service**: Verify application is healthy
2. **Check load**: Reduce user count if target is overwhelmed
3. **Check network**: Verify network connectivity
4. **Check logs**: Review application logs for errors

## Customization

### Modifying User Behavior

Edit `locustfile.py` to change:
- Task definitions
- Wait times
- Request patterns
- User flows

### Adding New Tasks

```python
@task(3)  # Weight: 3 (higher = more frequent)
def my_new_task(self):
    with self.client.get("/my-endpoint", name="my_task") as response:
        if response.status_code == 200:
            # Process response
            pass
```

### Changing Wait Times

```python
# In LoadUser class
wait_time = between(2, 5)  # Wait 2-5 seconds between tasks
```

## Development

### File Structure

```
load-generator/
├── locustfile.py
├── requirements.txt
├── Dockerfile
└── README.md
```

### Dependencies

Key dependencies:
- `locust`: Load testing framework
- `ddtrace`: Datadog APM Python library for automatic instrumentation

### Running Locally

```bash
# Install dependencies
pip install -r requirements.txt

# Run Locust
locust -f locustfile.py --host=http://localhost:8081

# Or run with specific config
locust -f locustfile.py \
  --host=http://localhost:8081 \
  --users 10 \
  --spawn-rate 2
```

## Integration with Chaos Engineering

Load Generator works well with chaos disruptions:

1. **Start load generator** to create baseline traffic
2. **Enable disruptions** via Chaos Control
3. **Observe impact** on response times and error rates
4. **Monitor recovery** when disruptions are disabled

### Example Scenario

```bash
# Start load generator
docker-compose up load-generator

# Enable latency disruption
# (via Chaos Control UI)

# Observe increased response times in Locust UI

# Disable disruption
# (via Chaos Control UI)

# Observe recovery to baseline
```

## Performance Considerations

- **User count**: Each user consumes resources
- **Spawn rate**: Gradual ramp-up is more realistic
- **Network**: High user counts may saturate network
- **Target capacity**: Don't overwhelm target application

## Related Components

- **Chaos Control**: Manage disruptions while load testing
- **Chaos Agent**: Apply disruptions during load tests
- **Observability**: Monitor application behavior under load
