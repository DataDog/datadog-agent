package gobpf

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/ebpf/probe/types"
)

func (m *Module) RegisterKprobe(k *types.KProbe) error {
	if k.EntryFunc != "" {
		if err := m.EnableKprobe(k.EntryFunc, 0); err != nil {
			return fmt.Errorf("failed to load Kprobe %v: %s", k.EntryFunc, err)
		}
	}
	if k.ExitFunc != "" {
		if err := m.EnableKprobe(k.ExitFunc, 0); err != nil {
			return fmt.Errorf("failed to load Kprobe %v: %s", k.ExitFunc, err)
		}
	}

	return nil
}
