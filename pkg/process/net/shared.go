// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package net

import model "github.com/DataDog/agent-payload/v5/process"

// SysProbeUtil fetches info from the SysProbe running remotely
type SysProbeUtil interface {
	GetConnections(clientID string) (*model.Connections, error)
	GetStats() (map[string]interface{}, error)
	GetProcStats(pids []int32) (*model.ProcStatsWithPermByPID, error)
	Register(clientID string) error
}
