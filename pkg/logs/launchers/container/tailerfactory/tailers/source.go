// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet || docker

// Package tailers provides tailers for API logs
package tailers

import (
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// WrappedSource wraps a LogSource and adds/removes it to the logs-agent on
// start/stop.
type WrappedSource struct {
	// Source is the LogSource to add or remove
	Source *sources.LogSource

	// Sources is the container in which Source is added or removed.
	Sources *sources.LogSources
}

// Start implements Tailer#Start.
//
// It is called from the container launcher's loop. Adding the source
// synchronously is safe because the container launcher only subscribes to
// container source types, so Source (a file source) is not delivered back to
// it.
func (t *WrappedSource) Start() error {
	t.Sources.AddSource(t.Source)
	return nil
}

// Stop implements Tailer#Stop.
func (t *WrappedSource) Stop() {
	t.Sources.RemoveSource(t.Source)
}
