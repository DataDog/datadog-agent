// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package datadoginstrumentation implements the validating admission webhook for
// DatadogInstrumentation custom resources.
package datadoginstrumentation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	admiv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

var gvr = datadoghq.GroupVersion.WithResource("datadoginstrumentations")

// Webhook validates DatadogInstrumentation custom resources via three ordered stages:
// unique targetRef, target compatibility, and product-specific validation.
type Webhook struct {
	name       string
	isEnabled  bool
	endpoint   string
	resources  map[string][]string
	operations []admissionregistrationv1.OperationType
	handlers   []instrumentation.Handler
}

// NewWebhook returns a new DatadogInstrumentation validating webhook.
func NewWebhook(datadogConfig config.Component, handlers []instrumentation.Handler) *Webhook {
	return &Webhook{
		name:      "datadog_instrumentation_validation",
		isEnabled: datadogConfig.GetBool("instrumentation_crd_controller.enabled"),
		endpoint:  "/datadog-instrumentation-validation",
		resources: map[string][]string{
			"datadoghq.com": {"datadoginstrumentations"},
		},
		operations: []admissionregistrationv1.OperationType{
			admissionregistrationv1.Create,
			admissionregistrationv1.Update,
		},
		handlers: handlers,
	}
}

// Name returns the webhook name.
func (w *Webhook) Name() string { return w.name }

// WebhookType returns the webhook type.
func (w *Webhook) WebhookType() common.WebhookType { return common.ValidatingWebhook }

// IsEnabled returns whether the webhook is enabled.
func (w *Webhook) IsEnabled() bool { return w.isEnabled }

// Endpoint returns the webhook endpoint path.
func (w *Webhook) Endpoint() string { return w.endpoint }

// Resources returns the Kubernetes resources this webhook handles.
func (w *Webhook) Resources() map[string][]string { return w.resources }

// Operations returns the admission operations this webhook handles.
func (w *Webhook) Operations() []admissionregistrationv1.OperationType { return w.operations }

// LabelSelectors returns the label selectors for this webhook (none required).
func (w *Webhook) LabelSelectors(_ bool) (*metav1.LabelSelector, *metav1.LabelSelector) {
	return nil, nil
}

// MatchConditions returns the match conditions for this webhook (none required).
func (w *Webhook) MatchConditions() []admissionregistrationv1.MatchCondition { return nil }

// Timeout returns the timeout for this webhook (0 uses the controller default).
func (w *Webhook) Timeout() int32 { return 0 }

// WebhookFunc returns the admission handler function.
func (w *Webhook) WebhookFunc() admission.WebhookFunc {
	return func(request *admission.Request) *admiv1.AdmissionResponse {
		return w.validate(request)
	}
}

func (w *Webhook) validate(request *admission.Request) *admiv1.AdmissionResponse {
	var cr datadoghq.DatadogInstrumentation
	if err := json.Unmarshal(request.Object, &cr); err != nil {
		return rejected(fmt.Sprintf("failed to decode DatadogInstrumentation: %v", err))
	}

	// Stage 1: no two CRs in the same namespace may target the same workload.
	if msg := w.checkDuplicateTargetRef(request, &cr); msg != "" {
		return rejected(msg)
	}

	// Stages 2 & 3 are driven by the handlers that own each product section.
	for _, h := range w.handlers {
		if !h.HasSection(&cr) {
			continue
		}
		// Stage 2: the handler must support the target workload kind.
		if !h.SupportsTarget(cr.Spec.TargetRef) {
			return rejected(fmt.Sprintf("handler %q does not support target kind %q", h.Name(), cr.Spec.TargetRef.Kind))
		}
		// Stage 3: product-specific validation.
		if errs := h.Validate(&cr); len(errs) > 0 {
			msgs := make([]string, 0, len(errs))
			for _, e := range errs {
				msgs = append(msgs, e.Error())
			}
			return rejected(strings.Join(msgs, "; "))
		}
	}

	return &admiv1.AdmissionResponse{Allowed: true}
}

// checkDuplicateTargetRef returns a non-empty rejection message when another
// DatadogInstrumentation in the same namespace already targets the same workload.
// On update the CR under review is excluded from the check by name.
func (w *Webhook) checkDuplicateTargetRef(request *admission.Request, incoming *datadoghq.DatadogInstrumentation) string {
	listGVR := schema.GroupVersionResource{
		Group:    gvr.Group,
		Version:  gvr.Version,
		Resource: gvr.Resource,
	}
	list, err := request.DynamicClient.Resource(listGVR).Namespace(request.Namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		// Fail-open: if we cannot list, admit the request rather than blocking all operations.
		return ""
	}

	for i := range list.Items {
		existing := &list.Items[i]
		if incoming.Name != "" && existing.GetName() == incoming.Name {
			// Skip the CR being updated.
			continue
		}
		var di datadoghq.DatadogInstrumentation
		if err := instrumentation.UnstructuredIntoDatadogInstrumentation(existing, &di); err != nil {
			continue
		}
		if di.Spec.TargetRef.Kind == incoming.Spec.TargetRef.Kind &&
			di.Spec.TargetRef.Name == incoming.Spec.TargetRef.Name {
			return fmt.Sprintf(
				"DatadogInstrumentation %q in namespace %q already targets %s/%s",
				existing.GetName(), request.Namespace,
				incoming.Spec.TargetRef.Kind, incoming.Spec.TargetRef.Name,
			)
		}
	}
	return ""
}

func rejected(message string) *admiv1.AdmissionResponse {
	return &admiv1.AdmissionResponse{
		Allowed: false,
		Result: &metav1.Status{
			Message: message,
		},
	}
}
