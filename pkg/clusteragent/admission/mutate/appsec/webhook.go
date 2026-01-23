// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

// Package appsec implements the admission webhook for injecting appsec processor sidecars
package appsec

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	appseccel "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/appsec/cel"
	configWebhook "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/tagsfromlabels"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/appsec"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	name       string
	isEnabled  bool
	endpoint   string
	resources  map[string][]string
	operations []admissionregistrationv1.OperationType
	patterns   []appsecconfig.SidecarInjectionPattern

	configMutator mutatecommon.Mutator
	celEvaluator  *appseccel.PodEvaluator
}

// NewWebhook creates a new appsec sidecar webhook
func NewWebhook(config config.Component) *Webhook {
	if appsecconfig.InjectionMode(config.GetString("cluster_agent.appsec.injector.mode")) != appsecconfig.InjectionModeSidecar {
		return nil
	}

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
		celEvaluator:  appseccel.NewPodEvaluator(),
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
func (w *Webhook) LabelSelectors(_ bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	return nil, nil
}

// MatchConditions returns the match conditions for the webhook using CEL expressions
// from each SidecarInjectionPattern
func (w *Webhook) MatchConditions() []admissionregistrationv1.MatchCondition {
	if len(w.patterns) == 0 {
		return nil
	}

	// Collect all MatchCondition CEL expressions from patterns
	expressions := make([]string, 0, len(w.patterns))
	for _, pattern := range w.patterns {
		matchConditionExpr, _ := pattern.PodMatchExpressions()
		if matchConditionExpr != "" {
			expressions = append(expressions, "("+matchConditionExpr+")")
		}
	}

	if len(expressions) == 0 {
		return nil
	}

	// Combine all expressions with OR logic
	// If any pattern matches, the webhook should be invoked
	combinedExpr := strings.Join(expressions, " || ")

	return []admissionregistrationv1.MatchCondition{{
		Name:       webhookName,
		Expression: combinedExpr,
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
					mutated, err := w.injectSidecar(request.Context, pod, ns)
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
			if err := w.sidecarDeleted(request.Context, &pod, request.Namespace); err != nil {
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

func (w *Webhook) injectSidecar(ctx context.Context, pod *corev1.Pod, ns string) (bool, error) {
	for _, pattern := range w.patterns {
		// Get the workloadfilter CEL expression for runtime evaluation
		_, filterExpr := pattern.PodMatchExpressions()

		// Evaluate the CEL expression against the pod using workloadfilter
		matches, err := w.celEvaluator.Matches(filterExpr, pod)
		if err != nil {
			log.Warnf("Failed to evaluate CEL expression for pattern: %v", err)
			continue
		}

		if !matches {
			continue
		}

		modified, err := pattern.InjectSidecar(ctx, pod, ns)
		if err != nil {
			return false, err
		}
		if modified {
			return true, nil
		}
	}
	return false, nil
}

func (w *Webhook) sidecarDeleted(ctx context.Context, pod *corev1.Pod, ns string) error {
	for _, pattern := range w.patterns {
		// Get the workloadfilter CEL expression for runtime evaluation
		_, filterExpr := pattern.PodMatchExpressions()

		// Evaluate the CEL expression against the pod using workloadfilter
		matches, err := w.celEvaluator.Matches(filterExpr, pod)
		if err != nil {
			log.Warnf("Failed to evaluate CEL expression for pattern: %v", err)
			continue
		}

		if !matches {
			continue
		}

		if err := pattern.SidecarDeleted(ctx, pod, ns); err != nil {
			return err
		}

		return nil
	}

	return nil
}
