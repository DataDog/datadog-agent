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
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	configWebhook "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/tagsfromlabels"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/appsec"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	admiv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/sets"
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

// MatchConditions returns the match conditions for the webhook. This one is generated from all the [labels.Selector]
// of each SidecarInjectionPattern, built into a CEL expression
func (w *Webhook) MatchConditions() []admissionregistrationv1.MatchCondition {
	selectors := make([]labels.Selector, 0, len(w.patterns))
	for _, pattern := range w.patterns {
		selectors = append(selectors, pattern.PodSelector())
	}

	expr, err := SelectorsToCEL(selectors, "object.metadata.labels")
	if err != nil {
		log.Errorf("failed to convert selectors to CEL: %v", err)
		return nil
	}

	return []admissionregistrationv1.MatchCondition{{
		Name:       webhookName,
		Expression: expr,
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
		if !pattern.PodSelector().Matches(labels.Set(pod.ObjectMeta.Labels)) {
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
		if !pattern.PodSelector().Matches(labels.Set(pod.ObjectMeta.Labels)) {
			continue
		}

		if err := pattern.SidecarDeleted(ctx, pod, ns); err != nil {
			return err
		}

		return nil
	}

	return nil
}

func SelectorsToCEL(selectors []labels.Selector, labelsExpr string) (string, error) {
	// labelsExpr e.g. "object.metadata.labels"
	var parts []string
	for _, s := range selectors {
		if s == nil {
			continue
		}
		e, err := SelectorToCEL(s, labelsExpr)
		if err != nil {
			return "", err
		}
		parts = append(parts, "("+e+")")
	}
	if len(parts) == 0 {
		return "false", nil
	}
	return strings.Join(parts, " || "), nil
}

func SelectorToCEL(sel labels.Selector, labelsExpr string) (string, error) {
	reqs, selectable := sel.Requirements()
	if !selectable {
		// "nothing" selector
		return "false", nil
	}
	if len(reqs) == 0 {
		// "everything" selector
		return "true", nil
	}

	var ands []string
	for _, r := range reqs {
		k := strconv.Quote(r.Key())
		switch r.Operator() {
		case selection.Exists:
			ands = append(ands, fmt.Sprintf("%s in %s", k, labelsExpr))
		case selection.DoesNotExist:
			ands = append(ands, fmt.Sprintf("!(%s in %s)", k, labelsExpr))

		case selection.Equals, selection.DoubleEquals:
			v := oneValue(r)
			ands = append(ands,
				fmt.Sprintf("(%s in %s) && %s[%s] == %s", k, labelsExpr, labelsExpr, k, strconv.Quote(v)),
			)

		case selection.NotEquals:
			v := oneValue(r)
			// NOTE: matches "absent OR different" per label selector semantics
			ands = append(ands,
				fmt.Sprintf("!(%s in %s) || %s[%s] != %s", k, labelsExpr, labelsExpr, k, strconv.Quote(v)),
			)

		case selection.In:
			ands = append(ands,
				fmt.Sprintf("(%s in %s) && %s[%s] in [%s]", k, labelsExpr, labelsExpr, k, quoteList(sets.New(r.Values().UnsortedList()...))),
			)

		case selection.NotIn:
			// NOTE: matches "absent OR not in set"
			ands = append(ands,
				fmt.Sprintf("!(%s in %s) || !(%s[%s] in [%s])", k, labelsExpr, labelsExpr, k, quoteList(sets.New(r.Values().UnsortedList()...))),
			)

		case selection.GreaterThan:
			n := oneValue(r)
			ands = append(ands,
				fmt.Sprintf("(%s in %s) && %s[%s].matches('^[0-9]+$') && int(%s[%s]) > %s",
					k, labelsExpr, labelsExpr, k, labelsExpr, k, n),
			)

		case selection.LessThan:
			n := oneValue(r)
			ands = append(ands,
				fmt.Sprintf("(%s in %s) && %s[%s].matches('^[0-9]+$') && int(%s[%s]) < %s",
					k, labelsExpr, labelsExpr, k, labelsExpr, k, n),
			)

		default:
			return "", fmt.Errorf("unsupported operator: %v", r.Operator())
		}
	}

	// Requirements inside one selector are ANDed
	return strings.Join(wrapParens(ands), " && "), nil
}

func oneValue(r labels.Requirement) string {
	vs := r.Values()
	for v := range vs {
		return v
	}
	return ""
}

func quoteList(vs sets.Set[string]) string {
	out := make([]string, 0, vs.Len())
	for _, v := range vs.UnsortedList() {
		out = append(out, strconv.Quote(v))
	}
	return strings.Join(out, ", ")
}

func wrapParens(xs []string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = "(" + strings.TrimSpace(x) + ")"
	}
	return out
}
