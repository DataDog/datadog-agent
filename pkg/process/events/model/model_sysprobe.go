// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate go run github.com/tinylib/msgp -tests=false

package model

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ProcessMonitoringEvent is an event sent by the ProcessMonitoring handler in the runtime-security module
type ProcessMonitoringEvent struct {
	*model.ProcessCacheEntry
	EventType      string    `json:"EventType" msg:"evt_type"`
	CollectionTime time.Time `json:"CollectionTime" msg:"collection_time"`
	ExitCode       uint32    `json:"ExitCode" msg:"exit_code"`
}
