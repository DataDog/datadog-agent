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
	"strings"
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
	// multiClusterGatewayClassSuffix identifies the GKE multi-cluster GatewayClasses
	// (gke-l7-global-external-managed-mc, gke-l7-rilb-mc, ...).
	multiClusterGatewayClassSuffix = "-mc"
	gatewayKind                    = "Gateway"
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
	// Managed GKE Gateway runs its Envoy data plane outside the cluster, so there is no proxy
	// pod to host a sidecar processor and gke-gateway only applies in external mode. Detection
	// registers this proxy type on any cluster carrying the GCPTrafficExtension CRD, including
	// clusters the operator configured for sidecar mode, where nothing was requested and
	// nothing is broken: report it as not applicable so it is skipped quietly.
	if g.config.Mode == appsecconfig.InjectionModeSidecar {
		return fmt.Errorf("%w: gke-gateway supports external mode only because managed GKE Gateway has no in-cluster Envoy data plane to host a sidecar processor", appsecconfig.ErrInjectionNotApplicable)
	}
	if g.config.Processor.ServiceName == "" {
		return stdErrors.New("processor service name is required for gke-gateway proxy type but is not configured")
	}
	if g.config.Processor.Port <= 0 || g.config.Processor.Port > 65535 {
		return fmt.Errorf("processor port must be between 1 and 65535, got: %d", g.config.Processor.Port)
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
	// Multi-cluster GatewayClasses (the "-mc" suffix) require the callout backendRef to be a
	// net.gke.io ServiceImport; a core Service backendRef, which is all this reconciler emits,
	// is not supported for them. Skip instead of creating a GCPTrafficExtension that cannot work.
	if strings.HasSuffix(gatewayClass, multiClusterGatewayClassSuffix) {
		g.logger.Warnf("Skipping GKE Gateway AppSec injection for gateway %s/%s: multi-cluster gatewayClassName %q requires a net.gke.io ServiceImport backendRef, which this injector does not emit", namespace, gatewayName, gatewayClass)
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

	extension := g.newGCPTrafficExtension(obj)
	_, err = g.client.Resource(trafficExtensionGVR).Namespace(namespace).Create(ctx, extension, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		// AlreadyExists means someone created the same name between our Get(NotFound) and Create;
		// re-check ownership so we do not silently claim a foreign object as our success.
		existing, getErr := g.client.Resource(trafficExtensionGVR).Namespace(namespace).Get(ctx, extName, metav1.GetOptions{})
		if getErr != nil {
			// Ownership is unknown: report the failure so the reconcile is retried instead of
			// silently claiming an object we never established ownership of.
			g.recordExtensionCreateFailed(namespace, gatewayName, extName, getErr)
			return fmt.Errorf("could not check ownership of existing GCPTrafficExtension %s/%s: %w", namespace, extName, getErr)
		}
		if !appsecconfig.IsManagedByDatadog(existing.GetLabels()) {
			g.logger.Warnf("Skipping GCPTrafficExtension %s/%s: object already exists and is not managed by Datadog", namespace, extName)
			return nil
		}
		g.logger.Debugf("GCPTrafficExtension %s/%s already exists and is managed by Datadog", namespace, extName)
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
	// A delayed delete event for an old Gateway must not remove the extension that
	// belongs to a Gateway recreated under the same name. Extensions created before
	// owner references were emitted carry none, and stay deletable on the label check
	// alone; an incoming Gateway without a UID is likewise not treated as a mismatch.
	if gatewayUID := obj.GetUID(); gatewayUID != "" {
		for _, owner := range existing.GetOwnerReferences() {
			if owner.Kind == gatewayKind && owner.UID != gatewayUID {
				g.logger.Warnf("Skipping GCPTrafficExtension %s/%s deletion: it is owned by Gateway %q with UID %q, not by Gateway %s/%s with UID %q", namespace, extName, owner.Name, owner.UID, namespace, gatewayName, gatewayUID)
				return nil
			}
		}
	}

	// Guard the delete with the UID we just read, so a resource recreated between the
	// Get above and this Delete is never removed. The API server reports the mismatch
	// as a conflict. A blank UID cannot be sent, as it would reject every delete.
	deleteOptions := metav1.DeleteOptions{}
	if uid := existing.GetUID(); uid != "" {
		deleteOptions.Preconditions = &metav1.Preconditions{UID: &uid}
	}

	err = g.client.Resource(trafficExtensionGVR).Namespace(namespace).Delete(ctx, extName, deleteOptions)
	if apierrors.IsConflict(err) {
		// The object we read was replaced by a different one, which is not ours to
		// delete. Retrying cannot help, so do not requeue the reconcile.
		g.logger.Warnf("Skipping GCPTrafficExtension %s/%s deletion: it was recreated between read and delete: %v", namespace, extName, err)
		return nil
	}
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

func (g *gkeGatewayInjectionPattern) newGCPTrafficExtension(gateway *unstructured.Unstructured) *unstructured.Unstructured {
	namespace := gateway.GetNamespace()
	gatewayName := gateway.GetName()
	labels := maps.Clone(g.config.CommonLabels)
	if labels == nil {
		labels = map[string]string{}
	}
	labels[kubernetes.KubeAppManagedByLabelKey] = appsecconfig.ManagedByLabelValue
	annotations := maps.Clone(g.config.CommonAnnotations)
	// GKE always routes the callout to a Service in the Gateway's own namespace, so the
	// processor endpoint derived from Processor.Namespace (or "localhost" in sidecar mode)
	// would advertise an address that does not apply here.
	delete(annotations, appsecconfig.AppsecProcessorResourceAnnotation)

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
					"kind":  gatewayKind,
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
							// authority is the gRPC :authority header Envoy sends on the callout; GKE
							// requires it when backendRef is set with kind Service. It does not resolve
							// the backend, so we default it to the Service FQDN under cluster.local, the
							// default cluster DNS domain.
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

	// Own the extension from the Gateway so Kubernetes garbage-collects it even when a
	// Gateway delete event is missed, for example while the cluster-agent is down. The
	// reference is always namespace-valid because GKE targetRefs and backendRefs have no
	// namespace field, so the extension always lives in the Gateway's own namespace.
	// Controller and BlockOwnerDeletion are deliberately omitted, as in the nginx
	// ConfigMap owner reference: BlockOwnerDeletion needs update access on the owner's
	// finalizers subresource, which this feature does not request.
	if uid := gateway.GetUID(); uid != "" {
		extension.SetOwnerReferences([]metav1.OwnerReference{{
			APIVersion: gateway.GetAPIVersion(),
			Kind:       gateway.GetKind(),
			Name:       gatewayName,
			UID:        uid,
		}})
	} else {
		// An owner reference with a blank UID is invalid and the API server rejects it,
		// so emit none at all and fall back to label-based cleanup.
		g.logger.Debugf("Creating GCPTrafficExtension %s/%s without an owner reference: Gateway %s/%s has no UID", namespace, extensionName(gatewayName), namespace, gatewayName)
	}

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
