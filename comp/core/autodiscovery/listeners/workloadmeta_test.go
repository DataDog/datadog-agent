// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build !serverless

package listeners

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type wlmListenerSvc struct {
	service Service
	parent  string
}

type testWorkloadmetaListener struct {
	t        *testing.T
	store    workloadmeta.Component
	services map[string]wlmListenerSvc
}

// Listen is not implemented
func (l *testWorkloadmetaListener) Listen(_ chan<- Service, _ chan<- Service) {
	panic("not implemented")
}

// Stop is not implemented
func (l *testWorkloadmetaListener) Stop() {
	panic("not implemented")
}

// Store returns the workloadmeta store
func (l *testWorkloadmetaListener) Store() workloadmeta.Component {
	return l.store
}

// AddService adds a service
func (l *testWorkloadmetaListener) AddService(svcID string, svc Service, parentSvcID string) {
	l.services[svcID] = wlmListenerSvc{
		service: svc,
		parent:  parentSvcID,
	}
}

func (l *testWorkloadmetaListener) assertServices(expectedServices map[string]wlmListenerSvc) {
	for svcID, expectedSvc := range expectedServices {
		actualSvc, ok := l.services[svcID]
		if !ok {
			l.t.Errorf("expected to find service %q, but it was not generated", svcID)
			continue
		}

		if diff := cmp.Diff(expectedSvc, actualSvc,
			cmp.AllowUnexported(wlmListenerSvc{}, WorkloadService{}),
			cmpopts.IgnoreFields(WorkloadService{}, "tagger", "wmeta")); diff != "" {
			l.t.Errorf("service %q mismatch (-want +got):\n%s", svcID, diff)
		}

		// Compare filter values
		filters := []workloadfilter.Scope{
			workloadfilter.GlobalFilter,
			workloadfilter.LogsFilter,
			workloadfilter.MetricsFilter,
		}

		for _, filter := range filters {
			expectedHasFilter := expectedSvc.service.HasFilter(filter)
			actualHasFilter := actualSvc.service.HasFilter(filter)
			if expectedHasFilter != actualHasFilter {
				l.t.Errorf("service %q %s mismatch: want %v, got %v",
					svcID, filter, expectedHasFilter, actualHasFilter)
			}
		}

		delete(l.services, svcID)
	}

	if len(l.services) > 0 {
		var serviceDetails []string
		for svcID, svc := range l.services {
			detail := fmt.Sprintf("ID: %s, Parent: %q, Service: %s", svcID, svc.parent, svc.service)
			serviceDetails = append(serviceDetails, detail)
		}
		l.t.Errorf("got unexpected services:\n%s", strings.Join(serviceDetails, "\n"))
	}
}

func newTestWorkloadmetaListener(t *testing.T) *testWorkloadmetaListener {
	w := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Supply(config.Params{}),
		fx.Supply(log.Params{}),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component { return config.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	return &testWorkloadmetaListener{
		t:        t,
		store:    w,
		services: make(map[string]wlmListenerSvc),
	}
}

func TestProcessSetEntity(t *testing.T) {
	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "pod1",
		},
	}
	svcID := "kubernetes_pod://pod1"

	tests := []struct {
		name            string
		isReadyFn       func(workloadmeta.Entity) bool
		maxWait         time.Duration
		expectProcessed bool
	}{
		{
			name:            "no readiness function",
			isReadyFn:       nil,
			expectProcessed: true,
		},
		{
			name:            "ready",
			isReadyFn:       func(workloadmeta.Entity) bool { return true },
			expectProcessed: true,
		},
		{
			name:            "not ready within max wait",
			isReadyFn:       func(workloadmeta.Entity) bool { return false },
			maxWait:         10 * time.Second,
			expectProcessed: false,
		},
		{
			name:            "readiness function set but feature disabled (max wait zero)",
			isReadyFn:       func(workloadmeta.Entity) bool { return false },
			maxWait:         0,
			expectProcessed: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			processed := false
			listener := newTestListenerWithWait(t,
				func(workloadmeta.Entity) { processed = true },
				test.isReadyFn,
				test.maxWait,
			)

			listener.processSetEntity(pod)

			assert.Equal(t, test.expectProcessed, processed)
			if !test.expectProcessed {
				assert.Contains(t, listener.pendingEntities, svcID)
			} else if listener.pendingEntities != nil {
				assert.NotContains(t, listener.pendingEntities, svcID)
			}
		})
	}
}

func newTestListenerWithWait(
	t *testing.T,
	processFn func(workloadmeta.Entity),
	isReadyFn func(workloadmeta.Entity) bool,
	maxWait time.Duration,
) *workloadmetaListenerImpl {
	wmetaMock := fxutil.Test[workloadmetamock.Mock](
		t,
		fx.Options(
			core.MockBundle(),
			fx.Supply(context.TODO()),
			workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		),
	)

	listener, err := newWorkloadmetaListenerWithTagWait(
		"test-listener",
		nil,
		processFn,
		wmetaMock,
		nil,
		isReadyFn,
		maxWait,
	)
	require.NoError(t, err)

	return listener.(*workloadmetaListenerImpl)
}
