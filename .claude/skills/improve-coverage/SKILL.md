---
name: improve-coverage
description: Improve test coverage for a Go package and all its subpackages to at least 70%
argument-hint: "<package-path e.g. pkg/util/cloudproviders>"
allowed-tools: Bash, Read, Glob
---

# Improve Test Coverage Skill

Improve test coverage for a Go package and all its subpackages to at least 70%.

## Usage
```
/improve-coverage <package-path>
```

Example:
```
/improve-coverage pkg/util/cloudproviders
```

This will improve coverage for `pkg/util/cloudproviders` AND all subpackages like `pkg/util/cloudproviders/network`, `pkg/util/cloudproviders/gce`, etc.

## Instructions

When this skill is invoked with a package path, follow these steps:

### 1. Discover All Subpackages

List all packages under the given path:
```bash
go list ./<package-path>/... 2>/dev/null | sed 's|github.com/DataDog/datadog-agent/||'
```

Store this list and process each package systematically.

### 2. For Each Package, Analyze Current State

Check if the package has Go files:
```bash
ls <pkg>/*.go 2>/dev/null | grep -v _test.go | head -10
```

If no Go files exist (only subpackages), skip to next package.

Run tests with coverage to get baseline:
```bash
dda inv test --targets ./<pkg> --coverage 2>&1 | tail -20
```

Check which functions need coverage:
```bash
go tool cover -func=coverage.out 2>/dev/null | grep "<pkg>" | sort -t'%' -k2 -n
```

### 3. Read and Understand the Code

- Read all non-test Go files in the package
- Identify exported functions and types that need testing
- Look for existing test patterns in `*_test.go` files
- Check build tags (e.g., `//go:build linux`) to match in test files

### 4. Identify Testing Patterns

Look for common patterns in the codebase:
- Mock configs: `configmock "github.com/DataDog/datadog-agent/pkg/config/mock"` with `cfg := configmock.New(t)`
- Cache cleanup: Use `t.Cleanup()` to clear caches between tests
- Function variable mocking: Replace function variables and restore with defer
- Table-driven tests: Use `[]struct{...}` with subtests

### 5. Write Tests

Create or update `*_test.go` files following these principles:

**Test file header:**
```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Match build tags from source file if any (e.g., //go:build linux)

package <packagename>

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    // Add other imports as needed
)
```

**Test patterns:**
- Use `require` for fatal assertions, `assert` for non-fatal
- Use `t.Run()` for subtests
- Use `t.Cleanup()` for cleanup instead of `defer` where appropriate
- Mock external dependencies (HTTP, file system, configs)
- Test error paths, not just happy paths
- Test edge cases (empty input, nil, invalid values)

**What to test:**
- All exported functions
- Error handling paths
- Boundary conditions
- Caching behavior (if applicable)
- Configuration handling

**What NOT to test:**
- Functions that only call external services without mocking
- Generated code
- Pure pass-through functions

### 6. Verify and Iterate

After writing tests, run them:
```bash
dda inv test --targets ./<pkg> --coverage 2>&1 | tail -20
```

Check new coverage:
```bash
go tool cover -func=coverage.out 2>/dev/null | grep "<pkg>"
```

If coverage is below 70%:
- Identify remaining uncovered functions
- Add more tests
- Consider if remaining code is testable (external dependencies may need mocking)
- Document why certain code cannot be tested if applicable

### 7. Review Tests

After improving each package, spawn a sub-agent to review the tests:
```
Use Task tool with subagent_type=general-purpose to review tests for:
- Test quality and usefulness
- Proper mocking of external dependencies
- Edge case coverage
- Test naming conventions
- Potential flakiness
```

Address any feedback from the review before moving to the next package.

### 8. Continue to Next Package

After completing one package:
1. Move to the next subpackage in the list
2. Repeat steps 2-7
3. Continue until all subpackages are processed

### 9. Final Summary

After all packages are processed, provide a comprehensive summary:

```
## Coverage Improvement Summary

| Package | Initial | Final | Tests Added |
|---------|---------|-------|-------------|
| pkg/util/foo | 45% | 78% | 12 |
| pkg/util/foo/bar | 0% | 85% | 8 |
| ... | ... | ... | ... |

### Packages That Could Not Reach 70%
- `pkg/util/foo/external`: 55% - Requires external service mocking
- Reason: ...

### Total Tests Added: X
### Average Coverage Improvement: Y%
```

## Common Mocking Patterns

### Config Mocking
```go
import configmock "github.com/DataDog/datadog-agent/pkg/config/mock"

func TestSomething(t *testing.T) {
    cfg := configmock.New(t)
    cfg.SetWithoutSource("config.key", "value")
    // Test code using config
}
```

### Function Variable Mocking
```go
func TestWithMock(t *testing.T) {
    origFunc := somePackageFunc
    defer func() { somePackageFunc = origFunc }()

    somePackageFunc = func() error {
        return nil // or mock behavior
    }
    // Test code
}
```

### Cache Cleanup
```go
func addCleanup(t *testing.T) {
    t.Cleanup(func() {
        cache.Cache.Delete(cacheKey)
    })
}
```

### HTTP Mocking
```go
import "net/http/httptest"

func TestHTTPCall(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`{"result": "ok"}`))
    }))
    defer server.Close()
    // Use server.URL in test
}
```

### Environment Variable Mocking
```go
func TestWithEnv(t *testing.T) {
    orig := os.Getenv("MY_VAR")
    defer os.Setenv("MY_VAR", orig)

    os.Setenv("MY_VAR", "test-value")
    // Test code
}
```

## Target Coverage

- Minimum: 70%
- Good: 80%+
- Excellent: 90%+

Focus on meaningful tests that verify behavior, not just line coverage. A well-tested function with 70% coverage is better than superficial tests achieving 90%.

## Handling Difficult Cases

### External Service Dependencies
If a function calls external services (AWS, GCP, HTTP endpoints):
1. Look for existing mock patterns in the codebase
2. Use `httptest.Server` for HTTP mocking
3. Use interface-based mocking if available
4. If no reasonable mock exists, document and skip

### Platform-Specific Code
For code with build tags like `//go:build linux`:
1. Match the build tag in test files
2. Tests will only run on that platform
3. Consider if behavior can be tested on current platform

### Generated Code
Skip coverage for:
- Protocol buffer generated files (`*.pb.go`)
- Mock files generated by tools
- Other auto-generated code
