// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"net/http"
	"net/http/httptest"
	"testing"
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
		name     string
		path     string
		muxVars  map[string]string
		expected map[string]string
	}{
		{
			name:     "no filters passed only host aliases annotations returned",
			path:     fmt.Sprintf("/annotations/node/%s", testNode),
			muxVars:  map[string]string{"nodeName": testNode},
			expected: map[string]string{"annotation1": "abc"}, // hardcoded above in workloadmeta mock
		},
		{
			name:     "only the filtered annotation is returned",
			path:     fmt.Sprintf("/annotations/node/%s?filter=annotation2", testNode),
			muxVars:  map[string]string{"nodeName": testNode},
			expected: map[string]string{"annotation2": "def"}, // hardcoded above in workloadmeta mock
		},
		{
			name:     "filter query parameters are sanitized",
			path:     fmt.Sprintf("/annotations/node/%s?filter= annotation1, annotation2", testNode),
			muxVars:  map[string]string{"nodeName": testNode},
			expected: map[string]string{"annotation1": "abc", "annotation2": "def"}, // hardcoded above in workloadmeta mock
		},
		{
			name:     "invalid node returns nothing",
			path:     "/annotations/node/NOT_A_REAL_NODE?filter= annotation1",
			muxVars:  map[string]string{"nodeName": "NOT_A_REAL_NODE"},
			expected: map[string]string{},
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

			var resp map[string]string
			err := json.Unmarshal(respw.Body.Bytes(), &resp)
			if err != nil {
				t.Fatal(err)
			}

			require.Equal(t, tt.expected, resp)
		})
	}
}
