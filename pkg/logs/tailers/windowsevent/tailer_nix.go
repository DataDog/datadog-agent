// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package windowsevent

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Start does not do much
func (t *Tailer) Start() {
	log.Warn("windows event log not supported on this system")
	go t.tail()
}

// Stop stops the tailer
func (t *Tailer) Stop() {
	t.stop <- struct{}{}
	<-t.done
}

// tail does nothing
func (t *Tailer) tail() {
	<-t.stop
	t.done <- struct{}{}
}
