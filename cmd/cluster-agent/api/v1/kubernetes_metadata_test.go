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

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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
	mockConfig.SetInTest("kubernetes_node_annotations_as_host_aliases", "annotation1")

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
