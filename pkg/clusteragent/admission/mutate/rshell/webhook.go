// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

// Package rshell implements the admission webhook for injecting the rshell binary
// into pods so that exec_command kubeactions can run a restricted shell inside them.
package rshell

import (
	"fmt"

	admiv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	webhookName     = "rshell_injection"
	webhookEndpoint = "/inject-rshell"

	rshellVolumeName        = "datadog-rshell"
	rshellMountPath         = "/datadog-rshell"
	rshellBinaryPath        = "/datadog-rshell/rshell"
	rshellInitContainerName = "rshell-injection"

	// podLabelEnabled gates which pods are mutated when mutate_unlabelled is false.
	podLabelEnabled = "admission.datadoghq.com/rshell-injection.enabled"

	// Annotations stamped on mutated pods. The exec_command executor
	// (pkg/clusteragent/kubeactions/executors) reads these to fail fast when the
	// binary is absent and to locate it. Keep the literals in sync with that package.
	rshellInjectedAnnotation   = "rshell.datadoghq.com/injected"
	rshellBinaryPathAnnotation = "rshell.datadoghq.com/binary-path"
)

// Webhook injects the rshell binary into pods via an init container and a shared volume.
type Webhook struct {
	name       string
	isEnabled  bool
	endpoint   string
	resources  []common.WebhookResourceRule
	operations []admissionregistrationv1.OperationType
	image      string
	datadogCfg config.Component
}

// NewWebhook creates a new rshell injection webhook.
func NewWebhook(datadogConfig config.Component) *Webhook {
	imageName := datadogConfig.GetString("admission_controller.rshell_injection.image_name")
	imageTag := datadogConfig.GetString("admission_controller.rshell_injection.image_tag")
	if imageTag == "" {
		imageTag = "latest"
	}
	registry := mutatecommon.ContainerRegistry(datadogConfig, "admission_controller.rshell_injection.container_registry")

	image := fmt.Sprintf("%s:%s", imageName, imageTag)
	if registry != "" {
		image = fmt.Sprintf("%s/%s", registry, image)
	}

	return &Webhook{
		name:       webhookName,
		isEnabled:  datadogConfig.GetBool("admission_controller.rshell_injection.enabled") && imageName != "",
		endpoint:   webhookEndpoint,
		resources:  []common.WebhookResourceRule{{APIGroup: "", APIVersion: "v1", Resources: []string{"pods"}}},
		operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
		image:      image,
		datadogCfg: datadogConfig,
	}
}

// Name returns the name of the webhook.
func (w *Webhook) Name() string { return w.name }

// WebhookType returns the type of the webhook.
func (w *Webhook) WebhookType() common.WebhookType { return common.MutatingWebhook }

// IsEnabled returns whether the webhook is enabled.
func (w *Webhook) IsEnabled() bool { return w.isEnabled }

// Endpoint returns the endpoint of the webhook.
func (w *Webhook) Endpoint() string { return w.endpoint }

// Resources returns the kubernetes resources for which the webhook should be invoked.
func (w *Webhook) Resources() []common.WebhookResourceRule { return w.resources }

// Operations returns the operations on the resources specified for which the webhook should be invoked.
func (w *Webhook) Operations() []admissionregistrationv1.OperationType { return w.operations }

// LabelSelectors returns the label selectors that specify when the webhook should be invoked.
func (w *Webhook) LabelSelectors(useNamespaceSelector bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	var labelSelector metav1.LabelSelector
	if w.datadogCfg.GetBool("admission_controller.rshell_injection.mutate_unlabelled") ||
		w.datadogCfg.GetBool("admission_controller.mutate_unlabelled") {
		// Accept all, ignoring pods that are explicitly filtered-out.
		labelSelector = metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      podLabelEnabled,
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"false"},
				},
			},
		}
	} else {
		// Ignore all, accepting only pods that are explicitly allowed.
		labelSelector = metav1.LabelSelector{
			MatchLabels: map[string]string{podLabelEnabled: "true"},
		}
	}

	if useNamespaceSelector {
		return &labelSelector, nil
	}
	return nil, &labelSelector
}

// MatchConditions returns the match conditions for the webhook.
func (w *Webhook) MatchConditions() []admissionregistrationv1.MatchCondition { return nil }

// Timeout returns the timeout for the webhook.
func (w *Webhook) Timeout() int32 { return 0 }

// WebhookFunc returns the function that mutates the pod.
func (w *Webhook) WebhookFunc() admission.WebhookFunc {
	return func(request *admission.Request) *admiv1.AdmissionResponse {
		return common.MutationResponse(mutatecommon.Mutate(request.Object, request.Namespace, w.Name(), w.mutatePod, request.DynamicClient))
	}
}

// mutatePod injects the shared volume, the init container that populates it, and the
// volume mounts on every container, then stamps the injection-marker annotations.
func (w *Webhook) mutatePod(pod *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
	if pod == nil {
		return false, nil
	}

	injectRshellVolume(pod)
	for i := range pod.Spec.Containers {
		injectRshellVolumeMount(&pod.Spec.Containers[i])
	}
	if !injectRshellInitContainer(pod, w.image) {
		log.Debugf("rshell init container already present in pod %s, skipping", pod.Name)
	}

	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[rshellInjectedAnnotation] = "true"
	pod.Annotations[rshellBinaryPathAnnotation] = rshellBinaryPath

	return true, nil
}

func injectRshellVolume(pod *corev1.Pod) {
	volumeSource := corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}
	for i, vol := range pod.Spec.Volumes {
		if vol.Name == rshellVolumeName {
			pod.Spec.Volumes[i].VolumeSource = volumeSource
			return
		}
	}
	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
		Name:         rshellVolumeName,
		VolumeSource: volumeSource,
	})
	mutatecommon.MarkVolumeAsSafeToEvictForAutoscaler(pod, rshellVolumeName)
}

func injectRshellVolumeMount(container *corev1.Container) {
	for i, mnt := range container.VolumeMounts {
		if mnt.Name == rshellVolumeName {
			container.VolumeMounts[i].MountPath = rshellMountPath
			container.VolumeMounts[i].ReadOnly = true
			return
		}
	}
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      rshellVolumeName,
		MountPath: rshellMountPath,
		ReadOnly:  true,
	})
}

// injectRshellInitContainer adds the init container that copies the rshell binary into
// the shared volume. It returns false if the init container is already present.
func injectRshellInitContainer(pod *corev1.Pod, image string) bool {
	for _, c := range pod.Spec.InitContainers {
		if c.Name == rshellInitContainerName {
			return false
		}
	}
	initContainer := corev1.Container{
		Name:    rshellInitContainerName,
		Image:   image,
		Command: []string{"/bin/sh", "-c", "--"},
		Args:    []string{fmt.Sprintf("cp /rshell %s/rshell", rshellMountPath)},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      rshellVolumeName,
				MountPath: rshellMountPath,
			},
		},
	}
	pod.Spec.InitContainers = append([]corev1.Container{initContainer}, pod.Spec.InitContainers...)
	return true
}
