// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"strings"
)

const (
	checkIDAnnotationFormat   = "ad.datadoghq.com/%s.check.id"
	checkIDSuffix             = ".check.id"
	NewPodAnnotationPrefix    = "ad.datadoghq.com/"
	NewPodAnnotationFormat    = NewPodAnnotationPrefix + "%s."
	LegacyPodAnnotationPrefix = "service-discovery.datadoghq.com/"
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
		if strings.LastIndex(annotation, checkIDSuffix) >= len(NewPodAnnotationPrefix) {
			// validate check.id annotation
			err := validateIdentifier(annotation, containerNames, strings.LastIndex(annotation, checkIDSuffix))
			if err != nil {
				errors = append(errors, err)
			}
		} else if strings.LastIndex(annotation[len(NewPodAnnotationPrefix):], ".") >= 0 {
			// validate other AD annotations
			err := validateIdentifier(annotation, containerIdentifiers, strings.LastIndex(annotation, "."))
			if err != nil {
				errors = append(errors, err)
			}
		}
	}
	return errors
}

// validateIdentifier checks an annotation's container identifier against a list of valid identifiers
func validateIdentifier(annotation string, containerIdentifiers map[string]struct{}, adSuffixLen int) error {
	var id string
	if len(NewPodAnnotationPrefix) >= adSuffixLen {
		return fmt.Errorf("unable to determine container identifier for annotation %s", annotation)
	}
	// annotation keys should only contain the characters [a-z0-9A-Z-_.]
	// https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/#syntax-and-character-set
	id = annotation[len(NewPodAnnotationPrefix):adSuffixLen]
	if _, found := containerIdentifiers[id]; !found {
		validIDs := make([]string, 0, len(containerIdentifiers))
		for validID := range containerIdentifiers {
			validIDs = append(validIDs, validID)
		}
		return fmt.Errorf("annotation %s is invalid: %s doesn't match a container identifier %v", annotation, id, validIDs)
	}
	return nil
}
