// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package proto provides functions to convert language detection results to proto messages
package proto

import (
	"github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
)

// ContainersLanguagesToProto returns two proto messages ContainerLanguageDetails
// The first one contains standard containers
// The second one contains init containers
func ContainersLanguagesToProto(c util.ContainersLanguages) (containersDetailsProto, initContainersDetailsProto []*pbgo.ContainerLanguageDetails) {
	containersDetailsProto = make([]*pbgo.ContainerLanguageDetails, 0, len(c))
	initContainersDetailsProto = make([]*pbgo.ContainerLanguageDetails, 0, len(c))
	for container, languageSet := range c {
		if container.Init {
			initContainersDetailsProto = append(initContainersDetailsProto, &pbgo.ContainerLanguageDetails{
				ContainerName: container.Name,
				Languages:     LanguageSetToProto(languageSet),
			})
		} else {
			containersDetailsProto = append(containersDetailsProto, &pbgo.ContainerLanguageDetails{
				ContainerName: container.Name,
				Languages:     LanguageSetToProto(languageSet),
			})
		}

	}
	return containersDetailsProto, initContainersDetailsProto
}

// ToProto returns a proto message Language
func LanguageSetToProto(s util.LanguageSet) []*pbgo.Language {
	res := make([]*pbgo.Language, 0, len(s))
	for lang := range s {
		res = append(res, &pbgo.Language{
			Name: string(lang),
		})
	}
	return res
}
