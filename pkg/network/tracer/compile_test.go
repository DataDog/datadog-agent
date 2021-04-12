// +build linux_bpf

package tracer

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/stretchr/testify/require"
)

func TestTracerCompile(t *testing.T) {
	cfg := config.New()
	cfg.BPFDebug = true
	cflags := getCFlags(cfg)
	_, err := runtime.Tracer.Compile(&cfg.Config, cflags)
	require.NoError(t, err)
}

func TestConntrackCompile(t *testing.T) {
	cfg := config.New()
	cfg.BPFDebug = true
	cflags := getCFlags(cfg)
	_, err := runtime.Conntrack.Compile(&cfg.Config, cflags)
	require.NoError(t, err)
}
