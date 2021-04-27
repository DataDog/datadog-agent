// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	checkIDAnnotationFormat = "ad.datadoghq.com/%s.check.id"
)

// GetCustomCheckID returns whether there is a custom check ID for a given container based on the pod annotations
func GetCustomCheckID(annotations map[string]string, containerName string) (string, bool) {
	id, found := annotations[fmt.Sprintf(checkIDAnnotationFormat, containerName)]
	return id, found
}

// ValidateAnnotationsMatching detects if AD annotations don't match a valid container identifier
func ValidateAnnotationsMatching(annotations map[string]string, containerIdentifiers map[string]bool, containerNames map[string]bool, adPrefix string) []error {
	var errors []error
	adAnnotationRegex := fmt.Sprintf(`^%s.+\..+$`, strings.ReplaceAll(adPrefix, ".", "\\."))
	checkIDAnnotationRegex := "^" + fmt.Sprintf(strings.ReplaceAll(checkIDAnnotationFormat, ".", "\\."), ".+") + "$"

	for annotation := range annotations {
		if matched, _ := regexp.MatchString(checkIDAnnotationRegex, annotation); matched {
			// validate check.id annotation
			err := validateIdentifier(annotation, containerNames, adPrefix, strings.LastIndex(annotation, ".check.id"))
			if err != nil {
				errors = append(errors, err)
			}
		} else if matched, _ := regexp.MatchString(adAnnotationRegex, annotation); matched {
			// validate other AD annotations
			err := validateIdentifier(annotation, containerIdentifiers, adPrefix, strings.LastIndex(annotation, "."))
			if err != nil {
				errors = append(errors, err)
			}
		}
	}
	return errors
}

func validateIdentifier(annotation string, containerIdentifiers map[string]bool, adPrefix string, adSuffix int) error {
	var id string
	if adSuffix > len(adPrefix) {
		// annotation keys should only contain the characters [a-z0-9A-Z-_.]
		// https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/#syntax-and-character-set
		id = annotation[len(adPrefix):adSuffix]
	} else {
		return fmt.Errorf("unable to determine container identifier for annotation %s", annotation)
	}
	if found := containerIdentifiers[id]; !found {
		return fmt.Errorf("annotation %s is invalid: %s doesn't match a container identifier", annotation, id)
	}
	return nil
}
