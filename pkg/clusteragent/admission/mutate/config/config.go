// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package config implements the webhook that injects DD_AGENT_HOST and
// DD_ENTITY_ID into a pod template as needed
package config

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
)

const (
	// Env vars
	agentHostEnvVarName      = "DD_AGENT_HOST"
	ddEntityIDEnvVarName     = "DD_ENTITY_ID"
	ddExternalDataEnvVarName = "DD_EXTERNAL_ENV"
	traceURLEnvVarName       = "DD_TRACE_AGENT_URL"
	dogstatsdURLEnvVarName   = "DD_DOGSTATSD_URL"
	podUIDEnvVarName         = "DD_INTERNAL_POD_UID"

	// External Data Prefixes
	// These prefixes are used to build the External Data Environment Variable.
	// This variable is then used for Origin Detection.
	externalDataInitPrefix          = "it-"
	externalDataContainerNamePrefix = "cn-"
	externalDataPodUIDPrefix        = "pu-"

	// Config injection modes
	hostIP  = "hostip"
	socket  = "socket"
	service = "service"
	// csi mode allows mounting datadog sockets using CSI volumes instead of hostpath volumes
	// in case CSI is disabled globally, the mutator will default to use 'socket' mode instead
	csi = "csi"

	// DatadogVolumeName is the name of the volume used to mount the sockets when the volume source is a directory
	DatadogVolumeName = "datadog"

	// TraceAgentSocketVolumeName is the name of the volume used to mount the trace agent socket
	TraceAgentSocketVolumeName = "datadog-trace-agent"

	// DogstatsdSocketVolumeName is the name of the volume used to mount the dogstatsd socket
	DogstatsdSocketVolumeName = "datadog-dogstatsd"

	webhookName = "agent_config"
)

// csiInjectionType defines the type CSI volume to be injected by the CSI driver
type csiInjectionType string

const (
	csiAPMSocket          csiInjectionType = "APMSocket"
	csiAPMSocketDirectory csiInjectionType = "APMSocketDirectory"
	csiDSDSocket          csiInjectionType = "DSDSocket"
	csiDSDSocketDirectory csiInjectionType = "DSDSocketDirectory"
)

// Webhook is the webhook that injects DD_AGENT_HOST and DD_ENTITY_ID into a pod
type Webhook struct {
	name            string
	isEnabled       bool
	endpoint        string
	resources       map[string][]string
	operations      []admissionregistrationv1.OperationType
	matchConditions []admissionregistrationv1.MatchCondition
	wmeta           workloadmeta.Component
	mutator         mutatecommon.Mutator
}

// NewWebhook returns a new Webhook
func NewWebhook(wmeta workloadmeta.Component, datadogConfig config.Component, mutator mutatecommon.Mutator) *Webhook {
	return &Webhook{
		name:            webhookName,
		isEnabled:       datadogConfig.GetBool("admission_controller.inject_config.enabled"),
		endpoint:        datadogConfig.GetString("admission_controller.inject_config.endpoint"),
		resources:       map[string][]string{"": {"pods"}},
		operations:      []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
		matchConditions: []admissionregistrationv1.MatchCondition{},
		wmeta:           wmeta,
		mutator:         mutator,
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
	return w.isEnabled
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
func (w *Webhook) LabelSelectors(useNamespaceSelector bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	return common.DefaultLabelSelectors(useNamespaceSelector)
}

// MatchConditions returns the Match Conditions used for fine-grained
// request filtering
func (w *Webhook) MatchConditions() []admissionregistrationv1.MatchCondition {
	return w.matchConditions
}

// WebhookFunc returns the function that mutates the resources
func (w *Webhook) WebhookFunc() admission.WebhookFunc {
	return func(request *admission.Request) *admiv1.AdmissionResponse {
		return common.MutationResponse(mutatecommon.Mutate(request.Object, request.Namespace, w.Name(), w.inject, request.DynamicClient))
	}
}

// inject is a helper method to call the underlying injector directly from the webook. This is useful for testing. All
// the logic must be in the injector itself and we should consider refactoring this method out.
func (w *Webhook) inject(pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error) {
	return w.mutator.MutatePod(pod, ns, dc)
}
