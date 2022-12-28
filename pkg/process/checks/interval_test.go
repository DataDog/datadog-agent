// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// TestProcessDiscoveryInterval tests to make sure that the process discovery interval validation works properly
func TestProcessDiscoveryInterval(t *testing.T) {
	for _, tc := range []struct {
		name             string
		interval         time.Duration
		expectedInterval time.Duration
	}{
		{
			name:             "allowed interval",
			interval:         8 * time.Hour,
			expectedInterval: 8 * time.Hour,
		},
		{
			name:             "below minimum",
			interval:         0,
			expectedInterval: discoveryMinInterval,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Mock(t)
			cfg.Set("process_config.process_discovery.interval", tc.interval)

			assert.Equal(t, tc.expectedInterval, GetInterval(DiscoveryCheckName))
		})
	}
}
