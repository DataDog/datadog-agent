// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package testing

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apps "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/apps"
	core "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/core"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func TestGeneratedActionsRedactCommands(t *testing.T) {
	tests := map[string]struct {
		command, args         []string
		wantCommand, wantArgs []string
	}{
		"inline password": {
			args:     []string{"--password=sensitive-value", "--port=8080"},
			wantArgs: []string{"********", "--port=8080"},
		},
		"password containing spaces": {
			args:     []string{"--password", "sensitive first second", "--message", "ordinary text"},
			wantArgs: []string{"********", "********", "--message", "ordinary text"},
		},
		"adjacent sensitive options": {
			args:     []string{"password", "--password", "sensitive-value"},
			wantArgs: []string{"********", "********", "********"},
		},
		"password split across command and args": {
			command:     []string{"app", "--password"},
			args:        []string{"sensitive-value"},
			wantCommand: []string{"app", "********"},
			wantArgs:    []string{"********"},
		},
	}
	for name, tt := range tests {
		for _, verb := range []string{"get", "list", "create", "update", "patch"} {
			t.Run(name+"/"+verb, func(t *testing.T) {
				pod := &corev1.Pod{
					TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
					ObjectMeta: metav1.ObjectMeta{Name: "password-app", Namespace: "default"},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name: "app", Command: tt.command, Args: tt.args,
					}}},
				}
				credentials := redactionTestCredentials(t, func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if verb == "list" {
						_ = json.NewEncoder(w).Encode(&corev1.PodList{
							TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PodList"},
							ListMeta: metav1.ListMeta{Continue: "next-page"},
							Items:    []corev1.Pod{*pod},
						})
					} else {
						_ = json.NewEncoder(w).Encode(pod)
					}
				})
				inputs := map[string]interface{}{"namespace": "default", "name": "password-app", "body": pod}
				if verb == "patch" {
					inputs["body"] = []map[string]interface{}{{"op": "replace", "path": "/spec/containers/0/args", "value": tt.args}}
				}

				output, err := core.NewKubernetesCore().GetAction(verb+"Pod").Run(t.Context(), generatedActionTask(inputs), credentials)
				require.NoError(t, err)
				encoded, err := json.Marshal(output)
				require.NoError(t, err)
				var returned corev1.Pod
				if verb == "list" {
					var list corev1.PodList
					require.NoError(t, json.Unmarshal(encoded, &list))
					require.Equal(t, "next-page", list.Continue)
					require.Len(t, list.Items, 1)
					returned = list.Items[0]
				} else {
					require.NoError(t, json.Unmarshal(encoded, &returned))
				}
				require.Equal(t, "password-app", returned.Name)
				require.Equal(t, "default", returned.Namespace)
				require.Len(t, returned.Spec.Containers, 1)
				require.Equal(t, tt.wantCommand, returned.Spec.Containers[0].Command)
				require.Equal(t, tt.wantArgs, returned.Spec.Containers[0].Args)
			})
		}
	}
}

func TestCreatePodRedactsOutputAfterSendingOriginalValues(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", Annotations: map[string]string{
			"kubectl.kubernetes.io/last-applied-configuration": "sensitive-applied-value",
		}},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Env: []corev1.EnvVar{
				{Name: "TOKEN", Value: "sensitive-env-value"},
				{Name: "PORT", Value: "8080"},
			}}},
			InitContainers: []corev1.Container{{Name: "init", Args: []string{"--password=sensitive-value"}}},
		},
	}
	requests := make(chan string, 1)
	credentials := redactionTestCredentials(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		requests <- string(body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pod)
	})

	output, err := core.NewCreatePodHandler().Run(t.Context(), generatedActionTask(map[string]interface{}{"namespace": "default", "body": pod}), credentials)
	require.NoError(t, err)
	require.Len(t, requests, 1)
	sent := <-requests
	require.Contains(t, sent, "sensitive-env-value")
	require.Contains(t, sent, "--password=sensitive-value")
	require.NotContains(t, sent, "********")
	encoded, err := json.Marshal(output)
	require.NoError(t, err)
	var returned corev1.Pod
	require.NoError(t, json.Unmarshal(encoded, &returned))
	require.Equal(t, "-", returned.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	require.Equal(t, []corev1.EnvVar{{Name: "TOKEN", Value: "********"}, {Name: "PORT", Value: "8080"}}, returned.Spec.Containers[0].Env)
	require.Equal(t, []string{"********"}, returned.Spec.InitContainers[0].Args)
}

func TestGeneratedActionsRedactEncodedFields(t *testing.T) {
	for _, tt := range []struct {
		name     string
		action   types.Action
		response string
	}{
		{name: "ConfigMap binary data", action: core.NewGetConfigMapHandler(), response: `{
			"data": {"password": "sensitive-value", "port": "8080"},
			"binaryData": {"password": "c2Vuc2l0aXZlLXZhbHVl", "public": "aGVsbG8="}
		}`},
		{name: "ControllerRevision raw data", action: apps.NewGetControllerRevisionHandler(), response: `{
			"data": {"password": "sensitive-value", "port": "8080"}
		}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			credentials := redactionTestCredentials(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tt.response)
			})
			output, err := tt.action.Run(t.Context(), generatedActionTask(map[string]interface{}{"namespace": "default", "name": "app"}), credentials)
			require.NoError(t, err)
			encoded, err := json.Marshal(output)
			require.NoError(t, err)
			require.Contains(t, string(encoded), `"password":"********"`)
			require.Contains(t, string(encoded), `"port":"8080"`)
			require.NotContains(t, string(encoded), "sensitive-value")
			require.NotContains(t, string(encoded), "c2Vuc2l0aXZlLXZhbHVl")
		})
	}
}

func redactionTestCredentials(t *testing.T, handler http.HandlerFunc) *privateconnection.PrivateCredentials {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
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
	t.Setenv("USERPROFILE", home)
	return &privateconnection.PrivateCredentials{Tokens: []privateconnection.PrivateCredentialsToken{{Name: "context", Value: "test"}}}
}
