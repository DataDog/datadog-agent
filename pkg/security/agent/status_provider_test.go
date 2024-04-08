// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
)

func TestStatus(t *testing.T) {
	provider := statusProvider{
		agent: &RuntimeSecurityAgent{
			client:               nil,
			hostname:             "test",
			telemetry:            nil,
			storage:              nil,
			running:              atomic.NewBool(false),
			connected:            atomic.NewBool(false),
			eventReceived:        atomic.NewUint64(0),
			activityDumpReceived: atomic.NewUint64(0),
		},
	}

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			provider.JSON(false, stats)

			assert.NotEmpty(t, stats)
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := provider.Text(false, b)

			assert.NoError(t, err)

			assert.NotEmpty(t, b.String())
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := provider.HTML(false, b)

			assert.NoError(t, err)

			assert.Empty(t, b.String())
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}
