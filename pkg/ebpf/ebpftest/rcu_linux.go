// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ebpftest

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// rcu_normal takes precedence over rcu_expedited, so it has to be cleared first.
var rcuKnobs = []struct {
	path      string
	expedited string
}{
	{path: "/sys/kernel/rcu_normal", expedited: "0"},
	{path: "/sys/kernel/rcu_expedited", expedited: "1"},
}

// ExpediteRCU switches the kernel to expedited RCU grace periods and returns a function restoring
// the previous setting.
//
// Detaching a kprobe waits for an RCU grace period, and a test suite detaches a few hundred probes
// every time it rebuilds its eBPF manager: probe detach drops from 7.11s to 0.69s per rebuild on
// kernel 5.15, and from 34.28s to 20.02s on 6.12. Expediting trades IPI noise and power for that,
// which is the right trade only while tests run, hence the restore function.
//
// Knobs the kernel does not expose are skipped. Writing them needs root and a writable sysfs, so on
// error the already-applied knobs are rolled back and the returned function is a no-op.
func ExpediteRCU() (func(), error) {
	var applied []func() error

	restore := func() {
		for i := len(applied) - 1; i >= 0; i-- {
			if err := applied[i](); err != nil {
				fmt.Fprintf(os.Stderr, "failed to restore RCU knob: %v\n", err)
			}
		}
	}

	for _, knob := range rcuKnobs {
		previous, err := os.ReadFile(knob.path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			restore()
			return func() {}, fmt.Errorf("read %s: %w", knob.path, err)
		}

		if err := writeRCUKnob(knob.path, knob.expedited); err != nil {
			restore()
			return func() {}, err
		}

		previousValue := strings.TrimSpace(string(previous))
		applied = append(applied, func() error {
			return writeRCUKnob(knob.path, previousValue)
		})
	}

	return restore, nil
}

func writeRCUKnob(path, value string) error {
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		return fmt.Errorf("write %s to %s: %w", value, path, err)
	}
	return nil
}
