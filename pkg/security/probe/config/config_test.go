// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeSchedulingConfig(t *testing.T) {
	tests := []struct {
		name     string
		policy   string
		priority int
		wantErr  string
	}{
		{
			name:     "disabled by default",
			policy:   "",
			priority: 0,
		},
		{
			name:     "valid SCHED_FIFO",
			policy:   "SCHED_FIFO",
			priority: 10,
		},
		{
			name:     "valid SCHED_RR",
			policy:   "SCHED_RR",
			priority: 1,
		},
		{
			name:     "valid SCHED_FIFO max priority",
			policy:   "SCHED_FIFO",
			priority: 99,
		},
		{
			name:     "invalid policy string",
			policy:   "SCHED_OTHER",
			priority: 10,
			wantErr:  "invalid event_monitoring_config.event_stream.scheduling_policy",
		},
		{
			name:     "policy set without priority",
			policy:   "SCHED_FIFO",
			priority: 0,
			wantErr:  "event_monitoring_config.event_stream.scheduling_priority must be between 1 and 99",
		},
		{
			name:     "priority too high",
			policy:   "SCHED_FIFO",
			priority: 100,
			wantErr:  "event_monitoring_config.event_stream.scheduling_priority must be between 1 and 99",
		},
		{
			name:     "priority negative",
			policy:   "SCHED_FIFO",
			priority: -1,
			wantErr:  "event_monitoring_config.event_stream.scheduling_priority must be between 1 and 99",
		},
		{
			name:     "priority set without policy",
			policy:   "",
			priority: 10,
			wantErr:  "event_monitoring_config.event_stream.scheduling_priority is set but event_monitoring_config.event_stream.scheduling_policy is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{
				EventStreamSchedulingPolicy:   tt.policy,
				EventStreamSchedulingPriority: tt.priority,
			}
			err := c.sanitize()
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
