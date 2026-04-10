// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/api"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

const testNode = "test_node"

func TestGetNodeAnnotations(t *testing.T) {
	// mock workloadmeta so that it has a metadata entry for node
	// /nodes//test_node with two annotations
	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	mockStore.Set(&workloadmeta.KubernetesMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesMetadata,
			ID:   "/nodes//" + testNode,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Annotations: map[string]string{
				"annotation1": "abc",
				"annotation2": "def",
			},
		},
	})

	// mock the config provider so that kubernetes_node_annotations_as_host_aliases
	// we need to do this because the default behavior for /annotations/node/{node}
	// applies a filter based on this config
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("kubernetes_node_annotations_as_host_aliases", "annotation1")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		getNodeAnnotations(w, r, mockStore)
	})

	tests := []struct {
		name    string
		path    string
		muxVars map[string]string
		body    map[string]string
		status  int
	}{
		{
			name:    "no filters passed only host aliases annotations returned",
			path:    "/annotations/node/" + testNode,
			muxVars: map[string]string{"nodeName": testNode},
			body:    map[string]string{"annotation1": "abc"}, // hardcoded above in workloadmeta mock
			status:  http.StatusOK,
		},
		{
			name:    "only the filtered annotation is returned",
			path:    fmt.Sprintf("/annotations/node/%s?filter=annotation2", testNode),
			muxVars: map[string]string{"nodeName": testNode},
			body:    map[string]string{"annotation2": "def"}, // hardcoded above in workloadmeta mock
			status:  http.StatusOK,
		},
		{
			name:    "filter query parameters are sanitized",
			path:    fmt.Sprintf("/annotations/node/%s?filter= annotation1&filter= annotation2", testNode),
			muxVars: map[string]string{"nodeName": testNode},
			body:    map[string]string{"annotation1": "abc", "annotation2": "def"}, // hardcoded above in workloadmeta mock
			status:  http.StatusOK,
		},
		{
			name:    "invalid node returns nothing",
			path:    "/annotations/node/NOT_A_REAL_NODE?filter=annotation1",
			muxVars: map[string]string{"nodeName": "NOT_A_REAL_NODE"},
			body:    nil,
			status:  http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// build a request setting the appropriate mux variables
			// we inject the mux vars here because we are not routing this request through a real router

			// we aren't using httptest.NewRequest() because it panics when invalid URLs are passed,
			// but we want to test that whitespace is stripped - so we instead ignore the error here
			req, _ := http.NewRequest("GET", tt.path, nil)
			req = mux.SetURLVars(req, tt.muxVars)

			respw := httptest.NewRecorder()

			handler.ServeHTTP(respw, req)

			require.Equal(t, tt.status, respw.Code)

			// if expected body is not nil, try to unmarshal it and check whether it
			// matches the expected
			if tt.body != nil {
				var resp map[string]string
				err := json.Unmarshal(respw.Body.Bytes(), &resp)
				if err != nil {
					t.Fatal(err)
				}
				require.Equal(t, tt.body, resp)
			}

		})
	}
}

func TestGetNodeUID(t *testing.T) {
	// mock workloadmeta and populate it with a single node entry
	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	mockStore.Set(&workloadmeta.KubernetesMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesMetadata,
			ID:   "/nodes//" + testNode,
		},
		EntityMeta: workloadmeta.EntityMeta{
			UID: "uid-12345",
		},
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		getNodeUID(w, r, mockStore)
	})

	tests := []struct {
		name   string
		node   string
		status int
		body   map[string]string
	}{
		{
			name:   "existing node returns uid",
			node:   testNode,
			status: http.StatusOK,
			body:   map[string]string{"uid": "uid-12345"},
		},
		{
			name:   "missing node returns not found",
			node:   "not-there",
			status: http.StatusInternalServerError,
			body:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/uid/node/"+tt.node, nil)
			req = mux.SetURLVars(req, map[string]string{"nodeName": tt.node})

			respw := httptest.NewRecorder()

			handler.ServeHTTP(respw, req)

			require.Equal(t, tt.status, respw.Code)

			if tt.body != nil {
				var resp map[string]string
				err := json.Unmarshal(respw.Body.Bytes(), &resp)
				if err != nil {
					t.Fatal(err)
				}
				require.Equal(t, tt.body, resp)
			}
		})
	}
}

func TestGetNodeMetadata_SpanCreation(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	mockStore.Set(&workloadmeta.KubernetesMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesMetadata,
			ID:   "/nodes//" + testNode,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Labels: map[string]string{"label1": "value1"},
		},
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		getNodeLabels(w, r, mockStore)
	})

	req := httptest.NewRequest("GET", "/tags/node/"+testNode, nil)
	req = mux.SetURLVars(req, map[string]string{"nodeName": testNode})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "cluster_agent.metadata.node_lookup", span.OperationName())
	assert.Equal(t, "nodeLookup", span.Tag("resource.name"))
	assert.Equal(t, testNode, span.Tag("node_name"))
	assert.Equal(t, "labels", span.Tag("metadata_type"))
	assert.Nil(t, span.Tag("error"))
}

func TestGetNodeMetadata_SpanError(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	// No data set — lookup will fail

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		getNodeLabels(w, r, mockStore)
	})

	req := httptest.NewRequest("GET", "/tags/node/missing_node", nil)
	req = mux.SetURLVars(req, map[string]string{"nodeName": "missing_node"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "cluster_agent.metadata.node_lookup", span.OperationName())
	assert.Equal(t, "nodeLookup", span.Tag("resource.name"))
	assert.Equal(t, "missing_node", span.Tag("node_name"))
	// Error should be set on the span
	err, ok := span.Tag("error").(error)
	require.True(t, ok, "error tag should be an error object, got %T", span.Tag("error"))
	assert.NotEmpty(t, err.Error())
}

func TestGetNamespaceMetadata_SpanCreation(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	mockStore.Set(&workloadmeta.KubernetesMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesMetadata,
			ID:   "/namespaces//default",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Labels: map[string]string{"env": "test"},
		},
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		getNamespaceLabels(w, r, mockStore)
	})

	req := httptest.NewRequest("GET", "/tags/namespace/default", nil)
	req = mux.SetURLVars(req, map[string]string{"ns": "default"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "cluster_agent.metadata.namespace_lookup", span.OperationName())
	assert.Equal(t, "namespaceLookup", span.Tag("resource.name"))
	assert.Equal(t, "default", span.Tag("namespace"))
	assert.Nil(t, span.Tag("error"))
}

func TestGetNamespaceMetadata_SpanError(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	// No data set — lookup will fail

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		getNamespaceLabels(w, r, mockStore)
	})

	req := httptest.NewRequest("GET", "/tags/namespace/missing_ns", nil)
	req = mux.SetURLVars(req, map[string]string{"ns": "missing_ns"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "cluster_agent.metadata.namespace_lookup", span.OperationName())
	assert.Equal(t, "namespaceLookup", span.Tag("resource.name"))
	assert.Equal(t, "missing_ns", span.Tag("namespace"))
	err, ok := span.Tag("error").(error)
	require.True(t, ok, "error tag should be an error object, got %T", span.Tag("error"))
	assert.NotEmpty(t, err.Error())
}

func TestGetPodMetadata_SpanCreation(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	// getPodMetadata calls as.GetPodMetadataNames which requires the apiserver cache.
	// When no cache is available it returns an error, so we test the error path which
	// still verifies span creation and tag propagation.
	handler := http.HandlerFunc(getPodMetadata)

	req := httptest.NewRequest("GET", "/tags/pod/node1/default/pod1", nil)
	req = mux.SetURLVars(req, map[string]string{"nodeName": "node1", "ns": "default", "podName": "pod1"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "cluster_agent.metadata.pod_lookup", span.OperationName())
	assert.Equal(t, "podLookup", span.Tag("resource.name"))
	assert.Equal(t, "node1", span.Tag("node_name"))
	assert.Equal(t, "default", span.Tag("namespace"))
	// pod_name should NOT be set (cardinality)
	assert.Nil(t, span.Tag("pod_name"))
}

func TestGetPodMetadataForNode_SpanCreation(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	handler := http.HandlerFunc(getPodMetadataForNode)

	req := httptest.NewRequest("GET", "/tags/pod/node1", nil)
	req = mux.SetURLVars(req, map[string]string{"nodeName": "node1"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "cluster_agent.metadata.pod_metadata_for_node", span.OperationName())
	assert.Equal(t, "podMetadataForNode", span.Tag("resource.name"))
	assert.Equal(t, "node1", span.Tag("node_name"))
	// Without a real apiserver, GetMetadataMapBundleOnNode fails, but this is a
	// non-fatal path (the handler continues with partial results), so the span
	// should not be marked as errored.
	assert.Nil(t, span.Tag("error"), "span should not be marked as errored for partial failures")
}

func TestGetAllMetadata_SpanCreation(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	// Without a real apiserver, getAllMetadata may fail at GetAPIClient (500) or
	// at GetMetadataMapBundleOnAllNodes (503) depending on the environment.
	// Either way, we verify span creation on the error path.
	handler := http.HandlerFunc(getAllMetadata)

	req := httptest.NewRequest("GET", "/tags/pod", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.GreaterOrEqual(t, rec.Code, 500, "expected a 5xx status code, got %d", rec.Code)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "cluster_agent.metadata.all_metadata", span.OperationName())
	assert.Equal(t, "allMetadata", span.Tag("resource.name"))
	err, ok := span.Tag("error").(error)
	require.True(t, ok, "error tag should be an error object, got %T", span.Tag("error"))
	assert.NotEmpty(t, err.Error())
}

func TestGetClusterID_SpanCreation(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	// getClusterID calls as.GetAPIClient which will fail without a real apiserver.
	// This tests the error path, which still verifies span creation.
	handler := http.HandlerFunc(getClusterID)

	req := httptest.NewRequest("GET", "/cluster/id", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "cluster_agent.metadata.cluster_id", span.OperationName())
	assert.Equal(t, "clusterID", span.Tag("resource.name"))
	err, ok := span.Tag("error").(error)
	require.True(t, ok, "error tag should be an error object, got %T", span.Tag("error"))
	assert.NotEmpty(t, err.Error())
}

func TestGetNodeMetadata_SpanErrorWithTelemetryWrapper(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	// No data set — lookup will fail

	// Wrap with WithTelemetryWrapper to exercise SetSpanError propagation to the parent span
	handler := api.WithTelemetryWrapper("getNodeLabels", func(w http.ResponseWriter, r *http.Request) {
		getNodeLabels(w, r, mockStore)
	})

	req := httptest.NewRequest("GET", "/tags/node/missing_node", nil)
	req = mux.SetURLVars(req, map[string]string{"nodeName": "missing_node"})
	rec := httptest.NewRecorder()
	handler(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 2, "expected parent (telemetry) span and child (node_lookup) span")

	// Find the parent telemetry span
	var parentSpan, childSpan mocktracer.Span
	for _, s := range spans {
		if s.OperationName() == "cluster_agent.api.request" {
			parentSpan = s
		} else if s.OperationName() == "cluster_agent.metadata.node_lookup" {
			childSpan = s
		}
	}
	require.NotNil(t, parentSpan, "parent telemetry span should exist")
	require.NotNil(t, childSpan, "child node_lookup span should exist")

	// Child span should have the error from spanErr
	childErr, ok := childSpan.Tag("error").(error)
	require.True(t, ok, "child span error tag should be an error, got %T", childSpan.Tag("error"))
	assert.NotEmpty(t, childErr.Error())

	// Parent span should also have the error from SetSpanError
	parentErr, ok := parentSpan.Tag("error").(error)
	require.True(t, ok, "parent span error tag should be an error from SetSpanError, got %T", parentSpan.Tag("error"))
	assert.NotEmpty(t, parentErr.Error())
}

func TestGetNodeInfo_SpanCreation(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		getNodeInfo(w, r, mockStore)
	})

	req := httptest.NewRequest("GET", "/info/node/"+testNode, nil)
	req = mux.SetURLVars(req, map[string]string{"nodeName": testNode})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "cluster_agent.metadata.node_info", span.OperationName())
	assert.Equal(t, "nodeInfo", span.Tag("resource.name"))
	assert.Equal(t, testNode, span.Tag("node_name"))
	// Without a real apiserver, GetAPIClient fails, so the span should capture the error
	spanErr, ok := span.Tag("error").(error)
	require.True(t, ok, "error tag should be an error object, got %T", span.Tag("error"))
	assert.NotEmpty(t, spanErr.Error())
}
