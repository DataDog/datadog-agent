// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagruleswebhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/tagrules"
)

const (
	// ValidatePath is the endpoint the ValidatingWebhookConfiguration points at.
	ValidatePath = "/validate/tagrules"

	// maxRequestSize bounds the AdmissionReview body (1 MiB, generous for a
	// single TagRule CR).
	maxRequestSize = 1 << 20
)

var tagRuleGVR = schema.GroupVersionResource{
	Group:    tagrules.GroupName,
	Version:  tagrules.GroupVersion,
	Resource: tagrules.TagRuleResource,
}

// TagKeyLister lists the tag keys claimed by the TagRule CRs in the cluster.
// It is an interface so tests can inject a fake client.
type TagKeyLister interface {
	// ListTagRuleTags maps CR name -> spec.tag for every TagRule in the cluster.
	ListTagRuleTags(ctx context.Context) (map[string]string, error)
}

// dynamicTagKeyLister lists claimed tag keys through the dynamic client.
type dynamicTagKeyLister struct {
	client dynamic.Interface
}

func (l *dynamicTagKeyLister) ListTagRuleTags(ctx context.Context) (map[string]string, error) {
	list, err := l.client.Resource(tagRuleGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing TagRules: %w", err)
	}
	claimed := make(map[string]string, len(list.Items))
	for i := range list.Items {
		tag, found, err := unstructured.NestedString(list.Items[i].Object, "spec", "tag")
		if err != nil {
			return nil, fmt.Errorf("reading spec.tag of rule %s: %w", list.Items[i].GetName(), err)
		}
		if found && tag != "" {
			claimed[list.Items[i].GetName()] = tag
		}
	}
	return claimed, nil
}

// Webhook is the HTTP handler validating TagRule admission requests. It is
// fail-closed: any parse or handler error denies the request.
type Webhook struct {
	lister   TagKeyLister
	compiler *celCompiler
	logger   *slog.Logger
}

// NewWebhook builds the handler. The lister is consulted on every request.
func NewWebhook(lister TagKeyLister, logger *slog.Logger) (*Webhook, error) {
	compiler, err := newCelCompiler()
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Webhook{lister: lister, compiler: compiler, logger: logger}, nil
}

func (h *Webhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestSize))
	if err != nil {
		h.denyUnreadable(w, fmt.Sprintf("reading request body: %v", err))
		return
	}
	review := admissionv1.AdmissionReview{}
	if err := json.Unmarshal(raw, &review); err != nil {
		h.denyUnreadable(w, fmt.Sprintf("malformed AdmissionReview: %v", err))
		return
	}
	if review.Request == nil {
		h.denyUnreadable(w, "AdmissionReview has no request")
		return
	}

	response := h.handleReview(r.Context(), review.Request)
	reply := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "ValidatingAdmissionReview",
		},
		Response: response,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&reply); err != nil {
		h.logger.Error("tag-rule webhook: encoding response", "error", err)
	}
}

// denyUnreadable answers a request whose UID could not be determined: denied
// with an empty UID, so the API server rejects the write (fail-closed).
func (h *Webhook) denyUnreadable(w http.ResponseWriter, message string) {
	reply := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "ValidatingAdmissionReview",
		},
		Response: &admissionv1.AdmissionResponse{
			Allowed: false,
			Result:  &metav1.Status{Message: "denied: " + message},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	if err := json.NewEncoder(w).Encode(&reply); err != nil {
		h.logger.Error("tag-rule webhook: encoding denial", "error", err)
	}
}

func (h *Webhook) handleReview(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	deny := func(message string) *admissionv1.AdmissionResponse {
		return &admissionv1.AdmissionResponse{
			UID:     req.UID,
			Allowed: false,
			Result:  &metav1.Status{Message: message},
		}
	}

	if req.Kind.Group != tagrules.GroupName || req.Kind.Kind != tagrules.TagRuleKind {
		return deny(fmt.Sprintf("unexpected kind %s/%s: only %s/%s is validated here",
			req.Kind.Group, req.Kind.Kind, tagrules.GroupName, tagrules.TagRuleKind))
	}
	// Deletions are always allowed: the controller's finalizer owns cleanup.
	if req.Operation == admissionv1.Delete {
		return &admissionv1.AdmissionResponse{UID: req.UID, Allowed: true}
	}
	if len(req.Object.Raw) == 0 {
		return deny("request object is empty")
	}

	rule, err := tagRuleFromRaw(req.Object.Raw)
	if err != nil {
		return deny(err.Error())
	}
	var old *tagrules.TagRule
	if len(req.OldObject.Raw) > 0 {
		if old, err = tagRuleFromRaw(req.OldObject.Raw); err != nil {
			return deny(err.Error())
		}
	}

	problems := validateRule(rule, h.compiler)
	if len(problems) == 0 {
		claimed, err := h.lister.ListTagRuleTags(ctx)
		if err != nil {
			return deny(fmt.Sprintf("fail-closed, could not list existing TagRules: %v", err))
		}
		problems = validateOwnership(rule, old, claimed)
	}
	if len(problems) > 0 {
		return deny("invalid TagRule: " + strings.Join(problems, "; "))
	}
	return &admissionv1.AdmissionResponse{UID: req.UID, Allowed: true}
}

// tagRuleFromRaw decodes a raw TagRule object from an AdmissionReview request.
func tagRuleFromRaw(raw []byte) (*tagrules.TagRule, error) {
	object := map[string]any{}
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("parsing TagRule object: %w", err)
	}
	var rule tagrules.TagRule
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object, &rule); err != nil {
		return nil, fmt.Errorf("converting TagRule: %w", err)
	}
	return &rule, nil
}
