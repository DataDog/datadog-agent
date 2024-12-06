// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build docker

package docker

import (
	"fmt"

	"github.com/docker/docker/api/types/events"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	pkgcontainersimage "github.com/DataDog/datadog-agent/pkg/util/containers/image"
)

func getProcessorFilter(legacyFilter *containers.Filter, store workloadmeta.Component) generic.ContainerFilter {
	// Reject all containers that are not run by Docker
	return generic.ANDContainerFilter{
		Filters: []generic.ContainerFilter{
			generic.RuntimeContainerFilter{Runtime: workloadmeta.ContainerRuntimeDocker},
			generic.LegacyContainerFilter{OldFilter: legacyFilter, Store: store},
		},
	}
}

func getImageTags(imageName string) ([]string, error) {
	long, _, short, tag, err := pkgcontainersimage.SplitImageName(imageName)
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

func isAlertTypeError(action events.Action) bool {
	return action == events.ActionOOM || action == events.ActionKill
}
