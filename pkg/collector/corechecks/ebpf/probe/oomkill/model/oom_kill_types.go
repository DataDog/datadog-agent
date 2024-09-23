// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model is the types for the OOM Kill check
package model

// OOMKillStats contains the statistics of a given socket
type OOMKillStats struct {
	CgroupName  string `json:"cgroupName"`
	VictimPid   uint32 `json:"victimPid"`
	TriggerPid  uint32 `json:"triggerPid"`
	VictimComm  string `json:"victimComm"`
	TriggerComm string `json:"triggerComm"`
	Score       int64  `json:"score"`
	ScoreAdj    int16  `json:"scoreAdj"`
	Pages       uint64 `json:"pages"`
	MemCgOOM    uint32 `json:"memcgoom"`
}
