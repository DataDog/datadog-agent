// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subscribers

import (
	"context"
	"testing"

	"go.uber.org/fx"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	fxutil "github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestProcessEvents(t *testing.T) {
	tests := []struct {
		name                string
		events              workloadmeta.EventBundle
		expectedGPUDetected bool
	}{
		{
			name:                "no events",
			events:              workloadmeta.EventBundle{},
			expectedGPUDetected: false,
		},
		{
			name: "non GPU event",
			events: workloadmeta.EventBundle{
				Events: []workloadmeta.Event{
					{Entity: &workloadmeta.Container{}},
				},
			},
			expectedGPUDetected: false,
		},
		{
			name: "one GPU event",
			events: workloadmeta.EventBundle{
				Events: []workloadmeta.Event{
					{Entity: &workloadmeta.GPU{}},
				},
			},
			expectedGPUDetected: true,
		},
		{
			name: "multiple GPU events",
			events: workloadmeta.EventBundle{
				Events: []workloadmeta.Event{
					{Entity: &workloadmeta.GPU{}},
					{Entity: &workloadmeta.GPU{}},
				},
			},
			expectedGPUDetected: true,
		},
		{
			name: "False detection bool overwritten",
			events: workloadmeta.EventBundle{
				Events: []workloadmeta.Event{
					{Entity: &workloadmeta.Container{}},
					{Entity: &workloadmeta.GPU{}},
				},
			},
			expectedGPUDetected: true,
		},
		{
			name: "True detection bool not overwritten",
			events: workloadmeta.EventBundle{
				Events: []workloadmeta.Event{
					{Entity: &workloadmeta.GPU{}},
					{Entity: &workloadmeta.Container{}},
				},
			},
			expectedGPUDetected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockWmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				core.MockBundle(),
				fx.Supply(context.Background()),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			)).(workloadmetamock.Mock)

			gpuDetector := NewGPUSubscriber(mockWmeta)
			gpuDetector.processEvents(tt.events)

			assert.Equal(t, tt.expectedGPUDetected, gpuDetector.IsGPUDetected())
		})
	}
}
