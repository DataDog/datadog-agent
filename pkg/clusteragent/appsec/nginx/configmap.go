// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package nginx

import (
	"context"
	"fmt"
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
)

var configMapGVR = corev1.SchemeGroupVersion.WithResource("configmaps")

// mainSnippetDirectives returns the nginx main-context directives for loading the datadog module.
// The env directive preserves DD_AGENT_HOST for nginx worker processes so the module
// can reach the Datadog agent (nginx strips env vars from workers by default).
func mainSnippetDirectives(moduleMountPath string) string {
	return fmt.Sprintf("load_module %s/ngx_http_datadog_module.so;\nthread_pool waf_thread_pool threads=2 max_queue=16;\nenv DD_AGENT_HOST;", moduleMountPath)
}

// httpSnippetDirectives are injected into the nginx http context to enable AppSec
const httpSnippetDirectivesContent = "datadog_appsec_enabled on;\ndatadog_waf_thread_pool_name waf_thread_pool;"

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
	if err == nil {
		data, _, _ := unstructured.NestedStringMap(originalCM.UnstructuredContent(), "data")
		if data != nil {
			originalData = data
		}
	} else if !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to get original ConfigMap %s/%s: %w", namespace, originalCMName, err)
	}

	// Build merged data
	mergedData := make(map[string]string, len(originalData)+2)
	for k, v := range originalData {
		mergedData[k] = v
	}
	mergedData[mainSnippetKey] = buildSnippet(mergedData[mainSnippetKey], mainSnippetDirectives(moduleMountPath))
	mergedData[httpSnippetKey] = buildSnippet(mergedData[httpSnippetKey], httpSnippetDirectivesContent)

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

	_, err = client.Resource(configMapGVR).Namespace(namespace).Create(ctx, &unstructured.Unstructured{Object: unstructuredCM}, metav1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		// Update existing DD ConfigMap
		existing, getErr := client.Resource(configMapGVR).Namespace(namespace).Get(ctx, ddName, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to get existing DD ConfigMap for update: %w", getErr)
		}
		// Preserve resourceVersion for update
		unstructuredCM["metadata"].(map[string]interface{})["resourceVersion"] = existing.GetResourceVersion()
		_, err = client.Resource(configMapGVR).Namespace(namespace).Update(ctx, &unstructured.Unstructured{Object: unstructuredCM}, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update DD ConfigMap %s/%s: %w", namespace, ddName, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to create DD ConfigMap %s/%s: %w", namespace, ddName, err)
	}

	return nil
}
