// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagsfromlabels

import (
	"errors"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var labelsToEnv = map[string]string{
	kubernetes.EnvTagLabelKey:     kubernetes.EnvTagEnvVar,
	kubernetes.ServiceTagLabelKey: kubernetes.ServiceTagEnvVar,
	kubernetes.VersionTagLabelKey: kubernetes.VersionTagEnvVar,
}

// TagsInjectorConfig holds the settings required for the tags injector.
type TagsInjectorConfig struct {
	ownerCacheTTL time.Duration
}

// NewTagsInjectorConfig instantiates the required settings for the tags injector from the datadog config.
func NewTagsInjectorConfig(datadogConfig config.Component) *TagsInjectorConfig {
	return &TagsInjectorConfig{
		ownerCacheTTL: ownerCacheTTL(datadogConfig),
	}
}

// TagsInjector satisfies the common.Injector interface for the tags injector.
type TagsInjector struct {
	config *TagsInjectorConfig
	filter mutatecommon.InjectionFilter
}

// NewTagsInjector creates a new injector interface for the tags injector.
func NewTagsInjector(cfg *TagsInjectorConfig, filter mutatecommon.InjectionFilter) *TagsInjector {
	return &TagsInjector{
		config: cfg,
		filter: filter,
	}
}

// InjectPod implements the common.Injector interface for the tags injector. It injects DD_ENV, DD_VERSION, DD_SERVICE
// env vars into a pod template if needed.
func (i *TagsInjector) InjectPod(pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error) {
	var injected bool

	if pod == nil {
		return false, errors.New(metrics.InvalidInput)
	}

	if !i.filter.ShouldMutatePod(pod) {
		// Ignore pod if it has the label admission.datadoghq.com/enabled=false
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

	owner, err := getOwner(owners[0], ns, dc, i.config.ownerCacheTTL)
	if err != nil {
		log.Error(err)
		return false, errors.New(metrics.InternalError)
	}

	log.Debugf("Looking for standard labels on '%s/%s' - kind '%s' owner of pod %s", owner.namespace, owner.name, owner.kind, mutatecommon.PodString(pod))
	_, injected = injectTagsFromLabels(owner.labels, pod)

	return injected, nil
}

// injectTagsFromLabels looks for standard tags in pod labels and injects them as environment variables if found
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

func ownerCacheTTL(datadogConfig config.Component) time.Duration {
	if datadogConfig.IsSet("admission_controller.pod_owners_cache_validity") { // old option. Kept for backwards compatibility
		return datadogConfig.GetDuration("admission_controller.pod_owners_cache_validity") * time.Minute
	}

	return datadogConfig.GetDuration("admission_controller.inject_tags.pod_owners_cache_validity") * time.Minute
}
