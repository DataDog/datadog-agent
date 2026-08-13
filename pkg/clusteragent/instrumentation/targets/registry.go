// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0. This product includes software developed
// at Datadog (https://www.datadoghq.com/). Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package targets contains the workload kinds supported by
// DatadogInstrumentation and the owner-chain resolver used to associate Pods
// with those workloads.
package targets

import (
	"fmt"
	"strings"

	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const customWorkloadTargetsConfigKey = "instrumentation_crd_controller.custom_workload_targets"

// Resource identifies a namespaced Kubernetes workload resource.
type Resource struct {
	APIVersion string `mapstructure:"apiVersion" json:"apiVersion" yaml:"apiVersion"`
	Kind       string `mapstructure:"kind" json:"kind" yaml:"kind"`
	Resource   string `mapstructure:"resource" json:"resource" yaml:"resource"`
}

// Profile describes a supported DatadogInstrumentation target and the
// intermediate owner kinds that may occur between a Pod and that target.
type Profile struct {
	Target Resource   `mapstructure:"target" json:"target" yaml:"target"`
	Via    []Resource `mapstructure:"via" json:"via,omitempty" yaml:"via,omitempty"`
}

type resourceDescriptor struct {
	resource    Resource
	isTarget    bool
	traversable bool
}

// Registry is the immutable set of built-in and customer-configured workload
// profiles used by both DDI handlers and Pod owner resolution.
type Registry struct {
	resources map[string]resourceDescriptor
	targets   map[string]Resource
	hasCustom bool
}

// NewRegistry returns a registry containing native workload kinds plus custom
// profiles configured by the cluster administrator.
func NewRegistry(cfg configmodel.Reader) (*Registry, error) {
	registry := &Registry{
		resources: make(map[string]resourceDescriptor),
		targets:   make(map[string]Resource),
	}
	for i, profile := range builtinProfiles() {
		if err := registry.addProfile(profile); err != nil {
			return registry, fmt.Errorf("built-in workload target profile %d: %w", i, err)
		}
	}

	var custom []Profile
	if err := structure.UnmarshalKey(cfg, customWorkloadTargetsConfigKey, &custom, structure.ErrorUnused); err != nil {
		return registry, fmt.Errorf("parse %s: %w", customWorkloadTargetsConfigKey, err)
	}
	for i, profile := range custom {
		if err := registry.addProfile(profile); err != nil {
			return registry, fmt.Errorf("custom workload target profile %d: %w", i, err)
		}
	}
	registry.hasCustom = len(custom) > 0
	return registry, nil
}

// HasCustomTargets reports whether at least one customer workload profile was configured.
func (r *Registry) HasCustomTargets() bool {
	return r != nil && r.hasCustom
}

// Supports reports whether a DDI target reference identifies a registered
// workload target. Empty API versions are accepted only for built-in kinds to
// preserve compatibility with older DDI objects and tests.
func (r *Registry) Supports(apiVersion, kind string) bool {
	if r == nil {
		return false
	}
	if apiVersion != "" {
		_, found := r.targets[resourceKey(apiVersion, kind)]
		return found
	}
	for _, target := range r.targets {
		if target.Kind == kind && isBuiltinTarget(target) {
			return true
		}
	}
	return false
}

func (r *Registry) descriptor(apiVersion, kind string) (resourceDescriptor, bool) {
	descriptor, found := r.resources[resourceKey(apiVersion, kind)]
	return descriptor, found
}

func (r *Registry) addProfile(profile Profile) error {
	if err := validateResource(profile.Target); err != nil {
		return fmt.Errorf("target: %w", err)
	}
	if err := r.addResource(profile.Target, true, false); err != nil {
		return err
	}
	r.targets[resourceKey(profile.Target.APIVersion, profile.Target.Kind)] = profile.Target

	for i, via := range profile.Via {
		if err := validateResource(via); err != nil {
			return fmt.Errorf("via[%d]: %w", i, err)
		}
		if err := r.addResource(via, false, true); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) addResource(resource Resource, target, traversable bool) error {
	key := resourceKey(resource.APIVersion, resource.Kind)
	descriptor, found := r.resources[key]
	if found && descriptor.resource.Resource != resource.Resource {
		return fmt.Errorf("%s maps to both resources %q and %q", key, descriptor.resource.Resource, resource.Resource)
	}
	if !found {
		descriptor.resource = resource
	}
	descriptor.isTarget = descriptor.isTarget || target
	descriptor.traversable = descriptor.traversable || traversable
	r.resources[key] = descriptor
	return nil
}

func validateResource(resource Resource) error {
	if strings.TrimSpace(resource.APIVersion) == "" || strings.TrimSpace(resource.Kind) == "" || strings.TrimSpace(resource.Resource) == "" {
		return fmt.Errorf("apiVersion, kind, and resource are required")
	}
	if _, err := schema.ParseGroupVersion(resource.APIVersion); err != nil {
		return fmt.Errorf("invalid apiVersion %q: %w", resource.APIVersion, err)
	}
	return nil
}

func resourceKey(apiVersion, kind string) string {
	return apiVersion + "/" + kind
}

func isBuiltinTarget(resource Resource) bool {
	for _, profile := range builtinProfiles() {
		if profile.Target == resource {
			return true
		}
	}
	return false
}

func builtinProfiles() []Profile {
	replicaSet := Resource{APIVersion: "apps/v1", Kind: "ReplicaSet", Resource: "replicasets"}
	job := Resource{APIVersion: "batch/v1", Kind: "Job", Resource: "jobs"}
	return []Profile{
		{Target: Resource{APIVersion: "apps/v1", Kind: "Deployment", Resource: "deployments"}, Via: []Resource{replicaSet}},
		{Target: Resource{APIVersion: "apps/v1", Kind: "DaemonSet", Resource: "daemonsets"}},
		{Target: Resource{APIVersion: "apps/v1", Kind: "StatefulSet", Resource: "statefulsets"}},
		{Target: Resource{APIVersion: "batch/v1", Kind: "CronJob", Resource: "cronjobs"}, Via: []Resource{job}},
		{Target: job},
		{Target: Resource{APIVersion: "argoproj.io/v1alpha1", Kind: "Rollout", Resource: "rollouts"}, Via: []Resource{replicaSet}},
	}
}
