// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package testing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/scheme"

	autoscaling "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

func TestGeneratedHPAActionsRejectMissingInputs(t *testing.T) {
	assertGeneratedActionsRejectMissingInputs(t, autoscaling.NewKubernetesAutoscaling(), []generatedActionResource{{name: "HorizontalPodAutoscaler", namespaced: true}})
}

func TestGeneratedHPAActionsPreserveV2Fields(t *testing.T) {
	const hpa = `{"apiVersion":"autoscaling/v2","kind":"HorizontalPodAutoscaler","metadata":{"name":"checkout","namespace":"production"},"spec":{"scaleTargetRef":{"apiVersion":"apps/v1","kind":"Deployment","name":"checkout"},"minReplicas":2,"maxReplicas":10,"metrics":[{"type":"Resource","resource":{"name":"memory","target":{"type":"Utilization","averageUtilization":70}}}],"behavior":{"scaleDown":{"stabilizationWindowSeconds":300}}}}`
	var object map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(hpa), &object))
	for _, verb := range []string{"create", "update"} {
		t.Run(verb, func(t *testing.T) {
			type capturedRequest struct {
				path, method string
				object       map[string]interface{}
			}
			requests := make(chan capturedRequest, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var received map[string]interface{}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				decoded, _, err := scheme.Codecs.UniversalDeserializer().Decode(body, nil, nil)
				if err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				encoded, err := json.Marshal(decoded)
				if err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if err := json.Unmarshal(encoded, &received); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				requests <- capturedRequest{path: r.URL.Path, method: r.Method, object: received}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(hpa))
			}))
			defer server.Close()
			home := t.TempDir()
			configPath := filepath.Join(home, ".kube", "config")
			require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0700))
			require.NoError(t, os.WriteFile(configPath, []byte(fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: test
contexts:
- name: test
  context:
    cluster: test
clusters:
- name: test
  cluster:
    server: %s
`, server.URL)), 0600))
			t.Setenv("HOME", home)
			action := autoscaling.NewKubernetesAutoscaling().GetAction(verb + "HorizontalPodAutoscaler")
			output, err := action.Run(context.Background(), generatedActionTask(map[string]interface{}{"namespace": "production", "body": object}), &privateconnection.PrivateCredentials{Tokens: []privateconnection.PrivateCredentialsToken{{Name: "context", Value: "test"}}})
			require.NoError(t, err)
			received := <-requests
			expectedPath := "/apis/autoscaling/v2/namespaces/production/horizontalpodautoscalers"
			if verb == "update" {
				expectedPath += "/checkout"
				require.Equal(t, http.MethodPut, received.method)
			} else {
				require.Equal(t, http.MethodPost, received.method)
			}
			require.Equal(t, expectedPath, received.path)
			require.Equal(t, object["spec"], received.object["spec"])
			require.Equal(t, "autoscaling/v2", received.object["apiVersion"])
			encoded, err := json.Marshal(output)
			require.NoError(t, err)
			var returned map[string]interface{}
			require.NoError(t, json.Unmarshal(encoded, &returned))
			require.Equal(t, object["spec"], returned["spec"])
		})
	}
}
