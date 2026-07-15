// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package datadoginstrumentation implements the validating admission webhook for
// DatadogInstrumentation custom resources.
package datadoginstrumentation

import (
	"encoding/json"
	"fmt"
	"strings"

	admiv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// Webhook validates DatadogInstrumentation custom resources via three ordered stages:
// unique targetRef, target compatibility, and product-specific validation.
type Webhook struct {
	name       string
	isEnabled  bool
	endpoint   string
	resources  []common.WebhookResourceRule
	operations []admissionregistrationv1.OperationType
	handlers   []instrumentation.Handler
	lister     cache.GenericLister
}

// NewWebhook returns a new DatadogInstrumentation validating webhook.
func NewWebhook(datadogConfig config.Component, handlers []instrumentation.Handler, informerFactory dynamicinformer.DynamicSharedInformerFactory) *Webhook {
	var lister cache.GenericLister
	isEnabled := datadogConfig.GetBool("instrumentation_crd_controller.enabled")
	if isEnabled {
		lister = informerFactory.ForResource(instrumentation.DatadogInstrumentationGVR).Lister()
	}

	return &Webhook{
		name:      "datadog_instrumentation_validation",
		isEnabled: isEnabled,
		endpoint:  "/datadog-instrumentation-validation",
		resources: []common.WebhookResourceRule{{APIGroup: "datadoghq.com", APIVersion: "v1alpha1", Resources: []string{"datadoginstrumentations"}}},
		operations: []admissionregistrationv1.OperationType{
			admissionregistrationv1.Create,
			admissionregistrationv1.Update,
		},
		handlers: handlers,
		lister:   lister,
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
func (w *Webhook) Resources() []common.WebhookResourceRule { return w.resources }

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
	return w.validate
}

func (w *Webhook) validate(request *admission.Request) *admiv1.AdmissionResponse {
	var cr datadoghq.DatadogInstrumentation
	if err := json.Unmarshal(request.Object, &cr); err != nil {
		return rejected(fmt.Sprintf("failed to decode DatadogInstrumentation: %v", err))
	}

	// Stage 1: targetRef is immutable after creation.
	if request.Operation == admissionregistrationv1.Update {
		if msg := w.checkTargetRefImmutable(request, &cr); msg != "" {
			return rejected(msg)
		}
	}

	// Stage 2: no two CRs in the same namespace may target the same workload.
	if msg := w.checkDuplicateTargetRef(request, &cr); msg != "" {
		return rejected(msg)
	}

	// Stages 3 & 4 are driven by the handlers that own each product section.
	for _, h := range w.handlers {
		if !h.HasSection(&cr) {
			continue
		}
		// Stage 3: the handler must support the target workload kind.
		if !h.SupportsTarget(cr.Spec.TargetRef) {
			return rejected(fmt.Sprintf("handler %q does not support target kind %q", h.Name(), cr.Spec.TargetRef.Kind))
		}
		// Stage 4: product-specific validation.
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

// checkTargetRefImmutable returns a non-empty rejection message when an update
// attempts to change the targetRef. The targetRef is immutable after creation.
func (w *Webhook) checkTargetRefImmutable(request *admission.Request, incoming *datadoghq.DatadogInstrumentation) string {
	if len(request.OldObject) == 0 {
		return ""
	}
	var old datadoghq.DatadogInstrumentation
	if err := json.Unmarshal(request.OldObject, &old); err != nil {
		log.Warnf("DatadogInstrumentation validation: failed to decode old object, skipping immutability check: %v", err)
		return ""
	}
	if old.Spec.TargetRef.Kind != incoming.Spec.TargetRef.Kind ||
		old.Spec.TargetRef.Name != incoming.Spec.TargetRef.Name ||
		old.Spec.TargetRef.APIVersion != incoming.Spec.TargetRef.APIVersion {
		return fmt.Sprintf(
			"spec.targetRef is immutable: cannot change from %s/%s to %s/%s",
			old.Spec.TargetRef.Kind, old.Spec.TargetRef.Name,
			incoming.Spec.TargetRef.Kind, incoming.Spec.TargetRef.Name,
		)
	}
	return ""
}

// checkDuplicateTargetRef returns a non-empty rejection message when another
// DatadogInstrumentation in the same namespace already targets the same workload.
// On update the CR under review is excluded from the check by name.
func (w *Webhook) checkDuplicateTargetRef(request *admission.Request, incoming *datadoghq.DatadogInstrumentation) string {
	if w.lister == nil {
		log.Warn("DatadogInstrumentation validation: lister not available, skipping duplicate check")
		return ""
	}

	items, err := w.lister.ByNamespace(request.Namespace).List(labels.Everything())
	if err != nil {
		log.Warnf("DatadogInstrumentation validation: failed to list CRs in namespace %q, admitting request: %v", request.Namespace, err)
		return ""
	}

	for _, obj := range items {
		existing, err := instrumentation.DatadogInstrumentationFromObject(obj)
		if err != nil {
			log.Warnf("DatadogInstrumentation validation: failed to parse existing CR %v, skipping: %v", obj, err)
			continue
		}
		// On update, skip the CR being updated.
		if incoming.Name != "" && existing.Name == incoming.Name {
			continue
		}
		if existing.Spec.TargetRef.Kind == incoming.Spec.TargetRef.Kind &&
			existing.Spec.TargetRef.Name == incoming.Spec.TargetRef.Name {
			return fmt.Sprintf(
				"DatadogInstrumentation %q in namespace %q already targets %s/%s",
				existing.Name, request.Namespace,
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
