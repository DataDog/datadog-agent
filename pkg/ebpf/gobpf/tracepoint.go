package gobpf

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/ebpf/probe/types"
)

func (m *Module) RegisterTracepoint(tp *types.Tracepoint) error {
	if err := m.EnableTracepoint(tp.Name); err != nil {
		return fmt.Errorf("failed to load tracepoint %v: %s", tp.Name, err)
	}

	return nil
}

func (m *Module) UnregisterTracepoint(tp *types.Tracepoint) error {
	return nil
}
