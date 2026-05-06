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
	"path/filepath"
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
	webhookResources  = map[string][]string{"": {"pods"}}
	webhookOperations = []admissionregistrationv1.OperationType{admissionregistrationv1.Create}
)

// Webhook injects the NCCL profiler plugin into GPU training pods.
type Webhook struct {
	isEnabled      bool
	injectorImage  string
	hostSocketPath string
	socketPath     string
	initResources  *corev1.ResourceRequirements
}

// NewWebhook creates a new NCCL profiler webhook from agent config.
// The injector_image config is required when enabled; if empty the webhook
// is disabled with a warning so pods are not mutated with a broken image ref.
// hostSocketPath and socketPath default to the agent's gpu.nccl config so the
// injected pod's NCCL_DD_SOCKET_PATH matches where the agent listens.
// Self-disables if socketPath does not live under hostSocketPath, since that
// mismatch silently breaks delivery (mounted dir doesn't contain the socket).
func NewWebhook(datadogConfig config.Component) *Webhook {
	enabled := datadogConfig.GetBool("admission_controller.nccl_profiler.enabled")
	image := datadogConfig.GetString("admission_controller.nccl_profiler.injector_image")
	hostSocketPath := datadogConfig.GetString("gpu.nccl.host_socket_path")
	socketPath := datadogConfig.GetString("gpu.nccl.socket_path")
	if enabled && image == "" {
		log.Errorf("NCCL profiler webhook is enabled but admission_controller.nccl_profiler.injector_image is not set; disabling webhook")
		enabled = false
	}
	if enabled && !socketPathUnderHost(socketPath, hostSocketPath) {
		log.Errorf("NCCL profiler webhook: gpu.nccl.socket_path=%q is not under gpu.nccl.host_socket_path=%q; injected pods would write to a location not mounted from the host. Disabling webhook.", socketPath, hostSocketPath)
		enabled = false
	}
	return &Webhook{
		isEnabled:      enabled,
		injectorImage:  image,
		hostSocketPath: hostSocketPath,
		socketPath:     socketPath,
		initResources:  parseInitResources(datadogConfig),
	}
}

// parseInitResources reads optional CPU/memory limits for the injected
// init container from agent config. Returns nil if neither is set so the
// admission webhook emits a Container with no Resources block (cluster
// default applies). Mirrors cwsinstrumentation's pattern.
func parseInitResources(datadogConfig config.Component) *corev1.ResourceRequirements {
	cpuStr := datadogConfig.GetString("admission_controller.nccl_profiler.init_resources.cpu")
	memStr := datadogConfig.GetString("admission_controller.nccl_profiler.init_resources.memory")
	if cpuStr == "" && memStr == "" {
		return nil
	}
	r := &corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}}
	if cpuStr != "" {
		q, err := resource.ParseQuantity(cpuStr)
		if err != nil {
			log.Warnf("NCCL profiler webhook: invalid init_resources.cpu=%q: %v", cpuStr, err)
		} else {
			r.Requests[corev1.ResourceCPU] = q
			r.Limits[corev1.ResourceCPU] = q
		}
	}
	if memStr != "" {
		q, err := resource.ParseQuantity(memStr)
		if err != nil {
			log.Warnf("NCCL profiler webhook: invalid init_resources.memory=%q: %v", memStr, err)
		} else {
			r.Requests[corev1.ResourceMemory] = q
			r.Limits[corev1.ResourceMemory] = q
		}
	}
	if len(r.Requests) == 0 && len(r.Limits) == 0 {
		return nil
	}
	return r
}

// socketPathUnderHost returns true if socketPath is the same as or a
// descendant of hostSocketPath. Both inputs are cleaned before comparison
// to handle trailing slashes and `.` segments.
func socketPathUnderHost(socketPath, hostSocketPath string) bool {
	if socketPath == "" || hostSocketPath == "" {
		return false
	}
	host := filepath.Clean(hostSocketPath)
	sock := filepath.Clean(socketPath)
	if !strings.HasSuffix(host, string(filepath.Separator)) {
		host += string(filepath.Separator)
	}
	return strings.HasPrefix(sock, host)
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
func (w *Webhook) Resources() map[string][]string { return webhookResources }

// Operations returns the operations for which the webhook is invoked.
func (w *Webhook) Operations() []admissionregistrationv1.OperationType { return webhookOperations }

// LabelSelectors returns the label selectors for the webhook.
// Object selector: pod must carry the opt-in label.
// Namespace selector: exclude kube-system and the Datadog agent namespace
// (defense in depth, in case the opt-in label is applied to a system pod).
func (w *Webhook) LabelSelectors(_ bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	return &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      common.NamespaceLabelKey,
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   mutatecommon.DefaultDisabledNamespaces(),
				},
			},
		}, &metav1.LabelSelector{
			MatchLabels: map[string]string{EnabledLabel: "true"},
		}
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
				return mutatePod(pod, w.injectorImage, w.hostSocketPath, w.socketPath, w.initResources)
			},
			request.DynamicClient,
		))
	}
}
