/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profiling

import (
	"sync"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/profiler"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	mu      sync.RWMutex
	running bool
)

const (
	// ProfileURLTemplate constant template for expected profiling endpoint URL.
	ProfileURLTemplate = "https://intake.profile.%s/v1/input"
	// ProfileCoreService default service for the core agent profiler.
	ProfileCoreService = "datadog-agent"
	// ProfilingLocalURLTemplate is the constant used to compute the URL of the local trace agent
	ProfilingLocalURLTemplate = "http://%v/profiling/v1/input"
	// DefaultProfilingPeriod defines the default profiling period
	DefaultProfilingPeriod = 5 * time.Minute
)

// Start initiates profiling with the supplied parameters;
// this function is thread-safe.
func Start(site, env, service string, period time.Duration, cpuDuration time.Duration, mutexFraction, blockRate int, withGoroutine bool, tags ...string) error {
	mu.Lock()
	defer mu.Unlock()
	if running {
		return nil
	}

	types := []profiler.ProfileType{profiler.CPUProfile, profiler.HeapProfile}
	if withGoroutine {
		types = append(types, profiler.GoroutineProfile)
	}

	options := []profiler.Option{
		profiler.WithEnv(env),
		profiler.WithService(service),
		profiler.WithURL(site),
		profiler.WithPeriod(period),
		profiler.WithProfileTypes(types...),
		profiler.CPUDuration(cpuDuration),
		profiler.WithTags(tags...),
	}

	// If block or mutex profiling was configured via runtime configuration, pass current
	// values to profiler. This prevents profiler from resetting mutex profile rate to the
	// default value; and enables collection of blocking profile data if it is enabled.
	if mutexFraction > 0 {
		options = append(options, profiler.MutexProfileFraction(mutexFraction))
	}
	if blockRate > 0 {
		options = append(options, profiler.BlockProfileRate(blockRate))
	}

	err := profiler.Start(options...)

	if err == nil {
		running = true
		log.Debugf("Profiling started! Submitting to: %s", site)
	}

	return err
}

// Stop stops the profiler if running - idempotent; this function is thread-safe.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if running {
		profiler.Stop()
		running = false
	}
}
