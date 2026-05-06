// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package discoverer

import (
	"github.com/DataDog/datadog-agent/pkg/collector/python"
)

// pythonBridge satisfies Bridge by delegating to pkg/collector/python.RunDiscover.
type pythonBridge struct{}

// NewPythonBridge returns a Bridge backed by rtloader. The Python runtime
// must be initialised (rtloader != nil) before any Discover call; if it
// isn't, the bridge returns the package's ErrNotInitialized.
func NewPythonBridge() Bridge {
	return &pythonBridge{}
}

func (b *pythonBridge) RunDiscover(integrationName, serviceJSON string) (string, error) {
	return python.RunDiscover(integrationName, serviceJSON)
}
