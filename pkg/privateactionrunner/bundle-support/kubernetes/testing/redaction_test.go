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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/scheme"

	apps "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/apps"
	core "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/core"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func TestGeneratedActionsRedactResponses(t *testing.T) {
	const podSpec = `{"containers":[{"name":"app","env":[{"name":"TOKEN","value":"sensitive-env-value"},{"name":"PORT","value":"8080"}],"args":["--password","sensitive first second","--message","ordinary text"]}],"initContainers":[{"name":"init","args":["--password=sensitive-entrypoint-value"]}]}`
	for _, resource := range []struct {
		name, apiVersion, path, spec string
		bundle                       types.Bundle
	}{
		{"Pod", "v1", "/api/v1/namespaces/default/pods", podSpec, core.NewKubernetesCore()},
		{"Deployment", "apps/v1", "/apis/apps/v1/namespaces/default/deployments", `{"replicas":2,"template":{"metadata":{"labels":{"app":"example"}},"spec":` + podSpec + `}}`, apps.NewKubernetesApps()},
		{"ConfigMap", "v1", "/api/v1/namespaces/default/configmaps", "", core.NewKubernetesCore()},
		{"ControllerRevision", "apps/v1", "/apis/apps/v1/namespaces/default/controllerrevisions", "", apps.NewKubernetesApps()},
	} {
		for verb, method := range map[string]string{"get": http.MethodGet, "list": http.MethodGet, "create": http.MethodPost, "update": http.MethodPut, "patch": http.MethodPatch} {
			t.Run(verb+resource.name, func(t *testing.T) {
				objectJSON := fmt.Sprintf(`{"apiVersion":%q,"kind":%q,"metadata":{"name":"password-app","namespace":"default","resourceVersion":"12345","annotations":{"kubectl.kubernetes.io/last-applied-configuration":"sensitive-applied-value","settings":"{\"password\":\"sensitive-annotation-value\"}"},"labels":{"app":"example"}}`, resource.apiVersion, resource.name)
				if resource.spec != "" {
					objectJSON += `,"spec":` + resource.spec
				} else {
					objectJSON += `,"data":{"password":"sensitive-config-value","port":"8080"}`
				}
				if resource.name == "ConfigMap" {
					objectJSON += `,"binaryData":{"password":"c2Vuc2l0aXZlLWJpbmFyeS12YWx1ZQ==","public":"aGVsbG8="}`
				}
				objectJSON += `}`
				var object map[string]interface{}
				require.NoError(t, json.Unmarshal([]byte(objectJSON), &object))
				response := objectJSON
				if verb == "list" {
					response = fmt.Sprintf(`{"apiVersion":%q,"kind":%q,"metadata":{"resourceVersion":"12345","continue":"next-page"},"items":[%s,%s]}`, resource.apiVersion, resource.name+"List", objectJSON, objectJSON)
				}
				type request struct {
					method, path string
					body         []byte
				}
				requests := make(chan request, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					requests <- request{r.Method, r.URL.Path, body}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(response))
				}))
				defer server.Close()
				credentials := redactionTestCredentials(t, server.URL)
				inputs := map[string]interface{}{"namespace": "default", "name": "password-app"}
				if verb == "create" || verb == "update" {
					inputs["body"] = object
				} else if verb == "patch" {
					inputs["body"] = []map[string]interface{}{{"op": "add", "path": "/metadata/annotations/password", "value": "sensitive-patch-value"}}
				}
				action := resource.bundle.GetAction(verb + resource.name)
				require.NotNil(t, action)
				output, err := action.Run(t.Context(), generatedActionTask(inputs), credentials)
				require.NoError(t, err)
				received := <-requests
				require.Equal(t, method, received.method)
				wantPath := resource.path
				if verb != "create" && verb != "list" {
					wantPath += "/password-app"
				}
				require.Equal(t, wantPath, received.path)
				if verb == "create" || verb == "update" {
					decoded, _, err := scheme.Codecs.UniversalDeserializer().Decode(received.body, nil, nil)
					require.NoError(t, err)
					body, err := json.Marshal(decoded)
					require.NoError(t, err)
					require.Contains(t, string(body), "sensitive-applied-value", "requests must reach the API before redaction")
					require.NotContains(t, string(body), "********")
					if resource.spec == "" {
						require.Contains(t, string(body), "sensitive-config-value")
					} else {
						require.Contains(t, string(body), "sensitive-env-value")
						require.Contains(t, string(body), "sensitive first second")
					}
				} else if verb == "patch" {
					require.Contains(t, string(received.body), "sensitive-patch-value")
				}
				encoded, err := json.Marshal(output)
				require.NoError(t, err)
				require.NotContains(t, string(encoded), "sensitive", "all credential values should be masked")
				var returned map[string]interface{}
				require.NoError(t, json.Unmarshal(encoded, &returned))
				items := []interface{}{returned}
				if verb == "list" {
					require.Equal(t, map[string]interface{}{"resourceVersion": "12345", "continue": "next-page"}, returned["metadata"])
					items = returned["items"].([]interface{})
					require.Len(t, items, 2)
				}
				for _, item := range items {
					manifest := item.(map[string]interface{})
					metadata := manifest["metadata"].(map[string]interface{})
					require.Equal(t, "password-app", metadata["name"])
					require.Equal(t, "default", metadata["namespace"])
					require.Equal(t, "12345", metadata["resourceVersion"])
					require.Equal(t, map[string]interface{}{"app": "example"}, metadata["labels"])
					annotations := metadata["annotations"].(map[string]interface{})
					require.Equal(t, "-", annotations["kubectl.kubernetes.io/last-applied-configuration"])
					require.JSONEq(t, `{"password":"********"}`, annotations["settings"].(string))
					if resource.spec == "" {
						require.Equal(t, map[string]interface{}{"password": "********", "port": "8080"}, manifest["data"])
						if resource.name == "ConfigMap" {
							require.Equal(t, map[string]interface{}{"password": "********", "public": "aGVsbG8="}, manifest["binaryData"])
						}
						continue
					}
					spec := manifest["spec"].(map[string]interface{})
					if resource.name == "Deployment" {
						require.Equal(t, float64(2), spec["replicas"])
						spec = spec["template"].(map[string]interface{})["spec"].(map[string]interface{})
					}
					container := spec["containers"].([]interface{})[0].(map[string]interface{})
					require.Equal(t, []interface{}{"********", "********", "--message", "ordinary text"}, container["args"])
					env := container["env"].([]interface{})
					require.Equal(t, "********", env[0].(map[string]interface{})["value"])
					require.Equal(t, "8080", env[1].(map[string]interface{})["value"])
					initContainer := spec["initContainers"].([]interface{})[0].(map[string]interface{})
					require.Equal(t, []interface{}{"********"}, initContainer["args"])
				}
			})
		}
	}
}

func redactionTestCredentials(t *testing.T, serverURL string) *privateconnection.PrivateCredentials {
	t.Helper()
	home := t.TempDir()
	configPath := filepath.Join(home, ".kube", "config")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0700))
	config := strings.ReplaceAll(`apiVersion: v1
kind: Config
current-context: test
contexts:
- name: test
  context:
    cluster: test
clusters:
- name: test
  cluster:
    server: SERVER_URL
`, "SERVER_URL", serverURL)
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0600))
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return &privateconnection.PrivateCredentials{Tokens: []privateconnection.PrivateCredentialsToken{{Name: "context", Value: "test"}}}
}
