// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package listeners

import (
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

const (
	newIdentifierLabel    = "com.datadoghq.ad.check.id"
	legacyIdentifierLabel = "com.datadoghq.sd.check.id"
)

// ComputeContainerServiceIDs takes a container ID, an image (resolved to an actual name) and labels
// and computes the service IDs for this container service.
func ComputeContainerServiceIDs(cid string, image string, labels map[string]string) []string {
	// check for an identifier label
	if l, found := labels[newIdentifierLabel]; found {
		return []string{l}
	}
	if l, found := labels[legacyIdentifierLabel]; found {
		log.Warnf("found legacy %s label for %s, please use the new name %s",
			legacyIdentifierLabel, cid, newIdentifierLabel)
		return []string{l}
	}

	var ids []string

	// add the container ID for templates in labels/annotations
	ids = append(ids, docker.ContainerIDToEntityName(cid))

	// add the image names (long then short if different)
	long, short, _, err := docker.SplitImageName(image)
	if err != nil {
		log.Warnf("error while spliting image name: %s", err)
	}
	if len(long) > 0 {
		ids = append(ids, long)
	}
	if len(short) > 0 && short != long {
		ids = append(ids, short)
	}

	return ids
}
