// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoscaling

import (
	"encoding/json"
	"fmt"

	admiv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	hpaWebhookName     = "hpa-autoscaling"
	hpaWebhookEndpoint = "/hpa-autoscaling"
)

// HPAWebhook intercepts UPDATE operations on HorizontalPodAutoscaler resources that are
// managed by a DatadogPodAutoscaler. It reverts any spec change to keep the HPA in the
// disabled state (scaleUp/scaleDown selectPolicy: Disabled) and warns the user that the
// DPA is now in control of horizontal scaling.
type HPAWebhook struct {
	name       string
	isEnabled  bool
	endpoint   string
	resources  map[string][]string
	operations []admissionregistrationv1.OperationType
}

// NewHPAWebhook returns a new HPAWebhook.
func NewHPAWebhook(datadogConfig config.Component) *HPAWebhook {
	return &HPAWebhook{
		name:      hpaWebhookName,
		isEnabled: datadogConfig.GetBool("autoscaling.workload.enabled"),
		endpoint:  hpaWebhookEndpoint,
		resources: map[string][]string{"autoscaling": {"horizontalpodautoscalers"}},
		operations: []admissionregistrationv1.OperationType{
			admissionregistrationv1.Update,
		},
	}
}

// Name returns the name of the webhook.
func (w *HPAWebhook) Name() string { return w.name }

// WebhookType returns the type of the webhook.
func (w *HPAWebhook) WebhookType() common.WebhookType { return common.MutatingWebhook }

// IsEnabled returns whether the webhook is enabled.
func (w *HPAWebhook) IsEnabled() bool { return w.isEnabled }

// Endpoint returns the endpoint path of the webhook.
func (w *HPAWebhook) Endpoint() string { return w.endpoint }

// Resources returns the Kubernetes resources this webhook applies to.
func (w *HPAWebhook) Resources() map[string][]string { return w.resources }

// Timeout returns the webhook timeout (0 = server default).
func (w *HPAWebhook) Timeout() int32 { return 0 }

// Operations returns the operations this webhook is invoked for.
func (w *HPAWebhook) Operations() []admissionregistrationv1.OperationType { return w.operations }

// LabelSelectors returns nil selectors — the webhook filters via MatchConditions instead.
func (w *HPAWebhook) LabelSelectors(_ bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	return nil, nil
}

// MatchConditions returns a CEL condition that restricts the webhook to HPA objects that
// carry the DPA-management annotation, avoiding unnecessary invocations.
func (w *HPAWebhook) MatchConditions() []admissionregistrationv1.MatchCondition {
	return []admissionregistrationv1.MatchCondition{
		{
			Name: "managed-by-dpa",
			Expression: fmt.Sprintf(
				`has(object.metadata.annotations) && "%s" in object.metadata.annotations`,
				model.HPAManagedByDPAAnnotation,
			),
		},
	}
}

// WebhookFunc returns the admission handler.
func (w *HPAWebhook) WebhookFunc() admission.WebhookFunc {
	return func(request *admission.Request) *admiv1.AdmissionResponse {
		return w.revertHPASpec(request)
	}
}

// revertHPASpec ensures that any update to an HPA managed by a DPA is reverted back
// to the old (disabled) spec. It also surfaces a warning to the user.
func (w *HPAWebhook) revertHPASpec(request *admission.Request) *admiv1.AdmissionResponse {
	// Decode incoming (proposed) HPA.
	var incomingHPA autoscalingv2.HorizontalPodAutoscaler
	if err := json.Unmarshal(request.Object, &incomingHPA); err != nil {
		log.Warnf("HPA webhook: failed to decode incoming HPA: %v", err)
		return admissionAllowed()
	}

	// Only act when the DPA-management annotation is present.
	dpaRef := incomingHPA.Annotations[model.HPAManagedByDPAAnnotation]
	if dpaRef == "" {
		return admissionAllowed()
	}

	// Decode the old (current, disabled) HPA to get the spec we want to preserve.
	var oldHPA autoscalingv2.HorizontalPodAutoscaler
	if err := json.Unmarshal(request.OldObject, &oldHPA); err != nil {
		log.Warnf("HPA webhook: failed to decode old HPA for %s/%s: %v", request.Namespace, request.Name, err)
		return admissionAllowed()
	}

	// Build a JSON merge-patch that replaces the spec with the old (disabled) spec.
	oldSpecJSON, err := json.Marshal(oldHPA.Spec)
	if err != nil {
		log.Warnf("HPA webhook: failed to serialise old HPA spec for %s/%s: %v", request.Namespace, request.Name, err)
		return admissionAllowed()
	}
	patch := fmt.Sprintf(`{"spec":%s}`, string(oldSpecJSON))

	warning := fmt.Sprintf(
		"HPA %s/%s is managed by DatadogPodAutoscaler %s and cannot be modified directly. "+
			"Your change has been reverted. If you no longer need the HPA, you can safely delete it.",
		request.Namespace, request.Name, dpaRef,
	)

	return &admiv1.AdmissionResponse{
		Allowed:   true,
		Warnings:  []string{warning},
		PatchType: patchTypePtr(admiv1.PatchTypeJSONPatch),
		Patch:     buildJSONPatch("replace", "/spec", json.RawMessage(oldSpecJSON)),
		Result: &metav1.Status{
			// Include the patch inline as a merge patch too for clarity in logs.
			Message: patch,
		},
	}
}

func admissionAllowed() *admiv1.AdmissionResponse {
	return &admiv1.AdmissionResponse{Allowed: true}
}

func patchTypePtr(pt admiv1.PatchType) *admiv1.PatchType { return &pt }

// buildJSONPatch serialises a single JSON Patch operation as a byte slice.
func buildJSONPatch(op, path string, value json.RawMessage) []byte {
	type jsonPatchOp struct {
		Op    string          `json:"op"`
		Path  string          `json:"path"`
		Value json.RawMessage `json:"value"`
	}
	patch, _ := json.Marshal([]jsonPatchOp{{Op: op, Path: path, Value: value}})
	return patch
}
