// +build linux_bpf

package probe

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/stretchr/testify/require"
)

func TestProbeCompile(t *testing.T) {
	cfg := ebpf.NewDefaultConfig()
	var cflags []string
	_, err := runtime.RuntimeSecurity.Compile(cfg, cflags)
	require.NoError(t, err)
}
