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

// GetOrInitialize returns the language set of a container if it exists, or initializes it otherwise
func (c ContainersLanguages) GetOrInitialize(container Container) *LanguageSet {
	_, found := c[container]
	if !found {
		c[container] = make(LanguageSet)
	}
	languageSet := c[container]
	return &languageSet
}

// Merge merges another containers languages object to the current object
func (c ContainersLanguages) Merge(other ContainersLanguages) {
	if len(other) == 0 {
		return
	}

	for container, languageSet := range other {
		c.GetOrInitialize(container).Merge(languageSet)
	}
}

// ToProto returns a proto message ContainerLanguageDetails
func (c ContainersLanguages) ToProto() []*pbgo.ContainerLanguageDetails {
	res := make([]*pbgo.ContainerLanguageDetails, 0, len(c))
	for container, languageSet := range c {
		res = append(res, &pbgo.ContainerLanguageDetails{
			ContainerName: container.Name,
			Languages:     languageSet.ToProto(),
		})
	}
	return res
}

// EqualTo checks if current ContainersLanguages object has identical content
// in comparison another ContainersLanguages
func (c ContainersLanguages) EqualTo(other ContainersLanguages) bool {
	if other == nil {
		return false
	}

	if len(c) != len(other) {
		return false
	}

	for container, languageSet := range c {
		otherLanguageSet, found := other[container]

		if !found || !reflect.DeepEqual(languageSet, otherLanguageSet) {
			return false
		}
	}
	return true
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
