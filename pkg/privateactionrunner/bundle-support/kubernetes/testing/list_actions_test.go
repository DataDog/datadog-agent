// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package testing

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	apps "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/apps"
	batch "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/batch"
	core "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/core"
	discovery "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/discovery"
	networking "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/networking"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func TestGeneratedActionsListNamespaces(t *testing.T) {
	tests := []struct {
		action types.Action
		path   string
	}{
		{apps.NewListControllerRevisionHandler(), "/apis/apps/v1/controllerrevisions"},
		{apps.NewListDaemonSetHandler(), "/apis/apps/v1/daemonsets"},
		{apps.NewListDeploymentHandler(), "/apis/apps/v1/deployments"},
		{apps.NewListReplicaSetHandler(), "/apis/apps/v1/replicasets"},
		{apps.NewListStatefulSetHandler(), "/apis/apps/v1/statefulsets"},
		{batch.NewListCronJobHandler(), "/apis/batch/v1/cronjobs"},
		{batch.NewListJobHandler(), "/apis/batch/v1/jobs"},
		{core.NewListConfigMapHandler(), "/api/v1/configmaps"},
		{core.NewListEndpointsHandler(), "/api/v1/endpoints"},
		{core.NewListEventHandler(), "/api/v1/events"},
		{core.NewListLimitRangeHandler(), "/api/v1/limitranges"},
		{core.NewListPersistentVolumeClaimHandler(), "/api/v1/persistentvolumeclaims"},
		{core.NewListPodHandler(), "/api/v1/pods"},
		{core.NewListPodTemplateHandler(), "/api/v1/podtemplates"},
		{core.NewListReplicationControllerHandler(), "/api/v1/replicationcontrollers"},
		{core.NewListResourceQuotaHandler(), "/api/v1/resourcequotas"},
		{core.NewListServiceHandler(), "/api/v1/services"},
		{core.NewListServiceAccountHandler(), "/api/v1/serviceaccounts"},
		{discovery.NewListEndpointSliceHandler(), "/apis/discovery.k8s.io/v1/endpointslices"},
		{networking.NewListIngressHandler(), "/apis/networking.k8s.io/v1/ingresses"},
		{networking.NewListNetworkPolicyHandler(), "/apis/networking.k8s.io/v1/networkpolicies"},
	}
	requests := make(chan string, len(tests)*3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		requests <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"metadata":{},"items":[]}`))
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
    namespace: default
clusters:
- name: test
  cluster:
    server: %s
`, server.URL)), 0600))
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	credentials := &privateconnection.PrivateCredentials{
		Tokens: []privateconnection.PrivateCredentialsToken{{Name: "context", Value: "test"}},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			for _, namespace := range []struct {
				name   string
				inputs map[string]interface{}
			}{
				{name: "omitted"},
				{name: "empty", inputs: map[string]interface{}{"namespace": ""}},
				{name: "explicit", inputs: map[string]interface{}{"namespace": "production"}},
			} {
				t.Run(namespace.name, func(t *testing.T) {
					output, err := tt.action.Run(context.Background(), generatedActionTask(namespace.inputs), credentials)
					require.NoError(t, err)
					require.NotNil(t, output)
					expectedPath := tt.path
					if namespace.name == "explicit" {
						index := strings.LastIndex(expectedPath, "/")
						expectedPath = expectedPath[:index] + "/namespaces/production" + expectedPath[index:]
					}
					require.Equal(t, expectedPath, <-requests)
				})
			}
		})
	}
}
