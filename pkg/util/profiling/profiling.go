// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profiling

import (
	"sync"

	"gopkg.in/DataDog/dd-trace-go.v1/profiler"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	mu      sync.RWMutex
	running bool
)

const (
	// ProfilingURLTemplate constant template for expected profiling endpoint URL.
	ProfilingURLTemplate = "https://intake.profile.%s/v1/input"
	// ProfilingLocalURLTemplate is the constant used to compute the URL of the local trace agent
	ProfilingLocalURLTemplate = "http://%v/profiling/v1/input"
)

// Start initiates profiling with the supplied parameters;
// this function is thread-safe.
func Start(settings Settings) error {
	mu.Lock()
	defer mu.Unlock()
	if running {
		return nil
	}

	settings.applyDefaults()

	types := []profiler.ProfileType{profiler.CPUProfile, profiler.HeapProfile}
	if settings.WithGoroutineProfile {
		types = append(types, profiler.GoroutineProfile)
	}

	options := []profiler.Option{
		profiler.WithURL(settings.ProfilingURL),
		profiler.WithEnv(settings.Env),
		profiler.WithService(settings.Service),
		profiler.WithPeriod(settings.Period),
		profiler.WithProfileTypes(types...),
		profiler.CPUDuration(settings.CPUDuration),
		profiler.WithDeltaProfiles(settings.WithDeltaProfiles),
		profiler.WithTags(settings.Tags...),
		profiler.WithAPIKey(""), // to silence the error log about `DD_API_KEY`
	}

	if settings.Socket != "" {
		options = append(options, profiler.WithUDS(settings.Socket))
	}

	// If block or mutex profiling was configured via runtime configuration, pass current
	// values to profiler. This prevents profiler from resetting mutex profile rate to the
	// default value; and enables collection of blocking profile data if it is enabled.
	if settings.MutexProfileFraction > 0 {
		options = append(options, profiler.MutexProfileFraction(settings.MutexProfileFraction))
	}
	if settings.BlockProfileRate > 0 {
		options = append(options, profiler.BlockProfileRate(settings.BlockProfileRate))
	}

	if len(settings.CustomAttributes) > 0 {
		customContextTags := make([]string, len(settings.CustomAttributes))
		for _, customAttribute := range settings.CustomAttributes {
			customContextTags = append(customContextTags, "ddprof.custom_ctx:"+customAttribute)
		}
		options = append(options, profiler.WithTags(customContextTags...))
	}

	err := profiler.Start(options...)

	if err == nil {
		running = true
		log.Debugf("Profiling started! Submitting to: %s", settings.ProfilingURL)
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

// IsRunning returns true if the profiler is running; this function is thread-safe.
func IsRunning() bool {
	mu.RLock()
	v := running
	mu.RUnlock()
	return v
}
