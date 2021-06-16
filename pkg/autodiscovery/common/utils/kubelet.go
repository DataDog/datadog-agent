// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"sort"
	"strings"
)

const (
	checkIDAnnotationFormat = "ad.datadoghq.com/%s.check.id"
	checkIDSuffix           = ".check.id"
	// NewPodAnnotationPrefix is the new autodiscovery prefix for pod annotations
	NewPodAnnotationPrefix = "ad.datadoghq.com/"
	// NewPodAnnotationFormat shows the prefix + identifier format for new autodiscovery annotations
	NewPodAnnotationFormat = NewPodAnnotationPrefix + "%s."
	// LegacyPodAnnotationPrefix is the legacy autodiscovery prefix for pod annotations
	LegacyPodAnnotationPrefix = "service-discovery.datadoghq.com/"
	// LegacyPodAnnotationFormat shows the prefix + identifier format for legacy autodiscovery annotations
	LegacyPodAnnotationFormat = LegacyPodAnnotationPrefix + "%s."
)

// GetCustomCheckID returns whether there is a custom check ID for a given container based on the pod annotations
func GetCustomCheckID(annotations map[string]string, containerName string) (string, bool) {
	id, found := annotations[fmt.Sprintf(checkIDAnnotationFormat, containerName)]
	return id, found
}

// ValidateAnnotationsMatching detects if annotations using the new AD annotation format don't match a valid container identifier
func ValidateAnnotationsMatching(annotations map[string]string, containerIdentifiers map[string]struct{}, containerNames map[string]struct{}) []error {
	var errors []error

	for annotation := range annotations {
		if !strings.HasPrefix(annotation, NewPodAnnotationPrefix) {
			continue
		}
		var idToValidate string
		checkIDIndex := strings.LastIndex(annotation, checkIDSuffix)
		adSuffixIndex := strings.LastIndex(annotation, ".")
		if checkIDIndex >= len(NewPodAnnotationPrefix) {
			// validate check.id annotation
			idToValidate = annotation[len(NewPodAnnotationPrefix):checkIDIndex]
			err := validateIdentifier(annotation, containerNames, idToValidate)
			if err != nil {
				errors = append(errors, err)
			}
		} else if adSuffixIndex >= len(NewPodAnnotationPrefix) {
			// validate other AD annotations
			idToValidate = annotation[len(NewPodAnnotationPrefix):adSuffixIndex]
			err := validateIdentifier(annotation, containerIdentifiers, idToValidate)
			if err != nil {
				errors = append(errors, err)
			}
		}
	}
	return errors
}

// validateIdentifier checks an annotation's container identifier against a list of valid identifiers
func validateIdentifier(annotation string, containerIdentifiers map[string]struct{}, idToValidate string) error {
	if _, found := containerIdentifiers[idToValidate]; !found {
		validIDs := make([]string, 0, len(containerIdentifiers))
		for validID := range containerIdentifiers {
			validIDs = append(validIDs, validID)
		}
		sort.Strings(validIDs)
		return fmt.Errorf("annotation %s is invalid: %s doesn't match a container identifier %v", annotation, idToValidate, validIDs)
	}
	return nil
}
