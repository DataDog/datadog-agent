// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package nginx

import (
	"context"
	"fmt"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/record"
)

type nginxInjectionPattern struct {
	client        dynamic.Interface
	logger        log.Component
	config        appsecconfig.Config
	eventRecorder eventRecorder
}

// Mode always returns InjectionModeSidecar for nginx.
// nginx has no external mode — it always uses pod mutation (init container + ConfigMap redirect).
func (n *nginxInjectionPattern) Mode() appsecconfig.InjectionMode {
	return appsecconfig.InjectionModeSidecar
}

// IsInjectionPossible verifies that at least one IngressClass with the ingress-nginx controller exists
func (n *nginxInjectionPattern) IsInjectionPossible(ctx context.Context) error {
	found, err := Detect(ctx, n.client)
	if err != nil {
		return fmt.Errorf("error checking for ingress-nginx IngressClass: %w", err)
	}
	if !found {
		return fmt.Errorf("no IngressClass with controller %q found", ingressNginxControllerName)
	}
	return nil
}

// Resource returns the IngressClass GVR that this pattern watches
func (n *nginxInjectionPattern) Resource() schema.GroupVersionResource {
	return ingressClassGVR
}

// Namespace returns NamespaceAll for cluster-scoped IngressClass resources
func (n *nginxInjectionPattern) Namespace() string {
	return corev1.NamespaceAll
}

// Added is a no-op for nginx sidecar mode.
// ConfigMap creation is deferred to MutatePod() to avoid race conditions
// between the controller workqueue and the admission webhook.
func (n *nginxInjectionPattern) Added(_ context.Context, _ *unstructured.Unstructured) error {
	return nil
}

// Deleted cleans up DD-owned ConfigMaps when an IngressClass is removed.
// It searches all namespaces for ConfigMaps with DD labels and deletes them.
func (n *nginxInjectionPattern) Deleted(ctx context.Context, obj *unstructured.Unstructured) error {
	controllerName, found, err := unstructured.NestedString(obj.UnstructuredContent(), "spec", "controller")
	if err != nil || !found {
		return nil // Not parseable, skip
	}
	if controllerName != ingressNginxControllerName {
		return nil // Not an ingress-nginx IngressClass
	}

	name := obj.GetName()
	n.logger.Debugf("Processing deleted IngressClass for ingress-nginx: %s", name)

	// Check if other ingress-nginx IngressClasses still exist before cleaning up.
	// This prevents deleting ConfigMaps still needed by other controllers.
	otherExists, err := anyOtherIngressNginxClassExists(ctx, n.client, name)
	if err != nil {
		return fmt.Errorf("could not check for remaining ingress-nginx IngressClasses: %w", err)
	}
	if otherExists {
		n.logger.Debug("Skipping ConfigMap cleanup: other ingress-nginx IngressClasses still exist")
		return nil
	}

	// Find namespaces with DD ConfigMaps by listing all ConfigMaps with our labels
	cmList, err := n.client.Resource(configMapGVR).Namespace(corev1.NamespaceAll).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/part-of=datadog,app.kubernetes.io/component=datadog-appsec-injector,appsec.datadoghq.com/proxy-type=ingress-nginx",
	})
	if err != nil {
		n.eventRecorder.recordConfigMapDeleteFailed(name, err)
		return fmt.Errorf("failed to list DD ConfigMaps for cleanup: %w", err)
	}

	for _, cm := range cmList.Items {
		err := n.client.Resource(configMapGVR).Namespace(cm.GetNamespace()).Delete(ctx, cm.GetName(), metav1.DeleteOptions{})
		if err != nil {
			n.logger.Warnf("Failed to delete DD ConfigMap %s/%s: %v", cm.GetNamespace(), cm.GetName(), err)
			continue
		}
		n.logger.Infof("Deleted DD ConfigMap %s/%s", cm.GetNamespace(), cm.GetName())
	}

	if len(cmList.Items) > 0 {
		n.eventRecorder.recordConfigMapDeleted(name)
	}

	return nil
}

// anyOtherIngressNginxClassExists checks if any IngressClass with the ingress-nginx
// controller exists, excluding the one being deleted (by name).
func anyOtherIngressNginxClassExists(ctx context.Context, client dynamic.Interface, excludeName string) (bool, error) {
	list, err := client.Resource(ingressClassGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to list IngressClasses: %w", err)
	}
	for _, item := range list.Items {
		if item.GetName() == excludeName {
			continue
		}
		controllerName, found, _ := unstructured.NestedString(item.UnstructuredContent(), "spec", "controller")
		if found && controllerName == ingressNginxControllerName {
			return true, nil
		}
	}
	return false, nil
}

// New creates a new InjectionPattern for ingress-nginx.
// It always returns a nginxSidecarPattern (pod mutation mode) regardless of the global
// injection mode setting, because nginx has no external processing mode.
func New(client dynamic.Interface, logger log.Component, config appsecconfig.Config, recorder record.EventRecorder) appsecconfig.InjectionPattern {
	base := &nginxInjectionPattern{
		client: client,
		logger: logger,
		config: config,
		eventRecorder: eventRecorder{
			recorder: recorder,
		},
	}
	return &nginxSidecarPattern{nginxInjectionPattern: base}
}
