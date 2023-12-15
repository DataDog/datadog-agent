// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profiling

import (
	"fmt"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/profiler"
)

// Settings contains the settings for internal profiling, to be passed to Start().
type Settings struct {
	// Socket specifies a unix socket to which profiles will be sent.
	Socket string
	// ProfilingURL specifies the URL to which profiles will be sent in
	// agentless mode. This can be constructed from a site value with
	// ProfilingURLTemplate.
	ProfilingURL string
	// Env specifies the environment to which profiles should be registered.
	Env string
	// Service specifies the service name to attach to a profile.
	Service string
	// Period specifies the interval at which to collect profiles.
	Period time.Duration
	// CPUDuration specifies the length at which to collect CPU profiles.
	CPUDuration time.Duration
	// MutexProfileFraction, if set, turns on mutex profiles with rate
	// indicating the fraction of mutex contention events reported in the mutex
	// profile.
	MutexProfileFraction int
	// BlockProfileRate turns on block profiles with the given rate.
	BlockProfileRate int
	// WithGoroutineProfile additionally reports stack traces of all current goroutines
	WithGoroutineProfile bool
	// WithDeltaProfiles specifies if delta profiles are enabled
	WithDeltaProfiles bool
	// Tags are the additional tags to attach to profiles.
	Tags []string
	// CustomAttributes names of goroutine labels to use as custom attributes in Datadog Profiling UI
	CustomAttributes []string
}

func (settings *Settings) String() string {
	return fmt.Sprintf("[Socket:%q][Target:%q][Env:%q][Period:%s][CPU:%s][Mutex:%d][Block:%d][Routines:%v][DeltaProfiles:%v]",
		settings.Socket,
		settings.ProfilingURL,
		settings.Env,
		settings.Period,
		settings.CPUDuration,
		settings.MutexProfileFraction,
		settings.BlockProfileRate,
		settings.WithGoroutineProfile,
		settings.WithDeltaProfiles,
	)
}

// Apply default value for a struct created using struct-literal notation
func (settings *Settings) applyDefaults() {
	if settings.CPUDuration == 0 {
		settings.CPUDuration = profiler.DefaultDuration
	}

	if settings.Tags == nil {
		settings.Tags = []string{}
	}
	if settings.CustomAttributes == nil {
		settings.CustomAttributes = []string{}
	}
}
