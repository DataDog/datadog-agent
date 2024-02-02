//go:build kubeapiserver

package agentsidecar

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const agentSidecarContainerName = "datadog-agent-injected"

func getDefaultSidecarTemplate() *corev1.Container {
	agentContainer := &corev1.Container{
		Env: []corev1.EnvVar{
			{
				Name: "DD_API_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						Key: "api-key",
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "datadog-secret",
						},
					},
				},
			},
			{
				Name:  "DD_SITE",
				Value: "datadoghq.com",
			},
			{
				Name:  "DD_CLUSTER_NAME",
				Value: config.Datadog.GetString("cluster_name"),
			},
			{
				Name: "DD_KUBERNETES_KUBELET_NODENAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						APIVersion: "v1",
						FieldPath:  "spec.nodeName",
					},
				},
			},
		},
		Image:           "datadog/agent",
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            agentSidecarContainerName,
		Resources: corev1.ResourceRequirements{
			Requests: map[corev1.ResourceName]resource.Quantity{
				"memory": resource.MustParse("256Mi"),
				"cpu":    resource.MustParse("200m"),
			},
			Limits: map[corev1.ResourceName]resource.Quantity{
				"memory": resource.MustParse("256Mi"),
				"cpu":    resource.MustParse("200m"),
			},
		},
	}

	return agentContainer
}
