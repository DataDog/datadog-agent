// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profiling

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestProfiling(t *testing.T) {
	settings := Settings{
		ProfilingURL:         "https://nowhere.testing.dev",
		Env:                  "testing",
		Service:              "test-agent",
		Period:               time.Minute,
		CPUDuration:          15 * time.Second,
		MutexProfileFraction: 0,
		BlockProfileRate:     0,
		WithGoroutineProfile: false,
		WithDeltaProfiles:    false,
		Tags:                 []string{"1.0.0"},
	}
	err := Start(settings)
	assert.NoError(t, err)

	Stop()
}
