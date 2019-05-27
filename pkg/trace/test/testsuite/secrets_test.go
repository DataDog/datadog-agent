package testsuite

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSecrets ensures that secrets placed in environment variables get loaded.
func TestSecrets(t *testing.T) {
	tmpDir := os.TempDir()

	// install trace-agent with -tags=secrets
	binTraceAgent := filepath.Join(tmpDir, "/tmp/trace-agent.test")
	cmd := exec.Command("go", "build", "-o", binTraceAgent, "-tags=secrets", "github.com/DataDog/datadog-agent/cmd/trace-agent")
	cmd.Stdout = ioutil.Discard
	if err := cmd.Run(); err != nil {
		t.Skip(err.Error())
	}
	defer os.Remove(binTraceAgent)

	// install a secrets provider script
	binSecrets := filepath.Join(tmpDir, "secret-script.test")
	cmd = exec.Command("go", "build", "-o", binSecrets, "./testdata/secretscript.go")
	cmd.Stdout = ioutil.Discard
	if err := cmd.Run(); err != nil {
		t.Skip(err.Error())
	}
	defer os.Remove(binSecrets)
	if err := os.Chmod(binSecrets, 0700); err != nil {
		t.Skip(err.Error())
	}

	// run the trace-agent
	var buf bytes.Buffer
	cmd = exec.Command("/tmp/trace-agent.test")
	cmd.Env = []string{
		"DD_SECRET_BACKEND_COMMAND=" + binSecrets,
		"DD_HOSTNAME=ENC[secret1]",
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
