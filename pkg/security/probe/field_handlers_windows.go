// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package probe

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ResolveEventTime resolves the monolitic kernel event timestamp to an absolute time
func (fh *FieldHandlers) ResolveEventTime(ev *model.Event) time.Time {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	return ev.Timestamp
}

// ResolveContainerContext retrieve the ContainerContext of the event
func (fh *FieldHandlers) ResolveContainerContext(ev *model.Event) (*model.ContainerContext, bool) {
	return ev.ContainerContext, ev.ContainerContext != nil
}

// ResolveProcessArgv resolves the args of the event as an array
func (fh *FieldHandlers) ResolveProcessArgv(ev *model.Event, process *model.Process) []string {
	return process.Argv
}
