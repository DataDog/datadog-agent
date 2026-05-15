// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build openmetrics_differential

package differential

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// fixtureCase is one differential-testing input.
type fixtureCase struct {
	name        string
	payloadPath string                 // gzipped Prometheus/OpenMetrics text
	instance    map[string]interface{} // instance config; openmetrics_endpoint is filled in by the harness
}

var fixtureCases = []fixtureCase{
	{
		name:        "ksm/wildcard",
		payloadPath: "../testdata/upstream_benchmarks/ksm.txt.gz",
		instance: map[string]interface{}{
			"namespace": "diff",
			"metrics":   []interface{}{".+"},
		},
	},
	{
		name:        "msk_jmx/wildcard",
		payloadPath: "../testdata/upstream_benchmarks/amazon_msk_jmx_metrics.txt.gz",
		instance: map[string]interface{}{
			"namespace": "diff",
			"metrics":   []interface{}{".+"},
		},
	},
}

// TestOpenMetricsDifferential serves each fixture from a shared httptest.Server,
// runs the Go scraper against it, runs the Python check against the same URL
// via the sidecar, and asserts the two emit equivalent submission sets.
//
// Run with:  go test -tags openmetrics_differential -v -run TestOpenMetricsDifferential \
//                    ./pkg/collector/corechecks/openmetrics/differential/
func TestOpenMetricsDifferential(t *testing.T) {
	t.Parallel()

	sidecar, err := startPythonSidecar(t)
	if err != nil {
		t.Skipf("python sidecar unavailable, skipping differential: %v", err)
	}
	t.Cleanup(sidecar.Close)

	ps := newPayloadServer()
	t.Cleanup(ps.Close)

	for _, tc := range fixtureCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runOneCase(t, ps, sidecar, tc)
		})
	}
}

func runOneCase(t *testing.T, ps *payloadServer, sidecar *pythonSidecar, tc fixtureCase) {
	payload, err := loadGzipped(tc.payloadPath)
	if err != nil {
		t.Fatalf("load payload: %v", err)
	}
	t.Logf("payload bytes: %d", len(payload))

	out := runIteration(ps, sidecar, payload, tc.instance)
	if out.GoErr != nil {
		t.Fatalf("go scrape: %v", out.GoErr)
	}
	if out.PyErr != "" {
		t.Logf("python sidecar reported error: %s", out.PyErr)
	}
	t.Logf("go submissions: %d  py submissions: %d", len(out.GoSubs), len(out.PySubs))

	if len(out.Diffs) == 0 {
		t.Logf("no divergences \u2713")
		return
	}

	// Bucket by kind so a single payload-wide systemic difference doesn't
	// produce 10k log lines.
	byKind := map[string]int{}
	for _, d := range out.Diffs {
		byKind[d.Kind]++
	}
	t.Logf("%d divergences (%s)", len(out.Diffs), summarizeKinds(byKind))

	const sample = 40
	for i, d := range out.Diffs {
		if i >= sample {
			t.Logf("... (%d more)", len(out.Diffs)-sample)
			break
		}
		t.Log(FormatDiff(d))
	}
	t.Fail()
}

func summarizeKinds(m map[string]int) string {
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%s=%d", k, v))
	}
	return strings.Join(parts, " ")
}

// ---- Python sidecar ---------------------------------------------------------

type pythonSidecarResp struct {
	Submissions []Submission `json:"submissions"`
	Error       string       `json:"error"`
	Ready       bool         `json:"ready,omitempty"`
}

type pythonSidecar struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	mu     sync.Mutex // serialize requests; sidecar is single-threaded
}

func (p *pythonSidecar) Close() {
	if p == nil {
		return
	}
	_ = p.stdin.Close()
	_ = p.cmd.Process.Kill()
	_, _ = p.cmd.Process.Wait()
}

func startPythonSidecar(t *testing.T) (*pythonSidecar, error) {
	t.Helper()

	_, here, _, _ := runtime.Caller(0)
	sidecarPath := filepath.Join(filepath.Dir(here), "sidecar.py")
	if _, err := os.Stat(sidecarPath); err != nil {
		return nil, fmt.Errorf("sidecar.py not found at %s: %w", sidecarPath, err)
	}
	uvPath, err := exec.LookPath("uv")
	if err != nil {
		return nil, fmt.Errorf("uv not on PATH: %w", err)
	}

	cmd := exec.Command(uvPath, "run", "--quiet", sidecarPath)
	cmd.Stderr = os.Stderr // surface Python tracebacks during development
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(stdout)
	// Some Prometheus payloads serialize as fairly long JSON-line responses.
	// 16 MiB upper bound is overkill but cheap.
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<24)

	// Wait for the readiness handshake, with a generous timeout for first-run
	// uv environment creation (downloads + sync). Caller's test timeout is the
	// ultimate backstop.
	ready := make(chan error, 1)
	go func() {
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				ready <- err
			} else {
				ready <- io.EOF
			}
			return
		}
		var handshake pythonSidecarResp
		if err := json.Unmarshal(scanner.Bytes(), &handshake); err != nil {
			ready <- fmt.Errorf("unparseable handshake: %s", scanner.Text())
			return
		}
		if !handshake.Ready {
			ready <- fmt.Errorf("sidecar did not announce ready: %s", scanner.Text())
			return
		}
		ready <- nil
	}()

	select {
	case err := <-ready:
		if err != nil {
			_ = cmd.Process.Kill()
			return nil, fmt.Errorf("sidecar handshake: %w", err)
		}
	case <-time.After(120 * time.Second):
		_ = cmd.Process.Kill()
		return nil, errors.New("sidecar handshake timed out after 120s")
	}

	return &pythonSidecar{cmd: cmd, stdin: stdin, stdout: scanner}, nil
}

func (p *pythonSidecar) run(endpoint string, instance map[string]interface{}) (*pythonSidecarResp, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	req, err := json.Marshal(map[string]interface{}{
		"endpoint": endpoint,
		"instance": instance,
	})
	if err != nil {
		return nil, err
	}
	if _, err := p.stdin.Write(append(req, '\n')); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}
	if !p.stdout.Scan() {
		if err := p.stdout.Err(); err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		return nil, io.EOF
	}
	var resp pythonSidecarResp
	if err := json.Unmarshal(bytes.TrimSpace(p.stdout.Bytes()), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response (%q): %w", p.stdout.Text(), err)
	}
	return &resp, nil
}
