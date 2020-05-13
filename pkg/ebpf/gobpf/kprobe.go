package gobpf

import (
	"fmt"
	"os"
	"strings"
	"syscall"

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
			return fmt.Errorf("failed to load Kretprobe %v: %s", k.ExitFunc, err)
		}
	}

	return nil
}

func (m *Module) UnregisterKprobe(k *types.KProbe) error {
	if k.EntryFunc != "" {
		funcName := strings.TrimPrefix(k.EntryFunc, "kprobe/")
		if err := disableKprobe("r" + funcName); err != nil {
			return fmt.Errorf("failed to unregister Kprobe %v: %s", k.EntryFunc, err)
		}
	}
	if k.ExitFunc != "" {
		funcName := strings.TrimPrefix(k.EntryFunc, "kretprobe/")
		if err := disableKprobe("r" + funcName); err != nil {
			return fmt.Errorf("failed to unregister Kprobe %v: %s", k.EntryFunc, err)
		}
	}

	return nil
}

func disableKprobe(eventName string) error {
	kprobeEventsFileName := "/sys/kernel/debug/tracing/kprobe_events"
	f, err := os.OpenFile(kprobeEventsFileName, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("cannot open kprobe_events: %v", err)
	}
	defer f.Close()
	cmd := fmt.Sprintf("-:%s\n", eventName)
	if _, err = f.WriteString(cmd); err != nil {
		pathErr, ok := err.(*os.PathError)
		if ok && pathErr.Err == syscall.ENOENT {
			// This can happen when for example two modules
			// use the same elf object and both call `Close()`.
			// The second will encounter the error as the
			// probe already has been cleared by the first.
			return nil
		} else {
			return fmt.Errorf("cannot write %q to kprobe_events: %v", cmd, err)
		}
	}
	return nil
}
