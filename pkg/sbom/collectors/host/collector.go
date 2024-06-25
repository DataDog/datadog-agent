// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy || (windows && wmi)

package host

import (
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
)

// Type returns the container image scan type
func (c *Collector) Type() collectors.ScanType {
	return collectors.HostScanType
}

// Channel returns the channel to send scan results
func (c *Collector) Channel() chan sbom.ScanResult {
	return c.resChan
}

// Options returns the collector options
func (c *Collector) Options() sbom.ScanOptions {
	return c.opts
}

// Shutdown shuts down the collector
func (c *Collector) Shutdown() {
	if c.resChan != nil && !c.closed {
		close(c.resChan)
	}
	c.closed = true
}

func init() {
	collectors.RegisterCollector(collectors.HostCollector, &Collector{
		resChan: make(chan sbom.ScanResult, channelSize),
	})
}
