// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package deprecatedresources is an admission controller webhook use to detect the usage of deprecated APIGroup and APIVersions.
package deprecatedresources

import (
	"encoding/json"
	"fmt"
	"strings"

	adminv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	admissioncommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/controllers/webhook/types"
	validatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/validate/common"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// WebhookName is the name of the webhook.
	webhookName = "kubernetes_depreacted_resources"
)

type webhook struct {
	name                    string
	isEnabled               bool
	endpoint                string
	resources               types.ResourceRuleConfigList
	operations              []admissionregistrationv1.OperationType
	matchConditions         []admissionregistrationv1.MatchCondition
	demultiplexer           aggregator.Demultiplexer
	supportsMatchConditions bool
	checkid                 checkid.ID
}

// NewWebhook returns a new Kubernetes DeprecatedResources webhook.
func NewWebhook(datadogConfig config.Component, demultiplexer aggregator.Demultiplexer, supportsMatchConditions bool) types.Webhook {
	return &webhook{
		name:      webhookName,
		isEnabled: true,
		endpoint:  "/kubernetes-deprecated-resources",
		resources: types.ResourceRuleConfigList{
			types.ResourceRuleConfig{
				APIGroups:   []string{"*"},
				APIVersions: []string{"*"},
				Operations:  []string{"*"},
				Resources:   []string{"*"},
			},
		},
		operations: []admissionregistrationv1.OperationType{
			admissionregistrationv1.OperationAll,
		},
		// Only supported by Kubernetes 1.28+. Otherwise, filtering is done in the emitEvent() function.
		// This is to send events only for human users and not for system users as to avoid unneeded events.
		/*matchConditions: []admissionregistrationv1.MatchCondition{
			{
				Name:       "exclude-system-users",
				Expression: "!(request.userInfo.username.startsWith('system:'))",
			},
		},*/
		demultiplexer:           demultiplexer,
		supportsMatchConditions: supportsMatchConditions,
		checkid:                 "kubernetes_deprecated_resources",
	}
}

// Name returns the name of the webhook
func (w *webhook) Name() string {
	return w.name
}

// WebhookType returns the type of the webhook
func (w *webhook) WebhookType() admissioncommon.WebhookType {
	return admissioncommon.ValidatingWebhook
}

// IsEnabled returns whether the webhook is enabled
func (w *webhook) IsEnabled() bool {
	return w.isEnabled
}

// Endpoint returns the endpoint of the webhook
func (w *webhook) Endpoint() string {
	return w.endpoint
}

// Resources returns the kubernetes resources for which the webhook should
// be invoked
func (w *webhook) Resources() types.ResourceRuleConfigList {
	return w.resources
}

// Operations returns the operations on the resources specified for which
// the webhook should be invoked
func (w *webhook) Operations() []admissionregistrationv1.OperationType {
	return w.operations
}

// LabelSelectors returns the label selectors that specify when the webhook
// should be invoked
func (w *webhook) LabelSelectors(useNamespaceSelector bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	return nil, nil
}

// MatchConditions returns the Match Conditions used for fine-grained
// request filtering
func (w *webhook) MatchConditions() []admissionregistrationv1.MatchCondition {
	return w.matchConditions
}

// WebhookFunc returns the function that generates a Datadog Event and admits the request.
func (w *webhook) WebhookFunc() admission.WebhookFunc {
	return func(request *admission.Request) *adminv1.AdmissionResponse {
		return common.ValidationResponse(validatecommon.Validate(request, w.Name(), w.process, request.DynamicClient))
	}
}

// emitEvent generates a Datadog Event, sends it to the kubernetes_admission_events sender and admits the request.
func (w *webhook) process(request *admission.Request, _ string, _ dynamic.Interface) (bool, error) {
	if !w.supportsMatchConditions {
		// Manually filter out system users if match conditions are not supported.
		// This is to send events only for human users and not for system users as to avoid unneeded events.
		if strings.HasPrefix(request.UserInfo.Username, "system:") {
			log.Debugf("Skipping system user: %s", request.UserInfo.Username)
			return true, nil
		}
	}

	isDeprecated, object, err := w.detectDeprecatedResource(request)
	if err != nil {
		return true, fmt.Errorf("failed to generate event: %w", err)
	}

	if isDeprecated {
		// Send the event to the sender.
		s, err := w.demultiplexer.GetSender(w.checkid)
		if err != nil {
			_ = log.Errorf("Error getting the default sender: %s", err)
			return true, err
		}

		// TODO: build event
		log.Debugf("Sending Kubernetes Deprecated Resource Event: %#v", object)
		s.Event(buildEvent(object))
	}

	// Validation must always validate incoming request.
	return true, nil
}

type objectInfo struct {
	TypeMeta  metav1.TypeMeta
	Name      string
	Namespace string
}

func (w *webhook) detectDeprecatedResource(request *admission.Request) (bool, *objectInfo, error) {
	// Decode object and oldObject.
	var resource unstructured.Unstructured
	if request.Operation != admissionregistrationv1.Delete {
		if err := json.Unmarshal(request.Object, &resource); err != nil {
			return false, &objectInfo{}, fmt.Errorf("failed to unmarshal object: %w", err)
		}
	}

	// Check if the APIGroup and APIVersion are deprecated.
	gvk := resource.GetObjectKind().GroupVersionKind()

	object := &objectInfo{
		TypeMeta: metav1.TypeMeta{
			Kind:       gvk.Kind,
			APIVersion: gvk.Version,
		},
		Name:      resource.GetName(),
		Namespace: resource.GetNamespace(),
	}

	return w.isDeprecated(gvk), object, nil
}

func (w *webhook) isDeprecated(_ schema.GroupVersionKind) bool {
	// TODO implement this function
	return false
}

func buildEvent(obj *objectInfo) event.Event {
	tags := []string{
		fmt.Sprintf("kube_resource_api_version:%s", obj.TypeMeta.APIVersion),
		fmt.Sprintf("kube_resource_kind:%s", obj.TypeMeta.Kind),
		fmt.Sprintf("kube_resource_name:%s", obj.TypeMeta.Kind),
		fmt.Sprintf("kube_resource_namespace:%s", obj.TypeMeta.Kind),
	}
	return event.Event{
		Title:          fmt.Sprintf("Deprecated Kubernetes Resource %s/%s", obj.Namespace, obj.Name),
		Text:           fmt.Sprintf("APIVersion: %s, Kind: %s, Resource %s/%s", obj.TypeMeta.APIVersion, obj.TypeMeta.Kind, obj.Namespace, obj.Name),
		Tags:           tags,
		Ts:             0,
		Priority:       event.PriorityNormal,
		AlertType:      event.AlertTypeInfo,
		SourceTypeName: "kubernetes admission",
		EventType:      webhookName,
	}
}
