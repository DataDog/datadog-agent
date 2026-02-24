// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !linux && !windows

package net

import (
	"errors"
	"net/http"

	model "github.com/DataDog/agent-payload/v5/process"
)

// GetProcStats returns a set of process stats by querying system-probe
func GetProcStats(_ *http.Client, _ []int32) (*model.ProcStatsWithPermByPID, error) {
	return nil, errors.New("unsupported platform")
}
