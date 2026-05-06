// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !python

package discoverer

import "errors"

// pythonBridgeStub satisfies Bridge for builds without the python tag (e.g. the
// cluster agent). It always errors; templates with Discovery set will be
// skipped when the agent is built without Python support.
type pythonBridgeStub struct{}

// NewPythonBridge returns a no-op Bridge for non-Python builds. Calls to
// RunDiscover return an error.
func NewPythonBridge() Bridge {
	return &pythonBridgeStub{}
}

func (b *pythonBridgeStub) RunDiscover(string, string) (string, error) {
	return "", errors.New("python bridge: agent built without Python support")
}
