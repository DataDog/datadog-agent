// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

//nolint:revive // TODO(CAPP) Fix revive linter
package transformers

import (
	"fmt"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

const (
	tagKeyEnv     = "env"
	tagKeyVersion = "version"
	tagKeyService = "service"
)

var labelToTagKeys = map[string]string{
	kubernetes.EnvTagLabelKey:     tagKeyEnv,
	kubernetes.VersionTagLabelKey: tagKeyVersion,
	kubernetes.ServiceTagLabelKey: tagKeyService,
}

var ustLabelsWithoutFallback = []string{kubernetes.VersionTagLabelKey, kubernetes.ServiceTagLabelKey}

// RetrieveUnifiedServiceTags for cluster level resources
// the `env` is handled special because it being a host level tag.
func RetrieveUnifiedServiceTags(labels map[string]string) []string {
	var tags []string

	if tagValue, found := labels[kubernetes.EnvTagLabelKey]; found {
		tags = append(tags, fmt.Sprintf("%s:%s", labelToTagKeys[kubernetes.EnvTagLabelKey], tagValue))
	} else {
		if envTag := pkgconfigsetup.Datadog().GetString("env"); envTag != "" {
			tags = append(tags, fmt.Sprintf("%s:%s", tagKeyEnv, envTag))
		}
	}

	for _, labelKey := range ustLabelsWithoutFallback {
		if tagValue, found := labels[labelKey]; found {
			tags = append(tags, fmt.Sprintf("%s:%s", labelToTagKeys[labelKey], tagValue))
		}
	}
	return tags
}

func RetrieveMetadataTags(
	labels map[string]string,
	annotations map[string]string,
	labelsAsTags map[string]string,
	annotationsAsTags map[string]string,
) []string {
	tags := []string{}

	for name, value := range labels {
		if tagKey, ok := labelsAsTags[name]; ok {
			tags = append(tags, fmt.Sprintf("%s:%s", tagKey, value))
		}
	}

	for name, value := range annotations {
		if tagKey, ok := annotationsAsTags[name]; ok {
			tags = append(tags, fmt.Sprintf("%s:%s", tagKey, value))
		}
	}

	return tags
}
