// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ssigradualrollout

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeClient "k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// GetPodsInNamespace returns all pods in the given namespace.
func GetPodsInNamespace(t *testing.T, client kubeClient.Interface, namespace string) []corev1.Pod {
	res, err := client.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err, "received an error fetching pods")
	return res.Items
}

// FindPodInNamespace returns the first pod whose name contains appName in the given namespace.
func FindPodInNamespace(t *testing.T, client kubeClient.Interface, namespace string, appName string) *corev1.Pod {
	pods := GetPodsInNamespace(t, client, namespace)
	for _, pod := range pods {
		if strings.Contains(pod.Name, appName) {
			return &pod
		}
	}
	require.NoError(t, fmt.Errorf("did not find pod with app name %s in namespace %s", appName, namespace))
	return nil
}

// FindTracesForService returns all tracer payloads from the fakeintake for the given service name.
func FindTracesForService(t *testing.T, intake *fakeintake.Client, serviceName string) []*trace.TracerPayload {
	filtered := []*trace.TracerPayload{}
	serviceNameTag := "service:" + serviceName

	payloads, err := intake.GetTraces()
	require.NoError(t, err, "got error fetching traces from fake intake")
	for _, payload := range payloads {
		for _, tr := range payload.TracerPayloads {
			extracted, ok := tr.Tags["_dd.tags.container"]
			if !ok {
				continue
			}
			tags := strings.SplitSeq(extracted, ",")
			for tag := range tags {
				if tag == serviceNameTag {
					filtered = append(filtered, tr)
				}
			}
		}
	}

	return filtered
}

// FindLibInitContainer returns the init container whose image contains "dd-lib-{language}-init".
func FindLibInitContainer(pod *corev1.Pod, language string) (*corev1.Container, bool) {
	needle := fmt.Sprintf("dd-lib-%s-init", language)
	for i := range pod.Spec.InitContainers {
		if strings.Contains(pod.Spec.InitContainers[i].Image, needle) {
			return &pod.Spec.InitContainers[i], true
		}
	}
	return nil, false
}

// RequireDigestBasedLibImage asserts that the lib init container for the given language
// uses a digest-based image reference (i.e. the image contains "@sha256:").
func RequireDigestBasedLibImage(t *testing.T, pod *corev1.Pod, language string) {
	t.Helper()
	container, found := FindLibInitContainer(pod, language)
	require.True(t, found, "did not find dd-lib-%s-init container in pod %s", language, pod.Name)
	require.Contains(t, container.Image, "@sha256:",
		"expected digest-based image ref for dd-lib-%s-init in pod %s, got: %s", language, pod.Name, container.Image)
}

// RequireTagBasedLibImage asserts that the lib init container for the given language
// uses a tag-based image reference (i.e. the image does NOT contain "@sha256:").
// If expectedTag is non-empty, it also asserts that the image ends with ":{expectedTag}".
func RequireTagBasedLibImage(t *testing.T, pod *corev1.Pod, language string, expectedTag string) {
	t.Helper()
	container, found := FindLibInitContainer(pod, language)
	require.True(t, found, "did not find dd-lib-%s-init container in pod %s", language, pod.Name)
	require.NotContains(t, container.Image, "@sha256:",
		"expected tag-based image ref for dd-lib-%s-init in pod %s, got: %s", language, pod.Name, container.Image)
	if expectedTag != "" {
		require.True(t, strings.HasSuffix(container.Image, ":"+expectedTag),
			"expected dd-lib-%s-init image to end with :%s in pod %s, got: %s", language, expectedTag, pod.Name, container.Image)
	}
}

// findMutatedPod waits up to 2 minutes for a pod in the namespace whose name contains
// appName and which has a lib init container for the given language, then returns it.
func findMutatedPod(t *testing.T, k8s kubeClient.Interface, namespace, appName, language string) *corev1.Pod {
	t.Helper()
	var result *corev1.Pod
	require.Eventually(t, func() bool {
		pods := GetPodsInNamespace(t, k8s, namespace)
		for i := range pods {
			pod := &pods[i]
			if !strings.Contains(pod.Name, appName) {
				continue
			}
			if _, found := FindLibInitContainer(pod, language); found {
				result = pod
				return true
			}
		}
		return false
	}, 2*time.Minute, 5*time.Second,
		"timed out waiting for a mutated pod with app name %s and dd-lib-%s-init in namespace %s",
		appName, language, namespace)
	return result
}
