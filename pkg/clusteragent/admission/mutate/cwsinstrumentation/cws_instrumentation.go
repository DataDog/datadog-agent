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
	admiv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubectl/pkg/cmd/util/podcmd"
	"k8s.io/utils/strings/slices"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/cwsinstrumentation/k8scp"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/cwsinstrumentation/k8sexec"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usersessions"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	apiserverUtils "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	apiServerCommon "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	cwsVolumeName                        = "datadog-cws-instrumentation"
	cwsMountPath                         = "/datadog-cws-instrumentation"
	cwsInstrumentationEmbeddedPath       = "/opt/datadog-agent/bin/datadog-cws-instrumentation/"
	cwsInstrumentationPodAnotationStatus = "admission.datadoghq.com/cws-instrumentation.status"
	cwsInstrumentationPodAnotationReady  = "ready"
	cwsInjectorInitContainerName         = "cws-instrumentation"
	cwsUserSessionDataMaxSize            = 1024

	// PodLabelEnabled is used to label pods that should be instrumented or skipped by the CWS mutating webhook
	PodLabelEnabled = "admission.datadoghq.com/cws-instrumentation.enabled"

	webhookForPodsName     = "cws_pod_instrumentation"
	webhookForCommandsName = "cws_exec_instrumentation"

	// Failed or ignored instrumentation reasons
	cwsNilInputReason                      = "nil_input"
	cwsNilCommandReason                    = "nil_command"
	cwsClusterAgentServiceAccountReason    = "cluster_agent_service_account"
	cwsClusterAgentKubectlCPReason         = "cluster_agent_kubectl_cp"
	cwsClusterAgentKubectlExecHealthReason = "cluster_agent_kubectl_exec_health"
	cwsExcludedResourceReason              = "excluded_resource"
	cwsDescribePodErrorReason              = "describe_pod_error"
	cwsExcludedByAnnotationReason          = "excluded_by_annotation"
	cwsExcludedByLabelReason               = "excluded_by_label"
	cwsPodNotInstrumentedReason            = "pod_not_instrumented"
	cwsReadonlyFilesystemReason            = "readonly_filesystem"
	cwsMissingArchReason                   = "missing_arch"
	cwsCompletedPodReason                  = "completed_pod"
	cwsInvalidInputContainerReason         = "invalid_input_container"
	cwsRemoteCopyFailedReason              = "remote_copy_failed"
	cwsUnknownModeReason                   = "unknown_mode"
	cwsCredentialsSerializationErrorReason = "credentials_serialization_error"
	cwsAlreadyInstrumentedReason           = "already_instrumented"
	cwsNoInstrumentationNeededReason       = "no_instrumentation_needed"
)

type mutatePodExecFunc func(*corev1.PodExecOptions, string, string, *authenticationv1.UserInfo, dynamic.Interface, kubernetes.Interface) (bool, error)

// WebhookForPods is the webhook that injects CWS pod instrumentation
type WebhookForPods struct {
	name          string
	isEnabled     bool
	endpoint      string
	resources     []string
	operations    []admissionregistrationv1.OperationType
	admissionFunc admission.WebhookFunc
}

func newWebhookForPods(admissionFunc admission.WebhookFunc) *WebhookForPods {
	return &WebhookForPods{
		name: webhookForPodsName,
		isEnabled: pkgconfigsetup.Datadog().GetBool("admission_controller.cws_instrumentation.enabled") &&
			len(pkgconfigsetup.Datadog().GetString("admission_controller.cws_instrumentation.image_name")) > 0,
		endpoint:      pkgconfigsetup.Datadog().GetString("admission_controller.cws_instrumentation.pod_endpoint"),
		resources:     []string{"pods"},
		operations:    []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
		admissionFunc: admissionFunc,
	}
}

// Name returns the name of the webhook
func (w *WebhookForPods) Name() string {
	return w.name
}

// WebhookType returns the type of the webhook
func (w *WebhookForPods) WebhookType() common.WebhookType {
	return common.MutatingWebhook
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
func (w *WebhookForPods) Operations() []admissionregistrationv1.OperationType {
	return w.operations
}

// LabelSelectors returns the label selectors that specify when the webhook
// should be invoked
func (w *WebhookForPods) LabelSelectors(useNamespaceSelector bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	return labelSelectors(useNamespaceSelector)
}

// WebhookFunc returns the function that mutates the resources
func (w *WebhookForPods) WebhookFunc() admission.WebhookFunc {
	return w.admissionFunc
}

// WebhookForCommands is the webhook that injects CWS pods/exec instrumentation
type WebhookForCommands struct {
	name          string
	isEnabled     bool
	endpoint      string
	resources     []string
	operations    []admissionregistrationv1.OperationType
	admissionFunc admission.WebhookFunc
}

func newWebhookForCommands(admissionFunc admission.WebhookFunc) *WebhookForCommands {
	return &WebhookForCommands{
		name: webhookForCommandsName,
		isEnabled: pkgconfigsetup.Datadog().GetBool("admission_controller.cws_instrumentation.enabled") &&
			len(pkgconfigsetup.Datadog().GetString("admission_controller.cws_instrumentation.image_name")) > 0,
		endpoint:      pkgconfigsetup.Datadog().GetString("admission_controller.cws_instrumentation.command_endpoint"),
		resources:     []string{"pods/exec"},
		operations:    []admissionregistrationv1.OperationType{admissionregistrationv1.Connect},
		admissionFunc: admissionFunc,
	}
}

// Name returns the name of the webhook
func (w *WebhookForCommands) Name() string {
	return w.name
}

// WebhookType returns the name of the webhook
func (w *WebhookForCommands) WebhookType() common.WebhookType {
	return common.MutatingWebhook
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
func (w *WebhookForCommands) Operations() []admissionregistrationv1.OperationType {
	return w.operations
}

// LabelSelectors returns the label selectors that specify when the webhook
// should be invoked
func (w *WebhookForCommands) LabelSelectors(_ bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	return nil, nil
}

// WebhookFunc MutateFunc returns the function that mutates the resources
func (w *WebhookForCommands) WebhookFunc() admission.WebhookFunc {
	return w.admissionFunc
}

func parseCWSInitContainerResources() (*corev1.ResourceRequirements, error) {
	var resources = &corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}}
	if cpu := pkgconfigsetup.Datadog().GetString("admission_controller.cws_instrumentation.init_resources.cpu"); len(cpu) > 0 {
		quantity, err := resource.ParseQuantity(cpu)
		if err != nil {
			return nil, err
		}
		resources.Requests[corev1.ResourceCPU] = quantity
		resources.Limits[corev1.ResourceCPU] = quantity
	}

	if mem := pkgconfigsetup.Datadog().GetString("admission_controller.cws_instrumentation.init_resources.memory"); len(mem) > 0 {
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

// InstrumentationMode defines how the CWS Instrumentation admission controller endpoint should behave
type InstrumentationMode string

const (
	// InitContainer configures the CWS Instrumentation admission controller endpoint to use an init container
	InitContainer InstrumentationMode = "init_container"
	// RemoteCopy configures the CWS Instrumentation admission controller endpoint to use the `kubectl cp` method
	RemoteCopy InstrumentationMode = "remote_copy"
)

func (im InstrumentationMode) String() string {
	return string(im)
}

// ParseInstrumentationMode returns the instrumentation mode from an input string
func ParseInstrumentationMode(input string) (InstrumentationMode, error) {
	mode := InstrumentationMode(input)
	switch mode {
	case InitContainer, RemoteCopy:
		return mode, nil
	default:
		return "", fmt.Errorf("unknown instrumentation mode: %q, input: %q", mode, input)
	}
}

// CWSInstrumentation is the main handler for the CWS instrumentation mutating webhook endpoints
type CWSInstrumentation struct {
	// filter is used to filter the pods to instrument
	filter *containers.Filter
	// image is the full image string used to configure the init container of the CWS instrumentation
	image string
	// resources is the resources applied to the CWS instrumentation init container
	resources *corev1.ResourceRequirements
	// mode defines how pods are instrumented
	mode InstrumentationMode
	// mountVolumeForRemoteCopy
	mountVolumeForRemoteCopy bool
	// directoryForRemoteCopy
	directoryForRemoteCopy string
	// clusterAgentServiceAccount holds the service account name of the cluster agent
	clusterAgentServiceAccount string

	webhookForPods     *WebhookForPods
	webhookForCommands *WebhookForCommands
	wmeta              workloadmeta.Component
}

// NewCWSInstrumentation parses the webhook config and returns a new instance of CWSInstrumentation
func NewCWSInstrumentation(wmeta workloadmeta.Component, datadogConfig config.Component) (*CWSInstrumentation, error) {
	ci := CWSInstrumentation{
		wmeta: wmeta,
	}
	var err error

	// Parse filters
	ci.filter, err = containers.NewFilter(
		containers.GlobalFilter,
		pkgconfigsetup.Datadog().GetStringSlice("admission_controller.cws_instrumentation.include"),
		pkgconfigsetup.Datadog().GetStringSlice("admission_controller.cws_instrumentation.exclude"),
	)
	if err != nil {
		return nil, fmt.Errorf("couldn't initialize filter: %w", err)
	}

	// Parse init container image
	cwsInjectorImageName := pkgconfigsetup.Datadog().GetString("admission_controller.cws_instrumentation.image_name")
	cwsInjectorImageTag := pkgconfigsetup.Datadog().GetString("admission_controller.cws_instrumentation.image_tag")

	cwsInjectorContainerRegistry := mutatecommon.ContainerRegistry(datadogConfig, "admission_controller.cws_instrumentation.container_registry")

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

	// parse mode
	ci.mode, err = ParseInstrumentationMode(pkgconfigsetup.Datadog().GetString("admission_controller.cws_instrumentation.mode"))
	if err != nil {
		return nil, fmt.Errorf("can't initiatilize CWS Instrumentation: %v", err)
	}
	ci.mountVolumeForRemoteCopy = pkgconfigsetup.Datadog().GetBool("admission_controller.cws_instrumentation.remote_copy.mount_volume")
	ci.directoryForRemoteCopy = pkgconfigsetup.Datadog().GetString("admission_controller.cws_instrumentation.remote_copy.directory")

	if ci.mode == RemoteCopy {
		// build the cluster agent service account
		serviceAccountName := pkgconfigsetup.Datadog().GetString("cluster_agent.service_account_name")
		if len(serviceAccountName) == 0 {
			return nil, fmt.Errorf("can't initialize CWS Instrumentation in %s mode without providing a service account name in config (cluster_agent.service_account_name)", RemoteCopy)
		}
		ns := apiServerCommon.GetMyNamespace()
		ci.clusterAgentServiceAccount = fmt.Sprintf("system:serviceaccount:%s:%s", ns, serviceAccountName)
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

func (ci *CWSInstrumentation) injectForCommand(request *admission.Request) *admiv1.AdmissionResponse {
	return common.MutationResponse(mutatePodExecOptions(request.Raw, request.Name, request.Namespace, ci.webhookForCommands.Name(), request.UserInfo, ci.injectCWSCommandInstrumentation, request.DynamicClient, request.APIClient))
}

func (ci *CWSInstrumentation) resolveNodeArch(nodeName string, apiClient kubernetes.Interface) (string, error) {
	var arch string
	// try with the wmeta
	entityID := util.GenerateKubeMetadataEntityID("", "nodes", "", nodeName)

	out, err := ci.wmeta.GetKubernetesMetadata(entityID)
	if err == nil && out != nil {
		arch = out.Labels["kubernetes.io/arch"]
	}

	if out == nil {
		// try by querying the api directly
		node, err := apiClient.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("couldn't describe node %s from the API server: %v", nodeName, err)
		}
		if node.GetLabels() != nil {
			arch = node.GetLabels()["kubernetes.io/arch"]
		}
	}

	if len(arch) == 0 || !slices.Contains([]string{"arm64", "amd64"}, arch) {
		return "", fmt.Errorf("couldn't resolve the architecture of node %s from the API server", nodeName)
	}
	return arch, nil
}

func containerHasReadonlyRootfs(container corev1.Container) bool {
	if container.SecurityContext != nil && container.SecurityContext.ReadOnlyRootFilesystem != nil {
		return *container.SecurityContext.ReadOnlyRootFilesystem
	}
	return false
}

func (ci *CWSInstrumentation) hasReadonlyRootfs(pod *corev1.Pod, container string) bool {
	// check in the init containers
	for _, c := range pod.Spec.InitContainers {
		if c.Name == container {
			return containerHasReadonlyRootfs(c)
		}
	}

	// check the other containers
	for _, c := range pod.Spec.Containers {
		if c.Name == container {
			return containerHasReadonlyRootfs(c)
		}
	}

	return false
}

func (ci *CWSInstrumentation) injectCWSCommandInstrumentation(exec *corev1.PodExecOptions, name string, ns string, userInfo *authenticationv1.UserInfo, _ dynamic.Interface, apiClient kubernetes.Interface) (bool, error) {
	var injected bool

	if exec == nil || userInfo == nil {
		log.Errorf("cannot inject CWS instrumentation into nil exec options or nil userInfo")
		metrics.CWSExecInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsNilInputReason)
		return false, errors.New(metrics.InvalidInput)
	}
	if len(exec.Command) == 0 {
		metrics.CWSExecInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsNilCommandReason)
		return false, nil
	}

	// ignore the copy command from this admission controller
	if ci.mode == RemoteCopy {
		if userInfo.Username == ci.clusterAgentServiceAccount {
			log.Debugf("Ignoring exec request to %s from the cluster agent", name)
			metrics.CWSExecInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsClusterAgentServiceAccountReason)
			return false, nil
		}

		// fall back in case the service account filter somehow didn't work
		if len(exec.Command) >= len(k8scp.CWSRemoteCopyCommand) && slices.Equal(exec.Command[0:len(k8scp.CWSRemoteCopyCommand)], k8scp.CWSRemoteCopyCommand) {
			log.Debugf("Ignoring kubectl cp requests to %s from the cluster agent", name)
			metrics.CWSExecInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsClusterAgentKubectlCPReason)
			return false, nil
		}
	}

	// is the namespace / container targeted by the instrumentation ?
	if ci.filter.IsExcluded(nil, exec.Container, "", ns) {
		metrics.CWSExecInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsExcludedResourceReason)
		return false, nil
	}

	// check if the pod has been instrumented
	pod, err := apiClient.CoreV1().Pods(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil || pod == nil {
		log.Errorf("couldn't describe pod %s in namespace %s from the API server: %v", name, ns, err)
		metrics.CWSExecInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsDescribePodErrorReason)
		return false, errors.New(metrics.InternalError)
	}

	// is the pod excluded explicitly ? (we can filter out with labels in the webhook selector on pods / exec creation)
	if pod.Labels != nil && pod.Labels[PodLabelEnabled] == "false" {
		metrics.CWSExecInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsExcludedByLabelReason)
		return false, nil
	}

	// is the pod targeted by the instrumentation ?
	if ci.filter.IsExcluded(pod.Annotations, "", "", "") {
		metrics.CWSExecInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsExcludedByAnnotationReason)
		return false, nil
	}

	var cwsInstrumentationRemotePath string

	switch ci.mode {
	case InitContainer:
		cwsInstrumentationRemotePath = filepath.Join(cwsMountPath, "cws-instrumentation")
		// is the pod instrumentation ready ? (i.e. has the CWS Instrumentation init container been added ?)
		if !isPodCWSInstrumentationReady(pod.Annotations) {
			// pod isn't instrumented, do not attempt to override the pod exec command
			log.Debugf("Ignoring exec request into %s, pod not instrumented yet", mutatecommon.PodString(pod))
			metrics.CWSExecInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsPodNotInstrumentedReason)
			return false, nil
		}
	case RemoteCopy:
		cwsInstrumentationRemotePath = filepath.Join(ci.directoryForRemoteCopy, "/cws-instrumentation")

		// if we're using a shared volume, we need to make sure the pod is instrumented first
		if ci.mountVolumeForRemoteCopy {
			if !isPodCWSInstrumentationReady(pod.Annotations) {
				// pod isn't instrumented, do not attempt to override the pod exec command
				log.Debugf("Ignoring exec request into %s, pod not instrumented yet", mutatecommon.PodString(pod))
				metrics.CWSExecInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsPodNotInstrumentedReason)
				return false, nil
			}
			cwsInstrumentationRemotePath = filepath.Join(cwsMountPath, cwsInstrumentationRemotePath)
		} else {
			// check if the target pod has a read only filesystem
			if readOnly := ci.hasReadonlyRootfs(pod, exec.Container); readOnly {
				// readonly rootfs containers can't be instrumented
				log.Errorf("Ignoring exec request into %s, container %s has read only rootfs. Try enabling admission_controller.cws_instrumentation.remote_copy.mount_volume", mutatecommon.PodString(pod), exec.Container)
				metrics.CWSExecInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsReadonlyFilesystemReason)
				return false, errors.New(metrics.InvalidInput)
			}
		}

		// Now that we have computed the remote path of cws-instrumentation, we can make sure the current command isn't
		// remote health command from the cluster-agent (in which case we should simply ignore this request)
		if len(exec.Command) >= 2 && slices.Equal(exec.Command[0:2], []string{cwsInstrumentationRemotePath, k8sexec.CWSHealthCommand}) {
			log.Debugf("Ignoring kubectl health check exec requests to %s from the cluster agent", name)
			metrics.CWSExecInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsClusterAgentKubectlExecHealthReason)
			return false, nil
		}

		arch, err := ci.resolveNodeArch(pod.Spec.NodeName, apiClient)
		if err != nil {
			log.Errorf("Ignoring exec request into %s: %v", mutatecommon.PodString(pod), err)
			metrics.CWSExecInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsMissingArchReason)
			return false, errors.New(metrics.InternalError)
		}
		cwsInstrumentationLocalPath := filepath.Join(cwsInstrumentationEmbeddedPath, "cws-instrumentation."+arch)

		// check if the pod is ready to be exec-ed into
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			log.Errorf("Ignoring exec request into %s: cannot exec into a container in a completed pod; current phase is %s", mutatecommon.PodString(pod), pod.Status.Phase)
			metrics.CWSExecInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsCompletedPodReason)
			return false, errors.New(metrics.InvalidInput)
		}

		// check if the input container exists, or select the default one to which the user will be redirected
		container, err := podcmd.FindOrDefaultContainerByName(pod, exec.Container, true, nil)
		if err != nil {
			log.Errorf("Ignoring exec request into %s, invalid container: %v", mutatecommon.PodString(pod), err)
			metrics.CWSExecInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsInvalidInputContainerReason)
			return false, errors.New(metrics.InvalidInput)
		}

		// copy CWS instrumentation directly to the target container
		if err := ci.injectCWSCommandInstrumentationRemoteCopy(pod, container.Name, cwsInstrumentationLocalPath, cwsInstrumentationRemotePath); err != nil {
			log.Warnf("Ignoring exec request into %s, remote copy failed: %v", mutatecommon.PodString(pod), err)
			metrics.CWSExecInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsRemoteCopyFailedReason)
			return false, errors.New(metrics.InternalError)
		}
	default:
		log.Errorf("Ignoring exec request into %s, unknown CWS Instrumentation mode %v", mutatecommon.PodString(pod), ci.mode)
		metrics.CWSExecInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsUnknownModeReason)
		return false, errors.New(metrics.InvalidInput)
	}

	// prepare the user session context
	userSessionCtx, err := usersessions.PrepareK8SUserSessionContext(userInfo, cwsUserSessionDataMaxSize)
	if err != nil {
		log.Debugf("ignoring instrumentation of %s: %v", mutatecommon.PodString(pod), err)
		metrics.CWSExecInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsCredentialsSerializationErrorReason)
		return false, errors.New(metrics.InternalError)
	}

	if len(exec.Command) > 7 {
		// make sure the command hasn't already been instrumented (note: it shouldn't happen)
		if exec.Command[0] == cwsInstrumentationRemotePath &&
			exec.Command[1] == "inject" &&
			exec.Command[2] == "--session-type" &&
			exec.Command[3] == "k8s" &&
			exec.Command[4] == "--data" &&
			exec.Command[6] == "--" {

			if exec.Command[5] == string(userSessionCtx) {
				log.Debugf("Exec request into %s is already instrumented, ignoring", mutatecommon.PodString(pod))
				metrics.CWSExecInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsAlreadyInstrumentedReason)
				return true, nil
			}
		}
	}

	// override the command with the call to cws-instrumentation
	exec.Command = append([]string{
		cwsInstrumentationRemotePath,
		"inject",
		"--session-type",
		"k8s",
		"--data",
		string(userSessionCtx),
		"--",
	}, exec.Command...)

	log.Debugf("Pod exec request to %s by %s is now instrumented for CWS", mutatecommon.PodString(pod), userInfo.Username)
	metrics.CWSExecInstrumentationAttempts.Observe(1, ci.mode.String(), "true", "")
	injected = true

	return injected, nil
}

func (ci *CWSInstrumentation) injectCWSCommandInstrumentationRemoteCopy(pod *corev1.Pod, container string, cwsInstrumentationLocalPath, cwsInstrumentationRemotePath string) error {
	apiclient, err := apiserverUtils.WaitForAPIClient(context.Background())
	if err != nil {
		return fmt.Errorf("couldn't initialize API client")
	}

	cp := k8scp.NewCopy(apiclient)
	if err = cp.CopyToPod(cwsInstrumentationLocalPath, cwsInstrumentationRemotePath, pod, container); err != nil {
		return err
	}

	// check cws-instrumentation was properly copied by running "cws-instrumentation health"
	health := k8sexec.NewHealthCommand(apiclient)
	return health.Run(cwsInstrumentationRemotePath, pod, container)
}

func (ci *CWSInstrumentation) injectForPod(request *admission.Request) *admiv1.AdmissionResponse {
	return common.MutationResponse(mutatecommon.Mutate(request.Raw, request.Namespace, ci.webhookForPods.Name(), ci.injectCWSPodInstrumentation, request.DynamicClient))
}

func (ci *CWSInstrumentation) injectCWSPodInstrumentation(pod *corev1.Pod, ns string, _ dynamic.Interface) (bool, error) {
	if pod == nil {
		log.Errorf("cannot inject CWS instrumentation into nil pod")
		metrics.CWSPodInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsNilInputReason)
		return false, errors.New(metrics.InvalidInput)
	}

	// is the pod targeted by the instrumentation ?
	if ci.filter.IsExcluded(pod.Annotations, "", "", ns) {
		metrics.CWSPodInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsExcludedResourceReason)
		return false, nil
	}

	// check if the pod has already been instrumented
	if isPodCWSInstrumentationReady(pod.Annotations) {
		metrics.CWSPodInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsAlreadyInstrumentedReason)
		// nothing to do, return
		return true, nil
	}

	var instrumented bool

	switch ci.mode {
	case InitContainer:
		ci.injectCWSPodInstrumentationInitContainer(pod)
		instrumented = true
	case RemoteCopy:
		instrumented = ci.injectCWSPodInstrumentationRemoteCopy(pod)
	default:
		log.Errorf("Ignoring Pod %s admission request: unknown CWS Instrumentation mode %v", mutatecommon.PodString(pod), ci.mode)
		metrics.CWSPodInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsUnknownModeReason)
		return false, errors.New(metrics.InvalidInput)
	}

	if instrumented {
		// add label to indicate that the pod has been instrumented
		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}
		pod.Annotations[cwsInstrumentationPodAnotationStatus] = cwsInstrumentationPodAnotationReady
		log.Debugf("Pod %s is now instrumented for CWS", mutatecommon.PodString(pod))
		metrics.CWSPodInstrumentationAttempts.Observe(1, ci.mode.String(), "true", "")
	} else {
		metrics.CWSPodInstrumentationAttempts.Observe(1, ci.mode.String(), "false", cwsNoInstrumentationNeededReason)
	}

	return true, nil
}

func (ci *CWSInstrumentation) injectCWSPodInstrumentationInitContainer(pod *corev1.Pod) {
	// create a new volume that will be used to share cws-instrumentation across the containers of this pod
	injectCWSVolume(pod)

	// bind mount the volume to all the containers of the pod
	for i := range pod.Spec.Containers {
		injectCWSVolumeMount(&pod.Spec.Containers[i])
	}

	// same for other init containers
	for i := range pod.Spec.InitContainers {
		injectCWSVolumeMount(&pod.Spec.InitContainers[i])
	}

	// add init container to copy cws-instrumentation in the cws volume
	injectCWSInitContainer(pod, ci.resources, ci.image)
}

func (ci *CWSInstrumentation) injectCWSPodInstrumentationRemoteCopy(pod *corev1.Pod) bool {
	// are we using a mounted volume for the remote copy ?
	if ci.mountVolumeForRemoteCopy {
		// create a new volume that will be used to share cws-instrumentation across the containers of this pod
		injectCWSVolume(pod)

		// bind mount the volume to all the containers of the pod
		for i := range pod.Spec.Containers {
			injectCWSVolumeMount(&pod.Spec.Containers[i])
		}

		// same for other init containers
		for i := range pod.Spec.InitContainers {
			injectCWSVolumeMount(&pod.Spec.InitContainers[i])
		}

		return true
	}
	return false
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

	mutatecommon.MarkVolumeAsSafeToEvictForAutoscaler(pod, cwsVolumeName)
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

	if pkgconfigsetup.Datadog().GetBool("admission_controller.cws_instrumentation.mutate_unlabelled") ||
		pkgconfigsetup.Datadog().GetBool("admission_controller.mutate_unlabelled") {
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
