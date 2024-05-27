// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package model holds the security profile data model
package model

// EventFilteringProfileState is used to compute metrics for the event filtering feature
type EventFilteringProfileState uint8

const (
	// NoProfile is used to count the events for which we didn't have a profile
	NoProfile EventFilteringProfileState = iota
	// ProfileAtMaxSize is used to count the events that didn't make it into a profile because their matching profile
	// reached the max size threshold
	ProfileAtMaxSize
	// UnstableEventType is used to count the events that didn't make it into a profile because their matching profile was
	// unstable for their event type
	UnstableEventType
	// StableEventType is used to count the events linked to a stable profile for their event type
	StableEventType
	// AutoLearning is used to count the event during the auto learning phase
	AutoLearning
	// WorkloadWarmup is used to count the learned events due to workload warm up time
	WorkloadWarmup
)

// AllEventFilteringProfileState is the list of all EventFilteringProfileState
var AllEventFilteringProfileState = []EventFilteringProfileState{NoProfile, ProfileAtMaxSize, UnstableEventType, StableEventType, AutoLearning, WorkloadWarmup}

// String returns the string representation of the EventFilteringProfileState
func (efr EventFilteringProfileState) String() string {
	switch efr {
	case NoProfile:
		return "no_profile"
	case ProfileAtMaxSize:
		return "profile_at_max_size"
	case UnstableEventType:
		return "unstable_event_type"
	case StableEventType:
		return "stable_event_type"
	case AutoLearning:
		return "auto_learning"
	case WorkloadWarmup:
		return "workload_warmup"
	}
	return ""
}

// ToTag returns the tag representation of the EventFilteringProfileState
func (efr EventFilteringProfileState) ToTag() string {
	return "profile_state:" + efr.String()
}
