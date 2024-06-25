// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npcollectorimpl

import model "github.com/DataDog/agent-payload/v5/process"

func shouldScheduleNetworkPathForConn(conn *model.Connection) bool {
	if conn == nil || conn.Direction != model.ConnectionDirection_outgoing {
		return false
	}
	return conn.Family == model.ConnectionFamily_v4
}
