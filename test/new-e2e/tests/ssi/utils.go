// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ssi

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeClient "k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const DefaultAppName = "test-app"

// apmInjectionAnnotationPrefix is the prefix the APM admission controller uses for the observability
// annotations it adds to every pod it processes (whether the outcome is injected, skipped or error).
const apmInjectionAnnotationPrefix = "internal.apm.datadoghq.com/"

// datadogMutatingWebhookName is the default name of the MutatingWebhookConfiguration created by the
// cluster agent admission controller (admission_controller.webhook_name).
const datadogMutatingWebhookName = "datadog-webhook"

var DefaultExpectedContainers = []string{DefaultAppName}

func GetPodsInNamespace(t *testing.T, client kubeClient.Interface, namespace string) []corev1.Pod {
	res, err := client.CoreV1().Pods(namespace).List(context.Background(), v1.ListOptions{})
	require.NoError(t, err, "received an error fetching pods")
	return res.Items
}

func FindPodInNamespace(t *testing.T, client kubeClient.Interface, namespace string, appName string) *corev1.Pod {
	pods := GetPodsInNamespace(t, client, namespace)
	for _, pod := range pods {
		if strings.Contains(pod.Name, appName) && pod.DeletionTimestamp == nil {
			return &pod
		}
	}
	require.NoError(t, fmt.Errorf("did not find pod with app name %s in namespace %s", appName, namespace))
	return nil
}

// RestartPod deletes the pod backing appName so its Deployment recreates it, forcing a fresh pass
// through the admission webhook. It blocks until a new running pod (different from the deleted one)
// is observed.
func RestartPod(t *testing.T, client kubeClient.Interface, namespace string, appName string) {
	old := FindPodInNamespace(t, client, namespace, appName)
	err := client.CoreV1().Pods(namespace).Delete(context.Background(), old.Name, v1.DeleteOptions{})
	require.NoError(t, err, "failed to delete pod %s in namespace %s", old.Name, namespace)

	require.Eventually(t, func() bool {
		for _, pod := range GetPodsInNamespace(t, client, namespace) {
			if strings.Contains(pod.Name, appName) && pod.Name != old.Name &&
				pod.DeletionTimestamp == nil && pod.Status.Phase == corev1.PodRunning {
				return true
			}
		}
		return false
	}, 2*time.Minute, 5*time.Second, "pod %s was not recreated in namespace %s", appName, namespace)
}

// WaitForAdmissionWebhookReady blocks until the Datadog mutating webhook configuration exists. The
// cluster agent creates it only after acquiring leadership and reconciling the admission controller,
// which on slow clusters (e.g. GKE Autopilot, where nodes are provisioned on demand) can lag well
// behind the Helm release being reported ready.
func WaitForAdmissionWebhookReady(t *testing.T, client kubeClient.Interface) {
	require.Eventually(t, func() bool {
		_, err := client.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(context.Background(), datadogMutatingWebhookName, v1.GetOptions{})
		return err == nil
	}, 5*time.Minute, 5*time.Second, "Datadog mutating webhook %q was not created in time", datadogMutatingWebhookName)
}

// WaitForMutatedPodInNamespace returns a pod matching appName in the namespace that has been
// processed by the APM admission webhook (i.e. carries at least one internal.apm.datadoghq.com/*
// annotation).
//
// Pod mutation annotations are set at admission time (pod creation), so a pod that was admitted
// before the webhook was serving is never processed retroactively (failurePolicy is Ignore). This
// happens for the application pods created during provisioning on slow clusters (e.g. GKE Autopilot)
// where the cluster agent admission controller comes up well after the Helm release is ready.
//
// To get a mutated pod we therefore wait (without restarting) for the webhook to exist, then
// recreate the pod so it goes through it. A short retry absorbs the brief gap between the webhook
// configuration being created and its endpoint actually serving. On clusters where the webhook was
// already serving, the first matching pod is returned without any restart.
func WaitForMutatedPodInNamespace(t *testing.T, client kubeClient.Interface, namespace string, appName string) *corev1.Pod {
	pod := FindPodInNamespace(t, client, namespace, appName)
	if hasAPMInjectionAnnotation(pod) {
		return pod
	}

	WaitForAdmissionWebhookReady(t, client)

	require.Eventually(t, func() bool {
		RestartPod(t, client, namespace, appName)
		pod = FindPodInNamespace(t, client, namespace, appName)
		return hasAPMInjectionAnnotation(pod)
	}, 2*time.Minute, 5*time.Second, "pod %s in namespace %s was not processed by the admission webhook after restart", appName, namespace)
	return pod
}

// hasAPMInjectionAnnotation reports whether the pod carries an APM admission controller annotation,
// which is set on every pod the webhook processes regardless of the injection outcome.
func hasAPMInjectionAnnotation(pod *corev1.Pod) bool {
	for key := range pod.Annotations {
		if strings.HasPrefix(key, apmInjectionAnnotationPrefix) {
			return true
		}
	}
	return false
}

func FindTracesForService(t *testing.T, intake *fakeintake.Client, serviceName string) []*trace.TracerPayload {
	filtered := []*trace.TracerPayload{}
	serviceNameTag := "service:" + serviceName

	payloads, err := intake.GetTraces()
	require.NoError(t, err, "got error fetching traces from fake intake")
	for _, payload := range payloads {
		for _, trace := range payload.TracerPayloads {
			extracted, ok := trace.Tags["_dd.tags.container"]
			if !ok {
				continue
			}
			tags := strings.SplitSeq(extracted, ",")
			for tag := range tags {
				if tag == serviceNameTag {
					filtered = append(filtered, trace)
				}
			}
		}
	}

	return filtered
}
