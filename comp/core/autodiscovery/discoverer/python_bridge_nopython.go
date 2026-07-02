// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !python

package discoverer

// NewPythonBridge returns nil when the Agent is built without Python support.
// The nil ConfigDiscoverer causes autodiscovery to skip the Worker entirely
func NewPythonBridge() ConfigDiscoverer {
	return nil
}
