// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	"fmt"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
)

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
		containerName, isInitContainer := ExtractContainerFromAnnotationKey(annotation)
		if containerName != "" {
			if isInitContainer {
				containerName = fmt.Sprintf("init.%s", containerName)
			}
			containerslanguages.GetOrInitializeLanguageset(containerName).Parse(languages)
		}
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

// ToProto returns a proto message ContainerLanguageDetails
func (containerslanguages ContainersLanguages) ToProto() []*pbgo.ContainerLanguageDetails {
	res := make([]*pbgo.ContainerLanguageDetails, 0, len(containerslanguages))
	for containerName, languageSet := range containerslanguages {
		res = append(res, &pbgo.ContainerLanguageDetails{
			ContainerName: containerName,
			Languages:     languageSet.ToProto(),
		})
	}
	return res
}

// Equals returns if the ContainersLanguages is equal to another ContainersLanguages
func (containerslanguages ContainersLanguages) Equals(other ContainersLanguages) bool {
	if len(containerslanguages) != len(other) {
		return false
	}
	for key, val := range containerslanguages {
		if otherVal, ok := other[key]; !ok || !val.Equals(otherVal) {
			return false
		}
	}
	return true
}
