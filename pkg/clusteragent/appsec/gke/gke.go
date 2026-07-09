// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package gke

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	stdErrors "errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/record"
)

const (
	extensionNamePrefix = "datadog-appsec-"
)

var _ appsecconfig.InjectionPattern = (*gkeGatewayInjectionPattern)(nil)

type gkeGatewayInjectionPattern struct {
	client                   dynamic.Interface
	logger                   log.Component
	config                   appsecconfig.Config
	serviceNamespaceInfoOnce sync.Once
	eventRecorder
}

func (g *gkeGatewayInjectionPattern) Mode() appsecconfig.InjectionMode {
	return appsecconfig.InjectionModeExternal
}

func (g *gkeGatewayInjectionPattern) Resource() schema.GroupVersionResource {
	return gatewayGVR
}

func (g *gkeGatewayInjectionPattern) Namespace() string {
	return metav1.NamespaceAll
}

func (g *gkeGatewayInjectionPattern) IsInjectionPossible(ctx context.Context) error {
	if g.config.Processor.ServiceName == "" {
		return stdErrors.New("processor service name is required for gke-gateway proxy type but is not configured")
	}
	if g.config.Processor.Port <= 0 {
		return fmt.Errorf("processor port must be positive for gke-gateway proxy type, got: %d", g.config.Processor.Port)
	}

	_, err := g.client.Resource(crdGVR).Get(ctx, gcpTrafficExtensionCRDName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("%w: GCPTrafficExtension CRD not found, is GKE Gateway service extensions enabled in the cluster? Cannot enable appsec proxy injection for gke-gateway", err)
	}
	if err != nil {
		return fmt.Errorf("%w: error getting GCPTrafficExtension CRD", err)
	}

	g.serviceNamespaceInfoOnce.Do(func() {
		g.logger.Infof("GKE Gateway AppSec uses same-namespace Service backendRefs: the callout Service %q must exist in each Gateway namespace; processor namespace %q is not used for GKE", g.config.Processor.ServiceName, g.config.Processor.Namespace)
	})

	return nil
}

func (g *gkeGatewayInjectionPattern) Added(ctx context.Context, obj *unstructured.Unstructured) error {
	namespace := obj.GetNamespace()
	gatewayName := obj.GetName()
	gatewayClass, _, err := unstructured.NestedString(obj.Object, "spec", "gatewayClassName")
	if err != nil {
		g.logger.Debugf("Skipping GKE Gateway AppSec injection for gateway %s/%s: invalid spec.gatewayClassName: %v", namespace, gatewayName, err)
		return nil
	}
	if gatewayClass == "" || !slices.Contains(g.config.Product.GKE.GatewayClasses, gatewayClass) {
		g.logger.Debugf("Skipping GKE Gateway AppSec injection for gateway %s/%s: unsupported gatewayClassName %q", namespace, gatewayName, gatewayClass)
		return nil
	}

	extName := extensionName(gatewayName)
	existing, err := g.client.Resource(trafficExtensionGVR).Namespace(namespace).Get(ctx, extName, metav1.GetOptions{})
	if err == nil {
		if appsecconfig.IsManagedByDatadog(existing.GetLabels()) {
			g.logger.Debugf("GCPTrafficExtension %s/%s already exists and is managed by Datadog", namespace, extName)
			return nil
		}
		g.logger.Warnf("Skipping GCPTrafficExtension %s/%s creation: object already exists and is not managed by Datadog", namespace, extName)
		return nil
	}
	if !apierrors.IsNotFound(err) {
		g.recordExtensionCreateFailed(namespace, gatewayName, extName, err)
		return fmt.Errorf("could not check if GCPTrafficExtension %s/%s already exists: %w", namespace, extName, err)
	}

	extension := g.newGCPTrafficExtension(namespace, gatewayName)
	_, err = g.client.Resource(trafficExtensionGVR).Namespace(namespace).Create(ctx, extension, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		// AlreadyExists means someone created the same name between our Get(NotFound) and Create;
		// re-check ownership so we do not silently claim a foreign object as our success.
		existing, getErr := g.client.Resource(trafficExtensionGVR).Namespace(namespace).Get(ctx, extName, metav1.GetOptions{})
		if getErr == nil && !appsecconfig.IsManagedByDatadog(existing.GetLabels()) {
			g.logger.Warnf("Skipping GCPTrafficExtension %s/%s: object already exists and is not managed by Datadog", namespace, extName)
			return nil
		}
		g.logger.Debugf("GCPTrafficExtension %s/%s already exists", namespace, extName)
		return nil
	}
	if err != nil {
		g.recordExtensionCreateFailed(namespace, gatewayName, extName, err)
		return fmt.Errorf("could not create GCPTrafficExtension %s/%s: %w", namespace, extName, err)
	}

	g.logger.Infof("GCPTrafficExtension %s/%s created for Gateway %s/%s", namespace, extName, namespace, gatewayName)
	g.recordExtensionCreated(namespace, gatewayName, extName)
	return nil
}

func (g *gkeGatewayInjectionPattern) Deleted(ctx context.Context, obj *unstructured.Unstructured) error {
	namespace := obj.GetNamespace()
	gatewayName := obj.GetName()
	extName := extensionName(gatewayName)

	existing, err := g.client.Resource(trafficExtensionGVR).Namespace(namespace).Get(ctx, extName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		g.logger.Debugf("GCPTrafficExtension %s/%s already deleted", namespace, extName)
		return nil
	}
	if err != nil {
		g.recordExtensionDeleteFailed(namespace, gatewayName, extName, err)
		return fmt.Errorf("could not check if GCPTrafficExtension %s/%s was already deleted: %w", namespace, extName, err)
	}
	if !appsecconfig.IsManagedByDatadog(existing.GetLabels()) {
		g.logger.Warnf("Skipping GCPTrafficExtension %s/%s deletion: object is not managed by Datadog", namespace, extName)
		return nil
	}

	err = g.client.Resource(trafficExtensionGVR).Namespace(namespace).Delete(ctx, extName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		g.recordExtensionDeleteFailed(namespace, gatewayName, extName, err)
		return fmt.Errorf("could not delete GCPTrafficExtension %s/%s: %w", namespace, extName, err)
	}

	g.logger.Infof("GCPTrafficExtension %s/%s deleted for Gateway %s/%s", namespace, extName, namespace, gatewayName)
	g.recordExtensionDeleted(namespace, gatewayName, extName)
	return nil
}

// extensionName returns a DNS-1123 label-safe GCPTrafficExtension name. The 63
// character limit is the RFC-1123 label maximum for metadata.name; longer
// Gateway names are bounded with an 8-character sha256 suffix. Other proxies
// such as Envoy Gateway use a fixed per-namespace name and do not need this.
func extensionName(gatewayName string) string {
	if len(extensionNamePrefix)+len(gatewayName) <= 63 {
		return extensionNamePrefix + gatewayName
	}

	hash := sha256.Sum256([]byte(gatewayName))
	maxGatewayNameLength := 63 - len(extensionNamePrefix) - 1 - 8
	return extensionNamePrefix + gatewayName[:maxGatewayNameLength] + "-" + hex.EncodeToString(hash[:])[:8]
}

func (g *gkeGatewayInjectionPattern) newGCPTrafficExtension(namespace string, gatewayName string) *unstructured.Unstructured {
	labels := maps.Clone(g.config.CommonLabels)
	if labels == nil {
		labels = map[string]string{}
	}
	labels[kubernetes.KubeAppManagedByLabelKey] = appsecconfig.ManagedByLabelValue
	annotations := maps.Clone(g.config.CommonAnnotations)

	extension := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "networking.gke.io/v1",
		"kind":       "GCPTrafficExtension",
		"metadata": map[string]any{
			"name":      extensionName(gatewayName),
			"namespace": namespace,
		},
		"spec": map[string]any{
			"targetRefs": []any{
				map[string]any{
					"group": "gateway.networking.k8s.io",
					"kind":  "Gateway",
					"name":  gatewayName,
				},
			},
			"extensionChains": []any{
				map[string]any{
					"name": "datadog-aap-chain",
					"matchCondition": map[string]any{
						"celExpressions": []any{
							map[string]any{"celMatcher": "1 == 1"},
						},
					},
					"extensions": []any{
						map[string]any{
							"name": "datadog-aap-extension",
							"backendRef": map[string]any{
								"group": "",
								"kind":  "Service",
								"name":  g.config.Processor.ServiceName,
								"port":  int64(g.config.Processor.Port),
							},
							// cluster.local is GKE's fixed cluster DNS domain; GKE does not support custom cluster domains.
							"authority":       fmt.Sprintf("%s.%s.svc.cluster.local", g.config.Processor.ServiceName, namespace),
							"failOpen":        true,
							"supportedEvents": []any{"RequestHeaders", "ResponseHeaders"},
							"timeout":         "1s",
						},
					},
				},
			},
		},
	}}
	extension.SetLabels(labels)
	extension.SetAnnotations(annotations)
	return extension
}

// New returns a new InjectionPattern for GKE Gateway.
func New(client dynamic.Interface, logger log.Component, config appsecconfig.Config, eventRecorderInstance record.EventRecorder) appsecconfig.InjectionPattern {
	return &gkeGatewayInjectionPattern{
		client:        client,
		logger:        logger,
		config:        config,
		eventRecorder: eventRecorder{recorder: eventRecorderInstance},
	}
}
