// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package istio

import (
	"context"
	"fmt"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"

	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/record"
)

// istioNativeGatewayPattern handles external mode for networking.istio.io/v1 Gateway resources
type istioNativeGatewayPattern struct {
	*istioInjectionPattern
}

func (g *istioNativeGatewayPattern) Resource() schema.GroupVersionResource {
	return istioGatewayGVR
}

func (g *istioNativeGatewayPattern) Namespace() string {
	return v1.NamespaceAll
}

func (g *istioNativeGatewayPattern) IsInjectionPossible(ctx context.Context) error {
	gvrToName := func(gvr schema.GroupVersionResource) string {
		return gvr.Resource + "." + gvr.Group
	}

	// Check if the EnvoyFilter CRD is present
	_, err := g.client.Resource(crdGVR).Get(ctx, gvrToName(filterGVR), metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return fmt.Errorf("%w: EnvoyFilter CRD not found, are the Istio CRDs installed in the cluster? Cannot enable appsec proxy injection for istio-gateway", err)
	}
	if err != nil {
		return fmt.Errorf("%w: error getting EnvoyFilter CRD", err)
	}

	// Check if the Istio Gateway CRD is present
	_, err = g.client.Resource(crdGVR).Get(ctx, gvrToName(istioGatewayGVR), metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return fmt.Errorf("%w: Istio Gateway CRD not found, are the Istio CRDs installed in the cluster? Cannot enable appsec proxy injection for istio-gateway", err)
	}
	if err != nil {
		return fmt.Errorf("%w: error getting Istio Gateway CRD", err)
	}

	return nil
}

func (g *istioNativeGatewayPattern) Added(ctx context.Context, obj *unstructured.Unstructured) error {
	// No controllerName check needed — the CRD is inherently Istio
	name := obj.GetName()
	namespace := g.config.IstioNamespace
	g.logger.Debugf("Processing added Istio native gateway: %s/%s", obj.GetNamespace(), name)

	_, err := g.client.Resource(filterGVR).Namespace(namespace).Get(ctx, envoyFilterName, metav1.GetOptions{})
	if err == nil {
		g.logger.Debug("Envoy Filter already exists")
		return nil
	}

	if !k8serrors.IsNotFound(err) {
		return fmt.Errorf("could not check if Envoy Filter already exists: %w", err)
	}

	if err := g.createEnvoyFilter(ctx, namespace); err != nil {
		g.eventRecorder.recordExtensionPolicyCreateFailed(namespace, name, err)
		return fmt.Errorf("could not create Envoy Filter: %w", err)
	}

	g.eventRecorder.recordExtensionPolicyCreated(namespace, name)
	return nil
}

func (g *istioNativeGatewayPattern) Deleted(ctx context.Context, obj *unstructured.Unstructured) error {
	name := obj.GetName()
	objNamespace := obj.GetNamespace()
	g.logger.Debugf("Processing deleted Istio native gateway: %s/%s", objNamespace, name)

	// Distinguish watch-event mode (resource was actually deleted from the cluster) from cleanup mode
	// (cleanupPattern iterates live resources and calls Deleted without removing them).
	// Coordination checks — "skip if other gateways still exist" — only apply to watch events.
	// In cleanup mode we must always proceed to delete the EnvoyFilter.
	_, errGet := g.client.Resource(istioGatewayGVR).Namespace(objNamespace).Get(ctx, name, metav1.GetOptions{})
	isWatchEvent := k8serrors.IsNotFound(errGet)

	if isWatchEvent {
		// Watch-event mode: only delete the filter if no other gateways still need it.
		list, err := g.client.Resource(istioGatewayGVR).Namespace(v1.NamespaceAll).List(ctx, metav1.ListOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("could not list remaining Istio gateways: %w", err)
		}
		if list != nil && len(list.Items) > 0 {
			g.logger.Debug("Skipping EnvoyFilter deletion: other Istio native gateways still exist")
			return nil
		}

		// Cross-pattern coordination: skip if K8s GatewayClasses with Istio controller still exist.
		gatewayClassesExist, err := anyIstioGatewayClassExists(ctx, g.client, "")
		if err != nil {
			return fmt.Errorf("could not check for remaining Istio gateway classes: %w", err)
		}
		if gatewayClassesExist {
			g.logger.Debug("Skipping EnvoyFilter deletion: Istio GatewayClasses still exist")
			return nil
		}
	}

	namespace := g.config.IstioNamespace
	_, err := g.client.Resource(filterGVR).Namespace(namespace).Get(ctx, envoyFilterName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		g.logger.Debug("Envoy Filter already deleted")
		return nil
	}
	if err != nil {
		return fmt.Errorf("could not check if Envoy Filter was already deleted: %w", err)
	}

	err = g.client.Resource(filterGVR).
		Namespace(namespace).
		Delete(ctx, envoyFilterName, metav1.DeleteOptions{})
	if k8serrors.IsNotFound(err) {
		g.logger.Debug("Envoy Filter already deleted")
		err = nil
	}
	if err != nil {
		g.eventRecorder.recordExtensionPolicyDeleteFailed(namespace, name, err)
		return fmt.Errorf("could not delete Envoy Filter: %w", err)
	}

	g.eventRecorder.recordExtensionPolicyDeleted(namespace, name)
	return nil
}

// NewGateway returns a new InjectionPattern for Istio native Gateway (networking.istio.io/v1)
func NewGateway(client dynamic.Interface, logger log.Component, config appsecconfig.Config, eventRecorderInstance record.EventRecorder) appsecconfig.InjectionPattern {
	base := &istioInjectionPattern{
		client: client,
		logger: logger,
		config: config,
		eventRecorder: eventRecorder{
			recorder: eventRecorderInstance,
		},
	}

	pattern := &istioNativeGatewayPattern{istioInjectionPattern: base}

	if config.Mode == appsecconfig.InjectionModeSidecar {
		return &istioNativeGatewaySidecarPattern{istioNativeGatewayPattern: pattern}
	}

	return pattern
}
