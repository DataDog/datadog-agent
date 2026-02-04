// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

// Package istio implements the InjectionPattern interface for Istio
package istio

import (
	"context"
	"errors"
	"fmt"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"

	"google.golang.org/protobuf/types/known/structpb"
	istionetworkingv1alpha3 "istio.io/api/networking/v1alpha3"
	istiov1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/record"
)

const (
	envoyFilterName                    = "datadog-appsec-envoyfilter"
	gatewayGatewayNetworkingIstioIOCRD = "gateways.networking.istio.io"

	istioGatewayControllerName = "istio.io/gateway-controller"
)

var (
	gatewayClassGVR = schema.GroupVersionResource{Resource: "gatewayclasses", Group: "gateway.networking.k8s.io", Version: "v1"}
	filterGVR       = schema.GroupVersionResource{Resource: "envoyfilters", Group: "networking.istio.io", Version: "v1alpha3"}
	crdGVR          = schema.GroupVersionResource{Resource: "customresourcedefinitions", Group: "apiextensions.k8s.io", Version: "v1"}
)

type istioInjectionPattern struct {
	client        dynamic.Interface
	logger        log.Component
	config        appsecconfig.Config
	eventRecorder eventRecorder
}

func (i *istioInjectionPattern) Mode() appsecconfig.InjectionMode {
	return i.config.Mode
}

func (i *istioInjectionPattern) IsInjectionPossible(ctx context.Context) error {
	gvrToName := func(gvr schema.GroupVersionResource) string {
		return gvr.Resource + "." + gvr.Group
	}

	// Check if the EnvoyFilter CRD is present
	_, err := i.client.Resource(crdGVR).Get(ctx, gvrToName(filterGVR), metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return fmt.Errorf("%w: EnvoyFilter CRD not found, is the Istio CRDs installed in the cluster? Cannot enable appsec proxy injection for istio", err)
	}

	if err != nil {
		return fmt.Errorf("%w: error getting EnvoyFilter", err)
	}

	// Check if the Gateway CRDs is present
	_, err = i.client.Resource(crdGVR).Get(ctx, gvrToName(gatewayClassGVR), metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return fmt.Errorf("%w: Gateway CRD not found, are the Istio CRDs installed in the cluster? Cannot enable appsec proxy injection for istio", err)
	}

	if err != nil {
		return fmt.Errorf("%w: error getting Gateway", err)
	}

	return nil
}

func (i *istioInjectionPattern) Resource() schema.GroupVersionResource {
	return gatewayClassGVR
}

func (i *istioInjectionPattern) Namespace() string {
	return v1.NamespaceAll
}

func isGatewayClassFromIstio(obj *unstructured.Unstructured) (bool, error) {
	controllerName, found, err := unstructured.NestedString(obj.UnstructuredContent(), "spec", "controllerName")
	if err != nil || !found {
		if err == nil {
			err = errors.New("controllerName not found in gateway spec")
		}
		return false, fmt.Errorf("could not get gateway controller name: %w", err)
	}

	return controllerName == istioGatewayControllerName, nil
}

func (i *istioInjectionPattern) Added(ctx context.Context, obj *unstructured.Unstructured) error {
	if ok, err := isGatewayClassFromIstio(obj); !ok || err != nil {
		// An error happened or the gateway class is not from istio, either way we bail.
		return err
	}

	name := obj.GetName()
	namespace := i.config.IstioNamespace
	i.logger.Debugf("Processing added gatewayclass for istio: %s", name)
	_, err := i.client.Resource(filterGVR).Namespace(namespace).Get(ctx, envoyFilterName, metav1.GetOptions{})
	if err == nil {
		i.logger.Debug("Envoy Filter already exists")
		return nil
	}

	if !k8serrors.IsNotFound(err) {
		return fmt.Errorf("could not check if Envoy Filter already exists: %w", err)
	}

	if err := i.createEnvoyFilter(ctx, namespace); err != nil {
		i.eventRecorder.recordExtensionPolicyCreateFailed(namespace, name, err)
		return fmt.Errorf("could not create Envoy Filter: %w", err)
	}

	i.eventRecorder.recordExtensionPolicyCreated(namespace, name)
	return nil
}

func (i *istioInjectionPattern) Deleted(ctx context.Context, obj *unstructured.Unstructured) error {
	controllerName, found, err := unstructured.NestedString(obj.UnstructuredContent(), "spec", "controllerName")
	if err != nil || !found {
		if err == nil {
			err = errors.New("controllerName not found in gateway spec")
		}
		return fmt.Errorf("could not get gateway controller name: %w", err)
	}

	if controllerName != istioGatewayControllerName {
		return nil // Not an Istio gateway class, skip
	}

	namespace := i.config.IstioNamespace
	name := obj.GetName()
	i.logger.Debugf("Processing deleted gatewayclass for istio: %s", name)
	_, err = i.client.Resource(filterGVR).Namespace(namespace).Get(ctx, envoyFilterName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		i.logger.Debug("Envoy Filter already deleted")
		return nil
	}

	if err != nil {
		return fmt.Errorf("could not check if Envoy Filter was already deleted: %w", err)
	}

	err = i.client.Resource(filterGVR).
		Namespace(namespace).
		Delete(ctx, envoyFilterName, metav1.DeleteOptions{})
	if k8serrors.IsNotFound(err) {
		i.logger.Debug("Envoy Filter already deleted")
		err = nil
	}

	if err != nil {
		i.eventRecorder.recordExtensionPolicyDeleteFailed(namespace, name, err)
		return fmt.Errorf("could not delete Envoy Filter: %w", err)
	}

	i.eventRecorder.recordExtensionPolicyDeleted(namespace, name)

	return err
}

func (i *istioInjectionPattern) createEnvoyFilter(ctx context.Context, namespace string) error {
	filter, err := i.newFilter(namespace)
	if err != nil {
		return fmt.Errorf("could not create Envoy Filter: %w", err)
	}

	unstructuredFilter, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&filter)
	if err != nil {
		return fmt.Errorf("failed to convert EnvoyFilter to unstructured: %w", err)
	}

	_, err = i.client.Resource(filterGVR).
		Namespace(namespace).
		Create(ctx, &unstructured.Unstructured{Object: unstructuredFilter}, metav1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		i.logger.Debug("Envoy Filter already exists")
		return nil
	}

	if err != nil {
		return err
	}

	i.logger.Infof("Envoy Filter created in namespace %s", namespace)

	return nil
}

func (i *istioInjectionPattern) newFilter(namespace string) (istiov1alpha3.EnvoyFilter, error) {
	const clusterName = "datadog_appsec_ext_proc_cluster"
	var (
		processorAddress string
		processorPort    int
	)
	switch i.Mode() {
	case appsecconfig.InjectionModeExternal:
		if i.config.Processor.Address == "" {
			processorAddress = i.config.Processor.ServiceName + "." + i.config.Processor.Namespace + ".svc"
		} else {
			processorAddress = i.config.Processor.Address
		}
		processorPort = i.config.Processor.Port
	default:
		i.logger.Warnf("No injection mode defined, defaults to sending traffic to a sidecar")
		fallthrough
	case appsecconfig.InjectionModeSidecar:
		processorAddress = "127.0.0.1"
		processorPort = i.config.Sidecar.Port
	}

	httpFilterPatch, err := i.buildHTTPFilterPatch(clusterName)
	if err != nil {
		return istiov1alpha3.EnvoyFilter{}, fmt.Errorf("could not build Envoy Filter patch: %w", err)
	}

	clusterPatch, err := i.buildClusterPatch(clusterName, processorAddress, processorPort)
	if err != nil {
		return istiov1alpha3.EnvoyFilter{}, fmt.Errorf("could not build Envoy Filter patch: %w", err)
	}

	return istiov1alpha3.EnvoyFilter{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "networking.istio.io/v1alpha3",
			Kind:       "EnvoyFilter",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        envoyFilterName,
			Namespace:   namespace,
			Labels:      i.config.CommonLabels,
			Annotations: i.config.CommonAnnotations,
		},
		Spec: istionetworkingv1alpha3.EnvoyFilter{
			ConfigPatches: []*istionetworkingv1alpha3.EnvoyFilter_EnvoyConfigObjectPatch{
				httpFilterPatch,
				clusterPatch,
			},
		},
	}, nil
}

// buildHTTPFilterPatch creates the HTTP_FILTER patch for the ext_proc filter
func (i *istioInjectionPattern) buildHTTPFilterPatch(clusterName string) (*istionetworkingv1alpha3.EnvoyFilter_EnvoyConfigObjectPatch, error) {
	patchValue, err := structpb.NewStruct(map[string]any{
		"name": "envoy.filters.http.ext_proc",
		"typed_config": map[string]any{
			"@type": "type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor",
			"grpc_service": map[string]any{
				"envoy_grpc": map[string]any{
					"cluster_name": clusterName,
				},
				"initial_metadata": []any{
					map[string]any{
						"key":   "x-datadog-envoy-integration",
						"value": "1",
					},
					map[string]any{
						"key":   "x-datadog-appsec-injector",
						"value": "1",
					},
				},
			},
			"failure_mode_allow": true,
			"processing_mode": map[string]any{
				"request_header_mode":  "SEND",
				"response_header_mode": "SEND",
			},
			"allow_mode_override": true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create patch value struct: %w", err)
	}

	return &istionetworkingv1alpha3.EnvoyFilter_EnvoyConfigObjectPatch{
		ApplyTo: istionetworkingv1alpha3.EnvoyFilter_HTTP_FILTER,
		Match: &istionetworkingv1alpha3.EnvoyFilter_EnvoyConfigObjectMatch{
			Context: istionetworkingv1alpha3.EnvoyFilter_GATEWAY,
			ObjectTypes: &istionetworkingv1alpha3.EnvoyFilter_EnvoyConfigObjectMatch_Listener{
				Listener: &istionetworkingv1alpha3.EnvoyFilter_ListenerMatch{
					FilterChain: &istionetworkingv1alpha3.EnvoyFilter_ListenerMatch_FilterChainMatch{
						Filter: &istionetworkingv1alpha3.EnvoyFilter_ListenerMatch_FilterMatch{
							Name: "envoy.filters.network.http_connection_manager",
							SubFilter: &istionetworkingv1alpha3.EnvoyFilter_ListenerMatch_SubFilterMatch{
								Name: "envoy.filters.http.router",
							},
						},
					},
				},
			},
		},
		Patch: &istionetworkingv1alpha3.EnvoyFilter_Patch{
			Operation: istionetworkingv1alpha3.EnvoyFilter_Patch_INSERT_BEFORE,
			Value:     patchValue,
		},
	}, nil
}

// buildClusterPatch creates the CLUSTER patch for the Datadog External Processing service
func (i *istioInjectionPattern) buildClusterPatch(clusterName, processorAddress string, processorPort int) (*istionetworkingv1alpha3.EnvoyFilter_EnvoyConfigObjectPatch, error) {
	patchValue, err := structpb.NewStruct(map[string]any{
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
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster patch value struct: %w", err)
	}

	return &istionetworkingv1alpha3.EnvoyFilter_EnvoyConfigObjectPatch{
		ApplyTo: istionetworkingv1alpha3.EnvoyFilter_CLUSTER,
		Match: &istionetworkingv1alpha3.EnvoyFilter_EnvoyConfigObjectMatch{
			Context: istionetworkingv1alpha3.EnvoyFilter_GATEWAY,
			ObjectTypes: &istionetworkingv1alpha3.EnvoyFilter_EnvoyConfigObjectMatch_Cluster{
				Cluster: &istionetworkingv1alpha3.EnvoyFilter_ClusterMatch{
					Service: "*",
				},
			},
		},
		Patch: &istionetworkingv1alpha3.EnvoyFilter_Patch{
			Operation: istionetworkingv1alpha3.EnvoyFilter_Patch_ADD,
			Value:     patchValue,
		},
	}, nil
}

// New returns a new InjectionPattern for Envoy Gateway
func New(client dynamic.Interface, logger log.Component, config appsecconfig.Config, eventRecorderInstance record.EventRecorder) appsecconfig.InjectionPattern {
	pattern := &istioInjectionPattern{
		client: client,
		logger: logger,
		config: config,
		eventRecorder: eventRecorder{
			recorder: eventRecorderInstance,
		},
	}

	if config.Mode == appsecconfig.InjectionModeSidecar {
		return &istioGatewaySidecarPattern{istioInjectionPattern: pattern}
	}

	return pattern
}
