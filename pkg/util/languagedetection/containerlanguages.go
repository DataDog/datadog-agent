// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	"fmt"
	"regexp"
)

var re = regexp.MustCompile(`apm\.datadoghq\.com\/(init)?\.?(.+?)\.languages`)

type ContainersLanguagesInterface interface {
	Parse(containerName string, languageNames string)
	Add(containerName string, languageName string)
	TotalLanguages() int
}

// TODO handle the case of init containers
type ContainersLanguages struct {
	Languages map[string]*LanguageSet
}

func NewContainersLanguages() *ContainersLanguages {
	return &ContainersLanguages{
		Languages: make(map[string]*LanguageSet),
	}
}

// Parses a comma-separated string of language names and adds them to the specified container
func (containerslanguages *ContainersLanguages) Parse(containerName string, languageNames string) {
	_, found := containerslanguages.Languages[containerName]

	if !found {
		containerslanguages.Languages[containerName] = NewLanguageSet()
	}

	containerslanguages.Languages[containerName].Parse(languageNames)
}

// Adds a language to the specified container
func (containerslanguages *ContainersLanguages) Add(containerName string, languageName string) {
	_, found := containerslanguages.Languages[containerName]

	if !found {
		containerslanguages.Languages[containerName] = NewLanguageSet()
	}

	containerslanguages.Languages[containerName].Add(languageName)
}

// Gets the total number of languages that are added to all containers
func (containerslanguages *ContainersLanguages) TotalLanguages() int {
	numberOfLanguages := 0

	for _, languageset := range containerslanguages.Languages {
		numberOfLanguages += len(languageset.languages)
	}

	return numberOfLanguages
}

// Updates the containers languages based on existing language annotations
func (containerslanguages *ContainersLanguages) ParseAnnotations(annotations map[string]string) {

	for annotation, languages := range annotations {
		// find a match
		matches := re.FindStringSubmatch(annotation)
		if len(matches) != 3 {
			continue
		}
		containerslanguages.Parse(matches[2], languages)
	}
}

// Converts the containers languages into language annotations map
func (containerslanguages *ContainersLanguages) ToAnnotations() map[string]string {
	annotations := make(map[string]string)

	for container, languageset := range containerslanguages.Languages {
		annotationValue := fmt.Sprint(languageset)

		if len(annotationValue) > 0 {
			annotations[GetLanguageAnnotationKey(container)] = fmt.Sprint(languageset)
		}
	}

	return annotations
}
