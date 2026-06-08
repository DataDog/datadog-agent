// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package instrumentation

import (
	"testing"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestClassifySectionEvent(t *testing.T) {
	crWithChecks := &datadoghq.DatadogInstrumentation{
		Spec: datadoghq.DatadogInstrumentationSpec{
			Config: datadoghq.DatadogInstrumentationConfig{
				Checks: []datadoghq.DatadogInstrumentationCheckConfig{
					{Integration: "redisdb"},
				},
			},
		},
	}
	crEmpty := &datadoghq.DatadogInstrumentation{}

	// Use a handler that checks actual checks presence
	handler := &mockHandler{hasSectionFunc: func(cr *datadoghq.DatadogInstrumentation) bool {
		return cr != nil && len(cr.Spec.Config.Checks) > 0
	}}

	tests := []struct {
		name      string
		old       *datadoghq.DatadogInstrumentation
		new       *datadoghq.DatadogInstrumentation
		wantEvent EventType
		wantMatch bool
	}{
		{
			name:      "create: section added",
			old:       nil,
			new:       crWithChecks,
			wantEvent: EventCreate,
			wantMatch: true,
		},
		{
			name:      "update: section present in both",
			old:       crWithChecks,
			new:       crWithChecks,
			wantEvent: EventUpdate,
			wantMatch: true,
		},
		{
			name:      "delete: CR deleted",
			old:       crWithChecks,
			new:       nil,
			wantEvent: EventDelete,
			wantMatch: true,
		},
		{
			name:      "delete: section removed from spec",
			old:       crWithChecks,
			new:       crEmpty,
			wantEvent: EventDelete,
			wantMatch: true,
		},
		{
			name:      "create: section added to existing CR",
			old:       crEmpty,
			new:       crWithChecks,
			wantEvent: EventCreate,
			wantMatch: true,
		},
		{
			name:      "no-op: section absent in both",
			old:       crEmpty,
			new:       crEmpty,
			wantEvent: "",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventType, ok := classifySectionEvent(handler, tt.old, tt.new)
			assert.Equal(t, tt.wantMatch, ok)
			if ok {
				assert.Equal(t, tt.wantEvent, eventType)
			}
		})
	}
}
