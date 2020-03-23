// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package admissioncontroller

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

type ResponsePayload struct {
	Response ResponseObject
}

type ResponseObject struct {
	UID       string
	Allowed   bool
	Patch     string
	PatchType string
	Status    Result
}

type JSONPath struct {
	Op    string
	Path  string
	Value corev1.EnvVar
}

type Result struct {
	Message string
}

func marshalPodSpec(spec corev1.PodSpec) []byte {
	pod := corev1.Pod{Spec: spec}
	body, _ := json.Marshal(pod)
	return body
}

func marshalContainers(containers []corev1.Container) []byte {
	return marshalPodSpec(corev1.PodSpec{Containers: containers})
}

func newContainer() []corev1.Container {
	return newContainerWithVar("", "")
}

func newContainerWithVar(name string, value string) []corev1.Container {
	return []corev1.Container{{Env: []corev1.EnvVar{{Name: name, Value: value}}}}
}

func TestServe(t *testing.T) {
	tests := []struct {
		description string
		requestBody []byte
		status      int
		hasResponseBody  bool
		patchPaths  []string
		message string
	}{
		{
			"with container",
			marshalContainers(newContainer()),
			200,
			true,
			[]string{"/spec/containers/0/env/0"},
			"",
		},
		{
			"with initContainer",
			marshalPodSpec(corev1.PodSpec{InitContainers: newContainer()}),
			200,
			true,
			[]string{"/spec/initContainers/0/env/0"},
			"",
		},
		{
			"container with existing environment variable",
			marshalContainers(newContainerWithVar("DD_AGENT_HOST", "existing")),
			200,
			true,
			[]string{},
			"",
		},
		{
			"empty request body",
			[]byte{},
			400,
			false,
			[]string{},
			"",
		},
		{
			"invalid payload body",
			nil,
			200,
			true,
			[]string{},
			"unexpected end of JSON input",
		},
	}

	var whsvr WebhookServer

	for i, tt := range tests {
		t.Run(fmt.Sprintf("#%d %s", i, tt.description), func(t *testing.T) {
			// Build request object
			const requestUid = "request-id"
			podResource := metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

			request := v1beta1.AdmissionReview{Request: &v1beta1.AdmissionRequest{
				UID:      requestUid,
				Resource: podResource,
				Object:   runtime.RawExtension{Raw: tt.requestBody},
			}}

			body, _ := json.Marshal(request)
			req, _ := http.NewRequest("GET", "/", bytes.NewReader(body))
			writer := httptest.NewRecorder()

			// Exercise code
			whsvr.serve(writer, req)

			assert.Equal(t, tt.status, writer.Code)

			if !tt.hasResponseBody {
				data, err := ioutil.ReadAll(writer.Body)
				assert.Empty(t, data)
				assert.Nil(t, err)
				return
			}

			response := ResponsePayload{}
			data, err := ioutil.ReadAll(writer.Body)
			assert.Nil(t, err)
			err = json.Unmarshal(data, &response)
			assert.Nil(t, err)

			assert.Equal(t, requestUid, response.Response.UID)
			assert.Equal(t, true, response.Response.Allowed)

			if len(tt.patchPaths) == 0 {
				// No JSONPatch in response
				assert.Equal(t, tt.message, response.Response.Status.Message)
				return
			}

			// Assert JSONPatch
			assert.Equal(t, "JSONPatch", response.Response.PatchType)

			// Expect Base64 encoded JSONPatch payload
			decodedPatch, err := base64.StdEncoding.DecodeString(response.Response.Patch)
			assert.Nil(t, err)

			var patchList []JSONPath
			err = json.Unmarshal(decodedPatch, &patchList)
			assert.Nil(t, err)
			assert.Equal(t, len(tt.patchPaths), len(patchList))

			for i, path := range tt.patchPaths {
				patch := patchList[i]

				assert.Equal(t, "add", patch.Op)
				assert.Equal(t, path, patch.Path)

				value := patch.Value
				assert.Equal(t, "DD_AGENT_HOST", value.Name)
				assert.Equal(t, "status.hostIP", value.ValueFrom.FieldRef.FieldPath)
				assert.Empty(t, value.Value) // We use ValueFrom instead of Value
			}
		})
	}
}
