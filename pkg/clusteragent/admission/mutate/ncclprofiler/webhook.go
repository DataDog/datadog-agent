// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

// Package ncclprofiler implements the admission webhook that injects the NCCL
// profiler plugin (libnccl-profiler-inspector.so) into GPU training pods.
// Users opt in with the label: admission.datadoghq.com/nccl-profiler.enabled=true
package ncclprofiler

import (
	"path"
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
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	webhookName     = "nccl_profiler"
	webhookEndpoint = "/inject-nccl-profiler"

	// EnabledLabel is the pod label users set to opt in to NCCL plugin injection.
	EnabledLabel = "admission.datadoghq.com/nccl-profiler.enabled"
)

var (
	webhookResources  = []common.WebhookResourceRule{{APIGroup: "", APIVersion: "v1", Resources: []string{"pods"}}}
	webhookOperations = []admissionregistrationv1.OperationType{admissionregistrationv1.Create}
)

// Webhook injects the NCCL profiler plugin into GPU training pods.
type Webhook struct {
	isEnabled        bool
	mutateUnlabelled bool
	injectorImage    string
	hostSocketDir    string
	socketFilename   string
	clientSocketDir  string
	initResources    *corev1.ResourceRequirements
}

// NewWebhook creates a new NCCL profiler webhook from agent config.
// The injector_image config is required when enabled; if empty the webhook
// is disabled with a warning so pods are not mutated with a broken image ref.
// Mirrors `mutate/config` (APM/DSD): hostSocketDir + socketFilename describe
// the host bind-mount source, clientSocketDir + socketFilename describe the
// workload's in-container view. Self-disables on pathological inputs.
func NewWebhook(datadogConfig config.Component) *Webhook {
	enabled := datadogConfig.GetBool("admission_controller.nccl_profiler.enabled")
	image := datadogConfig.GetString("admission_controller.nccl_profiler.injector_image")
	hostSocketDir := datadogConfig.GetString("gpu.nccl.host_socket_path")
	socketPath := datadogConfig.GetString("gpu.nccl.socket_path")
	clientSocketDir := datadogConfig.GetString("admission_controller.nccl_profiler.socket_dir")
	if enabled && image == "" {
		log.Errorf("NCCL profiler webhook is enabled but admission_controller.nccl_profiler.injector_image is not set; disabling webhook")
		enabled = false
	}
	if enabled && !validSocketConfig(hostSocketDir, clientSocketDir, socketPath) {
		log.Errorf("NCCL profiler webhook: invalid gpu.nccl.host_socket_path=%q, admission_controller.nccl_profiler.socket_dir=%q, or gpu.nccl.socket_path=%q; both directories must be absolute and non-root, and socket_path must be an absolute file path with a non-empty basename (no trailing separator). Disabling webhook.", hostSocketDir, clientSocketDir, socketPath)
		enabled = false
	}
	return &Webhook{
		isEnabled:        enabled,
		mutateUnlabelled: mutateUnlabelledEnabled(datadogConfig),
		injectorImage:    image,
		hostSocketDir:    hostSocketDir,
		socketFilename:   path.Base(socketPath),
		clientSocketDir:  clientSocketDir,
		initResources:    parseInitResources(datadogConfig),
	}
}

// mutateUnlabelledEnabled is true when either the per-webhook or global
// mutate_unlabelled knob is set. See labelSelectors for the policy.
func mutateUnlabelledEnabled(datadogConfig config.Component) bool {
	return datadogConfig.GetBool("admission_controller.nccl_profiler.mutate_unlabelled") ||
		datadogConfig.GetBool("admission_controller.mutate_unlabelled")
}

// parseInitResources reads optional CPU/memory limits for the injected init
// container from agent config. Returns nil if neither key is set so the
// emitted Container has no Resources block (cluster default applies).
// Mirrors cwsinstrumentation's parseCWSInitContainerResources; both can be
// extracted into a shared helper as a future cleanup.
func parseInitResources(datadogConfig config.Component) *corev1.ResourceRequirements {
	cpuStr := datadogConfig.GetString("admission_controller.nccl_profiler.init_resources.cpu")
	memStr := datadogConfig.GetString("admission_controller.nccl_profiler.init_resources.memory")
	if cpuStr == "" && memStr == "" {
		return nil
	}
	r := &corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}}
	if cpuStr != "" {
		if q, err := resource.ParseQuantity(cpuStr); err == nil {
			r.Requests[corev1.ResourceCPU] = q
			r.Limits[corev1.ResourceCPU] = q
		} else {
			log.Warnf("NCCL profiler webhook: invalid init_resources.cpu=%q: %v", cpuStr, err)
		}
	}
	if memStr != "" {
		if q, err := resource.ParseQuantity(memStr); err == nil {
			r.Requests[corev1.ResourceMemory] = q
			r.Limits[corev1.ResourceMemory] = q
		} else {
			log.Warnf("NCCL profiler webhook: invalid init_resources.memory=%q: %v", memStr, err)
		}
	}
	if len(r.Requests) == 0 && len(r.Limits) == 0 {
		return nil
	}
	return r
}

// objectSelector builds the opt-in object selector for the webhook.
// When mutateUnlabelled is true, the selector matches every pod that does
// not carry EnabledLabel="false". Otherwise it requires EnabledLabel="true"
// (strict opt-in). Mirrors cwsinstrumentation's labelSelectors.
func objectSelector(mutateUnlabelled bool) *metav1.LabelSelector {
	if mutateUnlabelled {
		return &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      EnabledLabel,
				Operator: metav1.LabelSelectorOpNotIn,
				Values:   []string{"false"},
			}},
		}
	}
	return &metav1.LabelSelector{MatchLabels: map[string]string{EnabledLabel: "true"}}
}

// validSocketConfig rejects empty/relative paths, root dirs, and
// socket_path values that don't resolve to a real file basename
// (e.g. relative names or trailing-separator directory paths).
//
// Uses path (POSIX) rather than path/filepath because injected pod paths
// are always POSIX even when the cluster-agent runs on Windows.
func validSocketConfig(hostDir, clientDir, socketPath string) bool {
	if socketPath == "" || !path.IsAbs(socketPath) {
		return false
	}
	// Trailing separator => the configured value is a directory, not a
	// file. path.Clean strips the slash, so check the raw input.
	if strings.HasSuffix(socketPath, "/") {
		return false
	}
	base := path.Base(path.Clean(socketPath))
	if base == "." || base == "/" {
		return false
	}
	for _, d := range []string{hostDir, clientDir} {
		if d == "" {
			return false
		}
		c := path.Clean(d)
		if !path.IsAbs(c) || c == "/" {
			return false
		}
	}
	return true
}

// Name returns the name of the webhook.
func (w *Webhook) Name() string { return webhookName }

// WebhookType returns the type of the webhook.
func (w *Webhook) WebhookType() common.WebhookType { return common.MutatingWebhook }

// IsEnabled returns whether the webhook is enabled.
func (w *Webhook) IsEnabled() bool { return w.isEnabled }

// Endpoint returns the HTTP endpoint of the webhook.
func (w *Webhook) Endpoint() string { return webhookEndpoint }

// Resources returns the Kubernetes resources for which the webhook is invoked.
func (w *Webhook) Resources() []common.WebhookResourceRule { return webhookResources }

// Operations returns the operations for which the webhook is invoked.
func (w *Webhook) Operations() []admissionregistrationv1.OperationType { return webhookOperations }

// LabelSelectors returns the label selectors for the webhook.
// Object selector: opt-in via EnabledLabel; semantics flip with mutate_unlabelled.
// Namespace selector: exclude kube-system and the Datadog agent namespace.
//
// useNamespaceSelector=true is a fallback for K8s 1.10-1.14 (EOL since 2019)
// when admission_controller.namespace_selector_fallback is set. In that mode
// objectSelector is ignored by the API server, and pod-level opt-in cannot be
// expressed via namespaceSelector (which matches namespace labels, not pod
// labels). We fail closed: return a namespaceSelector that matches no
// namespace so no pod is mutated. Operators on those K8s versions must
// either upgrade or explicitly disable namespace_selector_fallback.
func (w *Webhook) LabelSelectors(useNamespaceSelector bool) (namespaceSelector *metav1.LabelSelector, objectSelectorOut *metav1.LabelSelector) {
	if useNamespaceSelector {
		log.Warnf("NCCL profiler webhook: namespace_selector_fallback is enabled but pod-level opt-in cannot be enforced via namespaceSelector; the webhook will not mutate any pod. Disable admission_controller.namespace_selector_fallback or upgrade Kubernetes to 1.15+.")
		return &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      common.NamespaceLabelKey,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"__nccl_profiler_disabled_fallback__"},
			}},
		}, nil
	}
	namespaceSelector = &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{{
			Key:      common.NamespaceLabelKey,
			Operator: metav1.LabelSelectorOpNotIn,
			Values:   mutatecommon.DefaultDisabledNamespaces(),
		}},
	}
	return namespaceSelector, objectSelector(w.mutateUnlabelled)
}

// MatchConditions returns the match conditions for the webhook (none required).
func (w *Webhook) MatchConditions() []admissionregistrationv1.MatchCondition { return nil }

// Timeout returns the admission webhook timeout in seconds (0 = use cluster default).
func (w *Webhook) Timeout() int32 { return 0 }

// WebhookFunc returns the function that mutates pods on admission.
func (w *Webhook) WebhookFunc() admission.WebhookFunc {
	return func(request *admission.Request) *admiv1.AdmissionResponse {
		return common.MutationResponse(mutatecommon.Mutate(
			request.Object,
			request.Namespace,
			w.Name(),
			func(pod *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
				log.Debugf("Injecting NCCL profiler plugin into pod %s", mutatecommon.PodString(pod))
				return mutatePod(pod, w.injectorImage, w.hostSocketDir, w.clientSocketDir, w.socketFilename, w.initResources)
			},
			request.DynamicClient,
		))
	}
}
