// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
)

// CheckForUpdates is a test helper that calls the private method
// checkForUpdates.
func (c *Controller) CheckForUpdates() {
	c.checkForUpdates()
}

// HandleRemovals is a test helper that calls the private method
// handleRemovals.
func (c *Controller) HandleRemovals(removals []procmon.ProcessID) {
	c.handleRemovals(removals)
}
