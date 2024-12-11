// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package utils

import (
	"encoding/json"
	"maps"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

const (
	pods       string = "pods"
	nodes      string = "nodes"
	namespaces string = "namespaces"
)

// MetadataAsTags contains the labels as tags and annotations as tags for each kubernetes resource based on the user configurations of the following options ordered in increasing order of priority:
//
// kubernetes_pod_labels_as_tags
// kubernetes_pod_annotations_as_tags
// kubernetes_node_labels_as_tags
// kubernetes_node_annotations_as_tags
// kubernetes_namespace_labels_as_tags
// kubernetes_namespace_annotations_as_tags
// kubernetes_resources_labels_as_tags
// kubernetes_resources_anotations_as_tags
//
// In case of conflict, higher priority configuration value takes precedences
// For example, if kubernetes_pod_labels_as_tags = {`l1`: `v1`, `l2`: `v2`} and kubernetes_resources_labels_as_tags = {`pods`: {`l1`: `x`}},
// the resulting labels as tags for pods will be {`l1`: `x`, `l2`: `v2`}
type MetadataAsTags interface {
	// GetPodLabelsAsTags returns pod labels as tags
	GetPodLabelsAsTags() map[string]string
	// GetPodAnnotationsAsTags returns pod annotations as tags
	GetPodAnnotationsAsTags() map[string]string
	// GetNodeLabelsAsTags returns node labels as tags
	GetNodeLabelsAsTags() map[string]string
	// GetNodeAnnotationsAsTags returns node annotations as tags
	GetNodeAnnotationsAsTags() map[string]string
	// GetNamespaceLabelsAsTags returns namespace labels as tags
	GetNamespaceLabelsAsTags() map[string]string
	// GetNamespaceAnnotationsAsTags returns namespace annotations as tags
	GetNamespaceAnnotationsAsTags() map[string]string
	// GetResourcesLabelsAsTags returns resources labels as tags
	GetResourcesLabelsAsTags() map[string]map[string]string
	// GetResourcesAnnotationsAsTags returns resources annotations as tags
	GetResourcesAnnotationsAsTags() map[string]map[string]string
}

type metadataAsTags struct {
	labelsAsTags      map[string]map[string]string
	annotationsAsTags map[string]map[string]string
}

var _ MetadataAsTags = &metadataAsTags{}

// GetPodLabelsAsTags implements MetadataAsTags#GetPodLabelsAsTags
func (m *metadataAsTags) GetPodLabelsAsTags() map[string]string {
	return m.labelsAsTags[pods]
}

// GetPodAnnotationsAsTags implements MetadataAsTags#GetPodAnnotationsAsTags
func (m *metadataAsTags) GetPodAnnotationsAsTags() map[string]string {
	return m.annotationsAsTags[pods]
}

// GetNodeLabelsAsTags implements MetadataAsTags#GetNodeLabelsAsTags
func (m *metadataAsTags) GetNodeLabelsAsTags() map[string]string {
	return m.labelsAsTags[nodes]
}

// GetNodeAnnotationsAsTags implements MetadataAsTags#GetNodeAnnotationsAsTags
func (m *metadataAsTags) GetNodeAnnotationsAsTags() map[string]string {
	return m.annotationsAsTags[nodes]
}

// GetNamespaceLabelsAsTags implements MetadataAsTags#GetNamespaceLabelsAsTags
func (m *metadataAsTags) GetNamespaceLabelsAsTags() map[string]string {
	return m.labelsAsTags[namespaces]
}

// GetNamespaceAnnotationsAsTags implements MetadataAsTags#GetNamespaceAnnotationsAsTags
func (m *metadataAsTags) GetNamespaceAnnotationsAsTags() map[string]string {
	return m.annotationsAsTags[namespaces]
}

// GetResourcesLabelsAsTags implements MetadataAsTags#GetResourcesLabelsAsTags
func (m *metadataAsTags) GetResourcesLabelsAsTags() map[string]map[string]string {
	return m.labelsAsTags
}

// GetResourcesAnnotationsAsTags implements MetadataAsTags#GetResourcesAnnotationsAsTags
func (m *metadataAsTags) GetResourcesAnnotationsAsTags() map[string]map[string]string {
	return m.annotationsAsTags
}

func (m *metadataAsTags) mergeGenericResourcesLabelsAsTags(cfg pkgconfigmodel.Reader) {
	resourcesToLabelsAsTags := retrieveDoubleMappingFromConfig(cfg, "kubernetes_resources_labels_as_tags")

	for resource, labelsAsTags := range resourcesToLabelsAsTags {
		// "pods.", "nodes.", "namespaces." are valid configurations, but they should be replaced here by "pods", "nodes" and "namespaces" respectively to ensure that they override the existing configurations for pods, nodes and namespaces
		cleanResource := strings.TrimSuffix(resource, ".")
		_, found := m.labelsAsTags[cleanResource]
		if !found {
			m.labelsAsTags[cleanResource] = map[string]string{}
		}
		// When a key in `labelsAsTags` exist in `m.labelsAsTags[cleanResource]`, the value in `m.labelsAsTags[cleanResource]` will be overwritten by the value associated with the key in `labelsAsTags`
		// source: https://pkg.go.dev/maps#Copy
		maps.Copy(m.labelsAsTags[cleanResource], labelsAsTags)
	}
}

func (m *metadataAsTags) mergeGenericResourcesAnnotationsAsTags(cfg pkgconfigmodel.Reader) {
	resourcesToAnnotationsAsTags := retrieveDoubleMappingFromConfig(cfg, "kubernetes_resources_annotations_as_tags")

	for resource, annotationsAsTags := range resourcesToAnnotationsAsTags {
		// "pods.", "nodes.", "namespaces." are valid configurations, but they should be replaced here by "pods", "nodes" and "namespaces" respectively to ensure that they override the existing configurations for pods, nodes and namesapces
		cleanResource := strings.TrimSuffix(resource, ".")
		_, found := m.annotationsAsTags[cleanResource]
		if !found {
			m.annotationsAsTags[cleanResource] = map[string]string{}
		}
		// When a key in `annotationsAsTags` exist in `m.annotationsAsTags[cleanResource]`, the value in `m.annotationsAsTags[cleanResource]` will be overwritten by the value associated with the key in `annotationsAsTags`
		// source: https://pkg.go.dev/maps#Copy
		maps.Copy(m.annotationsAsTags[cleanResource], annotationsAsTags)
	}
}

// GetMetadataAsTags returns a merged configuration of all labels and annotations as tags set by the user
func GetMetadataAsTags(c pkgconfigmodel.Reader) MetadataAsTags {

	metadataAsTags := metadataAsTags{
		labelsAsTags:      map[string]map[string]string{},
		annotationsAsTags: map[string]map[string]string{},
	}

	// node labels/annotations as tags
	if nodeLabelsAsTags := c.GetStringMapString("kubernetes_node_labels_as_tags"); nodeLabelsAsTags != nil {
		metadataAsTags.labelsAsTags[nodes] = lowerCaseMapKeys(nodeLabelsAsTags)
	}
	if nodeAnnotationsAsTags := c.GetStringMapString("kubernetes_node_annotations_as_tags"); nodeAnnotationsAsTags != nil {
		metadataAsTags.annotationsAsTags[nodes] = lowerCaseMapKeys(nodeAnnotationsAsTags)
	}

	// namespace labels/annotations as tags
	if namespaceLabelsAsTags := c.GetStringMapString("kubernetes_namespace_labels_as_tags"); namespaceLabelsAsTags != nil {
		metadataAsTags.labelsAsTags[namespaces] = lowerCaseMapKeys(namespaceLabelsAsTags)
	}
	if namespaceAnnotationsAsTags := c.GetStringMapString("kubernetes_namespace_annotations_as_tags"); namespaceAnnotationsAsTags != nil {
		metadataAsTags.annotationsAsTags[namespaces] = lowerCaseMapKeys(namespaceAnnotationsAsTags)
	}

	// pod labels/annotations as tags
	if podLabelsAsTags := c.GetStringMapString("kubernetes_pod_labels_as_tags"); podLabelsAsTags != nil {
		metadataAsTags.labelsAsTags[pods] = lowerCaseMapKeys(podLabelsAsTags)
	}
	if podAnnotationsAsTags := c.GetStringMapString("kubernetes_pod_annotations_as_tags"); podAnnotationsAsTags != nil {
		metadataAsTags.annotationsAsTags[pods] = lowerCaseMapKeys(podAnnotationsAsTags)
	}

	// generic resources labels/annotations as tags
	metadataAsTags.mergeGenericResourcesLabelsAsTags(c)
	metadataAsTags.mergeGenericResourcesAnnotationsAsTags(c)

	return &metadataAsTags
}

func retrieveDoubleMappingFromConfig(cfg pkgconfigmodel.Reader, configKey string) map[string]map[string]string {
	valueFromConfig := cfg.GetString(configKey)

	var doubleMap map[string]map[string]string
	err := json.Unmarshal([]byte(valueFromConfig), &doubleMap)

	if err != nil {
		log.Errorf("failed to parse %s with value %s into json: %v", configKey, valueFromConfig, err)
		return map[string]map[string]string{}
	}

	for resource, tags := range doubleMap {
		doubleMap[resource] = lowerCaseMapKeys(tags)
	}

	return doubleMap
}

// lowerCaseMapKeys lowercases all map keys
func lowerCaseMapKeys(m map[string]string) map[string]string {
	for label, value := range m {
		delete(m, label)
		m[strings.ToLower(label)] = value
	}
	return m
}
