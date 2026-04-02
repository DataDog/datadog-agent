// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package admission

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admiv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	corelisters "k8s.io/client-go/listers/core/v1"

	admicommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

func TestIsProbe(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected bool
	}{
		{
			name:     "object with probe label",
			raw:      `{"metadata":{"labels":{"` + admicommon.ProbeLabelKey + `":"true"}}}`,
			expected: true,
		},
		{
			name:     "object without probe label",
			raw:      `{"metadata":{"labels":{"app":"nginx"}}}`,
			expected: false,
		},
		{
			name:     "object with no labels",
			raw:      `{"metadata":{}}`,
			expected: false,
		},
		{
			name:     "invalid JSON",
			raw:      `not json`,
			expected: false,
		},
		{
			name:     "empty object",
			raw:      `{}`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isProbe([]byte(tt.raw)))
		})
	}
}

func TestProbeResponse_NonProbeObject(t *testing.T) {
	raw := []byte(`{"metadata":{"labels":{"app":"nginx"}}}`)
	resp := probeResponse(raw)
	assert.Nil(t, resp)
}

func TestProbeResponse_ProbeObject(t *testing.T) {
	raw := []byte(`{"metadata":{"labels":{"` + admicommon.ProbeLabelKey + `":"true"}}}`)
	resp := probeResponse(raw)
	require.NotNil(t, resp)

	assert.True(t, resp.Allowed)
	assert.NotNil(t, resp.PatchType)
	assert.Equal(t, admiv1.PatchTypeJSONPatch, *resp.PatchType)

	var patch []map[string]interface{}
	err := json.Unmarshal(resp.Patch, &patch)
	require.NoError(t, err)
	require.Len(t, patch, 1)
	assert.Equal(t, "add", patch[0]["op"])
	assert.Equal(t, "/metadata/annotations", patch[0]["path"])

	annotations, ok := patch[0]["value"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "true", annotations[admicommon.ProbeReceivedAnnotationKey])
}

// fakeSecretLister is a minimal SecretLister implementation for testing.
type fakeSecretLister struct{}

func (f *fakeSecretLister) List(_ labels.Selector) ([]*corev1.Secret, error) { return nil, nil }
func (f *fakeSecretLister) Secrets(_ string) corelisters.SecretNamespaceLister {
	return &fakeSecretNamespaceLister{}
}

type fakeSecretNamespaceLister struct{}

func (f *fakeSecretNamespaceLister) List(_ labels.Selector) ([]*corev1.Secret, error) {
	return nil, nil
}
func (f *fakeSecretNamespaceLister) Get(_ string) (*corev1.Secret, error) { return nil, nil }

func TestRunRegistersReadinessCheck(t *testing.T) {
	configmock.New(t)

	server := NewServer(&fakeSecretLister{})

	ctx, cancel := context.WithCancel(context.Background())

	// Verify that the admission controller webhook is NOT in the readiness status before Run.
	readyBefore := health.GetReady()
	for _, name := range readyBefore.Healthy {
		assert.NotEqual(t, "admission-controller-webhook", name)
	}
	for _, name := range readyBefore.Unhealthy {
		assert.NotEqual(t, "admission-controller-webhook", name)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx)
	}()

	// Give the server time to start and register the health check.
	time.Sleep(200 * time.Millisecond)

	// Verify that the admission controller webhook IS registered in the readiness status.
	readyAfter := health.GetReady()
	found := false
	for _, name := range readyAfter.Healthy {
		if name == "admission-controller-webhook" {
			found = true
			break
		}
	}
	for _, name := range readyAfter.Unhealthy {
		if name == "admission-controller-webhook" {
			found = true
			break
		}
	}
	assert.True(t, found, "admission-controller-webhook should be registered in readiness checks")

	// Cancel and verify clean shutdown.
	cancel()
	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down in time")
	}

	// Verify deregistration after shutdown.
	readyFinal := health.GetReady()
	for _, name := range readyFinal.Healthy {
		assert.NotEqual(t, "admission-controller-webhook", name)
	}
	for _, name := range readyFinal.Unhealthy {
		assert.NotEqual(t, "admission-controller-webhook", name)
	}
}
