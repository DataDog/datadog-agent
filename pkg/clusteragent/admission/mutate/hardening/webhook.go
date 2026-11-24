// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

// Package hardening implements the webhook that applies workload hardening
// trials to the pods of the Deployments the leader marked.
package hardening

import (
	"encoding/json"
	"time"

	admiv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/hardening"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

const (
	webhookName     = "hardening"
	webhookEndpoint = "/hardening"
)

// Webhook implements the MutatingWebhook interface
type Webhook struct {
	isEnabled bool
	store     *hardening.Store
	now       func() time.Time
}

// NewWebhook returns a new Webhook. It is disabled when the feature is off or
// failed to start (nil store).
func NewWebhook(datadogConfig config.Component, store *hardening.Store) *Webhook {
	return &Webhook{
		isEnabled: datadogConfig.GetBool("admission_controller.hardening.enabled"),
		store:     store,
		now:       time.Now,
	}
}

// Name returns the name of the webhook
func (w *Webhook) Name() string { return webhookName }

// WebhookType returns the type of the webhook
func (w *Webhook) WebhookType() common.WebhookType { return common.MutatingWebhook }

// IsEnabled returns whether the webhook is enabled
func (w *Webhook) IsEnabled() bool { return w.isEnabled && w.store != nil }

// Endpoint returns the endpoint of the webhook
func (w *Webhook) Endpoint() string { return webhookEndpoint }

// Resources returns the kubernetes resources for which the webhook should be invoked
func (w *Webhook) Resources() []common.WebhookResourceRule {
	return []common.WebhookResourceRule{{APIGroup: "", APIVersion: "v1", Resources: []string{"pods"}}}
}

// Operations returns the operations on the resources specified for which the webhook should be invoked
func (w *Webhook) Operations() []admissionregistrationv1.OperationType {
	return []admissionregistrationv1.OperationType{admissionregistrationv1.Create}
}

// LabelSelectors returns the label selectors that specify when the webhook
// should be invoked: only for pods of templates the leader labelled. The
// namespace selector fallback cannot express that, so it matches no namespace.
func (w *Webhook) LabelSelectors(useNamespaceSelector bool) (*metav1.LabelSelector, *metav1.LabelSelector) {
	if useNamespaceSelector {
		return &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      common.NamespaceLabelKey,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"__hardening_disabled_fallback__"},
			}},
		}, nil
	}
	return nil, &metav1.LabelSelector{MatchLabels: map[string]string{hardening.EnabledLabel: "true"}}
}

// MatchConditions returns the Match Conditions used for fine-grained request filtering
func (w *Webhook) MatchConditions() []admissionregistrationv1.MatchCondition { return nil }

// Timeout returns the timeout for the webhook (0 means the global default)
func (w *Webhook) Timeout() int32 { return 0 }

// WebhookFunc returns the function that mutates the resources
func (w *Webhook) WebhookFunc() admission.WebhookFunc {
	return func(request *admission.Request) *admiv1.AdmissionResponse {
		return common.MutationResponse(mutatecommon.Mutate(request.Object, request.Namespace, w.Name(), w.mutate, request.DynamicClient))
	}
}

// mutate applies every active request listed on the pod that targets it, and
// records the result of each listed id on the pod.
func (w *Webhook) mutate(pod *corev1.Pod, ns string, _ dynamic.Interface) (bool, error) {
	ids := hardening.ParseIDs(pod.Annotations[hardening.RequestsAnnotation])
	if len(ids) == 0 {
		return false, nil
	}
	deployment := owningDeployment(pod)
	now := w.now()
	results := make(map[string]string, len(ids))
	for _, id := range ids {
		results[id] = w.apply(pod, ns, deployment, id, now)
	}
	out, err := json.Marshal(results)
	if err != nil {
		return false, err
	}
	pod.Annotations[hardening.AppliedAnnotation] = string(out)
	return true, nil
}

func (w *Webhook) apply(pod *corev1.Pod, ns, deployment, id string, now time.Time) string {
	req, ok := w.store.Get(id)
	if !ok || !req.Active(now) {
		return "skipped: request is not active"
	}
	if req.Target.Namespace != ns || req.Target.Name != deployment {
		return "skipped: request targets another workload"
	}
	sc, reason := hardening.Render(req, &pod.Spec)
	if reason != "" {
		return "skipped: " + reason
	}
	hardening.FindContainer(&pod.Spec, req.Target.Container).SecurityContext = sc
	return "applied"
}

// owningDeployment returns the name of the Deployment that owns pod through a
// ReplicaSet, or "" if it has none. It makes no API call.
func owningDeployment(pod *corev1.Pod) string {
	ref := metav1.GetControllerOf(pod)
	if ref == nil {
		return ""
	}
	kind, name := kubernetes.ResolvePodRootOwner(ref.Kind, ref.Name, pod.Labels)
	if kind != kubernetes.DeploymentKind {
		return ""
	}
	return name
}
