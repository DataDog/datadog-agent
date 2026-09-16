# e2ectl: running existing Host tests against the local container agent — code plan

> **Implemented — live-verified (4/4 tests pass against a local container in 0.07s).**
> See the [plan status index](../qa-e2ectl-plans-index.md#5-category-c--pending-feature-designs-not-implemented).

**Status:** code excerpts, not a patch. Grounded in the current source.

## 1. What the test framework's Host actually is (the ground truth)

```go
// testing/utils/e2e/client/host.go
type Host struct {
    *sshExecutor              // ← provides Execute/MustExecute/Start/Reconnect (SSH)
    HostArtifactClient         // ← provides Get (SCP)
    convertPathSeparator convertPathSeparatorFn
    osFamily             oscomp.Family
    httpTransport        *http.Transport
}

// The SSH executor — a concrete type, not an interface:
type sshExecutor struct {
    client     *ssh.Client
    privileged *ssh.Client
    context    Context
    // ...
}
```

Two layers of methods on `Host`:

| Layer | Methods | Implementation |
|---|---|---|
| Executor (from `*sshExecutor`) | Execute, MustExecute, MustExecuteOn, Start, Reconnect | SSH sessions |
| File operations (on `Host` directly) | FileExists, ReadFile, ReadFilePrivileged, WriteFile, AppendFile, MkdirAll, Remove, RemoveAll, CopyFile, CopyFolder, GetFile, GetFolder, Lstat, ReadDir, FindFiles, DialPort, GetTmpFolder, NewHTTPClient, JoinPath | **SFTP** (`h.getSFTPClient()`) |

Usage counts across all tests (from grep):
```
MustExecute: 277   Execute: 164   MustExecuteOn: 35   OSFamily: 26
MkdirAll: 10       ReadFile: 7    FileExists: 6       CopyFile: 4  ...
```

The top three cover ~80% of what tests call. The file operations are the remaining 20%.

## 2. The executor interface extraction

### 2.1 New file: `testing/utils/e2e/client/executor.go`

```go
package client

import (
    "github.com/stretchr/testify/require"
)

// Executor is the command-execution surface of a Host. The SSH executor
// and the docker executor both satisfy it; Host embeds the interface so
// existing callers (Execute/MustExecute/MustExecuteOn) work unchanged.
type Executor interface {
    Execute(command string, options ...ExecuteOption) (string, error)
    MustExecute(command string, options ...ExecuteOption) string
    MustExecuteOn(tb require.TestingT, command string, options ...ExecuteOption) string
}
```

`sshExecutor` already satisfies this — no change to its methods.

### 2.2 `Host` struct change

```go
// Before:
type Host struct {
    *sshExecutor
    // ...
}

// After:
type Host struct {
    Executor          // interface: *sshExecutor or *dockerExecutor
    HostArtifactClient
    // ...
}
```

One-line change. All `host.Execute(...)` calls work identically — Go's embedding
dispatches to whichever implementation is stored.

### 2.3 The docker executor

```go
// testing/utils/e2e/client/docker_executor.go

// dockerExecutor runs commands inside a Docker container via docker exec.
// It is the container twin of sshExecutor — same interface, no SSH.
type dockerExecutor struct {
    containerName string
    context       Context
}

func newDockerExecutor(ctx Context, containerName string) *dockerExecutor {
    return &dockerExecutor{containerName: containerName, context: ctx}
}

func (d *dockerExecutor) Execute(command string, options ...ExecuteOption) (string, error) {
    params, err := optional.MakeParams(options...)
    if err != nil {
        return "", err
    }
    // Build the env-var prefix (same buildCommand semantics as SSH, but for sh).
    args := []string{"exec"}
    for _, env := range params.EnvVariables {
        args = append(args, "-e", env)
    }
    args = append(args, d.containerName, "sh", "-c", command)

    d.context.Logf("Running command in container %s: %s", d.containerName, command)
    cmd := exec.Command("docker", args...)
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    if err := cmd.Run(); err != nil {
        return stdout.String(), fmt.Errorf("docker exec %s: %w (stderr: %s)",
            d.containerName, err, stderr.String())
    }
    return stdout.String(), nil
}

func (d *dockerExecutor) MustExecute(command string, options ...ExecuteOption) string {
    stdout, err := d.Execute(command, options...)
    if err != nil {
        d.context.FailNow("Failed to run command in container %s: %s, err: %s",
            d.containerName, command, err)
    }
    return stdout
}

func (d *dockerExecutor) MustExecuteOn(tb require.TestingT, command string, options ...ExecuteOption) string {
    stdout, err := d.Execute(command, options...)
    if err != nil {
        require.FailNow(tb, "Failed to run command", "command: %s, err: %s", command, err)
    }
    return stdout
}
```

## 3. File operations — the SFTP problem and its docker solution

The file methods (`ReadFile`, `WriteFile`, `MkdirAll`, etc.) all call
`h.getSFTPClient()`. SFTP is a protocol over SSH — there is no SFTP in a docker
container. The pragmatic fix: implement them as **commands via the executor**.

### 3.1 File-operation interface

```go
// Executor with file operations — what the Host methods actually need.
type HostFileSystem interface {
    Executor
    FileExists(path string) (bool, error)
    ReadFile(path string) ([]byte, error)
    WriteFile(path string, content []byte) (int64, error)
    MkdirAll(path string) error
    Remove(path string) error
    RemoveAll(path string) error
}
```

### 3.2 Docker file operations (command-based)

Each SFTP file operation maps to a shell command inside the container:

```go
func (d *dockerExecutor) FileExists(path string) (bool, error) {
    // test -f is the portable equivalent of sftp.Lstat + regular-file check
    _, err := d.Execute(fmt.Sprintf("test -f %s", path))
    if err == nil { return true, nil }
    // distinguish "not found" from "command failed"
    if strings.Contains(err.Error(), "exit status 1") { return false, nil }
    return false, err
}

func (d *dockerExecutor) ReadFile(path string) ([]byte, error) {
    // cat is the command equivalent of sftp.Open + Read
    content, err := d.Execute(fmt.Sprintf("cat %s", path))
    return []byte(content), err
}

func (d *dockerExecutor) WriteFile(path string, content []byte) (int64, error) {
    // base64 avoids shell-quoting issues with content
    encoded := base64.StdEncoding.EncodeToString(content)
    _, err := d.Execute(fmt.Sprintf("echo %s | base64 -d > %s", encoded, path))
    return int64(len(content)), err
}

func (d *dockerExecutor) MkdirAll(path string) error {
    _, err := d.Execute(fmt.Sprintf("mkdir -p %s", path))
    return err
}
```

### 3.3 The `Host` struct holds the file-system interface

```go
type Host struct {
    HostFileSystem     // Executor + file operations (sshExecutor or dockerExecutor)
    HostArtifactClient  // Get (SCP for SSH; docker cp for docker)
    // ...
}
```

The existing SFTP-backed implementations on `Host` move to `sshExecutor` (they already
read from `h.getSFTPClient()`, which is SSH-specific). `dockerExecutor` provides the
docker equivalents. Both satisfy `HostFileSystem`.

## 4. `HostOutput` and transport detection

```go
// components/outputs/components.go — one new optional field:
type HostOutput struct {
    // ... existing fields ...
    Transport string `json:"transport,omitempty"` // "ssh" (default) | "docker"
    // Address doubles as the connection target: SSH host or docker container name
}
```

`Address` already answers "where to connect to". `Transport` answers "how". When
`Transport == "docker"`, `Address` holds the container name; the SSH-specific fields
(`Port`, `Username`, `Password`) are ignored. This is more orthogonal than a separate
`Transport` — one field for the target (reused: `Address`), one for the method (new).

### 4.1 `client.NewHost` transport detection

```go
// testing/utils/e2e/client/host.go
func NewHost(context Context, hostOutput outputs.HostOutput) (*Host, error) {
    if hostOutput.Transport == "docker" {
        return newDockerHost(context, hostOutput)  // Address = container name
    }
    return newSSHHost(context, hostOutput)  // the existing SSH path, unchanged
}

func newDockerHost(ctx Context, out outputs.HostOutput) (*Host, error) {
    executor := newDockerExecutor(ctx, out.Address)  // Address is the container name
    return &Host{
        HostFileSystem:     executor,
        HostArtifactClient: &dockerArtifactClient{executor: executor},
        osFamily:           out.OSFamily,
        convertPathSeparator: convertPathSeparatorForOS(out.OSFamily),
        // httpTransport: nil (no SSH tunnel; DialPort is deferred)
    }, nil
}
```

## 5. The binary installer writes `remoteHost` to the snapshot

### 5.1 The snapshot entry

```go
// installer/binary.go — in Install(), after the container is running:

// remoteHost makes the agent container look like a Host to the test
// framework: Transport=docker tells RemoteHost.Init to use docker exec
// with Address as the container name. The sudo shim makes sudo work.
func writeRemoteHostToSnapshot(entry envstore.Entry) error {
    hostJSON, err := json.Marshal(map[string]any{
        "transport":      "docker",
        "address":        localinfra.AgentContainer(entry.Name),
        "cloudProvider":  "local",
        "osFamily":       "linux",      // the runtime image's OS
        "osFlavor":       "ubuntu",
        "osVersion":      "24.04",
        "architecture":   "arm64",      // or amd64, from the image
    })
    if err != nil {
        return err
    }
    return provisioner.UpdateSnapshotResource(entry.SnapshotPath(), "remoteHost", hostJSON)
}
```

### 5.2 The sudo shim

```go
// installer/binary.go — in Install(), right after docker run:

func installSudoShim(entry envstore.Entry) error {
    // The container runs as root; `sudo cmd` → `cmd`. One line, permanent
    // for the container's lifetime (re-created on each install/update).
    shim := `printf "#!/bin/sh\nexec $@\n" > /usr/local/bin/sudo && chmod +x /usr/local/bin/sudo`
    cmd := exec.Command("docker", "exec", localinfra.AgentContainer(entry.Name), "sh", "-c", shim)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}
```

## 6. The attach provisioner

```go
// testing/provisioners/local/host/attach.go — or a simpler location

// Attach returns a provisioner that connects to a running e2ectl local
// environment. The test framework rehydrates environments.Host from the
// snapshot (fakeIntake + remoteHost with Transport=docker). Destroy is
// a no-op — the environment is owned by e2ectl and is long-lived.
func Attach(envName, snapshotPath string) provisioner.TypedProvisioner[environments.Host] {
    return provisioner.NewStaticStackProvisioner[environments.Host]("local-attach", snapshotPath)
}
```

The existing `StaticStackProvisioner` already reads the snapshot and imports the
`remoteHost` and `fakeIntake` components. `RemoteHost.Init` calls `client.NewHost`,
which detects `Transport == "docker"` and builds the docker executor. No provisioner changes
needed — the existing attach mechanism works as-is.

## 7. What the `RemoteHost.Init` change looks like

```go
// testing/components/remotehost.go — the ONLY change:

func (h *RemoteHost) Init(ctx common.Context) (err error) {
    h.context = ctx
    h.Host, err = client.NewHost(ctx, h.HostOutput)  // already delegates; now detects transport
    return err
}
```

Actually, zero changes — `RemoteHost.Init` already calls `client.NewHost`. The
transport detection happens inside `NewHost`. The existing SSH path is untouched.

## 8. File map

| File | Change |
|---|---|
| `testing/utils/e2e/client/executor.go` **new** | `Executor` interface (3 methods) |
| `testing/utils/e2e/client/docker_executor.go` **new** | `dockerExecutor` — Execute/MustExecute + file operations via commands |
| `testing/utils/e2e/client/host.go` | `Host` embeds `HostFileSystem` interface instead of `*sshExecutor`; `NewHost` detects `Transport == "docker"`; SSH file methods move to `sshExecutor` |
| `components/outputs/components.go` | `HostOutput.Transport` field (`"docker"` or absent for SSH) |
| `cmd/e2ectl/internal/installer/binary.go` | writes `remoteHost` to snapshot + installs sudo shim |
| `testing/provisioners/local/host/` **new** | `Attach` — one-liner wrapping `StaticStackProvisioner` |
| No change | `RemoteHost.Init`, `StaticStackProvisioner`, `environments.Host`, existing tests |

## 9. Tests

Offline (no Docker, no SSH):
- `Executor` interface compliance: both `sshExecutor` and `dockerExecutor` satisfy it
- `dockerExecutor.Execute` with a stub docker on PATH (or a test double): command
  construction, env-var handling, error messages include the container name
- `FileExists/ReadFile/WriteFile/MkdirAll` via commands: correct outputs, exit-code
  mapping (1 = not found)
- `NewHost` with `Transport == "docker"` → returns a Host with a dockerExecutor using
  `Address` as the container name, no SSH
- `NewHost` with `Transport` absent or `"ssh"` → returns a Host with an sshExecutor
  (existing behavior; existing tests stay green)
- The sudo shim script is valid sh
- Snapshot: `remoteHost` entry has the right fields; `StaticStackProvisioner[Host]`
  rehydrates with the docker executor

Live (local Docker only):
- Attach to a running e2ectl local environment
- `RemoteHost.Execute("whoami")` → "root"
- `RemoteHost.MustExecute("sudo cat /etc/datadog-agent/datadog.yaml")` → the wired
  config (sudo shim works)
- `RemoteHost.ReadFile("/etc/datadog-agent/datadog.yaml")` → same content
- `FakeIntake.Client().FilterMetrics("datadog.agent.running")` → heartbeat
- Pick one real Host test (e.g. a `file_tailing` variant) and run it unchanged

## 10. What doesn't work (unchanged from the design plan)

| Capability | Status |
|---|---|
| `Start` (streaming SSH session) | Not in the docker executor — no tests use it in the top-80%; deferred |
| `DialPort` / `NewHTTPClient` | The container's ports aren't exposed; deferred until a test needs it |
| `GetFolder` (SCP directory transfer) | `docker cp` works; `HostArtifactClient` implementation deferred |
| systemd / package management | Same errors as any non-systemd host |
| Host-level checks | Container view, not host view |

## 11. Sequencing

| Step | Gate |
|---|---|
| 1. `Executor` interface + `Host` struct change | All existing SSH tests stay green (the interface is satisfied by `sshExecutor`; zero behavior change) |
| 2. `dockerExecutor` (Execute + file ops) | Hermetic tests with a stub docker binary |
| 3. `HostOutput.Transport` + `NewHost` detection | Existing SSH path verified unchanged; docker path returns a working Host with `Address` as container name |
| 4. Sudo shim + `remoteHost` snapshot entry | The e2ectl local snapshot contains `remoteHost` + `fakeIntake` |
| 5. `Attach` provisioner | One real Host test runs unchanged against a live local env |
