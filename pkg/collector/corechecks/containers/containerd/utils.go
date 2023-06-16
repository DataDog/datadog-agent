// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func getProcessorFilter(legacyFilter *containers.Filter) generic.ContainerFilter {
	// Reject all containers that are not run by Containerd
	return generic.ANDContainerFilter{
		Filters: []generic.ContainerFilter{
			generic.RuntimeContainerFilter{Runtime: workloadmeta.ContainerRuntimeContainerd},
			generic.LegacyContainerFilter{OldFilter: legacyFilter},
		},
	}
}

func getImageTags(imageName string) []string {
	long, _, short, tag, err := containers.SplitImageName(imageName)
	if err != nil {
		return []string{fmt.Sprintf("image:%s", imageName)}
	}

	return []string{
		fmt.Sprintf("image:%s", imageName),
		fmt.Sprintf("image_name:%s", long),
		fmt.Sprintf("image_tag:%s", tag),
		fmt.Sprintf("short_image:%s", short),
	}
}
