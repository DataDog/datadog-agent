// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	"fmt"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"reflect"
	"sort"
	"strings"
)

////////////////////////////////
//                            //
//         Container          //
//                            //
////////////////////////////////

// Container identifies a pod container by its name and an init boolean flag
type Container struct {
	Name string
	Init bool
}

// NewContainer creates and returns a new Container object  with unset init flag
func NewContainer(containerName string) *Container {
	return &Container{
		Name: containerName,
		Init: false,
	}
}

// NewInitContainer creates and returns a new Container object  with set init flag
func NewInitContainer(containerName string) *Container {
	return &Container{
		Name: containerName,
		Init: true,
	}
}

////////////////////////////////
//                            //
//    ContainersLanguages     //
//                            //
////////////////////////////////

// ContainersLanguages handles mapping containers to language sets
type ContainersLanguages map[Container]LanguageSet

// ToProto returns two proto messages ContainerLanguageDetails
// The first one contains standard containers
// The second one contains init containers
func (c ContainersLanguages) ToProto() (containersDetailsProto, initContainersDetailsProto []*pbgo.ContainerLanguageDetails) {
	containersDetailsProto = make([]*pbgo.ContainerLanguageDetails, 0, len(c))
	initContainersDetailsProto = make([]*pbgo.ContainerLanguageDetails, 0, len(c))
	for container, languageSet := range c {
		if container.Init {
			initContainersDetailsProto = append(initContainersDetailsProto, &pbgo.ContainerLanguageDetails{
				ContainerName: container.Name,
				Languages:     languageSet.ToProto(),
			})
		} else {
			containersDetailsProto = append(containersDetailsProto, &pbgo.ContainerLanguageDetails{
				ContainerName: container.Name,
				Languages:     languageSet.ToProto(),
			})
		}

	}
	return containersDetailsProto, initContainersDetailsProto
}

// ToAnnotations converts the containers languages to language annotations
func (c ContainersLanguages) ToAnnotations() map[string]string {
	annotations := make(map[string]string)

	for container, langSet := range c {
		containerName := container.Name
		if container.Init {
			containerName = fmt.Sprintf("init.%s", containerName)
		}
		annotationKey := GetLanguageAnnotationKey(containerName)

		languagesNames := make([]string, 0, len(langSet))
		for lang := range langSet {
			languagesNames = append(languagesNames, string(lang))
		}

		sort.Strings(languagesNames)
		annotationValue := strings.Join(languagesNames, ",")

		if annotationValue != "" {
			annotations[annotationKey] = annotationValue
		}
	}

	return annotations
}

////////////////////////////////
//                            //
// Timed Containers Languages //
//                            //
////////////////////////////////

// TimedContainersLanguages handles mapping containers to timed language sets
type TimedContainersLanguages map[Container]TimedLanguageSet

// RemoveExpiredLanguages removes expired languages from each container language set
// Returns true if at least one language is expired and removed
func (c TimedContainersLanguages) RemoveExpiredLanguages() bool {
	atLeastOneLangRemoved := false

	for container, langset := range c {
		removedAny := langset.RemoveExpired()

		if len(langset) == 0 {
			delete(c, container)
		}

		atLeastOneLangRemoved = atLeastOneLangRemoved || removedAny
	}

	return atLeastOneLangRemoved
}

// GetOrInitialize returns the language set of a container if it exists, or initializes it otherwise
func (c TimedContainersLanguages) GetOrInitialize(container Container) *TimedLanguageSet {
	if _, found := c[container]; !found {
		c[container] = TimedLanguageSet{}
	}
	languageSet := c[container]
	return &languageSet
}

// Merge merges another containers languages object to the current object
// Returns true if new languages were added, and false otherwise
func (c TimedContainersLanguages) Merge(other TimedContainersLanguages) bool {
	modified := false
	for container, languageSet := range other {
		if c.GetOrInitialize(container).Merge(languageSet) {
			modified = true
		}
	}
	return modified
}

// EqualTo checks if current TimedContainersLanguages object has identical content
// in comparison another TimedContainersLanguages
func (c TimedContainersLanguages) EqualTo(other TimedContainersLanguages) bool {
	if other == nil {
		return false
	}

	if len(c) != len(other) {
		return false
	}

	for container, languageSet := range c {
		otherTimedLanguageSet, found := other[container]

		if !found || !reflect.DeepEqual(languageSet, otherTimedLanguageSet) {
			return false
		}
	}
	return true
}
