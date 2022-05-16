// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import "time"

// ProcessEvent is a common interface for collected process events shared across multiple event listener implementations
type ProcessEvent struct {
	EventType      string    `json:"event_type"`
	CollectionTime time.Time `json:"collection_time"`
	Pid            uint32    `json:"pid"`
	Ppid           uint32    `json:"ppid"`
	UID            uint32    `json:"uid"`
	GID            uint32    `json:"gid"`
	Username       string    `json:"username"`
	Group          string    `json:"group"`
	Exe            string    `json:"exe"`
	Cmdline        []string  `json:"cmdline"`
	ForkTime       time.Time `json:"fork_time,omitempty"`
	ExecTime       time.Time `json:"exec_time,omitempty"`
	ExitTime       time.Time `json:"exit_time,omitempty"`
}
