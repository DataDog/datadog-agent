// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !python

package discoverer

import "errors"

type nopythonBridge struct{}

// NewPythonBridge returns a ConfigDiscoverer that permanently fails on every
// call when the Agent is built without Python support.
func NewPythonBridge() ConfigDiscoverer {
	return &nopythonBridge{}
}

func (b *nopythonBridge) DiscoverConfig(_, _ string) (string, error) {
	return "", PermFail{Err: errors.New("python support is not available")}
}
