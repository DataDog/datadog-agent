// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package tailerfactory

import (
	"errors"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util/containersorpods"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// defaultSourceAndService gets the default "source" and "service" values for
// the given LogSource.  It always returns a result, logging any errors it
// encounters along the way
func (tf *factory) defaultSourceAndService(source *sources.LogSource, logWhat containersorpods.LogWhat) (sourceName, serviceName string) {
	getContainer := func(containerID string) (*workloadmeta.Container, error) {
		wmeta, ok := tf.workloadmetaStore.Get()
		if !ok {
			return nil, errors.New("workloadmeta store is not initialized")
		}
		return wmeta.GetContainer(containerID)
	}

	getServiceNameFromTags := func(containerID, containerName string) string {
		return util.ServiceNameFromTags(
			containerName,
			types.NewEntityID(types.ContainerID, containerID),
			tf.tagger.Standard,
		)
	}

	return defaultSourceAndServiceInner(source, logWhat,
		getContainer, getServiceNameFromTags)
}

// defaultSourceAndServiceInner implements defaultSourceAndService with function
// callbacks to allow testing.  Its behavior differs slightly depending whether
// we are logging containers or pods.
func defaultSourceAndServiceInner(
	source *sources.LogSource,
	logWhat containersorpods.LogWhat,
	getContainer func(containerID string) (*workloadmeta.Container, error),
	getServiceNameFromTags func(containerID, containerName string) string,
) (sourceName, serviceName string) {
	containerID := source.Config.Identifier

	if source.Config.Source != "" {
		sourceName = source.Config.Source
	}

	if source.Config.Service != "" {
		serviceName = source.Config.Service
	}

	if serviceName != "" && sourceName != "" {
		return
	}

	// determine the default service based on a "service:.." tag in the tagger

	container, err := getContainer(containerID)
	if err != nil {
		log.Warnf("Could not inspect container %s: %v", containerID, err)
		return
	}

	if serviceName == "" {
		serviceName = getServiceNameFromTags(containerID, container.Name)
	}

	if serviceName != "" && sourceName != "" {
		return
	}

	// determine the "short name" of the image, which is the final default for both values
	shortName := container.Image.ShortName

	// on kubernetes, if the short name is not available, default to
	// "kubernetes"; otherwise the empty string is OK.
	if logWhat == containersorpods.LogContainers {
		if shortName == "" {
			shortName = "kubernetes"
		}
	}

	if serviceName == "" {
		serviceName = shortName
	}

	if sourceName == "" {
		sourceName = shortName
	}

	return
}
