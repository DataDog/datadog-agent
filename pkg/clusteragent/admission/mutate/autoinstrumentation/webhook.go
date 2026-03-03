// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	admiv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// WebhookName is the name of the auto instrumentation webhook used for SSI.
	WebhookName = "lib_injection"
)

var (
	// WebhookResources are the Kubernetes resources this webhook should be invoked for.
	WebhookResources = map[string][]string{"": {"pods"}}

	// WebhookOperations are the operations on the resources specified for which the webhook should be invoked.
	WebhookOperations = []admissionregistrationv1.OperationType{admissionregistrationv1.Create}

	// WebhookMatchConditions are the Match Conditions used for fine-grained request filtering.
	WebhookMatchConditions = []admissionregistrationv1.MatchCondition{}
)

// WebhookConfig use to store options from the config.Component for the autoinstrumentation webhook.
type WebhookConfig struct {
	// IsEnabled is the flag to enable the autoinstrumentation webhook.
	IsEnabled bool
	// Endpoint is the endpoint to use for the autoinstrumentation webhook.
	Endpoint string
}

// NewWebhookConfig retrieves the configuration for the autoinstrumentation webhook from the datadog config.
func NewWebhookConfig(datadogConfig config.Component) *WebhookConfig {
	return &WebhookConfig{
		IsEnabled: datadogConfig.GetBool("admission_controller.auto_instrumentation.enabled"),
		Endpoint:  datadogConfig.GetString("admission_controller.auto_instrumentation.endpoint"),
	}
}

// Webhook is the auto instrumentation webhook used for Single Step Instrumentation.
type Webhook struct {
	name            string
	resources       map[string][]string
	operations      []admissionregistrationv1.OperationType
	matchConditions []admissionregistrationv1.MatchCondition
	wmeta           workloadmeta.Component
	mutator         mutatecommon.Mutator
	config          *WebhookConfig
	labelSelectors  *LabelSelectors
}

// NewWebhook returns a new Webhook dependent on the injection filter.
func NewWebhook(config *WebhookConfig, wmeta workloadmeta.Component, mutator mutatecommon.Mutator, labelSelectors *LabelSelectors) (*Webhook, error) {
	log.Debug("Successfully created SSI webhook")
	return &Webhook{
		name:            WebhookName,
		resources:       WebhookResources,
		operations:      WebhookOperations,
		matchConditions: WebhookMatchConditions,
		mutator:         mutator,
		wmeta:           wmeta,
		config:          config,
		labelSelectors:  labelSelectors,
	}, nil
}

// Name returns the name of the webhook.
func (w *Webhook) Name() string {
	return w.name
}

// WebhookType returns the type of the webhook.
func (w *Webhook) WebhookType() common.WebhookType {
	return common.MutatingWebhook
}

// IsEnabled returns whether the webhook is enabled.
func (w *Webhook) IsEnabled() bool {
	return w.config.IsEnabled
}

// Endpoint returns the endpoint of the webhook.
func (w *Webhook) Endpoint() string {
	return w.config.Endpoint
}

// Resources returns the kubernetes resources for which the webhook should be invoked.
func (w *Webhook) Resources() map[string][]string {
	return w.resources
}

// Timeout returns the timeout for the webhook.
func (w *Webhook) Timeout() int32 {
	return 0
}

// Operations returns the operations on the resources specified for which the webhook should be invoked.
func (w *Webhook) Operations() []admissionregistrationv1.OperationType {
	return w.operations
}

// LabelSelectors returns the label selectors that specify when the webhook should be invoked.
func (w *Webhook) LabelSelectors(useNamespaceSelector bool) (*metav1.LabelSelector, *metav1.LabelSelector) {
	return w.labelSelectors.Get(useNamespaceSelector)
}

// MatchConditions returns the Match Conditions used for fine-grained request filtering.
func (w *Webhook) MatchConditions() []admissionregistrationv1.MatchCondition {
	return w.matchConditions
}

// WebhookFunc returns the function that will optionally mutate a pod.
func (w *Webhook) WebhookFunc() admission.WebhookFunc {
	return func(request *admission.Request) *admiv1.AdmissionResponse {
		return common.MutationResponse(mutatecommon.Mutate(request.Object, request.Namespace, w.Name(), w.MutatePod, request.DynamicClient))
	}
}

// MutatePod will optionally mutate a pod, returning true if mutation occurs and an error if there is a problem. The
// actual logic should be implemented in the mutator and this function is exposed only for testing.
func (w *Webhook) MutatePod(pod *corev1.Pod, ns string, cl dynamic.Interface) (bool, error) {
	log.Debugf("Mutating pod with SSI %q", mutatecommon.PodString(pod))
	return w.mutator.MutatePod(pod, ns, cl)
}
