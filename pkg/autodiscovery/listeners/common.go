// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-2020 Datadog, Inc.

package listeners

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	v1 "k8s.io/api/core/v1"
)

const (
	// Label keys of Docker Autodiscovery
	newIdentifierLabel         = "com.datadoghq.ad.check.id"
	legacyIdentifierLabel      = "com.datadoghq.sd.check.id"
	dockerADTemplateCheckNames = "com.datadoghq.ad.check_names"
	// Label keys of standard tags
	labelKeyEnv     = "tags.datadoghq.com/env"
	labelKeyVersion = "tags.datadoghq.com/version"
	labelKeyService = "tags.datadoghq.com/service"
	// Keys of standard tags
	tagKeyEnv     = "env"
	tagKeyVersion = "version"
	tagKeyService = "service"
)

// ComputeContainerServiceIDs takes an entity name, an image (resolved to an actual name) and labels
// and computes the service IDs for this container service.
func ComputeContainerServiceIDs(entity string, image string, labels map[string]string) []string {
	// ID override label
	if l, found := labels[newIdentifierLabel]; found {
		return []string{l}
	}
	if l, found := labels[legacyIdentifierLabel]; found {
		log.Warnf("found legacy %s label for %s, please use the new name %s",
			legacyIdentifierLabel, entity, newIdentifierLabel)
		return []string{l}
	}

	ids := []string{entity}

	// Add Image names (long then short if different)
	long, short, _, err := containers.SplitImageName(image)
	if err != nil {
		log.Warnf("error while spliting image name: %s", err)
	}
	if len(long) > 0 {
		ids = append(ids, long)
	}
	if len(short) > 0 && short != long {
		ids = append(ids, short)
	}
	return ids
}

// getCheckNamesFromLabels unmarshals the json string of check names
// defined in docker labels and returns a slice of check names
func getCheckNamesFromLabels(labels map[string]string) ([]string, error) {
	if checkLabels, found := labels[dockerADTemplateCheckNames]; found {
		checkNames := []string{}
		err := json.Unmarshal([]byte(checkLabels), &checkNames)
		if err != nil {
			return nil, fmt.Errorf("Cannot parse check names: %v", err)
		}
		return checkNames, nil
	}
	return nil, nil
}

// getStandardTags extract standard tags from labels of kubernetes services
func getStandardTags(labels map[string]string) []string {
	tags := []string{}
	if labels == nil {
		return tags
	}
	labelToTagKeys := map[string]string{
		labelKeyEnv:     tagKeyEnv,
		labelKeyVersion: tagKeyVersion,
		labelKeyService: tagKeyService,
	}
	for labelKey, tagKey := range labelToTagKeys {
		if tagValue, found := labels[labelKey]; found {
			tags = append(tags, fmt.Sprintf("%s:%s", tagKey, tagValue))
		}
	}
	return tags
}

// standardTagsDigest computes the hash of standard tags in a map
func standardTagsDigest(labels map[string]string) string {
	if labels == nil {
		return ""
	}
	h := fnv.New64()
	h.Write([]byte(labels[labelKeyEnv]))
	h.Write([]byte(labels[labelKeyVersion]))
	h.Write([]byte(labels[labelKeyService]))
	return strconv.FormatUint(h.Sum64(), 16)
}

// isServiceAnnotated returns true if the Service has an annotation with a given key
func isServiceAnnotated(ksvc *v1.Service, annotationKey string) bool {
	if ksvc != nil {
		if _, found := ksvc.GetAnnotations()[annotationKey]; found {
			return true
		}
	}
	return false
}
