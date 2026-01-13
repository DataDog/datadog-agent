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

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeClient "k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const DefaultAppName = "test-app"

var DefaultExpectedContainers = []string{DefaultAppName}

func GetPodsInNamespace(t *testing.T, client kubeClient.Interface, namespace string) []corev1.Pod {
	res, err := client.CoreV1().Pods(namespace).List(context.Background(), v1.ListOptions{})
	require.NoError(t, err, "received an error fetching pods")
	return res.Items
}

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
