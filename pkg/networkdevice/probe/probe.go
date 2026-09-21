// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package probe probes a batch of network addresses with the configured probes.
package probe

import (
	"context"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/pingprobe"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/snmpprobe"
)

// Options is what to probe. A nil field is a probe that does not run.
type Options struct {
	Ping *pingprobe.Options
	SNMP *snmpprobe.Options
}

// Result is what every configured probe learned about one address.
type Result struct {
	Target string
	Ping   *pingprobe.Reading
	SNMP   *snmpprobe.Reading
}

// Empty reports whether no probe is configured.
func (o Options) Empty() bool {
	return o.Ping == nil && o.SNMP == nil
}

// Fingerprints is one stable digest per configured probe, in probe order.
func (o Options) Fingerprints() []string {
	var fps []string
	if o.Ping != nil {
		fps = append(fps, o.Ping.Fingerprint())
	}
	if o.SNMP != nil {
		fps = append(fps, o.SNMP.Fingerprint())
	}
	return fps
}

// Scan probes every target with every configured probe. It returns an error
// only when its context is done, alongside the results it already collected.
func Scan(ctx context.Context, workers int, targets []string, opts Options) ([]Result, error) {
	return scan(ctx, workers, targets, func(ctx context.Context, target string) Result {
		return scanOne(ctx, target, opts)
	})
}

// scanOne probes one address, ping first because it is the cheaper probe.
func scanOne(ctx context.Context, target string, opts Options) Result {
	res := Result{Target: target}
	if opts.Ping != nil {
		res.Ping = pingprobe.Run(target, *opts.Ping)
	}
	if ctx.Err() != nil {
		return res
	}
	if opts.SNMP != nil {
		res.SNMP = snmpprobe.Run(ctx, target, *opts.SNMP)
	}
	return res
}

// scan is the bounded fan-out. It is separate from Scan so that the pool is
// tested without a network.
func scan(ctx context.Context, workers int, targets []string, run func(context.Context, string) Result) ([]Result, error) {
	if workers < 1 {
		workers = 1
	}

	results := make([]Result, len(targets))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for i, target := range targets {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return collected(results), ctx.Err()
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = run(ctx, target)
		}()
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return collected(results), err
	}
	return results, nil
}

// collected drops the targets a cancelled scan never reached.
func collected(results []Result) []Result {
	out := make([]Result, 0, len(results))
	for _, r := range results {
		if r.Target == "" {
			continue
		}
		out = append(out, r)
	}
	return out
}
