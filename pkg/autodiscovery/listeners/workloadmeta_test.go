// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build !serverless

package listeners

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type wlmListenerSvc struct {
	service Service
	parent  string
}

type testWorkloadmetaListener struct {
	t       *testing.T
	filters *containerFilters
	// FIXME(components): these tests will likely remain broken until we actually
	//                    adopt the workloadmeta component mocks.
	store    workloadmeta.Component
	services map[string]wlmListenerSvc
}

func (l *testWorkloadmetaListener) Listen(newSvc chan<- Service, delSvc chan<- Service) {
	panic("not implemented")
}

func (l *testWorkloadmetaListener) Stop() {
	panic("not implemented")
}

// FIXME(components): these tests will likely remain broken until we actually
//
//	adopt the workloadmeta component mocks.
func (l *testWorkloadmetaListener) Store() workloadmeta.Component {
	return l.store
}

func (l *testWorkloadmetaListener) AddService(svcID string, svc Service, parentSvcID string) {
	l.services[svcID] = wlmListenerSvc{
		service: svc,
		parent:  parentSvcID,
	}
}

func (l *testWorkloadmetaListener) IsExcluded(ft containers.FilterType, annotations map[string]string, name string, image string, ns string) bool {
	return l.filters.IsExcluded(ft, annotations, name, image, ns)
}

func (l *testWorkloadmetaListener) assertServices(expectedServices map[string]wlmListenerSvc) {
	for svcID, expectedSvc := range expectedServices {
		actualSvc, ok := l.services[svcID]
		if !ok {
			l.t.Errorf("expected to find service %q, but it was not generated", svcID)
			continue
		}

		assert.Equal(l.t, expectedSvc, actualSvc)

		delete(l.services, svcID)
	}

	if len(l.services) > 0 {
		l.t.Errorf("got unexpected services: %+v", l.services)
	}
}

func newTestWorkloadmetaListener(t *testing.T) *testWorkloadmetaListener {
	filters, err := newContainerFilters()
	if err != nil {
		t.Fatalf("cannot initialize container filters: %s", err)
	}

	w := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle,
		workloadmeta.MockModule,
		fx.Supply(context.Background()),
	))

	return &testWorkloadmetaListener{
		t:       t,
		filters: filters,
		// FIXME(components): these tests will likely remain broken until we actually
		//                    adopt the workloadmeta component mocks.
		store:    w,
		services: make(map[string]wlmListenerSvc),
	}
}
