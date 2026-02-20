// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procsubscribe

import (
	"context"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/pkg/dyninst/procsubscribe/procscan"
)

const DefaultScanInterval = defaultScanInterval

// WithScanner overrides the scanner used to discover processes.
func WithProcessScanner(scanner processScanner) Option {
	return optionFunc(func(c *config) {
		c.processScanner = scanner
	})
}

// ProcessScannerFunc is an implementation of processScanner that calls a function.
type ProcessScannerFunc func() ([]procscan.DiscoveredProcess, []procscan.ProcessID, error)

// Scan implements the ProcessScanner interface.
func (f ProcessScannerFunc) Scan() ([]procscan.DiscoveredProcess, []procscan.ProcessID, error) {
	return f()
}

// WithClock overrides the clock used for scheduling scans.
func WithClock(clk clock.Clock) Option {
	return optionFunc(func(c *config) { c.clk = clk })
}

func WithJitterFactor(factor float64) Option {
	return optionFunc(func(c *config) { c.jitterFactor = factor })
}

func WithWaitFunc(waitFunc func(ctx context.Context, duration time.Duration) error) Option {
	return optionFunc(func(c *config) { c.wait = waitFunc })
}
