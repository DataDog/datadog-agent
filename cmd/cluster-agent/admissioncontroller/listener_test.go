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

type responsePayload struct {
	Response responseObject
}

type responseObject struct {
	UID       string
	Allowed   bool
	Patch     string
	PatchType string
	Status    result
}

type jsonPath struct {
	Op    string
	Path  string
	Value interface{}
}

type result struct {
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
	return []corev1.Container{{}}
}

func newContainerWithVar(name string, value string) []corev1.Container {
	return []corev1.Container{{Env: []corev1.EnvVar{{Name: name, Value: value}}}}
}

func expectedPatchFieldPath(name string, fieldPath string) interface{} {
	return map[string]interface{}{
		"name": name,
		"valueFrom": map[string]interface{}{
			"fieldRef": map[string]interface{}{
				"fieldPath": fieldPath,
			},
		},
	}
}

// Base path creation must have an empty array as value
func expectedBaseEnvPatchValue() interface{} {
	return []interface{}{}
}

type ExpectedPatch struct {
	path  string
	value interface{}
}

func TestServe(t *testing.T) {
	tests := []struct {
		description     string
		requestBody     []byte
		status          int
		hasResponseBody bool
		patches         []ExpectedPatch
		message         string
	}{
		{
			"with container",
			marshalContainers(newContainer()),
			200,
			true,
			[]ExpectedPatch{
				{"/spec/containers/0/env", expectedBaseEnvPatchValue()},
				{"/spec/containers/0/env/0", expectedPatchFieldPath("DD_AGENT_HOST", "status.hostIP")},
			},
			"",
		},
		{
			"with initContainer",
			marshalPodSpec(corev1.PodSpec{InitContainers: newContainer()}),
			200,
			true,
			[]ExpectedPatch{
				{"/spec/initContainers/0/env", expectedBaseEnvPatchValue()},
				{"/spec/initContainers/0/env/0", expectedPatchFieldPath("DD_AGENT_HOST", "status.hostIP")},
			},
			"",
		},
		{
			"container with conflicting environment variable",
			marshalContainers(newContainerWithVar("DD_AGENT_HOST", "existing")),
			200,
			true,
			[]ExpectedPatch{},
			"",
		},
		{
			"container with non-conflicting environment variable",
			marshalContainers(newContainerWithVar("no-dd", "existing")),
			200,
			true,
			[]ExpectedPatch{
				{"/spec/containers/0/env/0", expectedPatchFieldPath("DD_AGENT_HOST", "status.hostIP")},
			},
			"",
		},
		{
			"empty request body",
			[]byte{},
			400,
			false,
			[]ExpectedPatch{},
			"",
		},
		{
			"invalid payload body",
			nil,
			200,
			true,
			[]ExpectedPatch{},
			"unexpected end of JSON input",
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("#%d %s", i, tt.description), func(t *testing.T) {
			// Build request object
			const requestUID = "request-id"
			podResource := metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

			request := v1beta1.AdmissionReview{Request: &v1beta1.AdmissionRequest{
				UID:      requestUID,
				Resource: podResource,
				Object:   runtime.RawExtension{Raw: tt.requestBody},
			}}

			body, _ := json.Marshal(request)
			req, _ := http.NewRequest("GET", "/", bytes.NewReader(body))
			writer := httptest.NewRecorder()

			// Exercise code
			serve(writer, req)

			assert.Equal(t, tt.status, writer.Code)

			if !tt.hasResponseBody {
				data, err := ioutil.ReadAll(writer.Body)
				assert.Empty(t, data)
				assert.Nil(t, err)
				return
			}

			response := responsePayload{}
			data, err := ioutil.ReadAll(writer.Body)
			assert.Nil(t, err)
			err = json.Unmarshal(data, &response)
			assert.Nil(t, err)

			assert.Equal(t, requestUID, response.Response.UID)
			assert.Equal(t, true, response.Response.Allowed)

			if len(tt.patches) == 0 {
				// No JSONPatch in response
				assert.Equal(t, tt.message, response.Response.Status.Message)
				return
			}

			// Assert JSONPatch
			assert.Equal(t, "JSONPatch", response.Response.PatchType)

			// Expect Base64 encoded JSONPatch payload
			decodedPatch, err := base64.StdEncoding.DecodeString(response.Response.Patch)
			assert.Nil(t, err)

			var patchList []jsonPath
			err = json.Unmarshal(decodedPatch, &patchList)
			assert.Nil(t, err)
			assert.Equal(t, len(tt.patches), len(patchList))

			for i, expectedPatch := range tt.patches {
				patch := patchList[i]

				assert.Equal(t, "add", patch.Op)
				assert.Equal(t, expectedPatch.path, patch.Path)
				assert.Equal(t, expectedPatch.value, patch.Value)
			}
		})
	}
}
