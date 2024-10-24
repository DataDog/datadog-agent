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
	"strconv"

	admiv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
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
	operations        []admissionregistrationv1.OperationType
	namespaceSelector *metav1.LabelSelector
	objectSelector    *metav1.LabelSelector
	containerRegistry string

	// These fields store datadog agent config parameters
	// to avoid calling the config resolution each time the webhook
	// receives requests because the resolution is CPU expensive.
	profilesJSON                 string
	provider                     string
	imageName                    string
	imageTag                     string
	isLangDetectEnabled          bool
	isLangDetectReportingEnabled bool
	isClusterAgentEnabled        bool
	clusterAgentCmdPort          int
	clusterAgentServiceName      string
}

// NewWebhook returns a new Webhook
func NewWebhook(datadogConfig config.Component) *Webhook {
	nsSelector, objSelector := labelSelectors(datadogConfig)

	containerRegistry := mutatecommon.ContainerRegistry(datadogConfig, "admission_controller.agent_sidecar.container_registry")

	return &Webhook{
		name:              webhookName,
		isEnabled:         datadogConfig.GetBool("admission_controller.agent_sidecar.enabled"),
		endpoint:          datadogConfig.GetString("admission_controller.agent_sidecar.endpoint"),
		resources:         []string{"pods"},
		operations:        []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
		namespaceSelector: nsSelector,
		objectSelector:    objSelector,
		containerRegistry: containerRegistry,

		profilesJSON:                 datadogConfig.GetString("admission_controller.agent_sidecar.profiles"),
		provider:                     datadogConfig.GetString("admission_controller.agent_sidecar.provider"),
		imageName:                    datadogConfig.GetString("admission_controller.agent_sidecar.image_name"),
		imageTag:                     datadogConfig.GetString("admission_controller.agent_sidecar.image_tag"),
		clusterAgentServiceName:      datadogConfig.GetString("cluster_agent.kubernetes_service_name"),
		clusterAgentCmdPort:          datadogConfig.GetInt("cluster_agent.cmd_port"),
		isClusterAgentEnabled:        datadogConfig.GetBool("admission_controller.agent_sidecar.cluster_agent.enabled"),
		isLangDetectEnabled:          datadogConfig.GetBool("language_detection.enabled"),
		isLangDetectReportingEnabled: datadogConfig.GetBool("language_detection.reporting.enabled"),
	}
}

// Name returns the name of the webhook
func (w *Webhook) Name() string {
	return w.name
}

// WebhookType returns the type of the webhook
func (w *Webhook) WebhookType() common.WebhookType {
	return common.MutatingWebhook
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
func (w *Webhook) Operations() []admissionregistrationv1.OperationType {
	return w.operations
}

// LabelSelectors returns the label selectors that specify when the webhook
// should be invoked
func (w *Webhook) LabelSelectors(_ bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	return w.namespaceSelector, w.objectSelector
}

// WebhookFunc returns the function that mutates the resources
func (w *Webhook) WebhookFunc() admission.WebhookFunc {
	return func(request *admission.Request) *admiv1.AdmissionResponse {
		return common.MutationResponse(mutatecommon.Mutate(request.Raw, request.Namespace, w.Name(), w.injectAgentSidecar, request.DynamicClient))
	}
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
		agentSidecarContainer := w.getDefaultSidecarTemplate()
		pod.Spec.Containers = append(pod.Spec.Containers, *agentSidecarContainer)
		podUpdated = true
	}

	updated, err := applyProviderOverrides(pod, w.provider)
	if err != nil {
		log.Errorf("Failed to apply provider overrides: %v", err)
		return podUpdated, errors.New(metrics.InvalidInput)
	}
	podUpdated = podUpdated || updated

	// User-provided overrides should always be applied last in order to have
	// highest override-priority. They only apply to the agent sidecar container.
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == agentSidecarContainerName {
			updated, err = applyProfileOverrides(&pod.Spec.Containers[i], w.profilesJSON)
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

func (w *Webhook) getDefaultSidecarTemplate() *corev1.Container {
	ddSite := os.Getenv("DD_SITE")
	if ddSite == "" {
		ddSite = pkgconfigsetup.DefaultSite
	}

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
			{
				Name:  "DD_LANGUAGE_DETECTION_ENABLED",
				Value: strconv.FormatBool(w.isLangDetectEnabled && w.isLangDetectReportingEnabled),
			},
		},
		Image:           fmt.Sprintf("%s/%s:%s", w.containerRegistry, w.imageName, w.imageTag),
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

	if w.isClusterAgentEnabled {

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
			Value: fmt.Sprintf("https://%s.%s.svc.cluster.local:%v", w.clusterAgentServiceName, apiCommon.GetMyNamespace(), w.clusterAgentCmdPort),
		}, corev1.EnvVar{
			Name:  "DD_ORCHESTRATOR_EXPLORER_ENABLED",
			Value: "true",
		})
	}

	return agentContainer
}

// labelSelectors returns the mutating webhooks object selectors based on the configuration
func labelSelectors(datadogConfig config.Component) (namespaceSelector, objectSelector *metav1.LabelSelector) {
	// Read and parse selectors
	selectorsJSON := datadogConfig.GetString("admission_controller.agent_sidecar.selectors")
	profilesJSON := datadogConfig.GetString("admission_controller.agent_sidecar.profiles")

	// Get sidecar profiles
	_, err := loadSidecarProfiles(profilesJSON)
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

	provider := datadogConfig.GetString("admission_controller.agent_sidecar.provider")
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
