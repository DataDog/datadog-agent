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

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1k8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1k8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeClient "k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
)

// deployTestWorkload creates a namespace and a minimal Deployment in it to trigger
// the admission webhook. The Deployment uses pulumi.com/skipAwait so Pulumi does
// not wait for pods to become Ready — the pods will be stuck in ImagePullBackOff
// (the mock registry serves HEAD requests only, not actual image data), but the
// admission webhook still mutates the pod spec at creation time, which is all the
// gradual rollout tests need to inspect.
func deployTestWorkload(e config.Env, kubeProvider *kubernetes.Provider, namespace, appName string, opts ...pulumi.ResourceOption) error {
	baseOpts := append([]pulumi.ResourceOption{pulumi.Provider(kubeProvider)}, opts...)

	ns, err := corev1k8s.NewNamespace(e.Ctx(), e.CommonNamer().ResourceName(namespace+"-ns"), &corev1k8s.NamespaceArgs{
		Metadata: &metav1k8s.ObjectMetaArgs{
			Name: pulumi.String(namespace),
		},
	}, baseOpts...)
	if err != nil {
		return fmt.Errorf("failed to create namespace %s: %w", namespace, err)
	}

	nsOpts := append(baseOpts, pulumi.DependsOn([]pulumi.Resource{ns}))
	_, err = appsv1.NewDeployment(e.Ctx(), e.CommonNamer().ResourceName(appName+"-deployment"), &appsv1.DeploymentArgs{
		Metadata: &metav1k8s.ObjectMetaArgs{
			Name:      pulumi.String(appName),
			Namespace: pulumi.String(namespace),
			// Skip Pulumi's rollout-readiness wait: pods will be stuck in
			// ImagePullBackOff on the fake-digest init container image, but
			// the pod spec (with the @sha256: image) is set immediately.
			Annotations: pulumi.StringMap{
				"pulumi.com/skipAwait": pulumi.String("true"),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1k8s.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{"app": pulumi.String(appName)},
			},
			Template: &corev1k8s.PodTemplateSpecArgs{
				Metadata: &metav1k8s.ObjectMetaArgs{
					Labels: pulumi.StringMap{"app": pulumi.String(appName)},
				},
				Spec: &corev1k8s.PodSpecArgs{
					Containers: corev1k8s.ContainerArray{
						&corev1k8s.ContainerArgs{
							Name: pulumi.String(appName),
							// pause is a stable, language-neutral placeholder. SSI's webhook
							// injects lib init containers based on the namespace/target match,
							// not the workload image, so any image works here. Pods stay in
							// ImagePullBackOff (skipAwait above), but the mutated spec is set
							// at create.
							Image: pulumi.String("registry.k8s.io/pause:3.9"),
						},
					},
				},
			},
		},
	}, nsOpts...)
	if err != nil {
		return fmt.Errorf("failed to create deployment %s: %w", appName, err)
	}

	return nil
}

// getPodsInNamespace returns all pods in the given namespace.
func getPodsInNamespace(t *testing.T, client kubeClient.Interface, namespace string) []corev1.Pod {
	res, err := client.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err, "received an error fetching pods")
	return res.Items
}

// findLibInitContainer returns the init container whose image contains "dd-lib-{language}-init".
func findLibInitContainer(pod *corev1.Pod, language string) (*corev1.Container, bool) {
	needle := fmt.Sprintf("dd-lib-%s-init", language)
	for i := range pod.Spec.InitContainers {
		if strings.Contains(pod.Spec.InitContainers[i].Image, needle) {
			return &pod.Spec.InitContainers[i], true
		}
	}
	return nil, false
}

// requireDigestBasedLibImage asserts that the lib init container for the given language
// uses the exact digest the mock registry returned. Matching the precise digest (rather
// than just any "@sha256:..." string) proves the cluster-agent contacted *our* mock and
// used the response, ruling out a regression where the digest came from somewhere else
// or was fabricated without ever calling the resolver.
func requireDigestBasedLibImage(t *testing.T, pod *corev1.Pod, language string) {
	t.Helper()
	container, found := findLibInitContainer(pod, language)
	require.True(t, found, "did not find dd-lib-%s-init container in pod %s", language, pod.Name)
	require.Contains(t, container.Image, "@"+fakeRegistryDigest,
		"expected mock-registry digest for dd-lib-%s-init in pod %s, got: %s", language, pod.Name, container.Image)
}

// findMutatedPod waits up to 2 minutes for a pod in the namespace whose name contains
// appName and which has a lib init container for the given language, then returns it.
func findMutatedPod(t *testing.T, k8s kubeClient.Interface, namespace, appName, language string) *corev1.Pod {
	t.Helper()
	var result *corev1.Pod
	require.Eventually(t, func() bool {
		pods := getPodsInNamespace(t, k8s, namespace)
		for i := range pods {
			pod := &pods[i]
			if !strings.Contains(pod.Name, appName) {
				continue
			}
			if _, found := findLibInitContainer(pod, language); found {
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
