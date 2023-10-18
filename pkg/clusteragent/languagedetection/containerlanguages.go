// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package languagedetection

import (
	"fmt"
	"regexp"
)

var re = regexp.MustCompile(`apm\.datadoghq\.com\/(init)?\.?(.+?)\.languages`)

// ContainersLanguages maps container name to language set
type ContainersLanguages map[string]LanguageSet

// NewContainersLanguages initializes and returns a new ContainersLanguages object
func NewContainersLanguages() ContainersLanguages {
	return make(ContainersLanguages)
}

// GetOrInitializeLanguageset initializes the language set of a specific container if it doesn't exist, then it returns it
func (containerslanguages ContainersLanguages) GetOrInitializeLanguageset(containerName string) LanguageSet {
	_, found := containerslanguages[containerName]
	if !found {
		containerslanguages[containerName] = NewLanguageSet()
	}

	return containerslanguages[containerName]
}

// TotalLanguages gets the total number of languages that are added to all containers
func (containerslanguages ContainersLanguages) TotalLanguages() int {
	numberOfLanguages := 0

	for _, languageset := range containerslanguages {
		numberOfLanguages += len(languageset)
	}

	return numberOfLanguages
}

// ParseAnnotations updates the containers languages based on existing language annotations
func (containerslanguages ContainersLanguages) ParseAnnotations(annotations map[string]string) {
	for annotation, languages := range annotations {
		// find a match
		matches := re.FindStringSubmatch(annotation)
		if len(matches) != 3 {
			continue
		}

		containerName := matches[2]

		// matches[1] matches "init"
		if matches[1] != "" {
			containerName = fmt.Sprintf("init.%s", containerName)
		}

		containerslanguages.GetOrInitializeLanguageset(containerName).Parse(languages)
	}
}

// ToAnnotations converts the containers languages into language annotations map
func (containerslanguages ContainersLanguages) ToAnnotations() map[string]string {
	annotations := make(map[string]string)

	for container, languageset := range containerslanguages {
		annotationValue := fmt.Sprint(languageset)

		if len(annotationValue) > 0 {
			annotations[GetLanguageAnnotationKey(container)] = fmt.Sprint(languageset)
		}
	}

	return annotations
}
