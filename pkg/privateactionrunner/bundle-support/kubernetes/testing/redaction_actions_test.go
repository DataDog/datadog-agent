// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package testing

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	apiextensions "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/apiextensions"
	apps "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/apps"
	autoscaling "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/autoscaling"
	batch "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/batch"
	core "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/core"
	customresources "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/customresources"
	wpa "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/watermarkpodautoscaler"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func TestWorkloadActionsRedactResponses(t *testing.T) {
	for _, tt := range []struct {
		name   string
		action types.Action
		inputs map[string]interface{}
	}{
		{name: "scale deployment", action: apps.NewScaleDeploymentHandler(), inputs: map[string]interface{}{"replicas": 3}},
		{name: "scale deployment vertically", action: apps.NewScaleDeploymentVerticallyHandler(), inputs: map[string]interface{}{
			"containerUpdates": []map[string]interface{}{{"containerName": "app", "requests": map[string]string{"cpu": "100m"}}},
		}},
		{name: "scale stateful set", action: apps.NewScaleStatefulSetHorizontallyHandler(), inputs: map[string]interface{}{"replicas": 3}},
		{name: "scale stateful set vertically", action: apps.NewScaleStatefulSetVerticallyHandler(), inputs: map[string]interface{}{
			"containerUpdates": []map[string]interface{}{{"containerName": "app", "requests": map[string]string{"cpu": "100m"}}},
		}},
		{name: "restart deployment", action: apps.NewRestartDeploymentHandler()},
		{name: "update cron job", action: batch.NewUpdateCronJobScheduleHandler(), inputs: map[string]interface{}{"schedule": "0 * * * *"}},
		{name: "update HPA", action: autoscaling.NewUpdateAutoscalerLimitsHandler(), inputs: map[string]interface{}{"minReplicas": 2, "maxReplicas": 10}},
		{name: "update WPA", action: wpa.NewUpdateWatermarkAutoscalerLimitsHandler(), inputs: map[string]interface{}{"minReplicas": 2, "maxReplicas": 10}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			credentials := redactionTestCredentials(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				// Typed actions need no kind; the dynamic WPA client does.
				_, _ = io.WriteString(w, `{`)
				if strings.Contains(r.URL.Path, "watermarkpodautoscalers") {
					_, _ = io.WriteString(w, `"apiVersion":"datadoghq.com/v1alpha1","kind":"WatermarkPodAutoscaler",`)
				}
				_, _ = io.WriteString(w, `
					"metadata": {
						"name": "password-app", "namespace": "default",
						"labels": {"app": "public-value"},
						"annotations": {"settings": "{\"password\":\"sensitive-value\"}"}
					},
					"spec": {
						"minReplicas": 2, "maxReplicas": 10,
						"template": {"spec": {"containers": [{"name": "app"}]}}
					}
				}`)
			})
			inputs := map[string]interface{}{"namespace": "default", "name": "password-app"}
			for key, value := range tt.inputs {
				inputs[key] = value
			}

			output, err := tt.action.Run(t.Context(), generatedActionTask(inputs), credentials)
			require.NoError(t, err)
			encoded, err := json.Marshal(output)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "sensitive-value")
			require.Contains(t, string(encoded), "********")
			require.Contains(t, string(encoded), "public-value")
			require.Contains(t, string(encoded), `"name":"password-app"`)
		})
	}
}

func TestCustomResourceActionsRedactResponses(t *testing.T) {
	for _, scope := range []string{"namespaced", "cluster", "definition"} {
		for _, verb := range []string{"get", "list", "create", "update", "patch"} {
			t.Run(scope+"/"+verb, func(t *testing.T) {
				object := map[string]interface{}{
					"apiVersion": "example.com/v1", "kind": "Widget",
					"metadata": map[string]interface{}{"name": "password-app"},
					"spec":     map[string]interface{}{"password": "sensitive-value", "port": "8080"},
				}
				bundle := types.Bundle(customresources.NewKubernetesCustomResources(nil))
				actionName := verb + "CustomObject"
				if scope == "cluster" {
					actionName = verb + "ClusterCustomObject"
				}
				if scope == "definition" {
					bundle = apiextensions.NewKubernetesApiExtensions()
					actionName = verb + "CustomResourceDefinition"
					object["apiVersion"], object["kind"] = "apiextensions.k8s.io/v1", "CustomResourceDefinition"
				}
				credentials := redactionTestCredentials(t, func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if verb == "list" {
						_ = json.NewEncoder(w).Encode(map[string]interface{}{
							"apiVersion": object["apiVersion"], "kind": object["kind"].(string) + "List",
							"metadata": map[string]string{"continue": "next-page"},
							"items":    []interface{}{object},
						})
					} else {
						_ = json.NewEncoder(w).Encode(object)
					}
				})
				inputs := map[string]interface{}{
					"namespace": "default", "name": "password-app",
					"group": "example.com", "version": "v1", "plural": "widgets", "body": object,
				}
				if verb == "patch" {
					inputs["body"] = []map[string]interface{}{{"op": "replace", "path": "/spec/password", "value": "sensitive-value"}}
				}

				output, err := bundle.GetAction(actionName).Run(t.Context(), generatedActionTask(inputs), credentials)
				require.NoError(t, err)
				encoded, err := json.Marshal(output)
				require.NoError(t, err)
				require.NotContains(t, string(encoded), "sensitive-value")
				require.Contains(t, string(encoded), `"password":"********"`)
				require.Contains(t, string(encoded), `"port":"8080"`)
				require.Contains(t, string(encoded), `"name":"password-app"`)
				if verb == "list" {
					require.Contains(t, string(encoded), `"continue":"next-page"`)
				}
			})
		}
	}
}

func TestConnectionRedactsSensitiveUserDetails(t *testing.T) {
	credentials := redactionTestCredentials(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"apiVersion":"authentication.k8s.io/v1", "kind":"SelfSubjectReview",
			"status":{"userInfo":{"username":"alice","extra":{"api_key":["sensitive-value"],"team":["platform"]}}}
		}`)
	})
	output, err := core.NewTestConnectionHandler().Run(t.Context(), generatedActionTask(nil), credentials)
	require.NoError(t, err)
	encoded, err := json.Marshal(output)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "sensitive-value")
	require.Contains(t, string(encoded), `"api_key":"********"`)
	require.Contains(t, string(encoded), `"username":"alice"`)
	require.Contains(t, string(encoded), `"team":"platform"`)
}
