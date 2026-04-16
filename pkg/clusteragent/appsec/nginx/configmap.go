// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package nginx

import (
	"context"
	"fmt"
	"maps"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
)

const (
	ddConfigMapPrefix    = "datadog-appsec-"
	ddSnippetMarkerStart = "# datadog-appsec-begin"
	ddSnippetMarkerEnd   = "# datadog-appsec-end"
	mainSnippetKey       = "main-snippet"
	httpSnippetKey       = "http-snippet"

	// watchedConfigMapLabel is applied to the original ConfigMap so the reconciler
	// informer can use server-side LabelSelector filtering to watch only relevant ConfigMaps.
	watchedConfigMapLabel = "appsec.datadoghq.com/watched-configmap"
	// ddConfigMapAnnotation stores the name of the DD-owned ConfigMap copy on the original
	// ConfigMap so the reconciler knows which DD CM to update when the original changes.
	ddConfigMapAnnotation = "appsec.datadoghq.com/dd-configmap"
)

var configMapGVR = corev1.SchemeGroupVersion.WithResource("configmaps")

// mainSnippetDirectives returns the nginx main-context directives for loading the datadog module.
// The env directive preserves DD_AGENT_HOST for nginx worker processes so the module
// can reach the Datadog agent (nginx strips env vars from workers by default).
func mainSnippetDirectives(moduleMountPath string) string {
	return fmt.Sprintf("load_module %s/ngx_http_datadog_module.so;\nthread_pool waf_thread_pool threads=2 max_queue=16;\nenv DD_AGENT_HOST;", moduleMountPath)
}

// httpSnippetDirectives are injected into the nginx http context to enable AppSec
const httpSnippetDirectives = "datadog_appsec_enabled on;\ndatadog_waf_thread_pool_name waf_thread_pool;"

// ddConfigMapName returns the name for the DD-owned ConfigMap based on the original ConfigMap name
func ddConfigMapName(originalName string) string {
	return ddConfigMapPrefix + originalName
}

// buildSnippet prepends DD directives wrapped in comment markers to an existing snippet.
// It is idempotent: if markers already exist, they are stripped and re-applied.
func buildSnippet(existingSnippet, ddDirectives string) string {
	// Strip existing DD directives first for idempotency
	cleaned := stripDDSnippet(existingSnippet)
	marker := ddSnippetMarkerStart + "\n" + ddDirectives + "\n" + ddSnippetMarkerEnd
	if cleaned == "" {
		return marker
	}
	return marker + "\n" + cleaned
}

// stripDDSnippet removes the DD-injected section (between markers, inclusive) from a snippet
func stripDDSnippet(snippet string) string {
	startIdx := strings.Index(snippet, ddSnippetMarkerStart)
	if startIdx == -1 {
		return snippet
	}
	endIdx := strings.Index(snippet, ddSnippetMarkerEnd)
	if endIdx == -1 {
		return snippet
	}
	endIdx += len(ddSnippetMarkerEnd)
	// Remove trailing newline after end marker if present
	if endIdx < len(snippet) && snippet[endIdx] == '\n' {
		endIdx++
	}
	return snippet[:startIdx] + snippet[endIdx:]
}

// createOrUpdateDDConfigMap creates or updates the DD-owned ConfigMap by mirroring the original
// and prepending Datadog AppSec directives to main-snippet and http-snippet.
func createOrUpdateDDConfigMap(ctx context.Context, client dynamic.Interface, namespace, originalCMName, moduleMountPath string, labels, annotations map[string]string) error {
	ddName := ddConfigMapName(originalCMName)

	// Fetch original ConfigMap (may not exist if user hasn't customized anything)
	originalData := map[string]string{}
	originalCM, err := client.Resource(configMapGVR).Namespace(namespace).Get(ctx, originalCMName, metav1.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to get original ConfigMap %s/%s: %w", namespace, originalCMName, err)
	}
	if err == nil {
		data, found, err := unstructured.NestedStringMap(originalCM.UnstructuredContent(), "data")
		if err != nil {
			return fmt.Errorf("original ConfigMap %s/%s has non-string data values: %w", namespace, originalCMName, err)
		}
		if found {
			originalData = data
		}
	}

	// Build merged data
	mergedData := make(map[string]string, len(originalData)+2)
	maps.Copy(mergedData, originalData)
	mergedData[mainSnippetKey] = buildSnippet(mergedData[mainSnippetKey], mainSnippetDirectives(moduleMountPath))
	mergedData[httpSnippetKey] = buildSnippet(mergedData[httpSnippetKey], httpSnippetDirectives)

	// Build owner reference to the original ConfigMap so Kubernetes garbage-collects
	// the DD ConfigMap if the original is deleted.
	var ownerRefs []metav1.OwnerReference
	if originalCM != nil {
		ownerRefs = []metav1.OwnerReference{
			{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       originalCMName,
				UID:        originalCM.GetUID(),
			},
		}
	}

	ddCM := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            ddName,
			Namespace:       namespace,
			Labels:          labels,
			Annotations:     annotations,
			OwnerReferences: ownerRefs,
		},
		Data: mergedData,
	}

	unstructuredCM, err := runtime.DefaultUnstructuredConverter.ToUnstructured(ddCM)
	if err != nil {
		return fmt.Errorf("failed to convert ConfigMap to unstructured: %w", err)
	}

	target := &unstructured.Unstructured{Object: unstructuredCM}
	_, err = client.Resource(configMapGVR).Namespace(namespace).Create(ctx, target, metav1.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create DD ConfigMap %s/%s: %w", namespace, ddName, err)
	}
	if err != nil {
		// Update existing DD ConfigMap. Retry on conflict since concurrent MutatePod
		// calls (e.g. during rollouts) may race on the same ConfigMap's resourceVersion.
		for retries := 0; retries < 3; retries++ {
			existing, getErr := client.Resource(configMapGVR).Namespace(namespace).Get(ctx, ddName, metav1.GetOptions{})
			if getErr != nil {
				return fmt.Errorf("failed to get existing DD ConfigMap for update: %w", getErr)
			}
			target.SetResourceVersion(existing.GetResourceVersion())
			_, err = client.Resource(configMapGVR).Namespace(namespace).Update(ctx, target, metav1.UpdateOptions{})
			if err == nil || !k8serrors.IsConflict(err) {
				break
			}
		}
		if err != nil {
			return fmt.Errorf("failed to update DD ConfigMap %s/%s: %w", namespace, ddName, err)
		}
	}

	// Label the original ConfigMap so the reconciler can watch it for changes.
	// Skip if the original doesn't exist (originalCM is nil when IsNotFound).
	if originalCM != nil {
		if err := labelOriginalConfigMap(ctx, client, namespace, originalCMName, ddName); err != nil {
			return fmt.Errorf("failed to label original ConfigMap %s/%s: %w", namespace, originalCMName, err)
		}
	}

	return nil
}

// labelOriginalConfigMap adds the watched-configmap label and dd-configmap annotation
// to the original ConfigMap so the reconciler informer can discover and reconcile it.
// It is idempotent: if the label and annotation are already set correctly, no update is performed.
func labelOriginalConfigMap(ctx context.Context, client dynamic.Interface, namespace, originalCMName, ddCMName string) error {
	for retries := 0; retries < 3; retries++ {
		cm, err := client.Resource(configMapGVR).Namespace(namespace).Get(ctx, originalCMName, metav1.GetOptions{})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return nil // Original was deleted in the meantime, nothing to label
			}
			return fmt.Errorf("failed to get original ConfigMap: %w", err)
		}

		labels := cm.GetLabels()
		annotations := cm.GetAnnotations()

		if labels == nil {
			labels = make(map[string]string, 1)
		}

		if annotations == nil {
			annotations = make(map[string]string, 1)
		}

		// Check if already set correctly (idempotency)
		if labels[watchedConfigMapLabel] == "true" && annotations[ddConfigMapAnnotation] == ddCMName {
			return nil
		}

		labels[watchedConfigMapLabel] = "true"
		cm.SetLabels(labels)

		annotations[ddConfigMapAnnotation] = ddCMName
		cm.SetAnnotations(annotations)

		_, err = client.Resource(configMapGVR).Namespace(namespace).Update(ctx, cm, metav1.UpdateOptions{})
		if err == nil || !k8serrors.IsConflict(err) {
			return err
		}
	}
	return fmt.Errorf("failed to label original ConfigMap %s/%s after retries", namespace, originalCMName)
}

// unlabelOriginalConfigMap removes the watched-configmap label and dd-configmap annotation
// from the original ConfigMap. It is safe to call if the original no longer exists.
// It retries on conflict, consistent with labelOriginalConfigMap.
func unlabelOriginalConfigMap(ctx context.Context, client dynamic.Interface, namespace, originalCMName string) error {
	for retries := 0; retries < 3; retries++ {
		cm, err := client.Resource(configMapGVR).Namespace(namespace).Get(ctx, originalCMName, metav1.GetOptions{})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("failed to get original ConfigMap: %w", err)
		}

		labels := cm.GetLabels()
		annotations := cm.GetAnnotations()

		// Nothing to remove
		if labels[watchedConfigMapLabel] == "" && annotations[ddConfigMapAnnotation] == "" {
			return nil
		}

		delete(labels, watchedConfigMapLabel)
		cm.SetLabels(labels)

		delete(annotations, ddConfigMapAnnotation)
		cm.SetAnnotations(annotations)

		_, err = client.Resource(configMapGVR).Namespace(namespace).Update(ctx, cm, metav1.UpdateOptions{})
		if err == nil || k8serrors.IsNotFound(err) {
			return nil
		}
		if !k8serrors.IsConflict(err) {
			return fmt.Errorf("failed to unlabel original ConfigMap %s/%s: %w", namespace, originalCMName, err)
		}
	}
	return fmt.Errorf("failed to unlabel original ConfigMap %s/%s after retries", namespace, originalCMName)
}
