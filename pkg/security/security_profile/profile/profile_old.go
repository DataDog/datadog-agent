// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package profile holds profile related files
package profile

import "github.com/DataDog/datadog-agent/pkg/security/secl/model"

// VersionContext holds the context of one version (defined by its image tag)
type VersionContext struct {
	FirstSeenNano uint64
	LastSeenNano  uint64

	EventTypeState map[model.EventType]*EventTypeState

	// Syscalls is the syscalls profile
	Syscalls []uint32

	// Tags defines the tags used to compute this profile, for each present profile versions
	Tags []string
}

// EventTypeState defines an event type state
type EventTypeState struct {
	LastAnomalyNano uint64
	State           model.EventFilteringProfileState
}
