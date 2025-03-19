// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds profile related files
package securityprofile

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

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

var (
	allEventFilteringResults = []EventFilteringResult{InProfile, NotInProfile, NA}
)

type eventFilteringEntry struct {
	eventType model.EventType
	state     model.EventFilteringProfileState
	result    EventFilteringResult
}
