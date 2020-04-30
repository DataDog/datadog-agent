/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package profiling

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"gopkg.in/DataDog/dd-trace-go.v1/profiler"
)

var (
	mu      sync.RWMutex
	running bool
)

const (
	ProfileURLTemplate = "https://intake.profile.%s/v1/input"
)

func Active() bool {
	mu.RLock()
	defer mu.RUnlock()

	return running
}

func Start(api, site, env, service string, tags ...string) error {
	if Active() {
		return nil
	}

	err := profiler.Start(
		profiler.WithAPIKey(api),
		profiler.WithEnv(env),
		profiler.WithService(service),
		profiler.WithURL(site),
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

func Stop() {
	if Active() {
		mu.Lock()
		defer mu.Unlock()

		profiler.Stop()
		running = false
	}
}
