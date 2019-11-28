// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package listeners

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	newIdentifierLabel         = "com.datadoghq.ad.check.id"
	legacyIdentifierLabel      = "com.datadoghq.sd.check.id"
	dockerADTemplateLabelName  = "com.datadoghq.ad.instances"
	dockerADTemplateChechNames = "com.datadoghq.ad.check_names"
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
	checkNames := []string{}
	err := json.Unmarshal([]byte(labels[dockerADTemplateChechNames]), &checkNames)
	if err != nil {
		return nil, log.Errorf("Cannot parse check names: %v", err)
	}
	return checkNames, nil
}
