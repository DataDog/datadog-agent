// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package noopimpl provides a no-op recorder for the testbench.
package noopimpl

import (
	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
)

// NewComponent returns a recorder Component that does nothing.
func NewComponent() recorder.Component {
	return &noopRecorder{}
}

type noopRecorder struct{}

func (n *noopRecorder) GetHandle(handleFunc observer.HandleFunc) observer.HandleFunc {
	return handleFunc
}
