// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package discoverer

import (
	"github.com/DataDog/datadog-agent/pkg/collector/python"
)

// pythonBridge satisfies Bridge by delegating to pkg/collector/python.RunDiscover,
// which lazy-inits Python via the shared pythonOnce sync.Once on its first call.
type pythonBridge struct{}

// NewPythonBridge returns a Bridge backed by rtloader.
func NewPythonBridge() Bridge {
	return &pythonBridge{}
}

func (b *pythonBridge) RunDiscover(integrationName, serviceJSON string) (string, error) {
	return python.RunDiscover(integrationName, serviceJSON)
}
