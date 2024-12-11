// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package module

// Reloader aims to handle policies reloading triggers
type Reloader struct {
	reloadChan chan struct{}
}

// NewReloader returns a new Reloader
func NewReloader() *Reloader {
	return &Reloader{
		reloadChan: make(chan struct{}, 1),
	}
}

// Start start the reloader
func (r *Reloader) Start() error {
	return nil
}

// Chan returns the chan of reload events
func (r *Reloader) Chan() <-chan struct{} {
	return r.reloadChan
}

// Stop the Reloader
func (r *Reloader) Stop() {
	close(r.reloadChan)
}
