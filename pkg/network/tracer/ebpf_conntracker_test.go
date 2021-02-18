package tracer

import (
	"fmt"
	"net"
	"os/exec"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/stretchr/testify/require"
)

func TestEBPFConntracker(t *testing.T) {
	cmd := exec.Command("testdata/setup_dnat.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("setup command output: %s", string(out))
	}
	defer teardown(t)

	cfg := config.NewDefaultConfig()
	cfg.EnableRuntimeCompiler = true
	cfg.AllowPrecompiledFallback = false
	cfg.EnableConntrack = true
	cfg.BPFDebug = true

	ct, err := NewEBPFConntracker(cfg)
	require.NoError(t, err)
	defer ct.Close()

	testutil.TestConntracker(t, net.ParseIP("1.1.1.1"), net.ParseIP("2.2.2.2"), ct)
}

func TestEBPFConntracker6(t *testing.T) {
	defer func() {
		teardown6(t)
	}()

	cmd := exec.Command("testdata/setup_dnat6.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("setup command output: %s", string(out))
	}

	cfg := config.NewDefaultConfig()
	cfg.EnableRuntimeCompiler = true
	cfg.AllowPrecompiledFallback = false
	cfg.EnableConntrack = true
	cfg.BPFDebug = true

	ct, err := NewEBPFConntracker(cfg)
	require.NoError(t, err)
	defer ct.Close()

	testutil.TestConntracker(t, net.ParseIP("fd00::1"), net.ParseIP("fd00::2"), ct)
}

func teardown(t *testing.T) {
	cmd := exec.Command("testdata/teardown_dnat.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("teardown command output: %s", string(out))
		t.Errorf("error tearing down: %s", err)
	}
}

func teardown6(t *testing.T) {
	cmd := exec.Command("testdata/teardown_dnat6.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("teardown command output: %s", string(out))
		t.Errorf("error tearing down: %s", err)
	}
}
