// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagruleswebhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	fakedynamic "k8s.io/client-go/dynamic/fake"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/tagrules"
)

// Test partitions for the webhook server:
// - value kind: bool | string_set
// - outcome: allowed | denied
// - denial reason: empty values | default outside values | duplicate values |
//   node with namespace selector | tag key already owned | tag immutability |
//   CEL compile error | flipHold below floor | malformed request | wrong kind
// - operation: create | update | delete (always allowed)

func int64Ptr(v int64) *int64 { return &v }

// boolRule builds a valid bool TagRule with the given tag key.
func boolRule(name, tag string) *tagrules.TagRule {
	return &tagrules.TagRule{
		TypeMeta:   typeMeta(),
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: tagrules.TagRuleSpec{
			Entity: tagrules.EntityPod,
			Tag:    tag,
			Value: tagrules.Value{
				Type: tagrules.ValueBool,
				Bool: &tagrules.BoolValue{Expression: "source.spec.holderIdentity == entity.metadata.name"},
			},
			FlipHoldSeconds: int64Ptr(15),
		},
	}
}

// stringSetRule builds a valid string_set TagRule with the given tag key.
func stringSetRule(name, tag string) *tagrules.TagRule {
	return &tagrules.TagRule{
		TypeMeta:   typeMeta(),
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: tagrules.TagRuleSpec{
			Entity: tagrules.EntityPod,
			Tag:    tag,
			Value: tagrules.Value{
				Type: tagrules.ValueStringSet,
				StringSet: &tagrules.StringSetValue{
					Values:     []string{"frontend", "backend", "infra", "unknown"},
					Expression: `('owner' in entity.metadata.labels) ? entity.metadata.labels['owner'] : 'unknown'`,
					OnError:    tagrules.OnErrorDefault,
					Default:    "unknown",
				},
			},
		},
	}
}

func typeMeta() metav1.TypeMeta {
	return metav1.TypeMeta{APIVersion: tagrules.GroupName + "/" + tagrules.GroupVersion, Kind: tagrules.TagRuleKind}
}

// existingRule builds an unstructured TagRule CR with only spec.tag set, as
// the fake client fixture for uniqueness checks.
func existingRule(name, tag string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": tagrules.GroupName + "/" + tagrules.GroupVersion,
		"kind":       tagrules.TagRuleKind,
		"metadata":   map[string]any{"name": name},
		"spec":       map[string]any{"tag": tag},
	}}
}

func newTestWebhook(t *testing.T, existing ...*unstructured.Unstructured) (*Webhook, *httptest.Server) {
	t.Helper()
	listKinds := map[schema.GroupVersionResource]string{tagRuleGVR: tagrules.TagRuleKind + "List"}
	client := fakedynamic.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, toRuntimeObjects(existing...)...)
	webhook, err := NewWebhook(&dynamicTagKeyLister{client: client}, nil)
	if err != nil {
		t.Fatalf("NewWebhook: %v", err)
	}
	server := httptest.NewServer(webhook)
	t.Cleanup(server.Close)
	return webhook, server
}

func toRuntimeObjects(objects ...*unstructured.Unstructured) []runtime.Object {
	out := make([]runtime.Object, 0, len(objects))
	for _, obj := range objects {
		out = append(out, runtime.Object(obj))
	}
	return out
}

// reviewBody builds an AdmissionReview request around the TagRule objects.
func reviewBody(t *testing.T, op admissionv1.Operation, rule *tagrules.TagRule, old *tagrules.TagRule) []byte {
	t.Helper()
	ruleJSON, err := json.Marshal(rule)
	if err != nil {
		t.Fatalf("marshaling rule: %v", err)
	}
	request := &admissionv1.AdmissionRequest{
		UID:       types.UID("test-uid"),
		Kind:      metav1.GroupVersionKind{Group: tagrules.GroupName, Version: tagrules.GroupVersion, Kind: tagrules.TagRuleKind},
		Operation: op,
		Object:    runtime.RawExtension{Raw: ruleJSON},
	}
	if old != nil {
		oldJSON, err := json.Marshal(old)
		if err != nil {
			t.Fatalf("marshaling old rule: %v", err)
		}
		request.OldObject = runtime.RawExtension{Raw: oldJSON}
	}
	review := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"},
		Request:  request,
	}
	body, err := json.Marshal(review)
	if err != nil {
		t.Fatalf("marshaling review: %v", err)
	}
	return body
}

// postReview submits an AdmissionReview and returns (allowed, message).
func postReview(t *testing.T, server *httptest.Server, body []byte) (bool, string) {
	t.Helper()
	response, err := http.Post(server.URL+ValidatePath, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("posting review: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d", response.StatusCode)
	}
	var review admissionv1.AdmissionReview
	if err := json.NewDecoder(response.Body).Decode(&review); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if review.Response == nil {
		t.Fatal("no response in review")
	}
	if review.Response.UID != types.UID("test-uid") {
		t.Errorf("response UID: got %q, want test-uid", review.Response.UID)
	}
	message := ""
	if review.Response.Result != nil {
		message = review.Response.Result.Message
	}
	return review.Response.Allowed, message
}

// TestWebhookAllowsValidRules covers: a valid bool rule and a valid string_set
// rule are admitted when no other CR claims the tag key.
func TestWebhookAllowsValidRules(t *testing.T) {
	_, server := newTestWebhook(t)
	for _, rule := range []*tagrules.TagRule{boolRule("ben-bitdiddle", "is_leader"), stringSetRule("alyssa-hacker", "owning_team")} {
		if allowed, message := postReview(t, server, reviewBody(t, admissionv1.Create, rule, nil)); !allowed {
			t.Errorf("rule %s: denied with %q, want allowed", rule.Name, message)
		}
	}
}

// TestWebhookDeniesInvalidValues covers: empty string-set values; default
// outside the declared values.
func TestWebhookDeniesInvalidValues(t *testing.T) {
	_, server := newTestWebhook(t)

	rule := stringSetRule("eva-lu-ator", "owning_team")
	rule.Spec.Value.StringSet.Values = nil
	if allowed, message := postReview(t, server, reviewBody(t, admissionv1.Create, rule, nil)); allowed {
		t.Errorf("empty values: allowed, want denied")
	} else if want := "value.stringSet.values"; !strings.Contains(message, want) {
		t.Errorf("empty values: message %q missing %q", message, want)
	}

	rule = stringSetRule("eva-lu-ator", "owning_team")
	rule.Spec.Value.StringSet.Default = "mobile" // not in values
	if allowed, message := postReview(t, server, reviewBody(t, admissionv1.Create, rule, nil)); allowed {
		t.Errorf("default outside values: allowed, want denied")
	} else if want := "member of values"; !strings.Contains(message, want) {
		t.Errorf("default outside values: message %q missing %q", message, want)
	}
}

// TestWebhookDeniesDuplicateValues covers: duplicate entries in the declared
// string-set values.
func TestWebhookDeniesDuplicateValues(t *testing.T) {
	_, server := newTestWebhook(t)
	rule := stringSetRule("louis-reasoner", "owning_team")
	rule.Spec.Value.StringSet.Values = []string{"frontend", "frontend"}
	if allowed, message := postReview(t, server, reviewBody(t, admissionv1.Create, rule, nil)); allowed {
		t.Errorf("duplicate values: allowed, want denied")
	} else if want := "duplicate entry"; !strings.Contains(message, want) {
		t.Errorf("duplicate values: message %q missing %q", message, want)
	}
}

// TestWebhookDeniesNodeWithNamespaceSelector covers: a node rule carrying a
// namespace selector.
func TestWebhookDeniesNodeWithNamespaceSelector(t *testing.T) {
	_, server := newTestWebhook(t)
	rule := boolRule("lem-tweakit", "is_schedulable")
	rule.Spec.Entity = tagrules.EntityNode
	rule.Spec.Selector = tagrules.Selector{Namespace: "my-namespace"}
	if allowed, message := postReview(t, server, reviewBody(t, admissionv1.Create, rule, nil)); allowed {
		t.Errorf("node with namespace selector: allowed, want denied")
	} else if want := "namespace selector"; !strings.Contains(message, want) {
		t.Errorf("node with namespace selector: message %q missing %q", message, want)
	}
}

// TestWebhookDeniesOwnedTagKey covers: a tag key already claimed by another CR.
func TestWebhookDeniesOwnedTagKey(t *testing.T) {
	_, server := newTestWebhook(t, existingRule("ben-bitdiddle", "is_leader"))
	rule := boolRule("eva-lu-ator", "is_leader") // same tag key, different CR
	if allowed, message := postReview(t, server, reviewBody(t, admissionv1.Create, rule, nil)); allowed {
		t.Errorf("owned tag key: allowed, want denied")
	} else if want := "already owned by rule ben-bitdiddle"; !strings.Contains(message, want) {
		t.Errorf("owned tag key: message %q missing %q", message, want)
	}
}

// TestWebhookAllowsUnchangedTagOnUpdate covers: re-submitting the same rule
// keeps the tag key (its own CR is excluded from the uniqueness check).
func TestWebhookAllowsUnchangedTagOnUpdate(t *testing.T) {
	_, server := newTestWebhook(t, existingRule("ben-bitdiddle", "is_leader"))
	rule := boolRule("ben-bitdiddle", "is_leader")
	if allowed, message := postReview(t, server, reviewBody(t, admissionv1.Update, rule, rule)); !allowed {
		t.Errorf("unchanged rule update: denied with %q, want allowed", message)
	}
}

// TestWebhookDeniesTagImmutability covers: an update changing spec.tag.
func TestWebhookDeniesTagImmutability(t *testing.T) {
	_, server := newTestWebhook(t)
	oldRule := boolRule("ben-bitdiddle", "is_leader")
	newRule := boolRule("ben-bitdiddle", "is_leading")
	if allowed, message := postReview(t, server, reviewBody(t, admissionv1.Update, newRule, oldRule)); allowed {
		t.Errorf("tag change on update: allowed, want denied")
	} else if want := "immutable"; !strings.Contains(message, want) {
		t.Errorf("tag change on update: message %q missing %q", message, want)
	}
}

// TestWebhookDeniesCELCompileError covers: a rule whose bool expression does
// not compile; the denial message carries the CEL error.
func TestWebhookDeniesCELCompileError(t *testing.T) {
	_, server := newTestWebhook(t)
	rule := boolRule("alyssa-hacker", "is_leader")
	rule.Spec.Value.Bool.Expression = "entity.metadata."
	if allowed, message := postReview(t, server, reviewBody(t, admissionv1.Create, rule, nil)); allowed {
		t.Errorf("CEL compile error: allowed, want denied")
	} else if want := "compiling expression"; !strings.Contains(message, want) {
		t.Errorf("CEL compile error: message %q missing %q", message, want)
	}
}

// TestWebhookDeniesLowFlipHold covers: flipHoldSeconds below the 15s floor.
func TestWebhookDeniesLowFlipHold(t *testing.T) {
	_, server := newTestWebhook(t)
	rule := boolRule("louis-reasoner", "is_leader")
	rule.Spec.FlipHoldSeconds = int64Ptr(5)
	if allowed, message := postReview(t, server, reviewBody(t, admissionv1.Create, rule, nil)); allowed {
		t.Errorf("flip hold below floor: allowed, want denied")
	} else if want := "flipHoldSeconds must be at least 15"; !strings.Contains(message, want) {
		t.Errorf("flip hold below floor: message %q missing %q", message, want)
	}
}

// TestWebhookAlwaysAllowsDelete covers: deletions bypass validation; the
// controller's finalizer owns cleanup.
func TestWebhookAlwaysAllowsDelete(t *testing.T) {
	_, server := newTestWebhook(t, existingRule("ben-bitdiddle", "is_leader"))
	if allowed, message := postReview(t, server, reviewBody(t, admissionv1.Delete, boolRule("ben-bitdiddle", "is_leader"), nil)); !allowed {
		t.Errorf("delete: denied with %q, want allowed", message)
	}
}

// TestWebhookFailClosed covers: malformed body and wrong kind are denied
// (fail-closed), not 5xx'd or ignored.
func TestWebhookFailClosed(t *testing.T) {
	_, server := newTestWebhook(t)

	// Malformed JSON body.
	response, err := http.Post(server.URL+ValidatePath, "application/json", bytes.NewReader([]byte("{not json")))
	if err != nil {
		t.Fatalf("posting malformed review: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Errorf("malformed body: status %d, want %d", response.StatusCode, http.StatusBadRequest)
	}

	// Wrong kind: only TagRule is validated here.
	request := &admissionv1.AdmissionRequest{
		UID:       types.UID("test-uid"),
		Kind:      metav1.GroupVersionKind{Group: "example.com", Kind: "Widget"},
		Operation: admissionv1.Create,
		Object:    runtime.RawExtension{Raw: []byte(`{"spec":{}}`)},
	}
	body, _ := json.Marshal(admissionv1.AdmissionReview{Request: request})
	if allowed, message := postReview(t, server, body); allowed {
		t.Errorf("wrong kind: allowed, want denied")
	} else if want := "unexpected kind"; !strings.Contains(message, want) {
		t.Errorf("wrong kind: message %q missing %q", message, want)
	}
}

// TestWebhookListerFailureFailClosed covers: an error listing existing rules
// denies the request instead of letting a duplicate tag key through.
func TestWebhookListerFailureFailClosed(t *testing.T) {
	webhook, err := NewWebhook(failingLister{}, nil)
	if err != nil {
		t.Fatalf("NewWebhook: %v", err)
	}
	server := httptest.NewServer(webhook)
	t.Cleanup(server.Close)

	if allowed, message := postReview(t, server, reviewBody(t, admissionv1.Create, boolRule("ben-bitdiddle", "is_leader"), nil)); allowed {
		t.Errorf("lister failure: allowed, want denied")
	} else if want := "could not list existing TagRules"; !strings.Contains(message, want) {
		t.Errorf("lister failure: message %q missing %q", message, want)
	}
}

// failingLister always fails, to exercise the fail-closed uniqueness path.
type failingLister struct{}

func (failingLister) ListTagRuleTags(context.Context) (map[string]string, error) {
	return nil, fmt.Errorf(" apiserver unreachable")
}
