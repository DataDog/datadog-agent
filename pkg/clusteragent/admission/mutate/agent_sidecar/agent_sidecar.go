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
	"strings"

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
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
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
	resources         map[string][]string
	operations        []admissionregistrationv1.OperationType
	matchConditions   []admissionregistrationv1.MatchCondition
	namespaceSelector *metav1.LabelSelector
	objectSelector    *metav1.LabelSelector
	containerRegistry string

	// These fields store datadog agent config parameters
	// to avoid calling the config resolution each time the webhook
	// receives requests because the resolution is CPU expensive.
	profileOverrides             []ProfileOverride
	provider                     string
	imageName                    string
	imageTag                     string
	isLangDetectEnabled          bool
	isLangDetectReportingEnabled bool
	isClusterAgentEnabled        bool
	isKubeletAPILoggingEnabled   bool
	clusterAgentCmdPort          int
	clusterAgentServiceName      string
}

// NewWebhook returns a new Webhook
func NewWebhook(datadogConfig config.Component) *Webhook {
	profileOverrides, err := loadSidecarProfiles(datadogConfig.GetString("admission_controller.agent_sidecar.profiles"))
	if err != nil {
		log.Errorf("encountered issue when loading sidecar profiles: %s", err)
	}

	nsSelector, objSelector := labelSelectors(datadogConfig, profileOverrides)

	containerRegistry := mutatecommon.ContainerRegistry(datadogConfig, "admission_controller.agent_sidecar.container_registry")

	return &Webhook{
		name:              webhookName,
		isEnabled:         datadogConfig.GetBool("admission_controller.agent_sidecar.enabled"),
		endpoint:          datadogConfig.GetString("admission_controller.agent_sidecar.endpoint"),
		resources:         map[string][]string{"": {"pods"}},
		operations:        []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
		matchConditions:   []admissionregistrationv1.MatchCondition{},
		namespaceSelector: nsSelector,
		objectSelector:    objSelector,
		containerRegistry: containerRegistry,
		profileOverrides:  profileOverrides,

		provider:                     datadogConfig.GetString("admission_controller.agent_sidecar.provider"),
		imageName:                    datadogConfig.GetString("admission_controller.agent_sidecar.image_name"),
		imageTag:                     datadogConfig.GetString("admission_controller.agent_sidecar.image_tag"),
		clusterAgentServiceName:      datadogConfig.GetString("cluster_agent.kubernetes_service_name"),
		clusterAgentCmdPort:          datadogConfig.GetInt("cluster_agent.cmd_port"),
		isClusterAgentEnabled:        datadogConfig.GetBool("admission_controller.agent_sidecar.cluster_agent.enabled"),
		isKubeletAPILoggingEnabled:   datadogConfig.GetBool("admission_controller.agent_sidecar.kubelet_api_logging.enabled"),
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
func (w *Webhook) Resources() map[string][]string {
	return w.resources
}

// Timeout returns the timeout for the webhook
func (w *Webhook) Timeout() int32 {
	return 0
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

// MatchConditions returns the Match Conditions used for fine-grained
// request filtering
func (w *Webhook) MatchConditions() []admissionregistrationv1.MatchCondition {
	return w.matchConditions
}

// WebhookFunc returns the function that mutates the resources
func (w *Webhook) WebhookFunc() admission.WebhookFunc {
	return func(request *admission.Request) *admiv1.AdmissionResponse {
		return common.MutationResponse(mutatecommon.Mutate(request.Object, request.Namespace, w.Name(), w.injectAgentSidecar, request.DynamicClient))
	}
}

// isReadOnlyRootFilesystem returns whether the agent sidecar should have the readOnlyRootFilesystem security setup
func (w *Webhook) isReadOnlyRootFilesystem() bool {
	if len(w.profileOverrides) == 0 {
		return false
	}
	securityContext := w.profileOverrides[0].SecurityContext
	if securityContext != nil && securityContext.ReadOnlyRootFilesystem != nil {
		return *securityContext.ReadOnlyRootFilesystem
	}
	return false // default to false (temp)
}

func attachVolume(p *corev1.Pod, v corev1.Volume) error {
	for _, vol := range p.Spec.Volumes {
		if vol.Name == v.Name {
			return &VolumeAlreadyAttached{vol.Name}
		}
	}

	p.Spec.Volumes = append(p.Spec.Volumes, v)
	return nil
}

func mountVolume(c *corev1.Container, vm corev1.VolumeMount) error {
	for _, mnt := range c.VolumeMounts {
		if mnt.MountPath == vm.MountPath {
			return &PathAlreadyMounted{mnt.MountPath}
		}
	}

	c.VolumeMounts = append(c.VolumeMounts, vm)
	return nil
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
		if w.isReadOnlyRootFilesystem() {
			// Apply security context to container
			w.addSecurityConfigToAgent(agentSidecarContainer)

			// Don't want to apply any overrides to the agent sidecar init container
			defer func() {
				initContainer := w.getSecurityInitTemplate()
				pod.Spec.InitContainers = append(pod.Spec.InitContainers, *initContainer)
			}()
		}

		volumes := w.getVolumeTemplates()
		for _, vol := range volumes {
			err := attachVolume(pod, vol)
			if err != nil {
				var attached VolumeAlreadyAttached
				if errors.As(err, &attached) {
					log.Error(err)
				} else {
					// This should never happen
					log.Errorf("unexpected error: %v", err)
				}
			}
		}

		mounts := w.getVolumeMountTemplates()
		for _, m := range mounts {
			err := mountVolume(agentSidecarContainer, m)
			if err != nil {
				var mounted PathAlreadyMounted
				if errors.As(err, &mounted) {
					log.Error(err)
				} else {
					// This should never happen
					log.Errorf("unexpected error: %v", err)
				}
			}
		}

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
			if isOwnedByJob(pod.OwnerReferences) {
				updated, err = withEnvOverrides(&pod.Spec.Containers[i], corev1.EnvVar{
					Name:  "DD_AUTO_EXIT_NOPROCESS_ENABLED",
					Value: "true",
				})
			}
			if err != nil {
				log.Errorf("Failed to apply env overrides: %v", err)
				return podUpdated, errors.New(metrics.InternalError)
			}
			podUpdated = podUpdated || updated

			updated, err = applyProfileOverrides(&pod.Spec.Containers[i], w.profileOverrides)
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

func (w *Webhook) getSecurityInitTemplate() *corev1.Container {
	return &corev1.Container{
		Image:           fmt.Sprintf("%s/%s:%s", w.containerRegistry, w.imageName, w.imageTag),
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            "init-copy-agent-config",
		Command:         []string{"sh", "-c", "cp -R /etc/datadog-agent/* /agent-config/"},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      agentConfigVolumeName,
				MountPath: "/agent-config",
			},
		},
	}
}

func (w *Webhook) getVolumeTemplates() []corev1.Volume {
	volumes := newPseudoSet[corev1.Volume]()

	if w.isReadOnlyRootFilesystem() {
		for _, vol := range readOnlyRootFilesystemVolumes {
			volumes.Add(vol)
		}
	}

	if w.isKubeletAPILoggingEnabled {
		for _, vol := range kubernetesAPILoggingVolumes {
			volumes.Add(vol)
		}
	}

	return volumes.Slice()
}

func (w *Webhook) getVolumeMountTemplates() []corev1.VolumeMount {
	volumeMounts := newPseudoSet[corev1.VolumeMount]()

	if w.isReadOnlyRootFilesystem() {
		for _, vm := range readOnlyRootFilesystemVolumeMounts {
			volumeMounts.Add(vm)
		}
	}

	if w.isKubeletAPILoggingEnabled {
		for _, vm := range kubernetesAPILoggingVolumeMounts {
			volumeMounts.Add(vm)
		}
	}

	return volumeMounts.Slice()
}

func (w *Webhook) addSecurityConfigToAgent(agentContainer *corev1.Container) {
	if agentContainer.SecurityContext == nil {
		agentContainer.SecurityContext = &corev1.SecurityContext{}
	}
	agentContainer.SecurityContext.ReadOnlyRootFilesystem = pointer.Ptr(true)
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

	if w.isKubeletAPILoggingEnabled {
		_, _ = withEnvOverrides(agentContainer,
			corev1.EnvVar{
				Name:  "DD_LOGS_ENABLED",
				Value: "true",
			}, corev1.EnvVar{
				Name:  "DD_LOGS_CONFIG_K8S_CONTAINER_USE_KUBELET_API",
				Value: "true",
			}, corev1.EnvVar{
				Name:  "DD_LOGS_CONFIG_RUN_PATH",
				Value: "/opt/datadog-agent/run",
			})
	}

	return agentContainer
}

// labelSelectors returns the mutating webhooks object selectors based on the configuration
func labelSelectors(datadogConfig config.Component, profileOverrides []ProfileOverride) (namespaceSelector, objectSelector *metav1.LabelSelector) {
	// Read and parse selectors
	selectorsJSON := datadogConfig.GetString("admission_controller.agent_sidecar.selectors")

	// Get sidecar profiles
	if profileOverrides == nil {
		log.Error("sidecar profiles are not loaded")
		return nil, nil
	}

	var selectors []Selector

	err := json.Unmarshal([]byte(selectorsJSON), &selectors)
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

// isOwnedByJob returns true if the pod is owned by a Job
func isOwnedByJob(ownerReferences []metav1.OwnerReference) bool {
	for _, owner := range ownerReferences {
		if strings.HasPrefix(owner.APIVersion, "batch/") && owner.Kind == "Job" {
			return true
		}
	}
	return false
}
