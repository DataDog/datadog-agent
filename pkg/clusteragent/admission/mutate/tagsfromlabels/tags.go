// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package tagsfromlabels implements the webhook that injects DD_ENV,
// DD_VERSION, DD_SERVICE env vars into a pod template if needed
package tagsfromlabels

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	admiv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var labelsToEnv = map[string]string{
	kubernetes.EnvTagLabelKey:     kubernetes.EnvTagEnvVar,
	kubernetes.ServiceTagLabelKey: kubernetes.ServiceTagEnvVar,
	kubernetes.VersionTagLabelKey: kubernetes.VersionTagEnvVar,
}

const webhookName = "standard_tags"

// Webhook is the webhook that injects DD_ENV, DD_VERSION, DD_SERVICE env vars
type Webhook struct {
	name            string
	isEnabled       bool
	endpoint        string
	resources       []string
	operations      []admissionregistrationv1.OperationType
	ownerCacheTTL   time.Duration
	wmeta           workloadmeta.Component
	injectionFilter mutatecommon.InjectionFilter
}

// NewWebhook returns a new Webhook
func NewWebhook(wmeta workloadmeta.Component, datadogConfig config.Component, injectionFilter mutatecommon.InjectionFilter) *Webhook {
	return &Webhook{
		name:            webhookName,
		isEnabled:       datadogConfig.GetBool("admission_controller.inject_tags.enabled"),
		endpoint:        datadogConfig.GetString("admission_controller.inject_tags.endpoint"),
		resources:       []string{"pods"},
		operations:      []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
		ownerCacheTTL:   ownerCacheTTL(datadogConfig),
		wmeta:           wmeta,
		injectionFilter: injectionFilter,
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

// Resources returns the kubernetes resources for which the webhook should
// be invoked
func (w *Webhook) Resources() []string {
	return w.resources
}

// Operations returns the operations on the resources specified for which
// the webhook should be invoked
func (w *Webhook) Operations() []admissionregistrationv1.OperationType {
	return w.operations
}

// LabelSelectors returns the label selectors that specify when the webhook
// should be invoked
func (w *Webhook) LabelSelectors(useNamespaceSelector bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	return common.DefaultLabelSelectors(useNamespaceSelector)
}

type owner struct {
	name            string
	namespace       string
	kind            string
	labels          map[string]string
	ownerReferences []metav1.OwnerReference
}

// ownerInfo wraps the information needed to get pod's owner object
type ownerInfo struct {
	gvr  schema.GroupVersionResource
	name string
}

// buildID returns a unique identifier for the ownerInfo object
func (o *ownerInfo) buildID(ns string) string {
	return fmt.Sprintf("%s/%s/%s", ns, o.name, o.gvr.String())
}

// WebhookFunc returns the function that mutates the resources
func (w *Webhook) WebhookFunc() admission.WebhookFunc {
	return func(request *admission.Request) *admiv1.AdmissionResponse {
		return common.MutationResponse(mutatecommon.Mutate(request.Raw, request.Namespace, w.Name(), func(pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error) {
			// Adds the DD_ENV, DD_VERSION, DD_SERVICE env vars to the pod template from pod and higher-level resource labels.
			return w.injectTags(pod, ns, dc)
		}, request.DynamicClient))
	}
}

// injectTags injects DD_ENV, DD_VERSION, DD_SERVICE
// env vars into a pod template if needed
func (w *Webhook) injectTags(pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error) {
	var injected bool

	if pod == nil {
		return false, errors.New(metrics.InvalidInput)
	}

	if !w.injectionFilter.ShouldMutatePod(pod) {
		// Ignore pod if it has the label admission.datadoghq.com/enabled=false
		// or Single step configuration is disabled
		return false, nil
	}

	var found bool
	if found, injected = injectTagsFromLabels(pod.GetLabels(), pod); found {
		// Standard labels found in the pod's labels
		// No need to lookup the pod's owner
		return injected, nil
	}

	if ns == "" {
		if pod.GetNamespace() != "" {
			ns = pod.GetNamespace()
		} else {
			return false, errors.New(metrics.InvalidInput)
		}
	}

	// Try to discover standard labels on the pod's owner
	owners := pod.GetOwnerReferences()
	if len(owners) == 0 {
		return false, nil
	}

	owner, err := w.getOwner(owners[0], ns, dc)
	if err != nil {
		log.Error(err)
		return false, errors.New(metrics.InternalError)
	}

	log.Debugf("Looking for standard labels on '%s/%s' - kind '%s' owner of pod %s", owner.namespace, owner.name, owner.kind, mutatecommon.PodString(pod))
	_, injected = injectTagsFromLabels(owner.labels, pod)

	return injected, nil
}

// injectTagsFromLabels looks for standard tags in pod labels
// and injects them as environment variables if found
func injectTagsFromLabels(labels map[string]string, pod *corev1.Pod) (bool, bool) {
	found := false
	injectedAtLeastOnce := false
	for l, envName := range labelsToEnv {
		if tagValue, labelFound := labels[l]; labelFound {
			env := corev1.EnvVar{
				Name:  envName,
				Value: tagValue,
			}
			if injected := mutatecommon.InjectEnv(pod, env); injected {
				injectedAtLeastOnce = true
			}
			found = true
		}
	}
	return found, injectedAtLeastOnce
}

// getOwnerInfo returns the required information to get the owner object
func getOwnerInfo(owner metav1.OwnerReference) (*ownerInfo, error) {
	gv, err := schema.ParseGroupVersion(owner.APIVersion)
	if err != nil {
		return nil, err
	}
	return &ownerInfo{
		gvr:  gv.WithResource(fmt.Sprintf("%ss", strings.ToLower(owner.Kind))),
		name: owner.Name,
	}, nil
}

// getOwner returns the object of the pod's owner
// If the owner is a replicaset it returns the corresponding deployment
func (w *Webhook) getOwner(owner metav1.OwnerReference, ns string, dc dynamic.Interface) (*owner, error) {
	ownerInfo, err := getOwnerInfo(owner)
	if err != nil {
		return nil, err
	}

	obj, err := w.getAndCacheOwner(ownerInfo, ns, dc)
	if err != nil {
		return nil, err
	}

	// Try to discover standard labels from the deployment object if the owner is a replicaset
	if obj.kind == "ReplicaSet" && len(obj.ownerReferences) > 0 {
		rsOwnerInfo, err := getOwnerInfo(obj.ownerReferences[0])
		if err != nil {
			return nil, err
		}

		return w.getAndCacheOwner(rsOwnerInfo, ns, dc)
	}

	return obj, nil
}

// getAndCacheOwner tries to fetch the owner object from cache before querying the api server
func (w *Webhook) getAndCacheOwner(info *ownerInfo, ns string, dc dynamic.Interface) (*owner, error) {
	infoID := info.buildID(ns)
	if cachedObj, hit := cache.Cache.Get(infoID); hit {
		metrics.GetOwnerCacheHit.Inc(info.gvr.Resource)
		owner, valid := cachedObj.(*owner)
		if !valid {
			log.Debugf("Invalid owner object for '%s', forcing a cache miss", infoID)
		} else {
			return owner, nil
		}
	}

	log.Tracef("Cache miss while getting owner '%s'", infoID)
	metrics.GetOwnerCacheMiss.Inc(info.gvr.Resource)
	ownerObj, err := dc.Resource(info.gvr).Namespace(ns).Get(context.TODO(), info.name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	owner := &owner{
		name:            ownerObj.GetName(),
		kind:            ownerObj.GetKind(),
		namespace:       ownerObj.GetNamespace(),
		labels:          ownerObj.GetLabels(),
		ownerReferences: ownerObj.GetOwnerReferences(),
	}

	cache.Cache.Set(infoID, owner, w.ownerCacheTTL)
	return owner, nil
}

func ownerCacheTTL(datadogConfig config.Component) time.Duration {
	if datadogConfig.IsSet("admission_controller.pod_owners_cache_validity") { // old option. Kept for backwards compatibility
		return datadogConfig.GetDuration("admission_controller.pod_owners_cache_validity") * time.Minute
	}

	return datadogConfig.GetDuration("admission_controller.inject_tags.pod_owners_cache_validity") * time.Minute
}
