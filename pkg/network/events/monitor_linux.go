// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package events handles process events
package events

import (
	"fmt"
	"time"

	"go4.org/intern"

	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getProcessStartTime(ev *model.Event) time.Time {
	if ev.GetEventType() == model.ExecEventType {
		return ev.GetProcessExecTime()
	}
	if ev.GetEventType() == model.ForkEventType {
		return ev.GetProcessForkTime()
	}
	if ev.GetEventType() == model.TracerMemfdSealEventType {
		exec := ev.GetProcessExecTime()
		fork := ev.GetProcessForkTime()
		if exec.After(fork) {
			return exec
		}
		return fork
	}
	return time.Time{}
}

func getAPMTags(_ map[string]struct{}, _ string) []*intern.Value {
	return nil
}

// getTracerTags reads tracer metadata from a memfd file and extracts tags
func getTracerTags(pid uint32, fd uint32) []*intern.Value {
	fdPath := fmt.Sprintf("/proc/%d/fd/%d", pid, fd)

	metadata, err := tracermetadata.GetTracerMetadataFromPath(fdPath)
	if err != nil {
		log.Debugf("[memfd] Failed to read tracer metadata from %s: %v", fdPath, err)
		return nil
	}

	var tags []*intern.Value
	for tag := range metadata.GetTags() {
		tags = append(tags, intern.GetByString(tag))
	}

	log.Tracef("[memfd] Extracted %d tags from tracer metadata", len(tags))

	return tags
}

func handleTracerMemfdSeal(ev *model.Event, p *Process) *TracerMemfdSeal {
	return &TracerMemfdSeal{
		Fd:      ev.TracerMemfdSeal.Fd,
		Process: p,
	}
}
