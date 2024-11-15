// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"
)

func TestUtilizationMonitorLifecycle(_ *testing.T) {
	// The core logic of the UtilizationMonitor is tested in the utilizationtracker package.
	// This test just ensures the lifecycle methods don't block.
	um := NewTelemetryUtilizationMonitor("", "")
	um.Start()
	um.Stop()
	um.Cancel()
}
