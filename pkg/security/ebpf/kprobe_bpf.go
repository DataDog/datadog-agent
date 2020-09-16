// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package ebpf

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
)

const (
	// maxEnableRetry number of retry for resource busy fail
	maxEnableRetry = 3
)

func (m *Module) tryEnableKprobe(secName string) (err error) {
	for i := 0; i != maxEnableRetry; i++ {
		if err = m.EnableKprobe(secName, 512); err == nil {
			break
		}
		// not available, not a temporary error
		if strings.Contains(err.Error(), syscall.ENOENT.Error()) {
			break
		}
		time.Sleep(time.Second)
	}

	return err
}

// RegisterKprobe registers a Kprobe
func (m *Module) RegisterKprobe(k *KProbe) error {
	if k.EntryFunc != "" {
		if err := m.tryEnableKprobe(k.EntryFunc); err != nil {
			return fmt.Errorf("failed to load Kprobe %v: %s", k.EntryFunc, err)
		}
	}
	if k.ExitFunc != "" {
		if err := m.tryEnableKprobe(k.ExitFunc); err != nil {
			return fmt.Errorf("failed to load Kretprobe %v: %s", k.ExitFunc, err)
		}
	}

	return nil
}

// UnregisterKprobe unregisters a Kprobe
func (m *Module) UnregisterKprobe(k *KProbe) error {
	if k.EntryFunc != "" {
		kp := m.Kprobe(k.EntryFunc)
		if kp == nil {
			return fmt.Errorf("couldn't find kprobe %s with section %s", k.Name, k.EntryFunc)
		}
		if err := kp.Detach(); err != nil {
			return err
		}
	}
	if k.ExitFunc != "" {
		kp := m.Kprobe(k.ExitFunc)
		if kp == nil {
			return fmt.Errorf("couldn't find kretprobe %s with section %s", k.Name, k.ExitFunc)
		}
		if err := kp.Detach(); err != nil {
			return err
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
		} else {
			return fmt.Errorf("cannot write %q to kprobe_events: %v", cmd, err)
		}
	}
	return nil
}
