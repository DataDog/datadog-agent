// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

// Package appsec implements the admission webhook for injecting appsec processor sidecars
package appsec

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	configWebhook "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/tagsfromlabels"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/appsec"
	admiv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
)

const webhookName = "appsec_proxies"

// Webhook injects appsec processor sidecars
type Webhook struct {
	name          string
	isEnabled     bool
	endpoint      string
	resources     map[string][]string
	operations    []admissionregistrationv1.OperationType
	patterns      []appsecconfig.SidecarInjectionPattern
	configMutator mutatecommon.Mutator
}

// NewWebhook creates a new appsec sidecar webhook
func NewWebhook(config config.Component) *Webhook {
	mutatorFilter := newMutationFilter()

	configMutators := mutatecommon.NewMutators(
		tagsfromlabels.NewMutator(tagsfromlabels.NewMutatorConfig(config), mutatorFilter),
		configWebhook.NewMutator(configWebhook.NewMutatorConfig(config), mutatorFilter),
	)

	patterns := appsec.GetSidecarPatterns()
	return &Webhook{
		name:          webhookName,
		isEnabled:     len(patterns) > 0,
		endpoint:      "/appsec-proxies",
		resources:     map[string][]string{"": {"pods"}},
		operations:    []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Delete},
		patterns:      patterns,
		configMutator: configMutators,
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

// Resources returns the kubernetes resources for which the webhook should be invoked
func (w *Webhook) Resources() map[string][]string {
	return w.resources
}

// Operations returns the operations on the resources specified for which the webhook should be invoked
func (w *Webhook) Operations() []admissionregistrationv1.OperationType {
	return w.operations
}

// LabelSelectors returns the label selectors that specify when the webhook should be invoked
func (w *Webhook) LabelSelectors(bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	return nil, nil
}

// MatchConditions returns the match conditions for the webhook. This one is generated from all the pattern.MatchCondition
// of each SidecarInjectionPattern, built into a CEL expression OR-ed
func (w *Webhook) MatchConditions() []admissionregistrationv1.MatchCondition {
	if len(w.patterns) == 0 {
		return nil
	}

	var finalExpression strings.Builder
	for i, pattern := range w.patterns {
		finalExpression.WriteRune('(')
		finalExpression.WriteString(pattern.MatchCondition().Expression)
		finalExpression.WriteRune(')')
		if i != len(w.patterns)-1 {
			finalExpression.WriteString("||")
		}
	}

	return []admissionregistrationv1.MatchCondition{{
		Name:       webhookName,
		Expression: finalExpression.String(),
	}}
}

// Timeout returns the timeout for the webhook
func (w *Webhook) Timeout() int32 {
	return 0
}

// WebhookFunc returns the webhook function. It injects the sidecar and adds the proxy configuration to the cluster if not there.
// When the pod is deleted. It checks if it is the last pod and remove the config if so.
func (w *Webhook) WebhookFunc() admission.WebhookFunc {
	return func(request *admission.Request) *admiv1.AdmissionResponse {
		switch request.Operation {
		case admissionregistrationv1.Create:
			return common.MutationResponse(mutatecommon.Mutate(
				request.Object,
				request.Namespace,
				w.Name(),
				func(pod *corev1.Pod, ns string, cl dynamic.Interface) (bool, error) {
					mutated, err := w.callPattern(pod, ns, cl, appsecconfig.SidecarInjectionPattern.MutatePod)
					if err == nil && mutated {
						// Add APM config, label and tags so the pod is treated as a first-class citizen APM service.
						return w.configMutator.MutatePod(pod, ns, cl)
					}
					return mutated, err
				},
				request.DynamicClient,
			))
		case admissionregistrationv1.Delete:
			var pod corev1.Pod
			if err := json.Unmarshal(request.OldObject, &pod); err != nil {
				return common.MutationResponse(nil, fmt.Errorf("failed to decode raw object: %v", err))
			}
			if _, err := w.callPattern(&pod, request.Namespace, request.DynamicClient, appsecconfig.SidecarInjectionPattern.PodDeleted); err != nil {
				return common.MutationResponse(nil, fmt.Errorf("failed to delete resources associated with sidecar: %v", err))
			}
			const emptyPatch = "[]"
			return common.MutationResponse([]byte(emptyPatch), nil)
		default:
			// Should never happen, needs to be kept in sync with Webhook.operations
			return nil
		}
	}
}

func (w *Webhook) callPattern(pod *corev1.Pod, ns string, dl dynamic.Interface, podCallback func(appsecconfig.SidecarInjectionPattern, *corev1.Pod, string, dynamic.Interface) (bool, error)) (bool, error) {
	for _, pattern := range w.patterns {
		if !pattern.IsNamespaceEligible(ns) {
			continue
		}

		if !pattern.ShouldMutatePod(pod) {
			continue
		}

		return podCallback(pattern, pod, ns, dl)
	}
	return false, nil
}
