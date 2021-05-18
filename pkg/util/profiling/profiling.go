/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profiling

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"gopkg.in/DataDog/dd-trace-go.v1/profiler"
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

// Active returns a boolean indicating whether profiling is active or not;
// this function is thread-safe.
func Active() bool {
	mu.RLock()
	defer mu.RUnlock()

	return running
}

// Start initiates profiling with the supplied parameters;
// this function is thread-safe.
func Start(apiKey, site, env, service string, period time.Duration, cpuDuration time.Duration, tags ...string) error {
	if Active() {
		return nil
	}

	err := profiler.Start(
		profiler.WithAPIKey(apiKey),
		profiler.WithEnv(env),
		profiler.WithService(service),
		profiler.WithURL(site),
		profiler.WithPeriod(period),
		profiler.WithProfileTypes(profiler.CPUProfile, profiler.HeapProfile, profiler.MutexProfile),
		profiler.CPUDuration(cpuDuration),
		profiler.WithTags(tags...),
	)
	if err == nil {
		mu.Lock()
		defer mu.Unlock()

		log.Debugf("Profiling started! Submitting to: %s", site)
		running = true
	}

	return err
}

// Stop stops the profiler if running - idempotent; this function is thread-safe.
func Stop() {
	if Active() {
		mu.Lock()
		defer mu.Unlock()

		profiler.Stop()
		running = false
	}
}
