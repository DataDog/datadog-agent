// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package instrumentation

import (
	"errors"
	"testing"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"
)

// fakeLister creates a cache.GenericLister backed by the given CRs.
func fakeLister(crs ...*datadoghq.DatadogInstrumentation) cache.GenericLister {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
		cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
	})
	for _, cr := range crs {
		unstrObj := &unstructured.Unstructured{}
		_ = UnstructuredFromDatadogInstrumentation(cr, unstrObj)
		unstrObj.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   datadoghq.GroupVersion.Group,
			Version: datadoghq.GroupVersion.Version,
			Kind:    "DatadogInstrumentation",
		})
		_ = indexer.Add(unstrObj)
	}
	return cache.NewGenericLister(indexer, schema.GroupResource{
		Group:    datadoghq.GroupVersion.Group,
		Resource: "datadoginstrumentations",
	})
}

func TestReconcile(t *testing.T) {
	checksPresent := func(cr *datadoghq.DatadogInstrumentation) bool {
		return cr != nil && len(cr.Spec.Config.Checks) > 0
	}

	crWith := newTestCR("test-ddi", "default", 1, defaultChecks())
	crWithout := newTestCR("test-ddi", "default", 1, nil)

	tests := []struct {
		name          string
		current       *datadoghq.DatadogInstrumentation // CR in the lister cache (nil = not found)
		lastSeen      *datadoghq.DatadogInstrumentation // CR in the lastSeen store
		wantEventType EventType
		wantCallCount int
		wantCR        *datadoghq.DatadogInstrumentation
		handlerErr    error
		wantReconcErr bool
	}{
		{
			name:          "add event dispatches EventCreate",
			current:       crWith,
			lastSeen:      nil,
			wantEventType: EventCreate,
			wantCallCount: 1,
			wantCR:        crWith,
		},
		{
			name:          "update event dispatches EventUpdate",
			current:       crWith,
			lastSeen:      crWith,
			wantEventType: EventUpdate,
			wantCallCount: 1,
			wantCR:        crWith,
		},
		{
			name:          "delete event (CR gone) dispatches EventDelete with last seen CR",
			current:       nil,
			lastSeen:      crWith,
			wantEventType: EventDelete,
			wantCallCount: 1,
			wantCR:        crWith,
		},
		{
			name:          "delete event (section removed) dispatches EventDelete",
			current:       crWithout,
			lastSeen:      crWith,
			wantEventType: EventDelete,
			wantCallCount: 1,
			wantCR:        crWith,
		},
		{
			name:          "no section in either last seen or current results in no handler call",
			current:       crWithout,
			lastSeen:      crWithout,
			wantCallCount: 0,
		},
		{
			name:          "both nil results in no handler call",
			current:       nil,
			lastSeen:      nil,
			wantCallCount: 0,
		},
		{
			name:          "handler error propagates",
			current:       crWith,
			lastSeen:      nil,
			wantEventType: EventCreate,
			wantCallCount: 1,
			wantCR:        crWith,
			handlerErr:    errors.New("handler failed"),
			wantReconcErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &mockHandler{
				hasSectionFunc: checksPresent,
				conditionType:  "ChecksReady",
				handleErr:      tt.handlerErr,
			}

			var lister cache.GenericLister
			if tt.current != nil {
				lister = fakeLister(tt.current)
			} else {
				lister = fakeLister()
			}

			c := &Controller{
				lister:   lister,
				handlers: []Handler{handler},
				isLeader: func() bool { return false },
				lastSeen: make(map[string]*datadoghq.DatadogInstrumentation),
			}
			if tt.lastSeen != nil {
				key, _ := cache.MetaNamespaceKeyFunc(tt.lastSeen)
				c.lastSeen[key] = tt.lastSeen.DeepCopy()
			}

			err := c.reconcile(t.Context(), "default/test-ddi")
			if tt.wantReconcErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			calls := handler.getCalls()
			assert.Len(t, calls, tt.wantCallCount)
			if tt.wantCallCount > 0 {
				assert.Equal(t, tt.wantEventType, calls[0].eventType)
				assert.Equal(t, tt.wantCR.Name, calls[0].cr.Name)
			}
		})
	}
}

func TestReconcileMultipleHandlers(t *testing.T) {
	crWith := newTestCR("test-ddi", "default", 1, defaultChecks())

	handlerA := &mockHandler{
		name:           "handler-a",
		hasSectionFunc: func(cr *datadoghq.DatadogInstrumentation) bool { return len(cr.Spec.Config.Checks) > 0 },
		conditionType:  "AReady",
	}
	handlerB := &mockHandler{
		name:           "handler-b",
		hasSectionFunc: func(_ *datadoghq.DatadogInstrumentation) bool { return false },
		conditionType:  "BReady",
	}

	c := &Controller{
		lister:   fakeLister(crWith),
		handlers: []Handler{handlerA, handlerB},
		isLeader: func() bool { return false },
		lastSeen: make(map[string]*datadoghq.DatadogInstrumentation),
	}

	err := c.reconcile(t.Context(), "default/test-ddi")
	require.NoError(t, err)

	assert.Len(t, handlerA.getCalls(), 1, "handler-a should be called")
	assert.Equal(t, EventCreate, handlerA.getCalls()[0].eventType)

	assert.Empty(t, handlerB.getCalls(), "handler-b should not be called (no section)")
}

func TestReconcile_UpdateStatusOnlyAsLeader(t *testing.T) {
	tests := []struct {
		name             string
		isLeader         bool
		wantStatusUpdate bool
	}{
		{
			name:             "leader writes status condition",
			isLeader:         true,
			wantStatusUpdate: true,
		},
		{
			name:             "non-leader skips status update",
			isLeader:         false,
			wantStatusUpdate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			crWith := newTestCR("test-ddi", "default", 1, defaultChecks())

			handler := &mockHandler{
				hasSectionFunc: func(cr *datadoghq.DatadogInstrumentation) bool { return len(cr.Spec.Config.Checks) > 0 },
				handleStatus: HandlerStatus{
					Type:    "ChecksReady",
					Status:  metav1.ConditionTrue,
					Reason:  "Configured",
					Message: "all good",
				},
			}

			scheme := fakeScheme()
			fakeDynClient := dynamicfake.NewSimpleDynamicClient(scheme, crWith)

			c := &Controller{
				statusClient: fakeDynClient,
				lister:       fakeLister(crWith),
				handlers:     []Handler{handler},
				isLeader:     func() bool { return tt.isLeader },
				lastSeen:     make(map[string]*datadoghq.DatadogInstrumentation),
			}

			err := c.reconcile(t.Context(), "default/test-ddi")
			require.NoError(t, err)

			require.Len(t, handler.getCalls(), 1, "handler should always be called regardless of leadership")

			hasStatusUpdate := false
			for _, action := range fakeDynClient.Actions() {
				if action.GetVerb() == "update" && action.GetSubresource() == "status" {
					hasStatusUpdate = true
					break
				}
			}

			if tt.wantStatusUpdate {
				assert.True(t, hasStatusUpdate, "leader should write status condition")
			} else {
				assert.False(t, hasStatusUpdate, "non-leader should not write status condition")
			}
		})
	}
}

func TestReconcileDeleteSkipsStatusUpdate(t *testing.T) {
	crWith := newTestCR("test-ddi", "default", 1, defaultChecks())

	handler := &mockHandler{
		hasSectionFunc: func(cr *datadoghq.DatadogInstrumentation) bool { return len(cr.Spec.Config.Checks) > 0 },
		handleStatus: HandlerStatus{
			Type:    "ChecksReady",
			Status:  metav1.ConditionTrue,
			Reason:  "Configured",
			Message: "all good",
		},
	}

	scheme := fakeScheme()
	fakeDynClient := dynamicfake.NewSimpleDynamicClient(scheme, crWith)

	c := &Controller{
		statusClient: fakeDynClient,
		lister:       fakeLister(), // CR not in cache (deleted)
		handlers:     []Handler{handler},
		isLeader:     func() bool { return true },
		lastSeen:     map[string]*datadoghq.DatadogInstrumentation{"default/test-ddi": crWith.DeepCopy()},
	}

	err := c.reconcile(t.Context(), "default/test-ddi")
	require.NoError(t, err)

	calls := handler.getCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, EventDelete, calls[0].eventType)

	for _, action := range fakeDynClient.Actions() {
		assert.NotEqual(t, "update", action.GetVerb(), "should not attempt status update on delete")
	}
}

func TestReconcileUpdatesLastSeen(t *testing.T) {
	crWith := newTestCR("test-ddi", "default", 1, defaultChecks())

	handler := &mockHandler{
		hasSectionFunc: func(cr *datadoghq.DatadogInstrumentation) bool { return len(cr.Spec.Config.Checks) > 0 },
		conditionType:  "ChecksReady",
	}

	c := &Controller{
		lister:   fakeLister(crWith),
		handlers: []Handler{handler},
		isLeader: func() bool { return false },
		lastSeen: make(map[string]*datadoghq.DatadogInstrumentation),
	}

	// First reconcile: create
	err := c.reconcile(t.Context(), "default/test-ddi")
	require.NoError(t, err)
	require.Equal(t, EventCreate, handler.getCalls()[0].eventType)

	// lastSeen should now be populated
	assert.NotNil(t, c.lastSeen["default/test-ddi"])

	// Second reconcile with same state: update
	err = c.reconcile(t.Context(), "default/test-ddi")
	require.NoError(t, err)
	require.Equal(t, EventUpdate, handler.getCalls()[1].eventType)
}

func TestReconcileDeduplicatesCreateThenUpdate(t *testing.T) {
	crV1 := newTestCR("test-ddi", "default", 1, defaultChecks())
	crV2 := newTestCR("test-ddi", "default", 2, []datadoghq.DatadogInstrumentationCheckConfig{
		{Integration: "redisdb"},
		{Integration: "postgres"},
	})

	handler := &mockHandler{
		hasSectionFunc: func(cr *datadoghq.DatadogInstrumentation) bool { return len(cr.Spec.Config.Checks) > 0 },
		conditionType:  "ChecksReady",
	}

	// Simulate: create (v1) then immediate update (v2) enqueued, but the
	// key-only queue deduplicates them. By the time the worker processes the
	// key, the lister cache already has v2.
	c := &Controller{
		lister:   fakeLister(crV2),
		handlers: []Handler{handler},
		isLeader: func() bool { return false },
		lastSeen: make(map[string]*datadoghq.DatadogInstrumentation),
	}

	// Single reconcile — no lastSeen entry, cache has v2
	err := c.reconcile(t.Context(), "default/test-ddi")
	require.NoError(t, err)

	calls := handler.getCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, EventCreate, calls[0].eventType, "first reconcile with no lastSeen should be a create")
	assert.Equal(t, int64(2), calls[0].cr.Generation, "handler should receive v2, not v1")
	assert.Len(t, calls[0].cr.Spec.Config.Checks, 2, "handler should see the v2 checks")

	// Verify lastSeen was set to v2
	lastSeen := c.lastSeen["default/test-ddi"]
	require.NotNil(t, lastSeen)
	assert.Equal(t, int64(2), lastSeen.Generation)

	_ = crV1 // v1 was never seen by the handler — deduplication skipped it
}

func TestReconcileDeleteClearsLastSeen(t *testing.T) {
	crWith := newTestCR("test-ddi", "default", 1, defaultChecks())

	handler := &mockHandler{
		hasSectionFunc: func(cr *datadoghq.DatadogInstrumentation) bool { return len(cr.Spec.Config.Checks) > 0 },
		conditionType:  "ChecksReady",
	}

	c := &Controller{
		lister:   fakeLister(), // CR not in cache
		handlers: []Handler{handler},
		isLeader: func() bool { return false },
		lastSeen: map[string]*datadoghq.DatadogInstrumentation{"default/test-ddi": crWith.DeepCopy()},
	}

	err := c.reconcile(t.Context(), "default/test-ddi")
	require.NoError(t, err)
	require.Equal(t, EventDelete, handler.getCalls()[0].eventType)

	// lastSeen should be cleared after delete
	assert.Nil(t, c.lastSeen["default/test-ddi"])
}
