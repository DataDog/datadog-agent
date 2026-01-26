# Load Generator Customization for payment-service-baseline

## Overview

Customize the `locustfile.py` to create realistic load test scenarios for the **payment-service-baseline** application.

## Current Application Structure

**Application Name:** payment-service-baseline
**Components:** 2 components
**Test Targets:** 1 testable component(s)

## Available Endpoints

The following endpoints have been identified in your application:

### payment-service

- `POST /pay`
- `POST /internal/generate-report`


## Your Task

Edit `locustfile.py` to implement load test scenarios that:

### 1. Test Critical User Flows

Create Locust tasks that simulate real user behavior:

```python
class Payment_Service_BaselineUser(HttpUser):
    wait_time = between(1, 3)
    
    @task(3)  # Weight: Run this task more frequently
    def get_items(self):
        with self.client.get("/api/items", catch_response=True) as response:
            if response.status_code == 200:
                response.success()
            else:
                response.failure(f"Got status code {response.status_code}")
    
    @task(1)
    def create_item(self):
        self.client.post("/api/items", json={"name": "test"})
```

### 2. Implement Realistic Data

Use realistic test data based on your application:

```python
import random
import json

# Sample test data
SAMPLE_DATA = [
    {"field": "value1"},
    {"field": "value2"},
]

@task
def test_with_data(self):
    data = random.choice(SAMPLE_DATA)
    self.client.post("/api/endpoint", json=data)
```

### 3. Add Sequential Flows

Test multi-step user journeys:

```python
@task
def user_journey(self):
    # Step 1: Login or authenticate
    login_response = self.client.post("/api/login", json={"user": "test"})
    
    # Step 2: Fetch data
    if login_response.status_code == 200:
        token = login_response.json().get("token")
        self.client.get("/api/data", headers={"Authorization": f"Bearer {token}"})
    
    # Step 3: Update data
    self.client.put("/api/data/1", json={"status": "updated"})
```

### 4. Handle Different Response Types

Add proper response validation:

```python
@task
def validate_response(self):
    with self.client.get("/api/endpoint", catch_response=True) as response:
        if response.status_code == 200:
            try:
                json_data = response.json()
                if "expected_field" in json_data:
                    response.success()
                else:
                    response.failure("Missing expected field")
            except json.JSONDecodeError:
                response.failure("Invalid JSON response")
        else:
            response.failure(f"Status code: {response.status_code}")
```

### 5. Weight Tasks Appropriately

Use task weights to simulate realistic traffic patterns:

```python
@task(10)  # High frequency - read operations
def read_operation(self):
    self.client.get("/api/data")

@task(2)   # Medium frequency - write operations
def write_operation(self):
    self.client.post("/api/data", json={})

@task(1)   # Low frequency - delete operations
def delete_operation(self):
    self.client.delete("/api/data/1")
```

## Test Scenarios to Consider

Based on your application, implement these test types:

1. **Happy Path Tests**: Test normal user flows with valid data
2. **Edge Case Tests**: Test boundaries (empty lists, max values, etc.)
3. **Error Case Tests**: Test invalid inputs, missing parameters
4. **Load Patterns**: Mix of read-heavy and write-heavy scenarios
5. **Peak Load**: Sudden spikes in traffic
6. **Endurance**: Sustained load over time

## Performance Testing Goals

Define what metrics you want to validate:

```python
# Add custom metrics in your tests
from locust import events

@events.test_start.add_listener
def on_test_start(environment, **kwargs):
    print("Load test started for payment-service-baseline")
    print("Target metrics:")
    print("  - p50 response time: < 100ms")
    print("  - p95 response time: < 500ms")
    print("  - Error rate: < 1%")
```

## Current Configuration

The load generator is configured with:
- **Users:** 10 (adjustable via LOCUST_USERS env var)
- **Spawn Rate:** 2 users/sec (adjustable via LOCUST_SPAWN_RATE)
- **Web UI:** Available at http://localhost:8089
- **Observability:** Full Datadog APM tracing enabled via ddtrace

## Example Full Implementation

```python
from locust import HttpUser, task, between
import random
import json

class Payment_Service_BaselineUser(HttpUser):
    wait_time = between(1, 3)
    
    def on_start(self):
        """Called when a user starts - setup/authentication"""
        # Add any initialization logic
        pass
    
    @task(5)
    def list_items(self):
        """List all items - most common operation"""
        self.client.get("/api/items")
    
    @task(2)
    def get_item_details(self):
        """Get specific item details"""
        item_id = random.randint(1, 100)
        self.client.get(f"/api/items/{item_id}")
    
    @task(1)
    def create_item(self):
        """Create new item - less frequent"""
        data = {
            "name": f"test-item-{random.randint(1, 1000)}",
            "description": "Generated by load test"
        }
        self.client.post("/api/items", json=data)
```

## Testing Tips

1. **Start Small**: Test with 1-2 users first
2. **Verify Responses**: Check status codes and response bodies
3. **Monitor Resources**: Watch CPU, memory, database connections
4. **Check Logs**: Review application logs for errors
5. **Use Datadog**: View distributed traces to find bottlenecks

## Files to Modify

- **locustfile.py**: Main load test scenarios
- **requirements.txt**: Add any additional Python packages needed

## Running Your Tests

```bash
# Via Web UI (default)
docker compose up load-generator
open http://localhost:8089

# Headless mode
docker compose run -e LOCUST_HEADLESS=true -e LOCUST_RUN_TIME=60s load-generator

# Custom parameters
docker compose run -e LOCUST_USERS=50 -e LOCUST_SPAWN_RATE=5 load-generator
```

## Success Criteria

Your load tests should:
- ✅ Cover all major API endpoints
- ✅ Simulate realistic user behavior
- ✅ Include proper error handling
- ✅ Validate response data
- ✅ Help identify performance bottlenecks

**Start by implementing tests for the most critical user flows, then expand to cover edge cases and error scenarios.**
