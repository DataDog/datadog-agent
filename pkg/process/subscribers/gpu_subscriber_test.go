// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subscribers

import (
	"context"
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	fxutil "github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestGPUDetection(t *testing.T) {
	tests := []struct {
		name                string
		events              []workloadmeta.CollectorEvent
		expectedGPUDetected bool
	}{
		{
			name:                "no events",
			events:              []workloadmeta.CollectorEvent{},
			expectedGPUDetected: false,
		},
		{
			name: "non GPU event",
			events: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.Container{
						EntityID: workloadmeta.EntityID{
							ID:   "123",
							Kind: workloadmeta.KindContainer,
						},
					},
				},
			},
			expectedGPUDetected: false,
		},
		{
			name: "one GPU event",
			events: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.GPU{
						EntityID: workloadmeta.EntityID{
							ID:   "1",
							Kind: workloadmeta.KindGPU,
						},
					},
				},
			},
			expectedGPUDetected: true,
		},
		{
			name: "multiple GPU events",
			events: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.GPU{
						EntityID: workloadmeta.EntityID{
							ID:   "1",
							Kind: workloadmeta.KindGPU,
						},
					},
				},
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.GPU{
						EntityID: workloadmeta.EntityID{
							ID:   "2",
							Kind: workloadmeta.KindGPU,
						},
					},
				},
			},
			expectedGPUDetected: true,
		},
		{
			name: "False detection bool overwritten",
			events: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.Container{
						EntityID: workloadmeta.EntityID{
							ID:   "123",
							Kind: workloadmeta.KindContainer,
						},
					},
				},
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.GPU{
						EntityID: workloadmeta.EntityID{
							ID:   "1",
							Kind: workloadmeta.KindGPU,
						},
					},
				},
			},
			expectedGPUDetected: true,
		},
		{
			name: "True detection bool not overwritten",
			events: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.GPU{
						EntityID: workloadmeta.EntityID{
							ID:   "1",
							Kind: workloadmeta.KindGPU,
						},
					},
				},
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.Container{
						EntityID: workloadmeta.EntityID{
							ID:   "123",
							Kind: workloadmeta.KindContainer,
						},
					},
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
			))

			gpuDetector := NewGPUSubscriber(mockWmeta)
			go gpuDetector.Run()
			defer gpuDetector.Stop()

			// Notify subscribers of events
			mockWmeta.Notify(tt.events)

			// Eventually, GPU detector should finish processing all events
			assert.Eventually(t, func() bool {
				return assert.Equal(t, tt.expectedGPUDetected, gpuDetector.IsGPUDetected())
			}, 1*time.Second, 100*time.Millisecond)
		})
	}
}
