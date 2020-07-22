// +build linux_bpf

package ebpf

import (
	"fmt"
)

// RegisterTracepoint registers a kernel tracepoint
func (m *Module) RegisterTracepoint(name string) error {
	if err := m.EnableTracepoint(name); err != nil {
		return fmt.Errorf("failed to load tracepoint %v: %s", name, err)
	}

	return nil
}
