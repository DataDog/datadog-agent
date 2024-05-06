// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package agentsidecar defines the mutation logic for the agentsidecar webhook
package agentsidecar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"

	admiv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	apiCommon "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const webhookName = "agent_sidecar"

// Selector specifies an object label selector and a namespace label selector
type Selector struct {
	ObjectSelector    metav1.LabelSelector `json:"objectSelector,omitempty"`
	NamespaceSelector metav1.LabelSelector `json:"namespaceSelector,omitempty"`
}

// Webhook is the webhook that injects a Datadog Agent sidecar
type Webhook struct {
	name              string
	isEnabled         bool
	endpoint          string
	resources         []string
	operations        []admiv1.OperationType
	namespaceSelector *metav1.LabelSelector
	objectSelector    *metav1.LabelSelector
	containerRegistry string
}

// NewWebhook returns a new Webhook
func NewWebhook() *Webhook {
	nsSelector, objSelector := labelSelectors()

	containerRegistry := common.ContainerRegistry("admission_controller.agent_sidecar.container_registry")

	return &Webhook{
		name:              webhookName,
		isEnabled:         config.Datadog.GetBool("admission_controller.agent_sidecar.enabled"),
		endpoint:          config.Datadog.GetString("admission_controller.agent_sidecar.endpoint"),
		resources:         []string{"pods"},
		operations:        []admiv1.OperationType{admiv1.Create},
		namespaceSelector: nsSelector,
		objectSelector:    objSelector,
		containerRegistry: containerRegistry,
	}
}

// Name returns the name of the webhook
func (w *Webhook) Name() string {
	return w.name
}

// IsEnabled returns whether the webhook is enabled
func (w *Webhook) IsEnabled() bool {
	return w.isEnabled && (w.namespaceSelector != nil || w.objectSelector != nil)
}

// Endpoint returns the endpoint of the webhook
func (w *Webhook) Endpoint() string {
	return w.endpoint
}

// Resources returns the kubernetes resources for which the webhook should
// be invoked
func (w *Webhook) Resources() []string {
	return w.resources
}

// Operations returns the operations on the resources specified for which
// the webhook should be invoked
func (w *Webhook) Operations() []admiv1.OperationType {
	return w.operations
}

// LabelSelectors returns the label selectors that specify when the webhook
// should be invoked
func (w *Webhook) LabelSelectors(_ bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	return w.namespaceSelector, w.objectSelector
}

// MutateFunc returns the function that mutates the resources
func (w *Webhook) MutateFunc() admission.WebhookFunc {
	return w.mutate
}

// mutate handles mutating pod requests for the agentsidecar webhook
func (w *Webhook) mutate(request *admission.MutateRequest) ([]byte, error) {
	return common.Mutate(request.Raw, request.Namespace, w.Name(), w.injectAgentSidecar, request.DynamicClient)
}

func (w *Webhook) injectAgentSidecar(pod *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
	if pod == nil {
		return false, errors.New(metrics.InvalidInput)
	}

	agentSidecarExists := slices.ContainsFunc(pod.Spec.Containers, func(cont corev1.Container) bool {
		return cont.Name == agentSidecarContainerName
	})

	podUpdated := false

	if !agentSidecarExists {
		agentSidecarContainer := getDefaultSidecarTemplate(w.containerRegistry)
		pod.Spec.Containers = append(pod.Spec.Containers, *agentSidecarContainer)
		podUpdated = true
	}

	updated, err := applyProviderOverrides(pod)
	if err != nil {
		log.Errorf("Failed to apply provider overrides: %v", err)
		return podUpdated, errors.New(metrics.InvalidInput)
	}
	podUpdated = podUpdated || updated

	// User-provided overrides should always be applied last in order to have
	// highest override-priority. They only apply to the agent sidecar container.
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == agentSidecarContainerName {
			updated, err = applyProfileOverrides(&pod.Spec.Containers[i])
			if err != nil {
				log.Errorf("Failed to apply profile overrides: %v", err)
				return podUpdated, errors.New(metrics.InvalidInput)
			}
			podUpdated = podUpdated || updated
			break
		}
	}

	return podUpdated, nil
}

func getDefaultSidecarTemplate(containerRegistry string) *corev1.Container {
	ddSite := os.Getenv("DD_SITE")
	if ddSite == "" {
		ddSite = config.DefaultSite
	}

	imageName := config.Datadog.GetString("admission_controller.agent_sidecar.image_name")
	imageTag := config.Datadog.GetString("admission_controller.agent_sidecar.image_tag")

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
				Value: ddSite,
			},
			{
				Name:  "DD_CLUSTER_NAME",
				Value: clustername.GetClusterName(context.TODO(), ""),
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
		Image:           fmt.Sprintf("%s/%s:%s", containerRegistry, imageName, imageTag),
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

	clusterAgentEnabled := config.Datadog.GetBool("admission_controller.agent_sidecar.cluster_agent.enabled")

	if clusterAgentEnabled {
		clusterAgentCmdPort := config.Datadog.GetInt("cluster_agent.cmd_port")
		clusterAgentServiceName := config.Datadog.GetString("cluster_agent.kubernetes_service_name")

		_, _ = withEnvOverrides(agentContainer, corev1.EnvVar{
			Name:  "DD_CLUSTER_AGENT_ENABLED",
			Value: "true",
		}, corev1.EnvVar{
			Name: "DD_CLUSTER_AGENT_AUTH_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: "token",
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "datadog-secret",
					},
				},
			},
		}, corev1.EnvVar{
			Name:  "DD_CLUSTER_AGENT_URL",
			Value: fmt.Sprintf("https://%s.%s.svc.cluster.local:%v", clusterAgentServiceName, apiCommon.GetMyNamespace(), clusterAgentCmdPort),
		}, corev1.EnvVar{
			Name:  "DD_ORCHESTRATOR_EXPLORER_ENABLED",
			Value: "true",
		})
	}

	return agentContainer
}

// labelSelectors returns the mutating webhooks object selectors based on the configuration
func labelSelectors() (namespaceSelector, objectSelector *metav1.LabelSelector) {
	// Read and parse selectors
	selectorsJSON := config.Datadog.GetString("admission_controller.agent_sidecar.selectors")

	// Get sidecar profiles
	_, err := loadSidecarProfiles()
	if err != nil {
		log.Errorf("encountered issue when loading sidecar profiles: %s", err)
		return nil, nil
	}

	var selectors []Selector

	err = json.Unmarshal([]byte(selectorsJSON), &selectors)
	if err != nil {
		log.Errorf("failed to parse selectors for admission controller agent sidecar injection webhook: %s", err)
		return nil, nil
	}

	if len(selectors) > 1 {
		log.Errorf("configuring more than 1 selector is not supported")
		return nil, nil
	}

	provider := config.Datadog.GetString("admission_controller.agent_sidecar.provider")
	if !providerIsSupported(provider) {
		log.Errorf("agent sidecar provider is not supported: %v", provider)
		return nil, nil
	}

	if len(selectors) == 1 {
		namespaceSelector = &selectors[0].NamespaceSelector
		objectSelector = &selectors[0].ObjectSelector
	} else if provider != "" {
		log.Infof("using default selector \"agent.datadoghq.com/sidecar\": \"%v\" for provider %v", provider, provider)
		namespaceSelector = nil
		objectSelector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"agent.datadoghq.com/sidecar": provider,
			},
		}
	}

	return namespaceSelector, objectSelector
}
