// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build docker

package docker

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func getProcessorFilter(legacyFilter *containers.Filter) generic.ContainerFilter {
	// Reject all containers that are not run by Docker
	return generic.ANDContainerFilter{
		Filters: []generic.ContainerFilter{
			generic.RuntimeContainerFilter{Runtime: workloadmeta.ContainerRuntimeDocker},
			generic.LegacyContainerFilter{OldFilter: legacyFilter},
		},
	}
}

func getImageTagsFromContainer(taggerEntityID string, resolvedImageName string, isContainerExcluded bool) ([]string, error) {
	if isContainerExcluded {
		return getImageTags(resolvedImageName)
	}

	containerTags, err := tagger.Tag(taggerEntityID, collectors.LowCardinality)
	if err != nil {
		return nil, err
	}

	return containerTags, nil
}

func getImageTags(imageName string) ([]string, error) {
	long, _, short, tag, err := containers.SplitImageName(imageName)
	if err != nil {
		return nil, err
	}

	return []string{
		fmt.Sprintf("docker_image:%s", imageName),
		fmt.Sprintf("image_name:%s", long),
		fmt.Sprintf("image_tag:%s", tag),
		fmt.Sprintf("short_image:%s", short),
	}, nil
}

const (
	eventActionOOM  = "oom"
	eventActionKill = "kill"
)

func isAlertTypeError(action string) bool {
	return action == eventActionOOM || action == eventActionKill
}
