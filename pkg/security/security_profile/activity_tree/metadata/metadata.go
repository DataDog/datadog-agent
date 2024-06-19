// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package metadata holds metadata related files
package metadata

import "time"

// Metadata is used to provide context about the activity dump or the profile
type Metadata struct {
	AgentVersion      string `json:"agent_version"`
	AgentCommit       string `json:"agent_commit"`
	KernelVersion     string `json:"kernel_version"`
	LinuxDistribution string `json:"linux_distribution"`
	Arch              string `json:"arch"`

	Name              string    `json:"name"`
	ProtobufVersion   string    `json:"protobuf_version"`
	DifferentiateArgs bool      `json:"differentiate_args"`
	Comm              string    `json:"comm,omitempty"`
	ContainerID       string    `json:"-"`
	ContainerFlags    uint64    `json:"-"`
	Start             time.Time `json:"start"`
	End               time.Time `json:"end"`
	Size              uint64    `json:"activity_dump_size,omitempty"`
	Serialization     string    `json:"serialization,omitempty"`
}
