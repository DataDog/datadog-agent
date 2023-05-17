// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package transformers

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
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
		if envTag := config.Datadog.GetString("env"); envTag != "" {
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
