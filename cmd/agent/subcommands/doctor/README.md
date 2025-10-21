# Agent Doctor Command

## Overview

The `agent doctor` command provides an interactive terminal UI (TUI) for troubleshooting and monitoring the Datadog Agent in real-time. It aggregates telemetry from various agent components and presents them in an easy-to-navigate dashboard, helping users quickly identify issues with their agent setup.

## Goals

- **Simplified Troubleshooting**: Provide a single command that gives a comprehensive view of the agent's health without diving into logs or multiple commands
- **Real-Time Monitoring**: Continuously refresh agent status to show live telemetry and detect issues as they occur
- **Visual Data Flow**: Display the data pipeline from ingestion (checks, DogStatsD, logs) → Agent → Intake (backend) to help users understand where problems occur
- **Detailed Log Inspection**: Allow users to drill down into log sources and stream logs in real-time to diagnose collection issues
- **Non-Interactive Mode**: Support a `--no-tui` flag for CI/CD environments or scripting, outputting JSON for programmatic consumption

## Architecture

### Component-Based Design

The implementation follows the Datadog Agent's component architecture pattern:

```
┌─────────────────────────────────────────────────────────────┐
│                    CLI Command Layer                         │
│          cmd/agent/subcommands/doctor/                       │
│  - Cobra command definition                                  │
│  - FX dependency injection setup                             │
│  - IPC client integration                                    │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   ├─────────────────┐
                   ▼                 ▼
         ┌─────────────────┐  ┌─────────────────┐
         │   Doctor Comp   │  │   TUI Package   │
         │  comp/doctor/   │  │  tui/           │
         │                 │  │                 │
         │  - Aggregator   │  │  - Bubbletea    │
         │  - Expvar       │  │  - View/Update  │
         │  - HTTP API     │  │  - Log Stream   │
         └─────────────────┘  └─────────────────┘
                   │
                   ▼
         ┌─────────────────────────────────────┐
         │    Agent Daemon Components          │
         │  - Checks, DogStatsD, Logs          │
         │  - Forwarder, Health                │
         │  - Expvar telemetry                 │
         └─────────────────────────────────────┘
```

### Key Design Decisions

#### 1. **Daemon-Side Aggregation**

**Decision**: The agent daemon aggregates telemetry data; the CLI only displays it.

**Rationale**:
- Separation of concerns: Business logic stays in the daemon
- Reusability: The `/agent/doctor` HTTP endpoint can be consumed by other tools
- Consistency: All telemetry access goes through the same IPC mechanism
- Performance: Heavy aggregation happens once in the daemon, not in each CLI invocation

**Implementation**:
- `comp/doctor/doctorimpl/doctor.go`: Component that runs in the agent daemon
- `comp/doctor/doctorimpl/aggregator.go`: Collects data from expvars and agent subsystems
- HTTP API exposed at `/agent/doctor` endpoint

#### 2. **Expvar-Based Telemetry**

**Decision**: Use Go's `expvar` package and internal Prometheus metrics rather than direct component access.

**Rationale**:
- Decoupling: Doctor component doesn't depend on logs, DogStatsD, or forwarder components
- Stability: Expvar is Go's standard telemetry interface with stable APIs
- Existing infrastructure: Agent already exposes comprehensive expvars
- Read-only: No risk of modifying component state

**Implementation**:
- Read from `expvar.Get("dogstatsd")`, `expvar.Get("aggregator")`, etc.
- Integrate with `pkg/logs/status` for detailed log source information
- Parse health information from existing health check endpoints

#### 3. **Bubbletea TUI Framework**

**Decision**: Use [Bubbletea](https://github.com/charmbracelet/bubbletea) for the interactive terminal UI.

**Rationale**:
- Elm Architecture: Clean Model-Update-View pattern for state management
- Active ecosystem: Well-maintained with extensive documentation
- Composability: Works well with Lipgloss for styling and Bubbles for components
- Type-safe: Pure Go with excellent error handling

**Implementation**:
- `model.go`: Application state (Model)
- `update.go`: Event handling and state transitions (Update)
- `view.go`: Rendering functions (View)
- `messages.go`: Message types for state changes

#### 4. **Log Streaming Integration**

**Decision**: Reuse the existing `/agent/stream-logs` endpoint with a custom viewer in the TUI.

**Rationale**:
- Code reuse: Leverage existing log streaming infrastructure
- Consistency: Same filtering mechanism as `agent stream-logs` command
- Real-time: Use HTTP chunked transfer for live log streaming
- Context management: Proper cancellation when switching between log sources

**Implementation**:
- `logFetcher` struct: Manages streaming connection lifecycle
- `PostChunk` callback: Receives log chunks from the agent daemon
- `bufio.Scanner`: Parses log lines from chunks
- `context.Context`: Cancellation support for cleanup

## Data Flow

### Main Dashboard View

```
User Input → Bubbletea Update → IPC Client → Agent Daemon
     ↓                                              ↓
Keyboard Events                            Doctor Component
     ↓                                              ↓
Navigation/Refresh                         Aggregate Expvars
     ↓                                              ↓
Update Model ← JSON Response ← HTTP /agent/doctor ←┘
     ↓
Render View → Terminal
```

### Log Streaming View

```
User Selects Log → Create logFetcher → HTTP POST /agent/stream-logs
     ↓                                              ↓
Start Streaming                             Filter & Stream Logs
     ↓                                              ↓
Chunk Received → Parse Lines → Update Buffer → Render Logs
     ↓
User Navigates → Cancel Context → Close Stream
```

## Technical Stack

### Core Libraries

- **Cobra**: CLI command framework (standard across agent commands)
- **FX**: Dependency injection (agent's component system)
- **Bubbletea**: Terminal UI framework (Elm architecture)
- **Lipgloss**: Terminal styling (colors, borders, layout)

### Agent Components

- **IPC Client**: Secure inter-process communication with the daemon
- **Doctor Component**: Telemetry aggregation daemon component
- **Logs Status**: Integration with log collection status

### Go Standard Library

- `expvar`: Variable export for telemetry
- `bufio`: Efficient buffered I/O for log parsing
- `context`: Cancellation and timeout management
- `sync`: Concurrency primitives (Mutex, WaitGroup)

## File Structure

```
cmd/agent/subcommands/doctor/
├── command.go              # Cobra command definition and FX wiring
├── README.md               # This file
└── tui/                    # Terminal UI implementation
    ├── run.go              # TUI entry point
    ├── model.go            # Application state and logFetcher
    ├── update.go           # Event handling and state transitions
    ├── view.go             # Rendering functions (main, logs detail)
    ├── messages.go         # Message type definitions
    ├── styles.go           # Lipgloss styling definitions
    └── logfetcher_test.go  # Unit tests for log streaming

comp/doctor/
├── def/
│   └── component.go        # Component interface and data structures
├── fx/
│   └── fx.go               # FX module registration
└── doctorimpl/
    ├── doctor.go           # Component implementation
    └── aggregator.go       # Telemetry collection logic
```

## Key Types

### DoctorStatus

The root data structure containing all agent status:

```go
type DoctorStatus struct {
    Timestamp time.Time       // When status was collected
    Ingestion IngestionStatus // Checks, DogStatsD, Logs, Metrics
    Agent     AgentStatus     // Health, version, uptime, errors
    Intake    IntakeStatus    // Backend connectivity, endpoints
    Services  []ServiceStats  // Per-service aggregated stats
}
```

### logFetcher

Manages log streaming lifecycle:

```go
type logFetcher struct {
    filtersJSON  []byte          // JSON filters for log source
    url          string          // Stream endpoint URL
    client       ipc.HTTPClient  // IPC client
    logChunkChan chan []byte     // Channel for receiving chunks
    cmdCtx       context.Context // Cancellation context
    cmdCncl      func()          // Cancel function
    buf          bytes.Buffer    // Accumulation buffer
    scanner      *bufio.Scanner  // Line parser
    wg           sync.WaitGroup  // Goroutine synchronization
}
```

## Navigation

### Main View

- **`←/→` or `h/l`**: Navigate between panels (Ingestion | Agent | Intake)
- **`Enter`**: Drill down into selected panel (currently only Logs)
- **`r`**: Manual refresh
- **`q` or `Ctrl+C`**: Quit

### Logs Detail View

- **`↑/↓` or `k/j`**: Navigate between log sources
- **Auto-switch**: Automatically switches log stream when selecting different source
- **`Esc`**: Return to main view
- **`q` or `Ctrl+C`**: Quit

## Layout

### Main Dashboard (Three-Panel Triptych)

```
┌─────────────────────────────────────────────────────────────┐
│                    DATADOG AGENT DOCTOR                      │
├──────────────────┬──────────────────┬──────────────────────┬┘
│   INGESTION      │   AGENT HEALTH   │      INTAKE          │
│                  │                  │                      │
│ Checks: 5/5 ✓    │ Running ✓        │ Connected ✓          │
│ DogStatsD        │ Version: 7.x     │ API Key: Valid       │
│   Metrics: 1.2K  │ Uptime: 2h       │ Last Flush: 1s ago   │
│ ▶ Logs: Enabled  │ Errors: 0        │                      │
│   Sources: 3     │ Healthy:         │ Endpoints:           │
│   Lines: 45K     │   - forwarder    │   ✓ metrics          │
│ Metrics          │   - logs         │   ✓ logs             │
│   Queued: 0      │   - checks       │   ✓ traces           │
└──────────────────┴──────────────────┴──────────────────────┘
```

### Logs Detail View (Two-Panel Split)

```
┌─────────────────────────────────────────────────────────────┐
│                    LOGS DETAIL VIEW                          │
├─────────────────────────────┬───────────────────────────────┤
│ LOG SOURCES (40%)           │ STREAMING LOGS (60%)          │
│                             │                               │
│ ✓ ENABLED - 3 source(s)     │ Streaming: nginx              │
│                             │                               │
│ ▶ ✓ nginx                   │ 2025-01-21 10:00:00 INFO ...  │
│    Type: file               │ 2025-01-21 10:00:01 DEBUG ... │
│    Files:                   │ 2025-01-21 10:00:02 INFO ...  │
│      • /var/log/nginx/*.log │ 2025-01-21 10:00:03 WARN ...  │
│    Stats:                   │ 2025-01-21 10:00:04 INFO ...  │
│      BytesRead: 1.2MB       │ ...                           │
│                             │                               │
│  ✓ postgres                 │ (last 100 lines, auto-scroll) │
│  ✓ application              │                               │
│                             │                               │
└─────────────────────────────┴───────────────────────────────┘
│ ↑/↓: Navigate | Esc: Back | Q: Quit                         │
└─────────────────────────────────────────────────────────────┘
```

## Testing

### Unit Tests

The `logfetcher_test.go` file contains comprehensive tests for the log streaming functionality:

- **`TestLogFetcher_BasicStreaming`**: Validates chunk receiving and parsing
- **`TestLogFetcher_WaitCmd`**: Tests message processing from chunks
- **`TestLogFetcher_WaitCmd_PartialLines`**: Handles incomplete lines across chunks
- **`TestLogFetcher_Close`**: Validates proper cleanup
- **`TestLogFetcher_MultipleChunks`**: Sequential chunk processing
- **`TestLogFetcher_EmptyChunks`**: Edge case handling

**Run tests**:
```bash
dda inv test --targets=./cmd/agent/subcommands/doctor/tui
```

### Mock HTTP Client

Tests use a `mockHTTPClient` that implements `ipc.HTTPClient` interface, with a test HTTP server that simulates chunked log streaming.

## Future Enhancements

### Planned Features

1. **Drill-Down for Other Panels**:
   - Agent Health: Show detailed component health status
   - Checks: List all checks with their last execution status
   - Intake: Display per-endpoint metrics and error details

2. **Search and Filtering**:
   - Search logs in the streaming view
   - Filter checks by status (error/warning/ok)
   - Filter log sources by integration

3. **Historical View**:
   - Show metrics trends (last hour/day)
   - Graph error rates over time
   - Display flush success/failure history

4. **Export Functionality**:
   - Export current status to JSON file
   - Save log streams to file
   - Generate diagnostic reports

5. **Alert Indicators**:
   - Visual alerts for critical issues
   - Flash warnings for new errors
   - Sound notifications (optional)

### Technical Improvements

1. **Performance**:
   - Optimize expvar parsing for large installations
   - Implement efficient log line buffering
   - Add pagination for large check lists

2. **Testing**:
   - Integration tests with real agent components
   - E2E tests for full TUI workflow
   - Benchmark tests for large-scale scenarios

3. **Error Handling**:
   - Better error messages for common issues
   - Retry logic for transient IPC failures
   - Graceful degradation when components are unavailable

## Contributing

When contributing to the doctor command:

1. **Follow the Elm Architecture**: Keep Model-Update-View separation clean
2. **Use the Component System**: New telemetry should go through the doctor component
3. **Test Your Changes**: Add unit tests for new functionality
4. **Update This README**: Document new features and technical decisions
5. **Consider Non-Interactive Mode**: Ensure `--no-tui` works with new features

## References

- [Bubbletea Documentation](https://github.com/charmbracelet/bubbletea)
- [Lipgloss Documentation](https://github.com/charmbracelet/lipgloss)
- [Agent Component Architecture](https://github.com/DataDog/datadog-agent/tree/main/comp)
- [Expvar Package](https://pkg.go.dev/expvar)
