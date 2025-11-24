# DataDog Process Manager - Go Client

A simple, idiomatic Go client for the DataDog Process Manager using gRPC.

## Installation

```bash
go get github.com/DataDog/agent-process-manager/go-client
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    pm "github.com/DataDog/agent-process-manager/go-client"
)

func main() {
    // Connect to the daemon (Unix socket by default)
    client, err := pm.NewClient()
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()
    
    ctx := context.Background()
    
    // Create a process
    process, err := client.CreateProcess(ctx, &pm.CreateRequest{
        Name:    "my-app",
        Command: "/usr/bin/python3",
        Args:    []string{"app.py"},
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Created process: %s\n", process.Id)
    
    // Start the process
    if err := client.StartProcess(ctx, process.Id); err != nil {
        log.Fatal(err)
    }
    fmt.Println("Process started")
    
    // List all processes
    processes, err := client.ListProcesses(ctx)
    if err != nil {
        log.Fatal(err)
    }
    
    for _, p := range processes {
        fmt.Printf("%s: %s (PID: %d)\n", p.Name, p.State, p.Pid)
    }
    
    // Stop the process
    if err := client.StopProcess(ctx, process.Id); err != nil {
        log.Fatal(err)
    }
    fmt.Println("Process stopped")
}
```

## Connection Options

The daemon supports dual-mode operation: it can listen on both Unix socket and TCP simultaneously.

### Unix Socket (Default - Local)

Best for local connections (same machine). Fast and secure.

```go
client, err := pm.NewClient()
// Connects to /var/run/process-manager.sock
```

### TCP (Remote Access)

Best for remote connections (e.g., from host to Docker container).

```go
client, err := pm.NewClientWithAddress("localhost:50051")
// Connects via TCP
```

### Custom Unix Socket

```go
client, err := pm.NewClient(pm.WithUnixSocket("/custom/path.sock"))
```

### TCP Connection

```go
client, err := pm.NewClient(pm.WithTCP("localhost:50051"))
```

## API Reference

### Client Creation

```go
// NewClient creates a new Process Manager client
// Default: Unix socket at /var/run/process-manager.sock
func NewClient(opts ...ClientOption) (*Client, error)

// ClientOption configures the client
type ClientOption func(*clientConfig)

// WithUnixSocket uses a Unix socket connection
func WithUnixSocket(path string) ClientOption

// WithTCP uses a TCP connection
func WithTCP(address string) ClientOption
```

### Process Management

```go
// CreateProcess creates a new process
func (c *Client) CreateProcess(ctx context.Context, req *CreateRequest) (*Process, error)

// StartProcess starts a process by ID or name
func (c *Client) StartProcess(ctx context.Context, idOrName string) error

// StopProcess stops a process by ID or name
func (c *Client) StopProcess(ctx context.Context, idOrName string) error

// RestartProcess restarts a process by ID or name
func (c *Client) RestartProcess(ctx context.Context, idOrName string) error

// DeleteProcess deletes a process by ID or name
func (c *Client) DeleteProcess(ctx context.Context, idOrName string, force bool) error

// ListProcesses lists all processes
func (c *Client) ListProcesses(ctx context.Context) ([]*Process, error)

// DescribeProcess gets detailed information about a process
func (c *Client) DescribeProcess(ctx context.Context, idOrName string) (*Process, error)

// UpdateProcess updates a process configuration
func (c *Client) UpdateProcess(ctx context.Context, req *UpdateRequest) (*UpdateResponse, error)

// GetResourceUsage gets resource usage statistics for a process
func (c *Client) GetResourceUsage(ctx context.Context, idOrName string) (*ResourceUsage, error)
```

### Health Checks

```go
// Health checks if the daemon is healthy
func (c *Client) Health(ctx context.Context) error

// Status gets detailed daemon status
func (c *Client) Status(ctx context.Context) (*StatusResponse, error)
```

## Advanced Usage

### Process with Auto-Start

```go
process, err := client.CreateProcess(ctx, &pm.CreateRequest{
    Name:      "web-server",
    Command:   "/usr/bin/nginx",
    AutoStart: true, // Start immediately after creation
})
```

### Process with Resource Limits

```go
process, err := client.CreateProcess(ctx, &pm.CreateRequest{
    Name:    "limited-app",
    Command: "/usr/bin/myapp",
    ResourceLimits: &pm.ResourceLimits{
        Cpu:    1000,  // 1 CPU core (1000 millicores)
        Memory: 536870912, // 512 MB
        Pids:   100,   // Max 100 processes/threads
    },
})
```

### Process with Health Check

```go
process, err := client.CreateProcess(ctx, &pm.CreateRequest{
    Name:    "api-server",
    Command: "/usr/bin/api",
    HealthCheck: &pm.HealthCheck{
        Type:           "http",
        Endpoint:       "http://localhost:8080/health",
        Interval:       30,  // Check every 30 seconds
        Timeout:        5,   // 5 second timeout
        Retries:        3,   // Mark unhealthy after 3 failures
        RestartAfter:   0,   // Don't auto-restart (K8s liveness probe style)
    },
})
```

### Process with Restart Policy

```go
process, err := client.CreateProcess(ctx, &pm.CreateRequest{
    Name:          "worker",
    Command:       "/usr/bin/worker",
    RestartPolicy: "on-failure",  // always, never, on-failure, on-success
    RestartSec:    5,              // Wait 5 seconds before restart
})
```

### Update Process Configuration

```go
// Hot update (no restart required)
resp, err := client.UpdateProcess(ctx, &pm.UpdateRequest{
    ProcessName:   "my-app",
    RestartPolicy: stringPtr("always"),
    RestartSec:    intPtr(10),
})

// Update requiring restart
resp, err := client.UpdateProcess(ctx, &pm.UpdateRequest{
    ProcessName:     "my-app",
    Env:             map[string]string{"DEBUG": "true"},
    RestartProcess:  true, // Automatically restart to apply changes
})
```

## Error Handling

```go
process, err := client.CreateProcess(ctx, req)
if err != nil {
    switch {
    case pm.IsNotFound(err):
        fmt.Println("Process not found")
    case pm.IsAlreadyExists(err):
        fmt.Println("Process already exists")
    case pm.IsInvalidState(err):
        fmt.Println("Invalid state transition")
    default:
        log.Fatal(err)
    }
}
```

## Integration Example

### Integrating into Existing Codebase

```go
package myapp

import (
    "context"
    "time"
    
    pm "github.com/DataDog/agent-process-manager/go-client"
)

type AppManager struct {
    pm *pm.Client
}

func NewAppManager() (*AppManager, error) {
    client, err := pm.NewClient()
    if err != nil {
        return nil, err
    }
    return &AppManager{pm: client}, nil
}

func (m *AppManager) StartWorker(name string, cmd string, args []string) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    // Create process
    process, err := m.pm.CreateProcess(ctx, &pm.CreateRequest{
        Name:          name,
        Command:       cmd,
        Args:          args,
        RestartPolicy: "on-failure",
        AutoStart:     true,
    })
    if err != nil {
        return err
    }
    
    // Process is already started due to AutoStart=true
    return nil
}

func (m *AppManager) StopWorker(name string) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    return m.pm.StopProcess(ctx, name)
}

func (m *AppManager) GetWorkerStatus(name string) (string, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    process, err := m.pm.DescribeProcess(ctx, name)
    if err != nil {
        return "", err
    }
    
    return process.State, nil
}

func (m *AppManager) Close() error {
    return m.pm.Close()
}
```

## Building the Proto Files (for development)

If you need to regenerate the protobuf files:

```bash
# Install protoc and Go plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Generate from proto files
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       ../engine/proto/process_manager.proto
```

## Performance

- **Connection**: ~1ms (Unix socket), ~5ms (TCP localhost)
- **Create**: ~5-10ms
- **Start**: ~50-100ms (includes process spawn)
- **List**: ~1-5ms
- **Stop**: ~100-200ms (includes graceful shutdown)

## Thread Safety

The client is safe for concurrent use. All methods can be called from multiple goroutines.

## License

Apache 2.0 - See LICENSE file in the repository root.

