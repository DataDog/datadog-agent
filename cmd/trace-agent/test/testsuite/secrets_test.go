// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestSecrets ensures that secrets placed in environment variables get loaded.
func TestSecrets(t *testing.T) {
	tmpDir := t.TempDir()

	// install trace-agent with -tags=secrets
	binTraceAgent := filepath.Join(tmpDir, "trace-agent")
	cmd := exec.Command("go", "build", "-o", binTraceAgent, "github.com/DataDog/datadog-agent/cmd/trace-agent")
	cmd.Stdout = io.Discard
	if err := cmd.Run(); err != nil {
		t.Skip(err.Error())
	}
	defer os.Remove(binTraceAgent)

	// install a secrets provider script
	binSecrets := filepath.Join(tmpDir, "secret-script.test")
	cmd = exec.Command("go", "build", "-o", binSecrets, "./testdata/secretscript.go")
	cmd.Stdout = io.Discard
	if err := cmd.Run(); err != nil {
		t.Skip(err.Error())
	}
	defer os.Remove(binSecrets)
	if err := os.Chmod(binSecrets, 0700); err != nil {
		t.Skip(err.Error())
	}

	// CI environment might have no datadog.yaml; we don't care in this
	// case so we can just use an empty file to avoid failure.
	if err := os.WriteFile(filepath.Join(tmpDir, "datadog.yaml"), []byte(""), os.ModePerm); err != nil {
		t.Skip(err.Error())
	}

	// run the trace-agent
	var buf safeWriter
	cmd = exec.Command(binTraceAgent, "--config", filepath.Join(tmpDir, "datadog.yaml"))
	cmd.Env = []string{
		"DD_SECRET_BACKEND_COMMAND=" + binSecrets,
		"DD_HOSTNAME=ENC[secret1]",
		"DD_API_KEY=123",
		"DD_APM_REMOTE_TAGGER=false",
	}
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	exit := make(chan error, 1)
	go func() {
		if err := cmd.Run(); err != nil {
			exit <- err
		}
	}()
	defer func(cmd *exec.Cmd) {
		cmd.Process.Kill()
	}(cmd)
	timeout := time.After(2 * time.Second)
	for {
		select {
		case <-exit:
			t.Fatalf("error: %v", buf.String())
		case <-timeout:
			t.Fatalf("timed out: %v", buf.String())
		default:
			if strings.Contains(buf.String(), "running on host decrypted_secret1") {
				// test passed
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// safeWriter is an io.Writer implementation which allows the retrieval of what was written
// to it, as a string. It is safe for concurrent use.
type safeWriter struct {
	mu  sync.RWMutex
	buf bytes.Buffer
}

func (sb *safeWriter) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *safeWriter) String() string {
	sb.mu.RLock()
	defer sb.mu.RUnlock()
	return sb.buf.String()
}
