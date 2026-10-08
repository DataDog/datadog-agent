// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux && !darwin && !windows

package healthcheck

import (
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
)

// NewRemediationDispatcher uses event-only dispatch on platforms without rshell support.
func NewRemediationDispatcher(config configmodel.Reader, out chan<- event.Event, hostname string) RemediationDispatcher {
	if !config.GetBool("health_check_remediation.enabled") {
		return nil
	}
	return NewEventDispatcher(out, hostname)
}
