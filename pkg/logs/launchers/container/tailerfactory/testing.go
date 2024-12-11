// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package tailerfactory

import "errors"

// TestTailer is a dummy implementation of tailerfactory.Tailer.
type TestTailer struct {
	Name       string // name of this tailer
	StartError bool   // should Start return an error?
	Started    bool   // has Start been called?
	Stopped    bool   // has Stop been called?
}

// Start implements tailerfactory.Tailer#Start.
func (tt *TestTailer) Start() error {
	tt.Started = true
	if tt.StartError {
		return errors.New("uhoh")
	}
	return nil
}

// Stop implements tailerfactory.Tailer#Stop.
func (tt *TestTailer) Stop() {
	tt.Stopped = true
}
