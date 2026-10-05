// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagsfromlabels

import (
	"errors"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var labelsToEnv = map[string]string{
	kubernetes.EnvTagLabelKey:     kubernetes.EnvTagEnvVar,
	kubernetes.ServiceTagLabelKey: kubernetes.ServiceTagEnvVar,
	kubernetes.VersionTagLabelKey: kubernetes.VersionTagEnvVar,
}

// MutatorConfig holds the settings required for the tags mutator.
type MutatorConfig struct {
	ownerCacheTTL time.Duration
}

// NewMutatorConfig instantiates the required settings for the tags mutator from the datadog config.
func NewMutatorConfig(datadogConfig config.Component) *MutatorConfig {
	return &MutatorConfig{
		ownerCacheTTL: ownerCacheTTL(datadogConfig),
	}
}

// Mutator satisfies the common.PatchMutator interface for the tags mutator.
type Mutator struct {
	config *MutatorConfig
	filter mutatecommon.MutationFilter
}

// NewMutator creates a new injector interface for the tags mutator.
func NewMutator(cfg *MutatorConfig, filter mutatecommon.MutationFilter) *Mutator {
	return &Mutator{
		config: cfg,
		filter: filter,
	}
}

// PlanPod implements the common.PatchMutator interface for the tags mutator. It injects DD_ENV, DD_VERSION, DD_SERVICE
// env vars into a pod template if needed.
func (i *Mutator) PlanPod(session *patch.PodSession, ns string, dc dynamic.Interface) (bool, error) {
	if session == nil {
		return false, errors.New(metrics.InvalidInput)
	}
	pod, err := session.Snapshot()
	if err != nil {
		return false, err
	}
	var injected bool

	if pod == nil {
		return false, errors.New(metrics.InvalidInput)
	}

	if !i.filter.ShouldMutatePod(pod) {
		// Ignore pod if it has the label admission.datadoghq.com/enabled=false
		return false, nil
	}

	var found bool
	found, injected, err = planTagsFromLabels(pod.GetLabels(), session)
	if err != nil {
		return false, err
	}
	if found {
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
		log.Warnf("failed to get owner reference for pod, skipping owner-based tagging: %v", err)
		return false, nil // skip tagging, don't fail webhook
	}

	log.Debugf("Looking for standard labels on '%s/%s' - kind '%s' owner of pod %s", owner.namespace, owner.name, owner.kind, mutatecommon.PodString(pod))
	_, injected, err = planTagsFromLabels(owner.labels, session)

	return injected, err
}

// injectTagsFromLabels looks for standard tags in pod labels and injects them as environment variables if found
func planTagsFromLabels(labels map[string]string, session *patch.PodSession) (bool, bool, error) {
	found := false
	injectedAtLeastOnce := false
	keys := make([]string, 0, len(labelsToEnv))
	for key := range labelsToEnv {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, l := range keys {
		envName := labelsToEnv[l]
		if tagValue, labelFound := labels[l]; labelFound {
			env := corev1.EnvVar{
				Name:  envName,
				Value: tagValue,
			}
			injected, err := mutatecommon.PatchInjectEnv(session, env)
			if err != nil {
				return false, false, err
			}
			if injected {
				injectedAtLeastOnce = true
			}
			found = true
		}
	}
	return found, injectedAtLeastOnce, nil
}

func ownerCacheTTL(datadogConfig config.Component) time.Duration {
	if datadogConfig.IsConfigured("admission_controller.pod_owners_cache_validity") { // old option. Kept for backwards compatibility
		return datadogConfig.GetDuration("admission_controller.pod_owners_cache_validity") * time.Minute
	}

	return datadogConfig.GetDuration("admission_controller.inject_tags.pod_owners_cache_validity") * time.Minute
}
