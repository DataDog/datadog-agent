// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package deprecatedresources is an admission controller webhook use to detect the usage of deprecated APIGroup and APIVersions.
package deprecatedresources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	adminv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/kubectl/pkg/scheme"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/controllers/webhook/types"
	validatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/validate/common"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// WebhookName is the name of the webhook.
	webhookName = "kubernetes_depreacted_resources"
	eventType   = "kubernetes_deprecated_resources"
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

	context context.Context

	resourcesDeprecationInfo objectInfoMapInterface
	crdsDeprecationInfo      objectInfoMapInterface
	apiExtClient             clientset.Interface
}

// NewWebhook returns a new Kubernetes DeprecatedResources webhook.
func NewWebhook(datadogConfig config.Component, demultiplexer aggregator.Demultiplexer, apiExtClient clientset.Interface, supportsMatchConditions bool) types.Webhook {
	webh := &webhook{
		name:      webhookName,
		isEnabled: datadogConfig.GetBool("kubernetes_deprecated_resources_collection.enabled"),
		endpoint:  "/kubernetes-deprecated-resources",
		resources: types.ResourceRuleConfigList{
			types.ResourceRuleConfig{
				APIGroups:   []string{"*"},
				APIVersions: []string{"*"},
				Operations:  []string{"CREATE", "UPDATE", "PATCH"},
				Resources:   []string{"*"},
			},
		},
		operations: []admissionregistrationv1.OperationType{
			admissionregistrationv1.OperationAll,
		},
		demultiplexer:           demultiplexer,
		supportsMatchConditions: supportsMatchConditions,
		checkid:                 "kubernetes_deprecated_resources",

		apiExtClient:             apiExtClient,
		resourcesDeprecationInfo: newObjectInfoMap(constResourceDeprecationInfo),
		crdsDeprecationInfo:      newObjectInfoMap(nil),
	}

	if !datadogConfig.GetBool("kubernetes_deprecated_resources_collection.match_conditions.disabled") {
		// Only supported by Kubernetes 1.28+. Otherwise, filtering is done in the emitEvent() function.
		// This is to send events only for human users and not for system users as to avoid unneeded events.
		webh.matchConditions = []admissionregistrationv1.MatchCondition{
			{
				Name:       "exclude-system-users",
				Expression: "!(request.userInfo.username.startsWith('system:'))",
			},
		}
	}

	return webh
}

// Name returns the name of the webhook
func (w *webhook) Name() string {
	return w.name
}

// WebhookType returns the type of the webhook
func (w *webhook) WebhookType() common.WebhookType {
	return common.ValidatingWebhook
}

// IsEnabled returns whether the webhook is enabled
func (w *webhook) IsEnabled() bool {
	return w.isEnabled
}

func (w *webhook) Start(ctx context.Context) error {
	if !w.isEnabled {
		return nil
	}
	log.Debugf("Starting Kubernetes Deprecated Resources Webhook")

	// fetch the CRDs deprecation info at start time
	if err := w.refreshCRDsDeprecationInfo(w.apiExtClient.ApiextensionsV1()); err != nil {
		log.Errorf("Failed to refresh CRDs deprecation info: %s", err)
	}

	// We need to refresh the CRDs deprecation info periodically if an operator is managing the CRDs has been updated the CRDs
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := w.refreshCRDsDeprecationInfo(w.apiExtClient.ApiextensionsV1()); err != nil {
					log.Errorf("Failed to refresh CRDs deprecation info: %s", err)
				}
			}
		}
	}()
	return nil
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
func (w *webhook) LabelSelectors(_ bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
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

	deprecationInfo, object, err := w.detectDeprecatedResource(request)
	if err != nil {
		return true, fmt.Errorf("failed to generate event: %w", err)
	}

	if deprecationInfo.isDeprecated {
		// Send the event to the sender.
		s, err := w.demultiplexer.GetSender(w.checkid)
		if err != nil {
			_ = log.Errorf("Error getting the default sender: %s", err)
			return true, err
		}

		// TODO: build event
		log.Debugf("Sending Kubernetes Deprecated Resource Event: %#v", object)
		s.Event(buildEvent(object, deprecationInfo))
	}

	// Validation must always validate incoming request.
	return true, nil
}

func (w *webhook) detectDeprecatedResource(request *admission.Request) (deprecationInfoType, *objectInfoType, error) {
	// Decode object and oldObject.
	var resource unstructured.Unstructured
	if request.Operation != admissionregistrationv1.Delete {
		if err := json.Unmarshal(request.Object, &resource); err != nil {
			return deprecationInfoType{}, &objectInfoType{}, fmt.Errorf("failed to unmarshal object: %w", err)
		}
	}

	// Check if the APIGroup and APIVersion are deprecated.
	gvk := resource.GetObjectKind().GroupVersionKind()

	// create the runtime object to check if it is deprecated
	object := &objectInfoType{
		GroupVersionKind: gvk,
		Name:             resource.GetName(),
		Namespace:        resource.GetNamespace(),
	}

	deprecationInfo := w.isDeprecated(gvk)

	return deprecationInfo, object, nil
}

func (w *webhook) isDeprecated(gvk schema.GroupVersionKind) deprecationInfoType {
	// start by checking if the object is deprecated from the runtime object
	// since it is the most reliable way to know if an object is deprecated

	if deprecationInfo := w.isDeprecatedFromRuntimeObject(gvk); deprecationInfo.isDeprecated {
		return deprecationInfo
	}

	// if the object is not deprecated from the runtime object, we can check if
	// it is deprecated from the CRDs definitions
	if deprecationInfo := w.crdsDeprecationInfo.IsDeprecated(gvk); deprecationInfo.isDeprecated {
		return deprecationInfo
	}

	// lastly, we can check if the object is deprecated from the const configuration
	if deprecationInfo := w.resourcesDeprecationInfo.IsDeprecated(gvk); deprecationInfo.isDeprecated {
		return deprecationInfo
	}

	return deprecationInfoType{isDeprecated: false}
}

func (w *webhook) isDeprecatedFromRuntimeObject(gvk schema.GroupVersionKind) deprecationInfoType {
	deprecationInfo := deprecationInfoType{}

	runtimeObj, err := scheme.Scheme.New(gvk)
	if err != nil {
		// it means that his object is not registered in the scheme use by the current cluster-agent
		// however, we can still send the event (if the agent use a go-client too recent...)
		// in that case we should use another way to know if it was deprected
		log.Tracef("Unknown GroupVersionKind: %v", gvk)
		return deprecationInfo
	}

	// check if the object is deprecated
	depInfo, isDeprecated := runtimeObj.(apiLifecycleDeprecated)
	if !isDeprecated {
		return deprecationInfo
	}
	deprecationInfo.isDeprecated = true
	major, minor := depInfo.APILifecycleDeprecated()
	deprecationInfo.deprecationVersion.Major = major
	deprecationInfo.deprecationVersion.Minor = minor

	// check if the object is removed
	removeInfo, isRemoved := runtimeObj.(apiLifecycleRemoved)
	if isRemoved {
		major, minor = removeInfo.APILifecycleRemoved()
		deprecationInfo.removalVersion.Major = major
		deprecationInfo.removalVersion.Minor = minor
	}

	// check if the object has a replacement
	replacementInfo, isReplaced := runtimeObj.(apiLifecycleReplacement)
	if isReplaced {
		deprecationInfo.recommendedReplacement = replacementInfo.APILifecycleReplacement()
	}

	return deprecationInfo
}

func buildEvent(obj *objectInfoType, info deprecationInfoType) event.Event {
	tags := []string{
		fmt.Sprintf("kube_resource_version:%s", obj.GroupVersionKind.Version),
		fmt.Sprintf("kube_resource_group:%s", obj.GroupVersionKind.Group),
		fmt.Sprintf("kube_resource_kind:%s", obj.GroupVersionKind.Kind),
		fmt.Sprintf("kube_resource_name:%s", obj.Name),
		fmt.Sprintf("kube_resource_namespace:%s", obj.Namespace),
	}
	eventText := fmt.Sprintf(`
APIVersion: %s, Kind: %s, Resource %s/%s is deprecated.
* Depecrated since Kubernetes version: %d.%d.x.
* Will be removed in Kubernetes version: %d.%d.x.
* Recommended replacement: %s`,
		obj.GroupVersionKind.GroupVersion().String(), obj.GroupVersionKind.Kind, obj.Namespace, obj.Name,
		info.deprecationVersion.Major, info.deprecationVersion.Minor,
		info.removalVersion.Major, info.removalVersion.Minor,
		info.recommendedReplacement.String())

	return event.Event{
		Title:          fmt.Sprintf("Deprecated Kubernetes Resource %s/%s", obj.Namespace, obj.Name),
		Text:           eventText,
		Tags:           tags,
		Ts:             0,
		Priority:       event.PriorityNormal,
		AlertType:      event.AlertTypeWarning,
		SourceTypeName: "kubernetes deprecated resources",
		EventType:      webhookName,
	}
}

func (w *webhook) refreshCRDsDeprecationInfo(client apiextensionsv1.ApiextensionsV1Interface) error {
	log.Debugf("refreshCRDsDeprecationInfo")
	crds, err := client.CustomResourceDefinitions().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list CRDs: %w", err)
	}
	tmpMap := make(map[schema.GroupVersionKind]deprecationInfoType)
	for _, crd := range crds.Items {
		for _, crdVersion := range crd.Spec.Versions {
			if !crdVersion.Deprecated {
				// skip if the version is not deprecated
				continue
			}
			info := deprecationInfoType{
				isDeprecated: true,
				// we don't have the information about the deprecation or removal version
				// which makes sense since the CRD depends on the controller/operator that
				// manages the CRD
			}
			if crdVersion.DeprecationWarning != nil {
				info.infoMessage = *crdVersion.DeprecationWarning
			}

			log.Debugf("Add CRD DeprecationInfo: CRD %s/%s is deprecated since version %s", crd.Spec.Group, crd.Spec.Names.Kind, crdVersion.Name)
			tmpMap[schema.GroupVersionKind{
				Group:   crd.Spec.Group,
				Version: crdVersion.Name,
				Kind:    crd.Spec.Names.Kind,
			}] = info
		}
	}
	w.crdsDeprecationInfo.Replace(tmpMap)
	return nil
}
