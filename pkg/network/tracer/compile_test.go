// +build linux_bpf

package tracer

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/stretchr/testify/require"
)

func TestTracerCompile(t *testing.T) {
	cfg := config.NewDefaultConfig()
	cfg.BPFDebug = true
	cflags, err := getCFlags(cfg)
	require.NoError(t, err)
	_, err = runtime.Tracer.Compile(&cfg.Config, cflags)
	require.NoError(t, err)
}

func TestConntrackCompile(t *testing.T) {
	cfg := config.NewDefaultConfig()
	cfg.BPFDebug = true
	cflags, err := getCFlags(cfg)
	require.NoError(t, err)
	_, err = runtime.Conntrack.Compile(&cfg.Config, cflags)
	require.NoError(t, err)
}
