// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package cwsinstrumentation implements the webhook that injects CWS pod and
// pod exec instrumentation
package cwsinstrumentation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/wI2L/jsondiff"
	admiv1 "k8s.io/api/admissionregistration/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usersessions"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	cwsVolumeName                        = "datadog-cws-instrumentation"
	cwsMountPath                         = "/datadog-cws-instrumentation"
	cwsInstrumentationPodAnotationStatus = "admission.datadoghq.com/cws-instrumentation.status"
	cwsInstrumentationPodAnotationReady  = "ready"
	cwsInjectorInitContainerName         = "cws-instrumentation"
	cwsUserSessionDataMaxSize            = 1024

	// PodLabelEnabled is used to label pods that should be instrumented or skipped by the CWS mutating webhook
	PodLabelEnabled = "admission.datadoghq.com/cws-instrumentation.enabled"

	webhookForPodsName     = "cws_pod_instrumentation"
	webhookForCommandsName = "cws_exec_instrumentation"
)

type mutatePodExecFunc func(*corev1.PodExecOptions, string, string, *authenticationv1.UserInfo, dynamic.Interface, kubernetes.Interface) (bool, error)

// WebhookForPods is the webhook that injects CWS pod instrumentation
type WebhookForPods struct {
	name          string
	isEnabled     bool
	endpoint      string
	resources     []string
	operations    []admiv1.OperationType
	admissionFunc admission.WebhookFunc
}

func newWebhookForPods(admissionFunc admission.WebhookFunc) *WebhookForPods {
	return &WebhookForPods{
		name: webhookForPodsName,
		isEnabled: config.Datadog.GetBool("admission_controller.cws_instrumentation.enabled") &&
			len(config.Datadog.GetString("admission_controller.cws_instrumentation.image_name")) > 0,
		endpoint:      config.Datadog.GetString("admission_controller.cws_instrumentation.pod_endpoint"),
		resources:     []string{"pods"},
		operations:    []admiv1.OperationType{admiv1.Create},
		admissionFunc: admissionFunc,
	}
}

// Name returns the name of the webhook
func (w *WebhookForPods) Name() string {
	return w.name
}

// IsEnabled returns whether the webhook is enabled
func (w *WebhookForPods) IsEnabled() bool {
	return w.isEnabled
}

// Endpoint returns the endpoint of the webhook
func (w *WebhookForPods) Endpoint() string {
	return w.endpoint
}

// Resources returns the kubernetes resources for which the webhook should
// be invoked
func (w *WebhookForPods) Resources() []string {
	return w.resources
}

// Operations returns the operations on the resources specified for which
// the webhook should be invoked
func (w *WebhookForPods) Operations() []admiv1.OperationType {
	return w.operations
}

// LabelSelectors returns the label selectors that specify when the webhook
// should be invoked
func (w *WebhookForPods) LabelSelectors(useNamespaceSelector bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	return labelSelectors(useNamespaceSelector)
}

// MutateFunc returns the function that mutates the resources
func (w *WebhookForPods) MutateFunc() admission.WebhookFunc {
	return w.admissionFunc
}

// WebhookForCommands is the webhook that injects CWS pods/exec instrumentation
type WebhookForCommands struct {
	name          string
	isEnabled     bool
	endpoint      string
	resources     []string
	operations    []admiv1.OperationType
	admissionFunc admission.WebhookFunc
}

func newWebhookForCommands(admissionFunc admission.WebhookFunc) *WebhookForCommands {
	return &WebhookForCommands{
		name: webhookForCommandsName,
		isEnabled: config.Datadog.GetBool("admission_controller.cws_instrumentation.enabled") &&
			len(config.Datadog.GetString("admission_controller.cws_instrumentation.image_name")) > 0,
		endpoint:      config.Datadog.GetString("admission_controller.cws_instrumentation.command_endpoint"),
		resources:     []string{"pods/exec"},
		operations:    []admiv1.OperationType{admiv1.Connect},
		admissionFunc: admissionFunc,
	}
}

// Name returns the name of the webhook
func (w *WebhookForCommands) Name() string {
	return w.name
}

// IsEnabled returns whether the webhook is enabled
func (w *WebhookForCommands) IsEnabled() bool {
	return w.isEnabled
}

// Endpoint returns the endpoint of the webhook
func (w *WebhookForCommands) Endpoint() string {
	return w.endpoint
}

// Resources returns the kubernetes resources for which the webhook should
// be invoked
func (w *WebhookForCommands) Resources() []string {
	return w.resources
}

// Operations returns the operations on the resources specified for which
// the webhook should be invoked
func (w *WebhookForCommands) Operations() []admiv1.OperationType {
	return w.operations
}

// LabelSelectors returns the label selectors that specify when the webhook
// should be invoked
func (w *WebhookForCommands) LabelSelectors(_ bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	return nil, nil
}

// MutateFunc returns the function that mutates the resources
func (w *WebhookForCommands) MutateFunc() admission.WebhookFunc {
	return w.admissionFunc
}

func parseCWSInitContainerResources() (*corev1.ResourceRequirements, error) {
	var resources = &corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}}
	if cpu := config.Datadog.GetString("admission_controller.cws_instrumentation.init_resources.cpu"); len(cpu) > 0 {
		quantity, err := resource.ParseQuantity(cpu)
		if err != nil {
			return nil, err
		}
		resources.Requests[corev1.ResourceCPU] = quantity
		resources.Limits[corev1.ResourceCPU] = quantity
	}

	if mem := config.Datadog.GetString("admission_controller.cws_instrumentation.init_resources.memory"); len(mem) > 0 {
		quantity, err := resource.ParseQuantity(mem)
		if err != nil {
			return nil, err
		}
		resources.Requests[corev1.ResourceMemory] = quantity
		resources.Limits[corev1.ResourceMemory] = quantity
	}

	if len(resources.Limits) > 0 || len(resources.Requests) > 0 {
		return resources, nil
	}
	return nil, nil
}

// isPodCWSInstrumentationReady returns true if the pod has already been instrumented by CWS
func isPodCWSInstrumentationReady(annotations map[string]string) bool {
	return annotations[cwsInstrumentationPodAnotationStatus] == cwsInstrumentationPodAnotationReady
}

// CWSInstrumentation is the main handler for the CWS instrumentation mutating webhook endpoints
type CWSInstrumentation struct {
	// filter is used to filter the pods to instrument
	filter *containers.Filter
	// image is the full image string used to configure the init container of the CWS instrumentation
	image string
	// resources is the resources applied to the CWS instrumentation init container
	resources *corev1.ResourceRequirements

	webhookForPods     *WebhookForPods
	webhookForCommands *WebhookForCommands
}

// NewCWSInstrumentation parses the webhook config and returns a new instance of CWSInstrumentation
func NewCWSInstrumentation() (*CWSInstrumentation, error) {
	var ci CWSInstrumentation
	var err error

	// Parse filters
	ci.filter, err = containers.NewFilter(
		containers.GlobalFilter,
		config.Datadog.GetStringSlice("admission_controller.cws_instrumentation.include"),
		config.Datadog.GetStringSlice("admission_controller.cws_instrumentation.exclude"),
	)
	if err != nil {
		return nil, fmt.Errorf("couldn't initialize filter: %w", err)
	}

	// Parse init container image
	cwsInjectorImageName := config.Datadog.GetString("admission_controller.cws_instrumentation.image_name")
	cwsInjectorImageTag := config.Datadog.GetString("admission_controller.cws_instrumentation.image_tag")

	cwsInjectorContainerRegistry := common.ContainerRegistry("admission_controller.cws_instrumentation.container_registry")

	if len(cwsInjectorImageName) == 0 {
		return nil, fmt.Errorf("can't initialize CWS Instrumentation without an image_name")
	}
	if len(cwsInjectorImageTag) == 0 {
		cwsInjectorImageTag = "latest"
	}

	ci.image = fmt.Sprintf("%s:%s", cwsInjectorImageName, cwsInjectorImageTag)
	if len(cwsInjectorContainerRegistry) > 0 {
		ci.image = fmt.Sprintf("%s/%s", cwsInjectorContainerRegistry, ci.image)
	}

	// Parse init container resources
	ci.resources, err = parseCWSInitContainerResources()
	if err != nil {
		return nil, fmt.Errorf("couldn't parse CWS Instrumentation init container resources: %w", err)
	}

	ci.webhookForPods = newWebhookForPods(ci.injectForPod)
	ci.webhookForCommands = newWebhookForCommands(ci.injectForCommand)

	return &ci, nil
}

// WebhookForPods returns the webhook that injects CWS pod instrumentation
func (ci *CWSInstrumentation) WebhookForPods() *WebhookForPods {
	return ci.webhookForPods
}

// WebhookForCommands returns the webhook that injects CWS pod/exec
// instrumentation
func (ci *CWSInstrumentation) WebhookForCommands() *WebhookForCommands {
	return ci.webhookForCommands
}

func (ci *CWSInstrumentation) injectForCommand(request *admission.MutateRequest) ([]byte, error) {
	return mutatePodExecOptions(request.Raw, request.Name, request.Namespace, ci.webhookForCommands.Name(), request.UserInfo, ci.injectCWSCommandInstrumentation, request.DynamicClient, request.APIClient)
}

func (ci *CWSInstrumentation) injectCWSCommandInstrumentation(exec *corev1.PodExecOptions, name string, ns string, userInfo *authenticationv1.UserInfo, _ dynamic.Interface, apiClient kubernetes.Interface) (bool, error) {
	var injected bool

	if exec == nil || userInfo == nil {
		log.Errorf("cannot inject CWS instrumentation into nil exec options or nil userInfo")
		return false, errors.New(metrics.InvalidInput)

	}
	if len(exec.Command) == 0 {
		return false, nil
	}

	// is the namespace / container targeted by the instrumentation ?
	if ci.filter.IsExcluded(nil, exec.Container, "", ns) {
		return false, nil
	}

	// check if the pod has been instrumented
	pod, err := apiClient.CoreV1().Pods(ns).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil || pod == nil {
		log.Errorf("couldn't describe pod %s in namespace %s from the API server: %v", name, ns, err)
		return false, errors.New(metrics.InternalError)
	}

	// is the pod targeted by the instrumentation ?
	if ci.filter.IsExcluded(pod.Annotations, "", "", "") {
		return false, nil
	}

	// is the pod instrumentation ready ? (i.e. has the CWS Instrumentation pod admission controller run ?)
	if !isPodCWSInstrumentationReady(pod.Annotations) {
		// pod isn't instrumented, do not attempt to override the pod exec command
		log.Debugf("Ignoring exec request into %s, pod not instrumented yet", common.PodString(pod))
		return false, nil
	}

	// prepare the user session context
	userSessionCtx, err := usersessions.PrepareK8SUserSessionContext(userInfo, cwsUserSessionDataMaxSize)
	if err != nil {
		log.Debugf("ignoring instrumentation of %s: %v", common.PodString(pod), err)
		return false, errors.New(metrics.InternalError)
	}

	if len(exec.Command) > 7 {
		// make sure the command hasn't already been instrumented (note: it shouldn't happen)
		if exec.Command[0] == filepath.Join(cwsMountPath, "cws-instrumentation") &&
			exec.Command[1] == "inject" &&
			exec.Command[2] == "--session-type" &&
			exec.Command[3] == "k8s" &&
			exec.Command[4] == "--data" &&
			exec.Command[6] == "--" {

			if exec.Command[5] == string(userSessionCtx) {
				log.Debugf("Exec request into %s is already instrumented, ignoring", common.PodString(pod))
				return true, nil
			}
		}
	}

	// override the command with the call to cws-instrumentation
	exec.Command = append([]string{
		filepath.Join(cwsMountPath, "cws-instrumentation"),
		"inject",
		"--session-type",
		"k8s",
		"--data",
		string(userSessionCtx),
		"--",
	}, exec.Command...)

	log.Debugf("Pod exec request to %s is now instrumented for CWS", common.PodString(pod))
	injected = true

	return injected, nil
}

func (ci *CWSInstrumentation) injectForPod(request *admission.MutateRequest) ([]byte, error) {
	return common.Mutate(request.Raw, request.Namespace, ci.webhookForPods.Name(), ci.injectCWSPodInstrumentation, request.DynamicClient)
}

func (ci *CWSInstrumentation) injectCWSPodInstrumentation(pod *corev1.Pod, ns string, _ dynamic.Interface) (bool, error) {
	var injected bool

	if pod == nil {
		return injected, errors.New(metrics.InvalidInput)
	}

	// is the pod targeted by the instrumentation ?
	if ci.filter.IsExcluded(pod.Annotations, "", "", ns) {
		return injected, nil
	}

	// check if the pod has already been instrumented
	if isPodCWSInstrumentationReady(pod.Annotations) {
		injected = true
		// nothing to do, return
		return injected, nil
	}

	// create a new volume that will be used to share cws-instrumentation across the containers of this pod
	injectCWSVolume(pod)

	// bind mount the volume to all the containers of the pod
	for i := range pod.Spec.Containers {
		injectCWSVolumeMount(&pod.Spec.Containers[i])
	}

	// add init container to copy cws-instrumentation in the cws volume
	injectCWSInitContainer(pod, ci.resources, ci.image)

	// add label to indicate that the pod has been instrumented
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[cwsInstrumentationPodAnotationStatus] = cwsInstrumentationPodAnotationReady
	injected = true
	log.Debugf("Pod %s is now instrumented for CWS", common.PodString(pod))
	return injected, nil
}

func injectCWSVolume(pod *corev1.Pod) {
	volumeSource := corev1.VolumeSource{
		EmptyDir: &corev1.EmptyDirVolumeSource{},
	}

	// make sure that the cws volume doesn't already exists
	for i, vol := range pod.Spec.Volumes {
		if vol.Name == cwsVolumeName {
			// The volume exists but does it have the expected configuration ? Override just to be sure
			pod.Spec.Volumes[i].VolumeSource = volumeSource
			return
		}
	}

	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
		Name:         cwsVolumeName,
		VolumeSource: volumeSource,
	})
}

func injectCWSVolumeMount(container *corev1.Container) {
	// make sure that the volume mount doesn't already exist
	for i, mnt := range container.VolumeMounts {
		if mnt.Name == cwsVolumeName {
			// The volume mount exists but does it have the expected configuration ? Override just to be sure
			container.VolumeMounts[i].MountPath = cwsMountPath
			return
		}
	}

	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      cwsVolumeName,
		MountPath: cwsMountPath,
	})
}

func injectCWSInitContainer(pod *corev1.Pod, resources *corev1.ResourceRequirements, image string) {
	// check if the init container has already been added
	for _, c := range pod.Spec.InitContainers {
		if c.Name == cwsInjectorInitContainerName {
			// return now, the init container has already been added
			return
		}
	}

	initContainer := corev1.Container{
		Name:    cwsInjectorInitContainerName,
		Image:   image,
		Command: []string{"/cws-instrumentation", "setup", "--cws-volume-mount", cwsMountPath},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      cwsVolumeName,
				MountPath: cwsMountPath,
			},
		},
	}
	if resources != nil {
		initContainer.Resources = *resources
	}
	pod.Spec.InitContainers = append([]corev1.Container{initContainer}, pod.Spec.InitContainers...)
}

// labelSelectors returns the mutating webhook object selector based on the configuration
func labelSelectors(useNamespaceSelector bool) (namespaceSelector, objectSelector *metav1.LabelSelector) {
	var labelSelector metav1.LabelSelector

	if config.Datadog.GetBool("admission_controller.cws_instrumentation.mutate_unlabelled") ||
		config.Datadog.GetBool("admission_controller.mutate_unlabelled") {
		// Accept all, ignore pods if they're explicitly filtered-out
		labelSelector = metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      PodLabelEnabled,
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"false"},
				},
			},
		}
	} else {
		// Ignore all, accept pods if they're explicitly allowed
		labelSelector = metav1.LabelSelector{
			MatchLabels: map[string]string{
				PodLabelEnabled: "true",
			},
		}
	}

	if useNamespaceSelector {
		return &labelSelector, nil
	}

	return nil, &labelSelector
}

// mutatePodExecOptions handles mutating PodExecOptions and encoding and decoding admission
// requests and responses for the public mutate functions
func mutatePodExecOptions(rawPodExecOptions []byte, name string, ns string, mutationType string, userInfo *authenticationv1.UserInfo, m mutatePodExecFunc, dc dynamic.Interface, apiClient kubernetes.Interface) ([]byte, error) {
	var exec corev1.PodExecOptions
	if err := json.Unmarshal(rawPodExecOptions, &exec); err != nil {
		return nil, fmt.Errorf("failed to decode raw object: %v", err)
	}

	if _, err := m(&exec, name, ns, userInfo, dc, apiClient); err != nil {
		return nil, err
	}

	if injected, err := m(&exec, name, ns, userInfo, dc, apiClient); err != nil {
		metrics.MutationAttempts.Inc(mutationType, metrics.StatusError, strconv.FormatBool(injected), err.Error())
	} else {
		metrics.MutationAttempts.Inc(mutationType, metrics.StatusSuccess, strconv.FormatBool(injected), "")
	}

	bytes, err := json.Marshal(exec)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the mutated Pod object: %v", err)
	}

	patch, err := jsondiff.CompareJSON(rawPodExecOptions, bytes) // TODO: Try to generate the patch at the MutationFunc
	if err != nil {
		return nil, fmt.Errorf("failed to prepare the JSON patch: %v", err)
	}

	return json.Marshal(patch)
}
