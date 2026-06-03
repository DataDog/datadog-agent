// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sds wraps the Datadog Sensitive Data Scanner shared library and
// exposes a small scanner API (creation, reconfiguration and scanning). It is
// only fully functional when the Agent is compiled with the `sds` build tag and
// the shared library is available, otherwise a no-op mock is used.
package sds

import "sync"

var (
	defaultScanner     *Scanner
	defaultScannerOnce sync.Once
)

// DefaultScanner returns the process-wide default SDS scanner, lazily creating
// it on first use. It has to be configured through Reconfigure before it can
// scan events.
func DefaultScanner() *Scanner {
	defaultScannerOnce.Do(func() {
		defaultScanner = CreateScanner()
	})
	return defaultScanner
}

// Reconfigure reconfigures the default scanner with the given order.
func Reconfigure(order ReconfigureOrder) error {
	return DefaultScanner().Reconfigure(order)
}

// Scan scans the given event through the default scanner. It returns whether
// the event was mutated, the processed event (nil when not mutated) and an
// error if the scanner is not ready.
func Scan(event []byte) (bool, []byte, error) {
	return DefaultScanner().Scan(event)
}
