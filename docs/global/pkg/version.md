> **TL;DR:** Provides the agent's version string and a minimal SemVer library — build-time variables (`AgentVersion`, `Commit`, etc.) are injected via ldflags so any binary that links this package automatically carries the correct version.

# pkg/version

## Purpose

`pkg/version` provides the agent's version string and a small SemVer library for parsing,
formatting, and comparing version numbers. It has no runtime dependencies beyond the Go
standard library.

Version variables (`AgentVersion`, `Commit`, etc.) are package-level `var` declarations
injected at link time via `-X` ldflags. Any binary that links this package automatically
gets the correct version baked in; consuming code only needs to import the package.

## Key elements

### Key types

### Build-time variables

These are set by `get_version_ldflags` in `tasks/libs/common/utils.py` during every build.
They are empty strings in local `go test` runs (except `AgentVersion`, which falls back to
`"6.0.0"` via an `init()`).

| Variable | Description |
|----------|-------------|
| `AgentVersion` | Full version string, e.g. `"7.54.0"` or `"7.54.0-rc.1+git.42.abc1234"`. Defaults to `"6.0.0"` if not set. |
| `AgentVersionURLSafe` | Same as `AgentVersion` but safe for use in URLs (pipeline ID appended). |
| `AgentPackageVersion` | Package version reported by the updater; may differ from `AgentVersion` on Windows or when installed via Fleet Automation. |
| `Commit` | Short Git SHA of the build commit. |
| `AgentPayloadVersion` | Version of the `agent-payload` protobuf library used for serialization. |

### Version struct

```go
type Version struct {
    Major  int64
    Minor  int64
    Patch  int64
    Pre    string  // e.g. "rc.1"
    Meta   string  // e.g. "git.42"
    Commit string
}
```

### Key functions

```go
// Agent returns a Version parsed from AgentVersion and Commit.
func Agent() (Version, error)

// New parses a SemVer string and a commit identifier into a Version.
// Accepted format: "MAJOR.MINOR.PATCH[-pre][+meta]"
func New(version, commit string) (Version, error)
```

### Version methods

```go
// String returns the full version string, e.g. "7.54.0-rc.1+git.42.commit.abc1234"
func (v *Version) String() string

// GetNumber returns "MAJOR.MINOR.PATCH", dropping pre/meta/commit.
func (v *Version) GetNumber() string

// GetNumberAndPre returns "MAJOR.MINOR.PATCH[-pre]", dropping meta/commit.
func (v *Version) GetNumberAndPre() string

// CompareTo compares v against the given version string.
// Returns -1 if v < version, 0 if equal, +1 if v > version.
// Only Major, Minor, and Patch are compared; Pre/Meta are ignored.
func (v *Version) CompareTo(version string) (int, error)
```

## Usage

### Printing version information in a CLI command

```go
import pkgversion "github.com/DataDog/datadog-agent/pkg/version"

av, _ := pkgversion.Agent()
fmt.Printf("Agent %s - Commit: %s - Serialization: %s\n",
    av.GetNumberAndPre(),
    pkgversion.Commit,
    pkgversion.AgentPayloadVersion,
)
```

### Logging the version at startup

```go
log.Infof("Starting Datadog Agent v%v", version.AgentVersion)
```

### Conditional logic based on version (e.g. feature flags)

```go
agentVersion, err := version.Agent()
if err == nil {
    if res, err := agentVersion.CompareTo("7.50.0"); err == nil && res >= 0 {
        // enable feature available from 7.50.0 onwards
    }
}
```

### Parsing an arbitrary version string (e.g. in E2E tests)

```go
expected, err := version.New("7.54.0", "")
actual, err   := version.New(reportedVersion, "")
if cmp, _ := expected.CompareTo(actual.GetNumber()); cmp != 0 {
    t.Errorf("version mismatch: want %s, got %s", expected.GetNumber(), actual.GetNumber())
}
```

### Scrubbing version strings from logs

`pkg/util/scrubber` calls `version.New` to detect and redact version strings that appear in
agent logs before they are included in flares.

## Notes

- `CompareTo` only compares the numeric triplet (`Major.Minor.Patch`). Pre-release and
  metadata fields are intentionally ignored, so `7.54.0-rc.1` and `7.54.0` are considered
  equal by `CompareTo`.
- In tests that don't go through the build system, `AgentVersion` will be `"6.0.0"` (the
  default set in `init()`). Use `version.New("x.y.z", "")` directly if you need a specific
  version in a test.
- `AgentPackageVersion` is distinct from `AgentVersion` on Windows (where the updater appends
  a `-1` suffix) and when a custom `PACKAGE_VERSION` env var is set in the pipeline.
