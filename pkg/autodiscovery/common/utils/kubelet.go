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
	adAnnotation := fmt.Sprintf(`%s.+\..+`, adPrefix)
	checkIDAnnotation := fmt.Sprintf(checkIDAnnotationFormat, ".+\\")

	for annotation := range annotations {
		if matched, _ := regexp.MatchString(checkIDAnnotation, annotation); matched {
			// validate check.id annotation
			err := validateIdentifier(annotation, containerNames, adPrefix)
			if err != nil {
				errors = append(errors, err)
			}
		} else if matched, _ := regexp.MatchString(adAnnotation, annotation); matched {
			// validate other AD annotations
			err := validateIdentifier(annotation, containerIdentifiers, adPrefix)
			if err != nil {
				errors = append(errors, err)
			}
		}
	}
	return errors
}

func validateIdentifier(annotation string, containerIdentifiers map[string]bool, adPrefix string) error {
	id := strings.Split(annotation[len(adPrefix):], ".")[0]
	if found := containerIdentifiers[id]; !found {
		return fmt.Errorf("annotation %s is invalid: %s doesn't match a container identifier", annotation, id)
	}
	return nil
}
