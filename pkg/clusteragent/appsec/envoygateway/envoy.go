// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

// Package envoygateway implements the InjectionPattern interface for Envoy Gateway
package envoygateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"

	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	// patchPolicyPrefix is the prefix used for naming our EnvoyPatchPolicy resources.
	// The full name is patchPolicyPrefix + gateway name.
	patchPolicyPrefix = "datadog-appsec-"

	// clusterName is the xDS cluster name used for the ext_proc service
	clusterName = "datadog_appsec_ext_proc_cluster"

	// envoyPatchPolicyCRDName is the CRD name used for detection
	envoyPatchPolicyCRDName = "envoypatchpolicies.gateway.envoyproxy.io"

	// envoyGatewayControllerName is the controller name for Envoy Gateway managed GatewayClasses
	envoyGatewayControllerName = "gateway.envoyproxy.io/gatewayclass-controller"

	// envoyGatewayConfigMapName is the name of the EnvoyGateway configuration ConfigMap
	envoyGatewayConfigMapName = "envoy-gateway-config"

	// envoyGatewayConfigKey is the key in the ConfigMap containing the EnvoyGateway YAML config
	envoyGatewayConfigKey = "envoy-gateway.yaml"

	// old resource names for cleanup
	oldExtensionPolicyName = "datadog-appsec-extproc"
)

var (
	gatewayGVR      = schema.GroupVersionResource{Resource: "gateways", Group: "gateway.networking.k8s.io", Version: "v1"}
	gatewayClassGVR = schema.GroupVersionResource{Resource: "gatewayclasses", Group: "gateway.networking.k8s.io", Version: "v1"}
	patchPolicyGVR  = schema.GroupVersionResource{Resource: "envoypatchpolicies", Group: "gateway.envoyproxy.io", Version: "v1alpha1"}
	crdGVR          = schema.GroupVersionResource{Resource: "customresourcedefinitions", Group: "apiextensions.k8s.io", Version: "v1"}
	configMapGVR    = schema.GroupVersionResource{Resource: "configmaps", Version: "v1"}
	deploymentGVR   = schema.GroupVersionResource{Resource: "deployments", Group: "apps", Version: "v1"}

	// old GVRs for cleanup of resources from previous versions
	oldExtensionGVR      = schema.GroupVersionResource{Resource: "envoyextensionpolicies", Group: "gateway.envoyproxy.io", Version: "v1alpha1"}
	oldReferenceGrantGVR = schema.GroupVersionResource{Resource: "referencegrants", Group: "gateway.networking.k8s.io", Version: "v1beta1"}
)

var _ appsecconfig.InjectionPattern = (*envoyGatewayInjectionPattern)(nil)

type envoyGatewayInjectionPattern struct {
	client dynamic.Interface
	logger log.Component
	config appsecconfig.Config
	eventRecorder

	cleanupOnce sync.Once
}

func (e *envoyGatewayInjectionPattern) Mode() appsecconfig.InjectionMode {
	return e.config.Mode
}

func (e *envoyGatewayInjectionPattern) IsInjectionPossible(ctx context.Context) error {
	gvrToName := func(gvr schema.GroupVersionResource) string {
		return gvr.Resource + "." + gvr.Group
	}

	// Check if the EnvoyPatchPolicy CRD is present
	_, err := e.client.Resource(crdGVR).Get(ctx, gvrToName(patchPolicyGVR), metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return fmt.Errorf("%w: EnvoyPatchPolicy CRD not found, is the Envoy Gateway installed in the cluster? Cannot enable appsec proxy injection for envoy-gateway", err)
	}

	if err != nil {
		return fmt.Errorf("%w: error getting EnvoyPatchPolicy", err)
	}

	// Check if the Gateway CRD is present
	_, err = e.client.Resource(crdGVR).Get(ctx, gvrToName(gatewayGVR), metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return fmt.Errorf("%w: Gateway CRD not found, is the Gateway API installed in the cluster? Cannot enable appsec proxy injection for envoy-gateway", err)
	}

	if err != nil {
		return fmt.Errorf("%w: error getting Gateway", err)
	}

	// Ensure EnvoyPatchPolicy is enabled in the EnvoyGateway configuration.
	// This is best-effort: the user may have pre-configured it manually.
	if err := e.ensureEnvoyPatchPolicyEnabled(ctx); err != nil {
		e.logger.Warnf("Could not automatically enable EnvoyPatchPolicy in EnvoyGateway config: %v. "+
			"Please ensure extensionApis.enableEnvoyPatchPolicy is set to true in the envoy-gateway-config ConfigMap", err)
	}

	return nil
}

// ensureEnvoyPatchPolicyEnabled patches the EnvoyGateway ConfigMap to enable
// the EnvoyPatchPolicy extension API if it's not already enabled.
// This is required for Envoy Gateway to accept and process our EnvoyPatchPolicy resources.
func (e *envoyGatewayInjectionPattern) ensureEnvoyPatchPolicyEnabled(ctx context.Context) error {
	// Find envoy-gateway namespace by looking for the envoy-gateway deployment
	egNamespace, err := e.findEnvoyGatewayNamespace(ctx)
	if err != nil {
		return fmt.Errorf("could not find envoy-gateway namespace: %w", err)
	}

	// Get the current configmap
	cm, err := e.client.Resource(configMapGVR).Namespace(egNamespace).Get(ctx, envoyGatewayConfigMapName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("could not get envoy-gateway-config ConfigMap: %w", err)
	}

	configYAML, found, err := unstructured.NestedString(cm.Object, "data", envoyGatewayConfigKey)
	if err != nil {
		return fmt.Errorf("envoy-gateway.yaml field in ConfigMap has unexpected type: %w", err)
	}
	if !found {
		return fmt.Errorf("key %q not found in envoy-gateway-config ConfigMap data", envoyGatewayConfigKey)
	}

	// Check if enableEnvoyPatchPolicy is already set to true
	if strings.Contains(configYAML, "enableEnvoyPatchPolicy: true") {
		e.logger.Debug("EnvoyPatchPolicy is already enabled in EnvoyGateway config")
		return nil
	}

	// Patch the configmap to add enableEnvoyPatchPolicy: true under extensionApis
	var newConfigYAML string
	if strings.Contains(configYAML, "extensionApis:") {
		// extensionApis section exists, add the field
		newConfigYAML = strings.Replace(configYAML, "extensionApis:", "extensionApis:\n    enableEnvoyPatchPolicy: true", 1)
	} else {
		// No extensionApis section, add it
		newConfigYAML = configYAML + "\nextensionApis:\n    enableEnvoyPatchPolicy: true\n"
	}

	// Use strategic merge patch to update just the data field
	patchData := map[string]any{
		"data": map[string]any{
			envoyGatewayConfigKey: newConfigYAML,
		},
	}
	patchBytes, err := json.Marshal(patchData)
	if err != nil {
		return fmt.Errorf("could not marshal configmap patch: %w", err)
	}

	_, err = e.client.Resource(configMapGVR).Namespace(egNamespace).Patch(
		ctx, envoyGatewayConfigMapName, types.MergePatchType, patchBytes, metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("could not patch envoy-gateway-config ConfigMap: %w", err)
	}

	e.logger.Infof("Enabled EnvoyPatchPolicy in EnvoyGateway config (namespace: %s)", egNamespace)

	// Restart envoy-gateway deployment to pick up the config change
	restartPatch := []byte(`{"spec":{"template":{"metadata":{"annotations":{"appsec.datadoghq.com/restartedAt":"` + metav1.Now().Format("2006-01-02T15:04:05Z") + `"}}}}}`)
	_, err = e.client.Resource(deploymentGVR).Namespace(egNamespace).Patch(
		ctx, "envoy-gateway", types.StrategicMergePatchType, restartPatch, metav1.PatchOptions{},
	)
	if err != nil {
		e.logger.Warnf("Could not restart envoy-gateway deployment to apply config change: %v", err)
		e.logger.Warn("Please restart the envoy-gateway deployment manually for the config change to take effect")
	} else {
		e.logger.Infof("Restarted envoy-gateway deployment to apply EnvoyPatchPolicy config change")
	}

	return nil
}

// findEnvoyGatewayNamespace searches for the namespace where envoy-gateway is deployed
// by looking for the envoy-gateway-config ConfigMap across namespaces.
func (e *envoyGatewayInjectionPattern) findEnvoyGatewayNamespace(ctx context.Context) (string, error) {
	// Common namespaces where envoy-gateway is typically installed
	candidates := []string{"envoy-gateway-system", "envoy-gateway", "default"}
	var forbiddenNamespaces []string
	for _, ns := range candidates {
		_, err := e.client.Resource(configMapGVR).Namespace(ns).Get(ctx, envoyGatewayConfigMapName, metav1.GetOptions{})
		if err == nil {
			return ns, nil
		}
		if k8serrors.IsForbidden(err) {
			forbiddenNamespaces = append(forbiddenNamespaces, ns)
			continue
		}
		if !k8serrors.IsNotFound(err) {
			return "", fmt.Errorf("error checking namespace %s: %w", ns, err)
		}
	}
	if len(forbiddenNamespaces) > 0 {
		return "", fmt.Errorf("envoy-gateway-config ConfigMap not found; access was denied for namespaces %v (check RBAC permissions)", forbiddenNamespaces)
	}
	return "", errors.New("envoy-gateway-config ConfigMap not found in any known namespace")
}

func (e *envoyGatewayInjectionPattern) Resource() schema.GroupVersionResource {
	return gatewayGVR
}

func (e *envoyGatewayInjectionPattern) Namespace() string {
	return v1.NamespaceAll
}

// patchPolicyName returns the name of the EnvoyPatchPolicy for a given gateway
func patchPolicyName(gatewayName string) string {
	return patchPolicyPrefix + gatewayName
}

// isGatewayFromEnvoyGateway checks if a Gateway's GatewayClass is controlled by Envoy Gateway.
// This prevents creating EnvoyPatchPolicies for gateways managed by other controllers (e.g. Istio).
func (e *envoyGatewayInjectionPattern) isGatewayFromEnvoyGateway(ctx context.Context, gateway *unstructured.Unstructured) (bool, error) {
	className, found, err := unstructured.NestedString(gateway.Object, "spec", "gatewayClassName")
	if err != nil || !found {
		return false, fmt.Errorf("could not get gatewayClassName from gateway spec: %w", err)
	}

	gwClass, err := e.client.Resource(gatewayClassGVR).Get(ctx, className, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("could not get GatewayClass %q: %w", className, err)
	}

	controllerName, found, err := unstructured.NestedString(gwClass.Object, "spec", "controllerName")
	if err != nil {
		return false, fmt.Errorf("GatewayClass %q has malformed controllerName field: %w", className, err)
	}
	if !found {
		return false, nil
	}

	return controllerName == envoyGatewayControllerName, nil
}

func (e *envoyGatewayInjectionPattern) Added(ctx context.Context, obj *unstructured.Unstructured) error {
	namespace := obj.GetNamespace()
	name := obj.GetName()

	e.cleanupOnce.Do(func() {
		// Best-effort cleanup of old resources from previous versions
		e.logger.Debug("Cleaning up EnvoyExtensionPolicy and ReferenceGrant resources from previous versions of the cluster-agent")
		e.cleanupOldResources(context.Background(), namespace)
	})

	ok, err := e.isGatewayFromEnvoyGateway(ctx, obj)
	if err != nil {
		return fmt.Errorf("could not determine if gateway %s/%s is managed by Envoy Gateway: %w", namespace, name, err)
	}
	if !ok {
		return nil // Not an Envoy Gateway managed gateway, skip
	}

	e.logger.Debugf("Processing added gateway for envoygateway: %s/%s", namespace, name)

	if err := e.createEnvoyPatchPolicy(ctx, namespace, name, obj); err != nil {
		return fmt.Errorf("could not create EnvoyPatchPolicy: %w", err)
	}

	return nil
}

func (e *envoyGatewayInjectionPattern) Deleted(ctx context.Context, obj *unstructured.Unstructured) error {
	namespace := obj.GetNamespace()
	name := obj.GetName()
	policyName := patchPolicyName(name)

	e.logger.Debugf("Processing deleted gateway for envoygateway: %s/%s", namespace, name)
	_, err := e.client.Resource(patchPolicyGVR).Namespace(namespace).Get(ctx, policyName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		e.logger.Debug("EnvoyPatchPolicy already deleted for gateway ", name)
		return nil
	}

	if err != nil {
		return fmt.Errorf("could not check if EnvoyPatchPolicy was already deleted: %w", err)
	}

	err = e.client.Resource(patchPolicyGVR).
		Namespace(namespace).
		Delete(ctx, policyName, metav1.DeleteOptions{})
	if k8serrors.IsNotFound(err) {
		e.logger.Debug("EnvoyPatchPolicy already deleted")
		err = nil
	}

	if err != nil {
		e.recordPatchPolicyDeleteFailed(namespace, name, err)
	} else {
		e.recordPatchPolicyDeleted(namespace, name)
	}

	return err
}

func (e *envoyGatewayInjectionPattern) createEnvoyPatchPolicy(ctx context.Context, namespace, gatewayName string, gateway *unstructured.Unstructured) error {
	policy, err := e.newPatchPolicy(namespace, gatewayName, gateway)
	if err != nil {
		e.recordPatchPolicyCreateFailed(namespace, gatewayName, err)
		return err
	}

	unstructuredPolicy, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&policy)
	if err != nil {
		wrappedErr := fmt.Errorf("could not convert EnvoyPatchPolicy to unstructured: %w", err)
		e.recordPatchPolicyCreateFailed(namespace, gatewayName, wrappedErr)
		return wrappedErr
	}

	_, err = e.client.Resource(patchPolicyGVR).
		Namespace(namespace).
		Create(ctx, &unstructured.Unstructured{Object: unstructuredPolicy}, metav1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		e.logger.Debug("EnvoyPatchPolicy already exists")
		return nil
	}

	if err != nil {
		e.recordPatchPolicyCreateFailed(namespace, gatewayName, err)
		return err
	}

	e.logger.Infof("EnvoyPatchPolicy created in namespace %s for gateway %s", namespace, gatewayName)
	e.recordPatchPolicyCreated(namespace, gatewayName)

	return nil
}

// newPatchPolicy builds an EnvoyPatchPolicy that patches the envoy xDS config to add
// an ext_proc HTTP filter and a cluster for the processor service.
// This is similar to how the istio package creates an EnvoyFilter with HTTP_FILTER and CLUSTER patches.
func (e *envoyGatewayInjectionPattern) newPatchPolicy(namespace, gatewayName string, gateway *unstructured.Unstructured) (egv1a1.EnvoyPatchPolicy, error) {
	var (
		processorAddress string
		processorPort    int
	)
	switch e.config.Mode {
	case appsecconfig.InjectionModeExternal:
		if e.config.Processor.Address == "" {
			processorAddress = e.config.Processor.ServiceName + "." + e.config.Processor.Namespace + ".svc"
		} else {
			processorAddress = e.config.Processor.Address
		}
		processorPort = e.config.Processor.Port
	default:
		e.logger.Warnf("No injection mode defined, defaults to sending traffic to a sidecar")
		fallthrough
	case appsecconfig.InjectionModeSidecar:
		processorAddress = "127.0.0.1"
		processorPort = e.config.Sidecar.Port
	}

	patches, err := e.buildJSONPatches(namespace, gatewayName, gateway, processorAddress, processorPort)
	if err != nil {
		return egv1a1.EnvoyPatchPolicy{}, err
	}

	return egv1a1.EnvoyPatchPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "gateway.envoyproxy.io/v1alpha1",
			Kind:       "EnvoyPatchPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        patchPolicyName(gatewayName),
			Namespace:   namespace,
			Labels:      e.config.CommonLabels,
			Annotations: e.config.CommonAnnotations,
		},
		Spec: egv1a1.EnvoyPatchPolicySpec{
			Type: egv1a1.JSONPatchEnvoyPatchType,
			TargetRef: gwapiv1a2.LocalPolicyTargetReference{
				Group: "gateway.networking.k8s.io",
				Kind:  "Gateway",
				Name:  gwapiv1a2.ObjectName(gatewayName),
			},
			JSONPatches: patches,
		},
	}, nil
}

// buildJSONPatches creates the JSONPatch operations for the ext_proc filter and cluster.
func (e *envoyGatewayInjectionPattern) buildJSONPatches(namespace, gatewayName string, gateway *unstructured.Unstructured, processorAddress string, processorPort int) ([]egv1a1.EnvoyJSONPatchConfig, error) {
	var patches []egv1a1.EnvoyJSONPatchConfig

	// 1. Add the cluster for the ext_proc service (new resource with empty path)
	clusterPatch, err := buildClusterPatch(processorAddress, processorPort)
	if err != nil {
		return nil, fmt.Errorf("could not build cluster patch: %w", err)
	}
	patches = append(patches, clusterPatch)

	// 2. Add the ext_proc HTTP filter to each listener
	listeners, err := extractListeners(gateway)
	if err != nil {
		return nil, fmt.Errorf("could not extract listeners from gateway: %w", err)
	}

	for _, listener := range listeners {
		listenerName := fmt.Sprintf("%s/%s/%s", namespace, gatewayName, listener.name)
		filterPatch, err := buildHTTPFilterPatch(listenerName, listener.protocol)
		if err != nil {
			return nil, fmt.Errorf("could not build HTTP filter patch for listener %s: %w", listener.name, err)
		}
		patches = append(patches, filterPatch)
	}

	return patches, nil
}

type gatewayListener struct {
	name     string
	protocol string
}

// extractListeners reads the listeners from the Gateway spec
func extractListeners(gateway *unstructured.Unstructured) ([]gatewayListener, error) {
	listenersRaw, found, err := unstructured.NestedSlice(gateway.Object, "spec", "listeners")
	if err != nil {
		return nil, fmt.Errorf("could not get listeners from gateway spec: %w", err)
	}
	if !found || len(listenersRaw) == 0 {
		return nil, errors.New("gateway has no listeners")
	}

	listeners := make([]gatewayListener, 0, len(listenersRaw))
	for _, raw := range listenersRaw {
		l, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		name, _ := l["name"].(string)
		protocol, _ := l["protocol"].(string)
		if name == "" {
			continue
		}
		if protocol == "" {
			protocol = "HTTP"
		}

		listeners = append(listeners, gatewayListener{name: name, protocol: protocol})
	}

	return listeners, nil
}

// filterChainPath returns the JSON pointer path prefix to the filter chain based on the protocol.
// HTTP listeners use default_filter_chain, while TLS/HTTPS listeners use filter_chains.
func filterChainPath(protocol string) string {
	switch protocol {
	case "HTTPS", "TLS":
		return "/filter_chains/0"
	default:
		return "/default_filter_chain"
	}
}

// buildHTTPFilterPatch creates a JSONPatch operation to add the ext_proc filter to a listener.
func buildHTTPFilterPatch(listenerName, protocol string) (egv1a1.EnvoyJSONPatchConfig, error) {
	filterValue := map[string]any{
		"name": "envoy.filters.http.ext_proc",
		"typed_config": map[string]any{
			"@type": "type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor",
			"grpc_service": map[string]any{
				"envoy_grpc": map[string]any{
					"cluster_name": clusterName,
				},
				"initial_metadata": []any{
					map[string]any{"key": "x-datadog-envoy-integration", "value": "1"},
					map[string]any{"key": "x-datadog-appsec-injector", "value": "1"},
				},
			},
			"failure_mode_allow": true,
			"processing_mode": map[string]any{
				"request_header_mode":  "SEND",
				"response_header_mode": "SEND",
			},
			"allow_mode_override": true,
		},
	}

	valueBytes, err := json.Marshal(filterValue)
	if err != nil {
		return egv1a1.EnvoyJSONPatchConfig{}, fmt.Errorf("could not marshal ext_proc filter: %w", err)
	}

	return egv1a1.EnvoyJSONPatchConfig{
		Type: egv1a1.ListenerEnvoyResourceType,
		Name: listenerName,
		Operation: egv1a1.JSONPatchOperation{
			Op:    "add",
			Path:  ptr.To(filterChainPath(protocol) + "/filters/0/typed_config/http_filters/0"),
			Value: &apiextensionsv1.JSON{Raw: valueBytes},
		},
	}, nil
}

// buildClusterPatch creates a JSONPatch operation to add a new cluster for the ext_proc service.
// When path is empty and op is "add", Envoy Gateway treats the value as a whole new xDS resource to add.
func buildClusterPatch(processorAddress string, processorPort int) (egv1a1.EnvoyJSONPatchConfig, error) {
	clusterValue := map[string]any{
		"name":                   clusterName,
		"type":                   "STRICT_DNS",
		"lb_policy":              "ROUND_ROBIN",
		"http2_protocol_options": map[string]any{},
		"load_assignment": map[string]any{
			"cluster_name": clusterName,
			"endpoints": []any{
				map[string]any{
					"lb_endpoints": []any{
						map[string]any{
							"endpoint": map[string]any{
								"address": map[string]any{
									"socket_address": map[string]any{
										"address":    processorAddress,
										"port_value": processorPort,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	valueBytes, err := json.Marshal(clusterValue)
	if err != nil {
		return egv1a1.EnvoyJSONPatchConfig{}, fmt.Errorf("could not marshal cluster config: %w", err)
	}

	return egv1a1.EnvoyJSONPatchConfig{
		Type: egv1a1.ClusterEnvoyResourceType,
		Name: clusterName,
		Operation: egv1a1.JSONPatchOperation{
			Op:    "add",
			Path:  ptr.To(""),
			Value: &apiextensionsv1.JSON{Raw: valueBytes},
		},
	}, nil
}

// cleanupOldResources removes old EnvoyExtensionPolicy and ReferenceGrant resources
// from previous versions that used a different injection mechanism.
// This is best-effort and logs warnings instead of returning errors.
func (e *envoyGatewayInjectionPattern) cleanupOldResources(ctx context.Context, _ string) {
	// Cleanup old EnvoyExtensionPolicy in all namespaces where gateways exist
	oldPolicies, err := e.client.Resource(oldExtensionGVR).Namespace(v1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil && !k8serrors.IsForbidden(err) {
		e.logger.Warnf("Could not list old EnvoyExtensionPolicies for cleanup: %v", err)
	} else if err == nil {
		for _, policy := range oldPolicies.Items {
			delErr := e.client.Resource(oldExtensionGVR).
				Namespace(policy.GetNamespace()).
				Delete(ctx, policy.GetName(), metav1.DeleteOptions{})
			if delErr != nil && !k8serrors.IsNotFound(delErr) {
				e.logger.Warnf("Could not cleanup old EnvoyExtensionPolicy %q in namespace %s: %v", policy.GetName(), policy.GetNamespace(), delErr)
			} else if delErr == nil {
				e.logger.Infof("Cleaned up old EnvoyExtensionPolicy %q in namespace %s", policy.GetName(), policy.GetNamespace())
			}
		}
	}

	// Cleanup old ReferenceGrant in the processor namespace
	oldGrantName := oldExtensionPolicyName
	err = e.client.Resource(oldReferenceGrantGVR).
		Namespace(e.config.Processor.Namespace).
		Delete(ctx, oldGrantName, metav1.DeleteOptions{})
	if k8serrors.IsForbidden(err) {
		e.logger.Warnf("Insufficient permissions to cleanup old ReferenceGrant in namespace %s: %v", e.config.Processor.Namespace, err)
	} else if err != nil && !k8serrors.IsNotFound(err) {
		e.logger.Warnf("Could not cleanup old ReferenceGrant in namespace %s: %v", e.config.Processor.Namespace, err)
	} else if err == nil {
		e.logger.Infof("Cleaned up old ReferenceGrant %q in namespace %s", oldGrantName, e.config.Processor.Namespace)
	}
}

// New returns a new InjectionPattern for Envoy Gateway.
// When the injection mode is SIDECAR, it wraps the pattern with sidecar injection logic.
func New(client dynamic.Interface, logger log.Component, config appsecconfig.Config, eventRecorderInstance record.EventRecorder) appsecconfig.InjectionPattern {
	pattern := &envoyGatewayInjectionPattern{
		client: client,
		logger: logger,
		config: config,
		eventRecorder: eventRecorder{
			recorder: eventRecorderInstance,
		},
	}

	if config.Mode == appsecconfig.InjectionModeSidecar {
		return &envoyGatewaySidecarPattern{envoyGatewayInjectionPattern: pattern}
	}

	return pattern
}
