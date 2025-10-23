// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

// Package envoygateway implements the InjectionPattern interface for Envoy Gateway
package envoygateway

import (
	"context"
	"fmt"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
	"github.com/envoyproxy/gateway/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	extProcName                             = "datadog-appsec-extproc"
	envoyExtensionPolicyCRDName             = "envoyextensionpolicies.gateway.envoyproxy.io"
	gatewayGatewayNetworkingK8sIOCRD        = "gateways.gateway.networking.k8s.io"
	referenceGrantGatewayNetworkingK8sIOCRD = "referencegrants.gateway.networking.k8s.io"
)

var (
	gatewayGVR   = schema.GroupVersionResource{Resource: "gateways", Group: "gateway.networking.k8s.io", Version: "v1"}
	extensionGVR = schema.GroupVersionResource{Resource: "envoyextensionpolicies", Group: "gateway.envoyproxy.io", Version: "v1alpha1"}
	crdGVR       = schema.GroupVersionResource{Resource: "customresourcedefinitions", Group: "apiextensions.k8s.io", Version: "v1"}
)

type envoyGatewayInjectionPattern struct {
	client dynamic.Interface
	logger log.Component
	config appsecconfig.Config
	eventRecorder

	grantManager
}

func (e *envoyGatewayInjectionPattern) IsInjectionPossible(ctx context.Context) error {
	gvrToName := func(gvr schema.GroupVersionResource) string {
		return fmt.Sprintf("%s.%s", gvr.Resource, gvr.Group)
	}

	// Check if the EnvoyExtensionPolicy CRD is present
	_, err := e.client.Resource(crdGVR).Get(ctx, gvrToName(extensionGVR), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return fmt.Errorf("%w: EnvoyExtensionPolicy CRD not found, is the Envoy Gateway installed in the cluster? Cannot enable appsec proxy injection for envoy-gateway", err)
	}

	if err != nil {
		return err
	}

	// Check if the Gateway CRDs is present
	_, err = e.client.Resource(crdGVR).Get(ctx, gvrToName(gatewayGVR), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return fmt.Errorf("%w: Gateway CRD not found, is the Gateway API installed in the cluster? Cannot enable appsec proxy injection for envoy-gateway", err)
	}

	if err != nil {
		return err
	}

	// Check if the ReferenceGrant CRD is present
	_, err = e.client.Resource(crdGVR).Get(ctx, gvrToName(referenceGrantGVR), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return fmt.Errorf("%w: ReferenceGrant CRD not found, is the Gateway API installed in the cluster? Cannot enable appsec proxy injection for envoy-gateway", err)
	}

	return err
}

func (e *envoyGatewayInjectionPattern) Resource() schema.GroupVersionResource {
	return gatewayGVR
}

func (e *envoyGatewayInjectionPattern) Namespace() string {
	return v1.NamespaceAll
}

func (e *envoyGatewayInjectionPattern) Added(ctx context.Context, namespace, name string) error {
	e.logger.Debugf("Processing added gateway for envoygateway: %s/%s", name, namespace)
	_, err := e.client.Resource(extensionGVR).Namespace(namespace).Get(ctx, extProcName, metav1.GetOptions{})
	if err == nil {
		e.logger.Debug("Envoy extension policy already exists")
		return nil
	}

	if !errors.IsNotFound(err) {
		return fmt.Errorf("could not check if Envoy extension policy already exists: %w", err)
	}

	if err := e.grantManager.AddNamespaceToGrant(ctx, namespace); err != nil {
		return fmt.Errorf("could not ensure ReferenceGrant for namespace %s: %w", namespace, err)
	}

	if err := e.createEnvoyExtensionPolicy(ctx, namespace, name); err != nil {
		return fmt.Errorf("could not create Envoy extension policy: %w", err)
	}

	return nil
}

// isAloneInNamespace checks if the given gateway is the only one in its namespace
// Since ReferenceGrant and EnvoyExtensionPolicy are namespace-scoped, we only want to create them
// if the gateway is the first/last one in its namespace because what we do applies to all gateways in the namespace.
func (e *envoyGatewayInjectionPattern) isAloneInNamespace(ctx context.Context, namespace, name string) (bool, error) {
	// List gateway in the namespace to know if we are alone
	gateways, err := e.client.Resource(gatewayGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fields.OneTermNotEqualSelector("appsec.datadoghq.com/enabled", "false").String(),
	})
	if err != nil {
		return false, err
	}

	aloneInNamespace := true
	for _, gw := range gateways.Items {
		if gw.GetName() != name {
			aloneInNamespace = false
		}
	}

	return aloneInNamespace, nil
}

func (e *envoyGatewayInjectionPattern) Deleted(ctx context.Context, namespace, name string) error {
	e.logger.Debugf("Processing deleted gateway for envoygateway: %s/%s", name, namespace)
	_, err := e.client.Resource(extensionGVR).Namespace(namespace).Get(ctx, extProcName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		e.logger.Debug("Envoy extension policy already deleted")
		return nil
	}

	if err != nil {
		return fmt.Errorf("could not check if Envoy extension policy was already deleted: %w", err)
	}

	alone, err := e.isAloneInNamespace(ctx, namespace, name)
	if err != nil {
		return fmt.Errorf("could not determine if gateway is alone in namespace: %w", err)
	}
	if !alone {
		e.logger.Debugf("Skipping Envoy extension policy creation for gateway %s/%s: not alone in namespace", namespace, name)
		return nil
	}

	err = e.client.Resource(extensionGVR).
		Namespace(namespace).
		Delete(ctx, extProcName, metav1.DeleteOptions{})
	if errors.IsNotFound(err) {
		e.logger.Debug("Envoy extension policy already deleted")
		err = nil
	}

	if err != nil {
		e.recordExtensionPolicyDeleteFailed(namespace, name, err)
	} else {
		e.recordExtensionPolicyDeleted(namespace, name)
	}

	if err := e.grantManager.RemoveNamespaceToGrant(ctx, namespace); err != nil {
		return fmt.Errorf("could not remove namespace %s from ReferenceGrant: %w", namespace, err)
	}

	return err
}

func (e *envoyGatewayInjectionPattern) createEnvoyExtensionPolicy(ctx context.Context, namespace string, gatewayName string) error {
	policy := e.newPolicy(namespace)

	unstructuredGrant, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&policy)
	if err != nil {
		return err
	}

	_, err = e.client.Resource(extensionGVR).
		Namespace(namespace).
		Create(ctx, &unstructured.Unstructured{Object: unstructuredGrant}, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		e.logger.Debug("Envoy extension policy already exists")
		return nil
	}

	if err != nil {
		e.recordExtensionPolicyCreateFailed(namespace, gatewayName, err)
		return err
	}

	e.logger.Infof("Envoy extension policy created in namespace %s", namespace)
	e.recordExtensionPolicyCreated(namespace, gatewayName)

	return nil
}

func (e *envoyGatewayInjectionPattern) newPolicy(namespace string) v1alpha1.EnvoyExtensionPolicy {
	return v1alpha1.EnvoyExtensionPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "gateway.envoyproxy.io/v1alpha1",
			Kind:       "EnvoyExtensionPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        extProcName,
			Namespace:   namespace,
			Labels:      e.config.CommonLabels,
			Annotations: e.config.CommonAnnotations,
		},
		Spec: v1alpha1.EnvoyExtensionPolicySpec{
			PolicyTargetReferences: v1alpha1.PolicyTargetReferences{
				TargetSelectors: []v1alpha1.TargetSelector{
					{
						Kind:  "Gateway",
						Group: ptr.To[gwapiv1.Group]("gateway.networking.k8s.io"),
					},
				},
			},
			ExtProc: []v1alpha1.ExtProc{
				{
					FailOpen: ptr.To(true),
					BackendCluster: v1alpha1.BackendCluster{
						BackendRefs: []v1alpha1.BackendRef{
							{
								BackendObjectReference: gwapiv1.BackendObjectReference{
									Name:      gwapiv1.ObjectName(e.config.Processor.ServiceName),
									Namespace: ptr.To[gwapiv1.Namespace](gwapiv1.Namespace(e.config.Processor.Namespace)),
									Port:      ptr.To[gwapiv1.PortNumber](gwapiv1.PortNumber(e.config.Processor.Port)),
								},
							},
						},
					},
					ProcessingMode: &v1alpha1.ExtProcProcessingMode{
						AllowModeOverride: true,
						Request:           &v1alpha1.ProcessingModeOptions{},
						Response:          &v1alpha1.ProcessingModeOptions{},
					},
				},
			},
		},
	}
}

// New returns a new InjectionPattern for Envoy Gateway
func New(client dynamic.Interface, logger log.Component, config appsecconfig.Config, eventRecorderInstance record.EventRecorder) appsecconfig.InjectionPattern {
	recorder := eventRecorder{
		recorder: eventRecorderInstance,
	}
	return &envoyGatewayInjectionPattern{
		client:        client,
		logger:        logger,
		config:        config,
		eventRecorder: recorder,

		grantManager: grantManager{
			client:            client,
			logger:            logger,
			eventRecorder:     recorder,
			serviceName:       config.Processor.ServiceName,
			namespace:         config.Processor.Namespace,
			commonLabels:      config.CommonLabels,
			commonAnnotations: config.CommonAnnotations,
		},
	}
}
