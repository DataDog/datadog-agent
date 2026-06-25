// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package envoygateway

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/record"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
)

func newTestPatternForEnableBackend(t *testing.T, fakeRecorder *record.FakeRecorder) (*envoyGatewayInjectionPattern, *dynamicfake.FakeDynamicClient) {
	t.Helper()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			configMapGVR: "ConfigMapList",
		},
	)
	eventRec := eventRecorder{recorder: fakeRecorder}
	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{
				ServiceName: "appsec-processor",
				Namespace:   "datadog",
				Port:        8080,
			},
		},
	}
	pattern := &envoyGatewayInjectionPattern{
		client:        client,
		logger:        logger,
		config:        config,
		eventRecorder: eventRec,
		grantManager: grantManager{
			client:        client,
			logger:        logger,
			eventRecorder: eventRec,
			serviceName:   config.Processor.ServiceName,
			namespace:     config.Processor.Namespace,
		},
	}
	return pattern, client
}

func seedConfigMap(t *testing.T, client *dynamicfake.FakeDynamicClient, namespace, yamlContent string) {
	t.Helper()
	ctx := context.Background()
	cm := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      envoyGatewayConfigMapName,
				"namespace": namespace,
			},
			"data": map[string]any{
				envoyGatewayConfigDataKey: yamlContent,
			},
		},
	}
	_, err := client.Resource(configMapGVR).Namespace(namespace).Create(ctx, cm, metav1.CreateOptions{})
	require.NoError(t, err)
}

func TestIsBackendExtensionEnabled_TrueWithExtraKeys(t *testing.T) {
	ctx := context.Background()
	fakeRecorder := record.NewFakeRecorder(10)
	pattern, client := newTestPatternForEnableBackend(t, fakeRecorder)

	yaml := `
gateway:
  controllerName: example.com/gateway
provider:
  type: Kubernetes
logging:
  level:
    default: info
extensionApis:
  enableBackend: true
`
	seedConfigMap(t, client, envoyGatewaySystemNamespace, yaml)

	enabled, found, err := pattern.isBackendExtensionEnabled(ctx, envoyGatewaySystemNamespace)
	require.NoError(t, err)
	assert.True(t, found)
	assert.True(t, enabled)
}

func TestWarnIfBackendDisabled_NoeventWhenEnabled(t *testing.T) {
	ctx := context.Background()
	fakeRecorder := record.NewFakeRecorder(10)
	pattern, client := newTestPatternForEnableBackend(t, fakeRecorder)

	yaml := `
gateway:
  controllerName: example.com/gateway
extensionApis:
  enableBackend: true
`
	seedConfigMap(t, client, envoyGatewaySystemNamespace, yaml)

	pattern.warnIfBackendDisabled(ctx, envoyGatewaySystemNamespace)

	assert.Empty(t, fakeRecorder.Events, "no event must be recorded when enableBackend is true")
}

func TestIsBackendExtensionEnabled_FalseWhenDisabled(t *testing.T) {
	ctx := context.Background()
	fakeRecorder := record.NewFakeRecorder(10)
	pattern, client := newTestPatternForEnableBackend(t, fakeRecorder)

	yaml := `
extensionApis:
  enableBackend: false
`
	seedConfigMap(t, client, envoyGatewaySystemNamespace, yaml)

	enabled, found, err := pattern.isBackendExtensionEnabled(ctx, envoyGatewaySystemNamespace)
	require.NoError(t, err)
	assert.True(t, found)
	assert.False(t, enabled)
}

func TestIsBackendExtensionEnabled_MalformedYAMLReturnsError(t *testing.T) {
	ctx := context.Background()
	fakeRecorder := record.NewFakeRecorder(10)
	pattern, client := newTestPatternForEnableBackend(t, fakeRecorder)
	seedConfigMap(t, client, envoyGatewaySystemNamespace, "extensionApis: [")

	enabled, found, err := pattern.isBackendExtensionEnabled(ctx, envoyGatewaySystemNamespace)
	require.Error(t, err)
	assert.True(t, found)
	assert.False(t, enabled)
}

func TestIsBackendExtensionEnabled_MalformedDataValueReturnsError(t *testing.T) {
	ctx := context.Background()
	fakeRecorder := record.NewFakeRecorder(10)
	pattern, client := newTestPatternForEnableBackend(t, fakeRecorder)
	cm := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":      envoyGatewayConfigMapName,
			"namespace": envoyGatewaySystemNamespace,
		},
		"data": map[string]any{
			envoyGatewayConfigDataKey: []any{"not", "a", "string"},
		},
	}}
	_, createErr := client.Resource(configMapGVR).Namespace(envoyGatewaySystemNamespace).Create(ctx, cm, metav1.CreateOptions{})
	require.NoError(t, createErr)

	enabled, found, err := pattern.isBackendExtensionEnabled(ctx, envoyGatewaySystemNamespace)
	require.Error(t, err)
	assert.True(t, found)
	assert.False(t, enabled)
}

func TestWarnIfBackendDisabled_WarningEventWhenFalse(t *testing.T) {
	ctx := context.Background()
	fakeRecorder := record.NewFakeRecorder(10)
	pattern, client := newTestPatternForEnableBackend(t, fakeRecorder)

	yaml := `
extensionApis:
  enableBackend: false
`
	seedConfigMap(t, client, envoyGatewaySystemNamespace, yaml)

	pattern.warnIfBackendDisabled(ctx, envoyGatewaySystemNamespace)

	require.Len(t, fakeRecorder.Events, 1, "exactly one warning event must be recorded")
	evt := <-fakeRecorder.Events
	assert.True(t, strings.Contains(evt, EventReasonBackendExtensionDisabled), "event must contain the reason string")
}

func TestIsBackendExtensionEnabled_AbsentConfigMap(t *testing.T) {
	ctx := context.Background()
	fakeRecorder := record.NewFakeRecorder(10)
	pattern, _ := newTestPatternForEnableBackend(t, fakeRecorder)

	enabled, found, err := pattern.isBackendExtensionEnabled(ctx, envoyGatewaySystemNamespace)
	require.NoError(t, err)
	assert.False(t, found)
	assert.False(t, enabled)
}

func TestWarnIfBackendDisabled_WarningEventWhenAbsent(t *testing.T) {
	ctx := context.Background()
	fakeRecorder := record.NewFakeRecorder(10)
	pattern, _ := newTestPatternForEnableBackend(t, fakeRecorder)

	pattern.warnIfBackendDisabled(ctx, envoyGatewaySystemNamespace)

	require.Len(t, fakeRecorder.Events, 1, "exactly one warning event must be recorded when CM is absent")
	evt := <-fakeRecorder.Events
	assert.True(t, strings.Contains(evt, EventReasonBackendExtensionDisabled), "event must contain the reason string")
}
