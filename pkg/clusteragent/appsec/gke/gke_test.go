// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package gke

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

const (
	testGatewayClass = "gke-l7-global-external-managed"
	testServiceName  = "appsec-processor"
	testServicePort  = 8080
)

func newTestGKEPattern(_ *testing.T, client dynamic.Interface, logger log.Component, config appsecconfig.Config) (*gkeGatewayInjectionPattern, *record.FakeRecorder) {
	recorder := record.NewFakeRecorder(100)
	return &gkeGatewayInjectionPattern{
		client:        client,
		logger:        logger,
		config:        config,
		eventRecorder: eventRecorder{recorder: recorder},
	}, recorder
}

func newTestGateway(namespace string, name string, gatewayClass string) *unstructured.Unstructured {
	gateway := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "Gateway",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]any{
			"gatewayClassName": gatewayClass,
		},
	}}
	return gateway
}

func newTestCRD() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata": map[string]any{
			"name": gcpTrafficExtensionCRDName,
		},
	}}
}

func newTestGCPTrafficExtension(namespace string, gatewayName string, labels map[string]string) *unstructured.Unstructured {
	extension := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "networking.gke.io/v1",
		"kind":       "GCPTrafficExtension",
		"metadata": map[string]any{
			"name":      extensionName(gatewayName),
			"namespace": namespace,
		},
		"spec": map[string]any{
			"sentinel": "original",
		},
	}}
	extension.SetLabels(labels)
	return extension
}

func gkeListKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		gatewayGVR:          "GatewayList",
		trafficExtensionGVR: "GCPTrafficExtensionList",
		crdGVR:              "CustomResourceDefinitionList",
	}
}

func defaultGKEConfig() appsecconfig.Config {
	return appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{
				ServiceName: testServiceName,
				Namespace:   "ignored-by-gke",
				Port:        testServicePort,
			},
			GKE: appsecconfig.GKE{
				GatewayClasses: []string{"gke-l7-global-external-managed", "gke-l7-regional-external-managed"},
			},
		},
		Injection: appsecconfig.Injection{
			CommonLabels:      map[string]string{"app": "datadog"},
			CommonAnnotations: map[string]string{"managed-by": "datadog"},
		},
	}
}

func getExtension(t *testing.T, client dynamic.Interface, namespace string, gatewayName string) *unstructured.Unstructured {
	t.Helper()
	extension, err := client.Resource(trafficExtensionGVR).Namespace(namespace).Get(context.Background(), extensionName(gatewayName), metav1.GetOptions{})
	require.NoError(t, err)
	return extension
}

func requireEventReasons(t *testing.T, recorder *record.FakeRecorder, want ...string) {
	t.Helper()
	events := drainEvents(recorder)
	reasons := make([]string, 0, len(events))
	for _, event := range events {
		fields := strings.Fields(event)
		require.GreaterOrEqual(t, len(fields), 2, "unexpected fake recorder event format: %q", event)
		reasons = append(reasons, fields[1])
	}
	require.Equal(t, want, reasons, "events: %v", events)
}

func requireNoEvents(t *testing.T, recorder *record.FakeRecorder) {
	t.Helper()
	require.Empty(t, drainEvents(recorder))
}

func drainEvents(recorder *record.FakeRecorder) []string {
	events := make([]string, 0, len(recorder.Events))
	for len(recorder.Events) > 0 {
		events = append(events, <-recorder.Events)
	}
	return events
}

func requireNoExtensions(t *testing.T, client dynamic.Interface, namespace string) {
	t.Helper()
	list, err := client.Resource(trafficExtensionGVR).Namespace(namespace).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Empty(t, list.Items)
}

func requireExtensionNotFound(t *testing.T, client dynamic.Interface, namespace string, gatewayName string) {
	t.Helper()
	_, err := client.Resource(trafficExtensionGVR).Namespace(namespace).Get(context.Background(), extensionName(gatewayName), metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err), "expected GCPTrafficExtension %s/%s to be absent, got %v", namespace, extensionName(gatewayName), err)
}

func requireSingleExtensionObjectEqual(t *testing.T, client dynamic.Interface, namespace string, want map[string]any) {
	t.Helper()
	list, err := client.Resource(trafficExtensionGVR).Namespace(namespace).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	require.Equal(t, want, list.Items[0].Object)
}

func TestAdded_createsGCPTrafficExtension_whenGatewayClassIsSupported(t *testing.T) {
	// Given
	ctx := context.Background()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gkeListKinds())
	pattern, recorder := newTestGKEPattern(t, client, logmock.New(t), defaultGKEConfig())
	gateway := newTestGateway("test-ns", "test-gateway", testGatewayClass)

	// When
	err := pattern.Added(ctx, gateway)

	// Then
	require.NoError(t, err)
	extension := getExtension(t, client, "test-ns", "test-gateway")
	require.Equal(t, "networking.gke.io/v1", extension.GetAPIVersion())
	require.Equal(t, "GCPTrafficExtension", extension.GetKind())
	expectedLabels := map[string]string{"app": "datadog"}
	expectedLabels[kubernetes.KubeAppManagedByLabelKey] = appsecconfig.ManagedByLabelValue
	require.Equal(t, expectedLabels, extension.GetLabels())
	require.Equal(t, map[string]string{"managed-by": "datadog"}, extension.GetAnnotations())

	targetRefs, found, err := unstructured.NestedSlice(extension.Object, "spec", "targetRefs")
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, targetRefs, 1)
	targetRef := targetRefs[0].(map[string]any)
	require.Equal(t, "gateway.networking.k8s.io", targetRef["group"])
	require.Equal(t, "Gateway", targetRef["kind"])
	require.Equal(t, "test-gateway", targetRef["name"])
	require.NotContains(t, targetRef, "namespace")

	chains, found, err := unstructured.NestedSlice(extension.Object, "spec", "extensionChains")
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, chains, 1)
	chain := chains[0].(map[string]any)
	require.Equal(t, "datadog-aap-chain", chain["name"])
	celExpressions, found, err := unstructured.NestedSlice(chain, "matchCondition", "celExpressions")
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, celExpressions, 1)
	require.Equal(t, "1 == 1", celExpressions[0].(map[string]any)["celMatcher"])

	extensions := chain["extensions"].([]any)
	require.Len(t, extensions, 1)
	extensionSpec := extensions[0].(map[string]any)
	require.Equal(t, "datadog-aap-extension", extensionSpec["name"])
	require.Equal(t, "appsec-processor.test-ns.svc.cluster.local", extensionSpec["authority"])
	require.Equal(t, true, extensionSpec["failOpen"])
	require.Equal(t, []any{"RequestHeaders", "ResponseHeaders"}, extensionSpec["supportedEvents"])
	require.Equal(t, "1s", extensionSpec["timeout"])
	backendRef := extensionSpec["backendRef"].(map[string]any)
	require.Equal(t, "", backendRef["group"])
	require.Equal(t, "Service", backendRef["kind"])
	require.Equal(t, testServiceName, backendRef["name"])
	require.EqualValues(t, testServicePort, backendRef["port"])
	require.NotContains(t, backendRef, "namespace")
	requireEventReasons(t, recorder, EventReasonGCPTrafficExtensionCreated)
}

func TestAdded_isIdempotent_whenExtensionAlreadyExists(t *testing.T) {
	// Given
	ctx := context.Background()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gkeListKinds())
	pattern, recorder := newTestGKEPattern(t, client, logmock.New(t), defaultGKEConfig())
	gateway := newTestGateway("test-ns", "test-gateway", testGatewayClass)

	// When
	require.NoError(t, pattern.Added(ctx, gateway))
	before := getExtension(t, client, "test-ns", "test-gateway").DeepCopy()
	requireEventReasons(t, recorder, EventReasonGCPTrafficExtensionCreated)
	err := pattern.Added(ctx, gateway)

	// Then
	require.NoError(t, err)
	list, err := client.Resource(trafficExtensionGVR).Namespace("test-ns").List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	require.Equal(t, before.Object, getExtension(t, client, "test-ns", "test-gateway").Object)
	requireNoEvents(t, recorder)
}

func TestAdded_skipsGateway_whenClassIsEmptyOrUnsupported(t *testing.T) {
	tests := []struct {
		name         string
		gatewayClass string
	}{
		{name: "empty class", gatewayClass: ""},
		{name: "unsupported class", gatewayClass: "istio"},
		{name: "multi-cluster class excluded from default allowlist", gatewayClass: "gke-l7-global-external-managed-mc"},
		{name: "regional multi-cluster class excluded from default allowlist", gatewayClass: "gke-l7-regional-external-managed-mc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			ctx := context.Background()
			client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gkeListKinds())
			pattern, recorder := newTestGKEPattern(t, client, logmock.New(t), defaultGKEConfig())

			// When
			err := pattern.Added(ctx, newTestGateway("test-ns", "test-gateway", tt.gatewayClass))

			// Then
			require.NoError(t, err)
			requireNoExtensions(t, client, "test-ns")
			requireNoEvents(t, recorder)
		})
	}
}

func TestDeleted_removesManagedExtension_andIsNotFoundSafe(t *testing.T) {
	// Given
	ctx := context.Background()
	gateway := newTestGateway("test-ns", "test-gateway", "istio")
	extension := newTestGCPTrafficExtension("test-ns", "test-gateway", map[string]string{kubernetes.KubeAppManagedByLabelKey: appsecconfig.ManagedByLabelValue})
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gkeListKinds(), extension)
	pattern, recorder := newTestGKEPattern(t, client, logmock.New(t), defaultGKEConfig())

	// When
	err := pattern.Deleted(ctx, gateway)
	secondErr := pattern.Deleted(ctx, gateway)

	// Then
	require.NoError(t, err)
	require.NoError(t, secondErr)
	requireExtensionNotFound(t, client, "test-ns", "test-gateway")
	requireEventReasons(t, recorder, EventReasonGCPTrafficExtensionDeleted)
}

func TestMode_alwaysReturnsExternal(t *testing.T) {
	// Given
	config := defaultGKEConfig()
	config.Mode = appsecconfig.InjectionModeSidecar
	pattern, _ := newTestGKEPattern(t, dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()), logmock.New(t), config)

	// When / Then
	require.Equal(t, appsecconfig.InjectionModeExternal, pattern.Mode())
}

func TestIsInjectionPossible_returnsError_whenConfigurationOrCRDIsInvalid(t *testing.T) {
	tests := []struct {
		name       string
		config     appsecconfig.Config
		setup      func(*testing.T, *dynamicfake.FakeDynamicClient)
		wantErrSub string
	}{
		{name: "missing processor service name", config: func() appsecconfig.Config { c := defaultGKEConfig(); c.Processor.ServiceName = ""; return c }(), setup: func(t *testing.T, client *dynamicfake.FakeDynamicClient) {
			_, err := client.Resource(crdGVR).Create(context.Background(), newTestCRD(), metav1.CreateOptions{})
			require.NoError(t, err)
		}, wantErrSub: "processor service name"},
		{name: "zero processor port", config: func() appsecconfig.Config { c := defaultGKEConfig(); c.Processor.Port = 0; return c }(), setup: func(t *testing.T, client *dynamicfake.FakeDynamicClient) {
			_, err := client.Resource(crdGVR).Create(context.Background(), newTestCRD(), metav1.CreateOptions{})
			require.NoError(t, err)
		}, wantErrSub: "processor port must be positive"},
		{name: "negative processor port", config: func() appsecconfig.Config { c := defaultGKEConfig(); c.Processor.Port = -1; return c }(), setup: func(t *testing.T, client *dynamicfake.FakeDynamicClient) {
			_, err := client.Resource(crdGVR).Create(context.Background(), newTestCRD(), metav1.CreateOptions{})
			require.NoError(t, err)
		}, wantErrSub: "processor port must be positive"},
		{name: "CRD absent", config: defaultGKEConfig(), wantErrSub: "GCPTrafficExtension CRD not found"},
		{name: "CRD API error", config: defaultGKEConfig(), setup: func(_ *testing.T, client *dynamicfake.FakeDynamicClient) {
			client.PrependReactor("get", "customresourcedefinitions", func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, apierrors.NewInternalError(errors.New("internal server error"))
			})
		}, wantErrSub: "error getting GCPTrafficExtension CRD"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gkeListKinds())
			if tt.setup != nil {
				tt.setup(t, client)
			}
			pattern, _ := newTestGKEPattern(t, client, logmock.New(t), tt.config)

			// When
			err := pattern.IsInjectionPossible(context.Background())

			// Then
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErrSub)
		})
	}
}

func TestIsInjectionPossible_returnsNil_whenConfigurationAndCRDAreValid(t *testing.T) {
	// Given
	ctx := context.Background()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gkeListKinds(), newTestCRD())
	pattern, _ := newTestGKEPattern(t, client, logmock.New(t), defaultGKEConfig())

	// When
	err := pattern.IsInjectionPossible(ctx)

	// Then
	require.NoError(t, err)
}

func TestExtensionName_isDeterministicAndDNSLabelSafe(t *testing.T) {
	// Given
	maxPassthroughGatewayName := strings.Repeat("a", 63-len(extensionNamePrefix))
	longName := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	// When / Then
	require.Equal(t, "datadog-appsec-short-gateway", extensionName("short-gateway"))
	require.Equal(t, extensionNamePrefix+maxPassthroughGatewayName, extensionName(maxPassthroughGatewayName))
	longExtensionName := extensionName(longName)
	require.Equal(t, "datadog-appsec-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-11ee3912", longExtensionName)
	require.Equal(t, longExtensionName, extensionName(longName))
	require.LessOrEqual(t, len(longExtensionName), 63)
	require.Regexp(t, `^d.*[a-z0-9]$`, longExtensionName)
}

func TestAdded_createsDistinctExtensions_whenTwoGatewaysShareNamespace(t *testing.T) {
	// Given
	ctx := context.Background()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gkeListKinds())
	pattern, recorder := newTestGKEPattern(t, client, logmock.New(t), defaultGKEConfig())

	// When
	require.NoError(t, pattern.Added(ctx, newTestGateway("test-ns", "gateway-one", testGatewayClass)))
	require.NoError(t, pattern.Added(ctx, newTestGateway("test-ns", "gateway-two", testGatewayClass)))

	// Then
	list, err := client.Resource(trafficExtensionGVR).Namespace("test-ns").List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, list.Items, 2)
	require.NotEqual(t, list.Items[0].GetName(), list.Items[1].GetName())
	requireEventReasons(t, recorder, EventReasonGCPTrafficExtensionCreated, EventReasonGCPTrafficExtensionCreated)
}

func TestAdded_skipsExistingManagedExtension_withoutOverwriting(t *testing.T) {
	// Given
	ctx := context.Background()
	existing := newTestGCPTrafficExtension("test-ns", "test-gateway", map[string]string{kubernetes.KubeAppManagedByLabelKey: appsecconfig.ManagedByLabelValue})
	existing.SetAnnotations(map[string]string{"keep": "me"})
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gkeListKinds(), existing)
	pattern, recorder := newTestGKEPattern(t, client, logmock.New(t), defaultGKEConfig())

	// When
	err := pattern.Added(ctx, newTestGateway("test-ns", "test-gateway", testGatewayClass))

	// Then
	require.NoError(t, err)
	list, err := client.Resource(trafficExtensionGVR).Namespace("test-ns").List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	require.Equal(t, "me", list.Items[0].GetAnnotations()["keep"])
	requireNoEvents(t, recorder)
}

func TestAdded_skipsForeignExtension_withoutOverwriting(t *testing.T) {
	// Given
	ctx := context.Background()
	existing := newTestGCPTrafficExtension("test-ns", "test-gateway", map[string]string{"owner": "someone-else"})
	before := existing.DeepCopy()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gkeListKinds(), existing)
	pattern, recorder := newTestGKEPattern(t, client, logmock.New(t), defaultGKEConfig())

	// When
	err := pattern.Added(ctx, newTestGateway("test-ns", "test-gateway", testGatewayClass))

	// Then
	require.NoError(t, err)
	after := getExtension(t, client, "test-ns", "test-gateway")
	require.Equal(t, before.Object, after.Object)
	requireNoEvents(t, recorder)
}

func TestDeleted_skipsForeignExtension_withoutDeleting(t *testing.T) {
	// Given
	ctx := context.Background()
	existing := newTestGCPTrafficExtension("test-ns", "test-gateway", map[string]string{"owner": "someone-else"})
	before := existing.DeepCopy()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gkeListKinds(), existing)
	pattern, recorder := newTestGKEPattern(t, client, logmock.New(t), defaultGKEConfig())

	// When
	err := pattern.Deleted(ctx, newTestGateway("test-ns", "test-gateway", ""))

	// Then
	require.NoError(t, err)
	after := getExtension(t, client, "test-ns", "test-gateway")
	require.Equal(t, before.Object, after.Object)
	requireNoEvents(t, recorder)
}

func TestAdded_createsManagedExtension_whenCommonLabelsAreNil(t *testing.T) {
	// Given
	ctx := context.Background()
	config := defaultGKEConfig()
	config.CommonLabels = nil
	config.CommonAnnotations = nil
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gkeListKinds())
	pattern, _ := newTestGKEPattern(t, client, logmock.New(t), config)

	// When
	err := pattern.Added(ctx, newTestGateway("test-ns", "test-gateway", testGatewayClass))

	// Then
	require.NoError(t, err)
	extension := getExtension(t, client, "test-ns", "test-gateway")
	require.True(t, appsecconfig.IsManagedByDatadog(extension.GetLabels()))
	require.Equal(t, appsecconfig.ManagedByLabelValue, extension.GetLabels()[kubernetes.KubeAppManagedByLabelKey])
}

func TestAdded_returnsNilAndRecordsNoCreateFailedEvent_whenCreateAlreadyExists(t *testing.T) {
	// Given
	ctx := context.Background()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gkeListKinds())
	client.PrependReactor("create", "gcptrafficextensions", func(action k8stesting.Action) (bool, runtime.Object, error) {
		created := action.(k8stesting.CreateAction).GetObject().(*unstructured.Unstructured)
		return true, nil, apierrors.NewAlreadyExists(schema.GroupResource{Group: trafficExtensionGVR.Group, Resource: trafficExtensionGVR.Resource}, created.GetName())
	})
	pattern, recorder := newTestGKEPattern(t, client, logmock.New(t), defaultGKEConfig())

	// When
	err := pattern.Added(ctx, newTestGateway("test-ns", "test-gateway", testGatewayClass))

	// Then
	require.NoError(t, err)
	requireNoExtensions(t, client, "test-ns")
	requireNoEvents(t, recorder)
}

func TestAdded_skipsForeignExtensionAndRecordsNoEvent_whenCreateAlreadyExistsRaceRevealsForeignObject(t *testing.T) {
	// Given
	ctx := context.Background()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gkeListKinds())
	foreign := newTestGCPTrafficExtension("test-ns", "test-gateway", map[string]string{"owner": "someone-else"})
	before := foreign.DeepCopy()
	getCalls := 0
	client.PrependReactor("get", "gcptrafficextensions", func(action k8stesting.Action) (bool, runtime.Object, error) {
		getCalls++
		if getCalls == 1 {
			return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: trafficExtensionGVR.Group, Resource: trafficExtensionGVR.Resource}, action.(k8stesting.GetAction).GetName())
		}
		return true, foreign.DeepCopy(), nil
	})
	client.PrependReactor("create", "gcptrafficextensions", func(action k8stesting.Action) (bool, runtime.Object, error) {
		created := action.(k8stesting.CreateAction).GetObject().(*unstructured.Unstructured)
		return true, nil, apierrors.NewAlreadyExists(schema.GroupResource{Group: trafficExtensionGVR.Group, Resource: trafficExtensionGVR.Resource}, created.GetName())
	})
	pattern, recorder := newTestGKEPattern(t, client, logmock.New(t), defaultGKEConfig())

	// When
	err := pattern.Added(ctx, newTestGateway("test-ns", "test-gateway", testGatewayClass))

	// Then
	require.NoError(t, err)
	require.Equal(t, 2, getCalls)
	after := getExtension(t, client, "test-ns", "test-gateway")
	require.Equal(t, before.Object, after.Object)
	requireNoEvents(t, recorder)
}

func TestAdded_returnsErrorAndRecordsEvent_whenGetOrCreateFails(t *testing.T) {
	tests := []struct {
		name    string
		reactor func(*dynamicfake.FakeDynamicClient)
	}{
		{name: "forbidden get", reactor: func(client *dynamicfake.FakeDynamicClient) {
			client.PrependReactor("get", "gcptrafficextensions", func(action k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: trafficExtensionGVR.Group, Resource: trafficExtensionGVR.Resource}, action.(k8stesting.GetAction).GetName(), errors.New("forbidden"))
			})
		}},
		{name: "internal get", reactor: func(client *dynamicfake.FakeDynamicClient) {
			client.PrependReactor("get", "gcptrafficextensions", func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, apierrors.NewInternalError(errors.New("internal server error"))
			})
		}},
		{name: "forbidden create", reactor: func(client *dynamicfake.FakeDynamicClient) {
			client.PrependReactor("create", "gcptrafficextensions", func(action k8stesting.Action) (bool, runtime.Object, error) {
				created := action.(k8stesting.CreateAction).GetObject().(*unstructured.Unstructured)
				return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: trafficExtensionGVR.Group, Resource: trafficExtensionGVR.Resource}, created.GetName(), errors.New("forbidden"))
			})
		}},
		{name: "internal create", reactor: func(client *dynamicfake.FakeDynamicClient) {
			client.PrependReactor("create", "gcptrafficextensions", func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, apierrors.NewInternalError(errors.New("internal server error"))
			})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			ctx := context.Background()
			client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gkeListKinds())
			tt.reactor(client)
			pattern, recorder := newTestGKEPattern(t, client, logmock.New(t), defaultGKEConfig())

			// When
			err := pattern.Added(ctx, newTestGateway("test-ns", "test-gateway", testGatewayClass))

			// Then
			require.Error(t, err)
			require.Contains(t, err.Error(), "GCPTrafficExtension")
			requireNoExtensions(t, client, "test-ns")
			requireEventReasons(t, recorder, EventReasonGCPTrafficExtensionCreateFailed)
		})
	}
}

func TestDeleted_returnsErrorAndRecordsEvent_whenGetOrDeleteFails(t *testing.T) {
	tests := []struct {
		name    string
		reactor func(*dynamicfake.FakeDynamicClient)
	}{
		{name: "forbidden get", reactor: func(client *dynamicfake.FakeDynamicClient) {
			client.PrependReactor("get", "gcptrafficextensions", func(action k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: trafficExtensionGVR.Group, Resource: trafficExtensionGVR.Resource}, action.(k8stesting.GetAction).GetName(), errors.New("forbidden"))
			})
		}},
		{name: "internal get", reactor: func(client *dynamicfake.FakeDynamicClient) {
			client.PrependReactor("get", "gcptrafficextensions", func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, apierrors.NewInternalError(errors.New("internal server error"))
			})
		}},
		{name: "forbidden delete", reactor: func(client *dynamicfake.FakeDynamicClient) {
			client.PrependReactor("delete", "gcptrafficextensions", func(action k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: trafficExtensionGVR.Group, Resource: trafficExtensionGVR.Resource}, action.(k8stesting.DeleteAction).GetName(), errors.New("forbidden"))
			})
		}},
		{name: "internal delete", reactor: func(client *dynamicfake.FakeDynamicClient) {
			client.PrependReactor("delete", "gcptrafficextensions", func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, apierrors.NewInternalError(errors.New("internal server error"))
			})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			ctx := context.Background()
			existing := newTestGCPTrafficExtension("test-ns", "test-gateway", map[string]string{kubernetes.KubeAppManagedByLabelKey: appsecconfig.ManagedByLabelValue})
			client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gkeListKinds(), existing)
			tt.reactor(client)
			pattern, recorder := newTestGKEPattern(t, client, logmock.New(t), defaultGKEConfig())

			// When
			err := pattern.Deleted(ctx, newTestGateway("test-ns", "test-gateway", ""))

			// Then
			require.Error(t, err)
			require.Contains(t, err.Error(), "GCPTrafficExtension")
			requireSingleExtensionObjectEqual(t, client, "test-ns", existing.Object)
			requireEventReasons(t, recorder, EventReasonGCPTrafficExtensionDeleteFailed)
		})
	}
}
