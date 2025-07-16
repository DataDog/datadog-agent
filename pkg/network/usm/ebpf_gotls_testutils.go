// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux_bpf && test

package usm

import (
	"errors"
	"testing"
	"time"
)

// SetGoTLSExcludeSelf sets the GoTLSExcludeSelf configuration.
func SetGoTLSExcludeSelf(value bool) error {
	if goTLSSpec.Instance == nil {
		return errors.New("GoTLS is not enabled")
	}

	goTLSSpec.Instance.(*goTLSProgram).cfg.GoTLSExcludeSelf = value
	return nil
}

// SetGoTLSPeriodicTerminatedProcessesScanInterval sets the interval for the periodic scan of terminated processes in GoTLS.
func SetGoTLSPeriodicTerminatedProcessesScanInterval(tb testing.TB, interval time.Duration) {
	originalValue := scanTerminatedProcessesInterval
	tb.Cleanup(func() {
		scanTerminatedProcessesInterval = originalValue
	})
	scanTerminatedProcessesInterval = interval
}
