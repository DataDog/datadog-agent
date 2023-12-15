// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model is the types for the OOM Kill check
package model

// OOMKillStats contains the statistics of a given socket
type OOMKillStats struct {
	CgroupName string `json:"cgroupName"`
	Pid        uint32 `json:"pid"`
	TPid       uint32 `json:"tpid"`
	FComm      string `json:"fcomm"`
	TComm      string `json:"tcomm"`
	Pages      uint64 `json:"pages"`
	MemCgOOM   uint32 `json:"memcgoom"`
}
