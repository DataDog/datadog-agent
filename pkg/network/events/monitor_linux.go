// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package events handles process events
package events

import (
	"time"

	"go4.org/intern"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func getProcessStartTime(ev *model.Event) time.Time {
	if ev.GetEventType() == model.ExecEventType {
		return ev.GetProcessExecTime()
	}
	if ev.GetEventType() == model.ForkEventType {
		return ev.GetProcessForkTime()
	}
	return time.Time{}
}

func getAPMTags(_ string) []*intern.Value {
	return nil
}
