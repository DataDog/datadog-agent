// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

//nolint:revive // TODO(CAPP) Fix revive linter
package orchestrator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"

	model "github.com/DataDog/agent-payload/v5/process"

	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/k8s"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchcfg "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	pkgorchestratormodel "github.com/DataDog/datadog-agent/pkg/orchestrator/model"
)

func TestToTypedSlice(t *testing.T) {
	tests := []struct {
		name           string
		setupResource  func() interface{}
		setupCollector func(t *testing.T) collectors.K8sCollector
		expectedType   func(result interface{}) bool
	}{
		{
			name: "replicaSet collector",
			setupResource: func() interface{} {
				rs := &appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rs",
					},
				}
				return toInterface(rs)
			},
			setupCollector: func(t *testing.T) collectors.K8sCollector {
				return k8s.NewReplicaSetCollector(nil)
			},
			expectedType: func(result interface{}) bool {
				_, ok := result.([]*appsv1.ReplicaSet)
				return ok
			},
		},
		{
			name: "crd collector",
			setupResource: func() interface{} {
				es := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "customResourceDefinition",
					},
				}
				return toInterface(es)
			},
			setupCollector: func(t *testing.T) collectors.K8sCollector {
				collector, err := k8s.NewCRCollector("customResourceDefinition", "apiextensions.k8s.io/v1")
				require.NoError(t, err)
				return collector
			},
			expectedType: func(result interface{}) bool {
				_, ok := result.([]runtime.Object)
				return ok
			},
		},
		{
			name: "cr collector",
			setupResource: func() interface{} {
				es := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apiextensions.k8s.io/v1",
						"kind":       "datadogcustomresource",
					},
				}
				return toInterface(es)
			},
			setupCollector: func(t *testing.T) collectors.K8sCollector {
				collector, err := k8s.NewCRCollector("datadogcustomresource", "apiextensions.k8s.io/v1")
				require.NoError(t, err)
				return collector
			},
			expectedType: func(result interface{}) bool {
				_, ok := result.([]runtime.Object)
				return ok
			},
		},
		{
			name: "generic collector",
			setupResource: func() interface{} {
				es := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "EndpointSlice",
					},
				}
				return toInterface(es)
			},
			setupCollector: func(t *testing.T) collectors.K8sCollector {
				collector := k8s.GenericResource{
					Group:    "discovery.k8s.io",
					Name:     "endpointSlice",
					Version:  "v1",
					NodeType: pkgorchestratormodel.K8sEndpointSlice,
				}.NewGenericCollector()
				return collector
			},
			expectedType: func(result interface{}) bool {
				_, ok := result.([]runtime.Object)
				return ok
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := tt.setupResource()
			collector := tt.setupCollector(t)
			list := []interface{}{resource}

			typedList := toTypedSlice(collector, list)
			require.True(t, tt.expectedType(typedList))
		})
	}
}

func toInterface(i interface{}) interface{} {
	return i
}

func TestGetResource(t *testing.T) {
	tests := []struct {
		name     string
		resource interface{}
		wantOk   bool
	}{
		{
			name:     "nil resource",
			resource: nil,
			wantOk:   false,
		},
		{
			name: "DeletedFinalStateUnknown with nil object",
			resource: cache.DeletedFinalStateUnknown{
				Obj: nil,
			},
			wantOk: false,
		},
		{
			name: "DeletedFinalStateUnknown with ReplicaSet",
			resource: cache.DeletedFinalStateUnknown{
				Obj: &appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rs",
					},
				},
			},
			wantOk: true,
		},
		{
			name: "direct ReplicaSet",
			resource: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "rs",
				},
			},
			wantOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getResource(tt.resource)
			require.Equal(t, tt.wantOk, err == nil)
		})
	}
}

// fakeK8sCollector is a minimal K8sCollector whose Process returns a
// fixed result containing manifest messages.
type fakeK8sCollector struct {
	metadata      *collectors.CollectorMetadata
	processResult *collectors.CollectorRunResult
}

func (f *fakeK8sCollector) Init(*collectors.CollectorRunConfig) {}
func (f *fakeK8sCollector) Metadata() *collectors.CollectorMetadata {
	return f.metadata
}
func (f *fakeK8sCollector) Run(*collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	return f.processResult, nil
}
func (f *fakeK8sCollector) Process(*collectors.CollectorRunConfig, interface{}) (*collectors.CollectorRunResult, error) {
	return f.processResult, nil
}
func (f *fakeK8sCollector) Informer() cache.SharedInformer { return nil }

// TestDisableDoesNotBlockWithStoppedManifestBuffer proves that Disable
// completes even when the ManifestBuffer goroutine is not running.
//
// Before the fix, Disable → Run → BufferManifestProcessResult would send
// on the unbuffered ManifestChan with no reader, blocking forever while
// holding tb.mu.
func TestDisableDoesNotBlockWithStoppedManifestBuffer(t *testing.T) {
	checkBase := core.NewCheckBase("test-orchestrator")
	mockSender := mocksender.NewMockSender(checkBase.ID())
	mockSender.On("OrchestratorManifest", mock.Anything, mock.Anything).Return()
	mockSender.On("OrchestratorMetadata", mock.Anything, mock.Anything, mock.Anything).Return()

	orchCheck := &OrchestratorCheck{CheckBase: checkBase}
	_ = orchCheck.CommonConfigure(mockSender.GetSenderManager(), nil, nil, "test", "provider")

	runCfg := &collectors.CollectorRunConfig{
		ClusterID: "test-cluster",
		Config: &orchcfg.OrchestratorConfig{
			MaxPerMessage:            100,
			MaxWeightPerMessageBytes: 1000000,
		},
	}

	manifestBuffer := &ManifestBuffer{
		ManifestChan: make(chan interface{}),
		stopCh:       make(chan struct{}),
	}

	collector := &fakeK8sCollector{
		metadata: &collectors.CollectorMetadata{
			Name:                      "fake-replicaset",
			SupportsManifestBuffering: true,
			IsMetadataProducer:        true,
			NodeType:                  orchestrator.K8sReplicaSet,
			Version:                   "v1",
			Group:                     "apps",
		},
		processResult: &collectors.CollectorRunResult{
			Result: processors.ProcessResult{
				ManifestMessages: []model.MessageBody{
					&model.CollectorManifest{
						Manifests: []*model.Manifest{{Type: int32(1), Uid: "test-uid"}},
					},
				},
			},
			ResourcesListed:    1,
			ResourcesProcessed: 1,
		},
	}

	tb := NewTerminatedResourceBundle(orchCheck, runCfg, manifestBuffer)
	tb.Enable()

	tb.mu.Lock()
	tb.terminatedResources[collector] = []interface{}{
		&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "deleted-rs"}},
	}
	tb.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		tb.Disable()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Disable() blocked: ManifestBuffer goroutine is not running but flush tried to send through ManifestChan")
	}

	require.False(t, tb.enabled)
	mockSender.AssertCalled(t, "OrchestratorManifest", mock.Anything, mock.Anything)
}
