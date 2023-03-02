// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"net/http"

	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
)

var (
	// MemoryMonitor is the global system-probe memory monitor
	MemoryMonitor *utils.MemoryMonitor

	// ExpvarServer is the global expvar server
	ExpvarServer *http.Server

	// MainCtxCancel cancels the main system-probe context
	MainCtxCancel context.CancelFunc
)
