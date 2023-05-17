// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package mutate

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var ownerCacheTTL = config.Datadog.GetDuration("admission_controller.pod_owners_cache_validity") * time.Minute

var labelsToEnv = map[string]string{
	kubernetes.EnvTagLabelKey:     kubernetes.EnvTagEnvVar,
	kubernetes.ServiceTagLabelKey: kubernetes.ServiceTagEnvVar,
	kubernetes.VersionTagLabelKey: kubernetes.VersionTagEnvVar,
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

// InjectTags adds the DD_ENV, DD_VERSION, DD_SERVICE env vars to
// the pod template from pod and higher-level resource labels
func InjectTags(rawPod []byte, ns string, dc dynamic.Interface) ([]byte, error) {
	return mutate(rawPod, ns, injectTags, dc)
}

// injectTags injects DD_ENV, DD_VERSION, DD_SERVICE
// env vars into a pod template if needed
func injectTags(pod *corev1.Pod, ns string, dc dynamic.Interface) error {
	var injected bool
	defer func() {
		metrics.MutationAttempts.Inc(metrics.TagsMutationType, strconv.FormatBool(injected))
	}()

	if pod == nil {
		metrics.MutationErrors.Inc(metrics.TagsMutationType, "nil pod")
		return errors.New("cannot inject tags into nil pod")
	}

	if !shouldInjectTags(pod) {
		// Ignore pod if it has the label admission.datadoghq.com/enabled=false
		return nil
	}

	var found bool
	if found, injected = injectTagsFromLabels(pod.GetLabels(), pod); found {
		// Standard labels found in the pod's labels
		// No need to lookup the pod's owner
		return nil
	}

	if ns == "" {
		if pod.GetNamespace() != "" {
			ns = pod.GetNamespace()
		} else {
			metrics.MutationErrors.Inc(metrics.TagsMutationType, "empty namespace")
			return errors.New("cannot get standard tags from parent object: empty namespace")
		}
	}

	// Try to discover standard labels on the pod's owner
	owners := pod.GetOwnerReferences()
	if len(owners) == 0 {
		return nil
	}

	owner, err := getOwner(owners[0], ns, dc)
	if err != nil {
		metrics.MutationErrors.Inc(metrics.TagsMutationType, "cannot get owner")
		return err
	}

	log.Debugf("Looking for standard labels on '%s/%s' - kind '%s' owner of pod %s", owner.GetNamespace(), owner.GetName(), owner.GetKind(), podString(pod))
	_, injected = injectTagsFromLabels(owner.GetLabels(), pod)

	return nil
}

// shouldInjectConf returns whether we should try to inject standard tags
func shouldInjectTags(pod *corev1.Pod) bool {
	if val := pod.GetLabels()[common.EnabledLabelKey]; val == "false" {
		return false
	}
	return true
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
			if injected := injectEnv(pod, env); injected {
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
func getOwner(owner metav1.OwnerReference, ns string, dc dynamic.Interface) (*unstructured.Unstructured, error) {
	ownerInfo, err := getOwnerInfo(owner)
	if err != nil {
		return nil, err
	}

	obj, err := getAndCacheOwner(ownerInfo, ns, dc)
	if err != nil {
		return nil, err
	}

	// Try to discover standard labels from the deployment object if the owner is a replicaset
	if obj.GetKind() == "ReplicaSet" && len(obj.GetOwnerReferences()) > 0 {
		rsOwnerInfo, err := getOwnerInfo(obj.GetOwnerReferences()[0])
		if err != nil {
			return nil, err
		}

		return getAndCacheOwner(rsOwnerInfo, ns, dc)
	}

	return obj, nil
}

// getAndCacheOwner tries to fetch the owner object from cache before querying the api server
func getAndCacheOwner(info *ownerInfo, ns string, dc dynamic.Interface) (*unstructured.Unstructured, error) {
	infoID := info.buildID(ns)
	var ownerObj *unstructured.Unstructured

	if cachedObj, hit := cache.Cache.Get(infoID); hit {
		metrics.GetOwnerCacheHit.Inc(info.gvr.Resource)
		var valid bool
		ownerObj, valid = cachedObj.(*unstructured.Unstructured)
		if !valid {
			log.Debugf("Invalid owner object for '%s', forcing a cache miss", infoID)
		} else {
			return ownerObj, nil
		}
	}

	log.Tracef("Cache miss while getting owner '%s'", infoID)
	metrics.GetOwnerCacheMiss.Inc(info.gvr.Resource)
	ownerObj, err := dc.Resource(info.gvr).Namespace(ns).Get(context.TODO(), info.name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	cache.Cache.Set(infoID, ownerObj, ownerCacheTTL)
	return ownerObj, nil
}
