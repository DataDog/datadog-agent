# PAR architecture: before vs. after

## Previous (monolithic, pre-refactor)

```mermaid
flowchart LR
    subgraph Agent["Agent process"]
        Component[PrivateActionRunner<br/>fx component]
        WR["WorkflowRunner<br/>• registry<br/>• resolver<br/>• keysManager<br/>• taskVerifier<br/>• opmsClient<br/>• taskLoop"]
        Loop["Loop<br/>• semaphore chan(RunnerPoolSize)<br/>• shutdownChannel"]
        CR["CommonRunner<br/>• healthCheckLoop"]

        Component --> WR
        Component --> CR
        WR --> Loop
    end

    OPMS[(OPMS<br/>backend)]
    Action(((action.Run)))

    Loop -- "DequeueTask" --> OPMS
    Loop -- "validate / unwrap / resolve<br/>(serial, in poll goroutine)" --> Loop
    Loop -- "sem++ then go handleTask" --> WR
    WR -- "action lookup + heartbeat + span" --> Action
    WR -- "PublishSuccess / Failure" --> OPMS
    CR -. "HealthCheck" .-> OPMS

    classDef inproc fill:#dceefb,stroke:#3b82f6,color:#000
    class Agent,Component,WR,Loop,CR inproc
```

Properties:
- One type (`WorkflowRunner`) holds *both* OPMS-orchestration state and per-task execution state.
- Validate / unwrap / resolve happen serially in the polling goroutine; semaphore reserved *after* dequeue.
- No IPC, no subprocess: the action surface is always loaded and running in the agent process.

## New (PR 1 + PR 2)

The seam is **request/response**. Orchestrator owns the full OPMS task lifecycle; Executor owns per-task compute.

```mermaid
flowchart TB
    subgraph Orchestrator["Orchestrator process"]
        Component[PrivateActionRunner<br/>fx component]
        Orch["Orchestrator<br/>• sem chan(RunnerPoolSize)<br/>• per-task goroutine<br/>• heartbeat ticker<br/>• PublishSuccess / Failure<br/>• shutdownChannel"]
        ExecIF{{"Executor interface<br/>Prepare / Execute / Stop"}}

        subgraph InProc["mode=in-process"]
            IPE[InProcessExecutor<br/>thin shim]
            THIn["TaskHandler<br/>• registry / resolver / config<br/>• keysManager / taskVerifier"]
            IPE --> THIn
        end

        subgraph Bin["mode=binary"]
            BE[BinaryExecutor]
            Sup["supervisor<br/>• procCtx (owns child lifetime)<br/>• gRPC client<br/>• cmd.Cancel = SIGTERM<br/>• WaitDelay → SIGKILL"]
            BE --> Sup
        end

        CR[CommonRunner<br/>healthCheckLoop]

        Component --> Orch
        Component --> ExecIF
        Component --> CR
        Orch --> ExecIF
        ExecIF -. "if in-process" .-> IPE
        ExecIF -. "if binary" .-> BE
    end

    subgraph Child["Executor child process (binary mode only, spawned lazily)"]
        SubCmd["executor run subcommand<br/>(re-exec of current binary)"]
        Server["executor.Server<br/>single Execute(ExecuteRequest)<br/>→ ExecuteResponse RPC"]
        THBin["TaskHandler"]
        ChildAct(((action.Run)))

        SubCmd --> Server --> THBin --> ChildAct
    end

    OPMS[(OPMS backend)]
    Act(((action.Run)))

    Orch -- "DequeueTask" --> OPMS
    Orch -- "Heartbeat" --> OPMS
    Orch -- "PublishSuccess / Failure" --> OPMS
    THIn -- "verify → resolve → action.Run" --> Act
    CR -. "HealthCheck" .-> OPMS

    Sup == "Execute(task_json) → output_json / Error<br/>(submit ctx bounds RPC only)" ==> Server
    Sup -. "spawn / SIGTERM / SIGKILL<br/>(procCtx — independent of any submit ctx)" .-> SubCmd

    classDef proc fill:#dceefb,stroke:#3b82f6,color:#000
    classDef child fill:#fde68a,stroke:#d97706,color:#000
    classDef iface fill:#ede9fe,stroke:#7c3aed,color:#000
    class Orchestrator,Component,Orch,IPE,THIn,BE,Sup,CR,InProc,Bin proc
    class Child,SubCmd,Server,THBin child
    class ExecIF iface
```

Key properties:
- **Concerns split cleanly.** Orchestrator: dequeue, validate, capacity, heartbeat, publish, dispatch. Executor (and its TaskHandler): verify the signed envelope, resolve credentials, run the action, return `(output, error)`. Nothing OPMS-shaped lives in the executor.
- **Capacity stays in the orchestrator.** Same `chan struct{}` semaphore sized by `RunnerPoolSize` as before the refactor. Works identically for in-process and binary mode.
- **In-process mode is a direct Go call.** No socket, no gRPC, no readiness polling — the seam is just an interface dispatch.
- **Binary mode = re-exec.** The orchestrator process re-execs the same agent binary into the hidden `executor run` subcommand. Tasks travel as `bytes task_json` over a local Unix socket / Windows named pipe, gated by an `x-par-executor-token` bearer.
- **Heartbeats use the outer envelope.** `Data.ID`, `Attributes.Client`, `Attributes.JobId`, and `BundleID`/`Name` (for FQN) are all on the dequeued envelope before verify, so the orchestrator never has to unwrap.

## The drain protocol

```mermaid
sequenceDiagram
    autonumber
    participant L as Agent lifecycle
    participant O as Orchestrator
    participant S as Supervisor
    participant C as Executor child

    L->>O: Stop(ctx)
    O->>O: close shutdownChannel<br/>(stop dequeuing)

    Note over O: drain phase — bounded by<br/>ExecutorDrainTimeout
    O->>O: wait for in-flight per-task goroutines<br/>(each is blocking in Execute)

    alt drain finishes in time
        O->>O: all goroutines returned
    else drain times out
        O->>O: cancel per-task ctxs<br/>→ in-flight Execute RPCs cancel<br/>→ child propagates into action ctx
    end

    O->>S: Stop(ctx)
    S->>S: procCancel()
    S-->>C: SIGTERM (via cmd.Cancel)

    alt child exits within WaitDelay
        C-->>S: exit 0
    else WaitDelay reached
        S-->>C: SIGKILL
    end

    S-->>O: Stop returned
    O-->>L: Stop returned
```

What this design avoids:
- **No status-poll drain dance.** Earlier sketches had the orchestrator send a Shutdown RPC and poll Status for `active_tasks==0`. With request/response Execute, the orchestrator already knows when its own in-flight calls return — drain is just "wait for my goroutines."
- **Submit ctx never tears the child down.** The child's lifetime is owned by `procCtx`, a fresh `context.Background()`-rooted context held by the supervisor. Submit ctxs bound only the gRPC call. `procCtx` is cancelled exactly once — by `Stop`, after the drain phase finishes.
- **Accepted-but-incomplete tasks are not lost.** If drain times out the orchestrator cancels its in-flight ctxs (child cancels the actions). OPMS lease expiry retries those tasks rather than treating them as failures.
