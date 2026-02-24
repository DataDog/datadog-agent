// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build docker

package docker

import (
	"github.com/docker/docker/api/types/events"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	pkgcontainersimage "github.com/DataDog/datadog-agent/pkg/util/containers/image"
)

func getProcessorFilter(containerFilter workloadfilter.FilterBundle, store workloadmeta.Component) generic.ContainerFilter {
	// Reject all containers that are not run by Docker
	return generic.ANDContainerFilter{
		Filters: []generic.ContainerFilter{
			generic.RuntimeContainerFilter{Runtime: workloadmeta.ContainerRuntimeDocker},
			generic.LegacyContainerFilter{ContainerFilter: containerFilter, Store: store},
		},
	}
}

func getImageTags(imageName string) ([]string, error) {
	long, _, short, tag, err := pkgcontainersimage.SplitImageName(imageName)
	if err != nil {
		return nil, err
	}

	return []string{
		"docker_image:" + imageName,
		"image_name:" + long,
		"image_tag:" + tag,
		"short_image:" + short,
	}, nil
}

func isAlertTypeError(action events.Action) bool {
	return action == events.ActionOOM || action == events.ActionKill
}
