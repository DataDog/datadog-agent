// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	"sync"

	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

var (
	healthPlatformMu sync.RWMutex
	healthPlatform   healthplatformdef.Component
)

// SetHealthPlatform wires the health platform component for CGO callbacks (e.g. datadog_agent.report_issue, resolve_issue).
// Called in collector.go before InitPython / Initialize so Python checks can report issues once the interpreter starts.
func SetHealthPlatform(h healthplatformdef.Component) {
	healthPlatformMu.Lock()
	defer healthPlatformMu.Unlock()
	healthPlatform = h
}

func getHealthPlatform() healthplatformdef.Component {
	healthPlatformMu.Lock()
	defer healthPlatformMu.Unlock()
	return healthPlatform
}
