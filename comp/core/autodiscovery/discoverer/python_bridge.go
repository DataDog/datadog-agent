// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package discoverer

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/collector/python"
)

// pythonBridge satisfies Bridge by delegating to pkg/collector/python.RunDiscover.
type pythonBridge struct{}

// NewPythonBridge returns a Bridge backed by rtloader. The Python runtime
// must be initialised before any Discover call. When the runtime is not
// yet ready (early in agent startup, before rtloader.Initialize completes),
// RunDiscover surfaces ErrPythonNotReady so the discoverer can skip caching
// the failure and let the next AD reconcile event retry.
func NewPythonBridge() Bridge {
	return &pythonBridge{}
}

func (b *pythonBridge) RunDiscover(integrationName, serviceJSON string) (string, error) {
	res, err := python.RunDiscover(integrationName, serviceJSON)
	if errors.Is(err, python.ErrNotInitialized) {
		return "", ErrPythonNotReady
	}
	return res, err
}
