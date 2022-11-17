// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows
// +build linux windows

package probe

import (
	"sort"
	"strings"

	pconfig "github.com/DataDog/datadog-agent/pkg/process/config"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	// ServiceEnvVar environment variable used to report service
	ServiceEnvVar = "DD_SERVICE"
)

var eventZero Event

// Model describes the data model for the runtime security agent probe events
type Model struct {
	model.Model
	probe *Probe
}

// NewEvent returns a new Event
func (m *Model) NewEvent() eval.Event {
	return &Event{}
}

// Retain the event
func (ev *Event) Retain() Event {
	if ev.ProcessCacheEntry != nil {
		ev.ProcessCacheEntry.Retain()
	}
	return *ev
}

// Release the event
func (ev *Event) Release() {
	if ev.ProcessCacheEntry != nil {
		ev.ProcessCacheEntry.Release()
	}
}

// GetPathResolutionError returns the path resolution error as a string if there is one
func (ev *Event) GetPathResolutionError() error {
	return ev.pathResolutionError
}

func bestGuessServiceTag(serviceValues []string) string {
	if len(serviceValues) == 0 {
		return ""
	}

	firstGuess := serviceValues[0]

	// first we sort base on len, biggest len first
	sort.Slice(serviceValues, func(i, j int) bool {
		return len(serviceValues[j]) < len(serviceValues[i]) // reverse
	})

	// we then compare [i] and [i + 1] to check if [i + 1] is a prefix of [i]
	for i := 0; i < len(serviceValues)-1; i++ {
		if !strings.HasPrefix(serviceValues[i], serviceValues[i+1]) {
			// if it's not a prefix it means we have multiple disjoints services
			// we then return the first guess, closest in the process tree
			return firstGuess
		}
	}

	// we have a prefix chain, let's return the biggest one
	return serviceValues[0]
}

// NewEvent returns a new event
func NewEvent(resolvers *Resolvers, scrubber *pconfig.DataScrubber, probe *Probe) *Event {
	return &Event{
		Event:     model.Event{},
		resolvers: resolvers,
		scrubber:  scrubber,
		probe:     probe,
	}
}
