// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds profile related files
package securityprofile

import (
	adprotov1 "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// DefaultProfileName used as default profile name
const DefaultProfileName = "default"

// EventFilteringResult is used to compute metrics for the event filtering feature
type EventFilteringResult uint8

const (
	// NA not applicable for profil NoProfile and ProfileAtMaxSize state
	NA EventFilteringResult = iota
	// InProfile is used to count the events that matched a profile
	InProfile
	// NotInProfile is used to count the events that didn't match their profile
	NotInProfile
)

func (efr EventFilteringResult) toTag() string {
	switch efr {
	case NA:
		return ""
	case InProfile:
		return "in_profile:true"
	case NotInProfile:
		return "in_profile:false"
	}
	return ""
}

// ProtoToState converts a proto state to a profile one
func ProtoToState(eps adprotov1.EventProfileState) model.EventFilteringProfileState {
	switch eps {
	case adprotov1.EventProfileState_NO_PROFILE:
		return model.NoProfile
	case adprotov1.EventProfileState_PROFILE_AT_MAX_SIZE:
		return model.ProfileAtMaxSize
	case adprotov1.EventProfileState_UNSTABLE_PROFILE:
		return model.UnstableEventType
	case adprotov1.EventProfileState_STABLE_PROFILE:
		return model.StableEventType
	case adprotov1.EventProfileState_AUTO_LEARNING:
		return model.AutoLearning
	case adprotov1.EventProfileState_WORKLOAD_WARMUP:
		return model.WorkloadWarmup
	}
	return model.NoProfile
}

var (
	allEventFilteringResults = []EventFilteringResult{InProfile, NotInProfile, NA}
)

type eventFilteringEntry struct {
	eventType model.EventType
	state     model.EventFilteringProfileState
	result    EventFilteringResult
}
