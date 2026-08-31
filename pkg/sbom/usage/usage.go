// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package usage carries the host runtime usage enrichment ("package in use")
// from the remote SBOM collector, which receives it from system-probe, to the
// sbom check, which merges it onto the host SBOM it sends to the back end.
//
// system-probe serves a single SBOM stream, so the host overlay leaves the
// workloadmeta collector through this channel rather than through a second
// stream or a workloadmeta entity, of which there is none for the host. The
// channel is a process-wide singleton like the scanner registry in
// pkg/sbom/collectors.
package usage

import (
	"sync"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
)

var (
	mu       sync.Mutex
	overlays = make(chan *cyclonedx_v1_4.Bom, 1)
)

// PublishHostOverlay hands the latest host overlay to the sbom check. Only the
// latest one matters, so a pending overlay is replaced rather than queued, and
// the call never blocks whether or not anyone is reading.
func PublishHostOverlay(bom *cyclonedx_v1_4.Bom) {
	mu.Lock()
	defer mu.Unlock()

	for {
		select {
		case overlays <- bom:
			return
		default:
			// Drop the pending overlay and retry. The receive can lose the race
			// against a reader taking the same value, hence the loop.
			select {
			case <-overlays:
			default:
			}
		}
	}
}

// HostOverlays returns the channel the host overlays arrive on.
func HostOverlays() <-chan *cyclonedx_v1_4.Bom {
	return overlays
}
