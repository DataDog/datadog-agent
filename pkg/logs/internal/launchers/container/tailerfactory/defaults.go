// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package tailerfactory

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/util"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util/containersorpods"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	dockerutilPkg "github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/docker/docker/api/types"
)

// defaultSourceAndService gets the default "source" and "service" values for
// the given LogSource.  It always returns a result, logging any errors it
// encounters along the way
func (tf *factory) defaultSourceAndService(source *sources.LogSource, logWhat containersorpods.LogWhat) (sourceName, serviceName string) {
	// inspectContainer
	inspectContainer := func(containerID string) (types.ContainerJSON, error) {
		du, err := tf.getDockerUtil()
		if err != nil {
			return types.ContainerJSON{}, err
		}

		return du.Inspect(context.Background(), containerID, false)
	}

	// getServiceNameFromTags
	getServiceNameFromTags := func(containerID, containerName string) string {
		return util.ServiceNameFromTags(
			containerName,
			dockerutilPkg.ContainerIDToTaggerEntityName(containerID))
	}

	// resolveImageName
	resolveImageName := func(imageName string) (string, error) {
		du, err := tf.getDockerUtil()
		if err != nil {
			return "", err
		}

		return du.ResolveImageName(context.Background(), imageName)
	}

	return defaultSourceAndServiceInner(source, logWhat,
		inspectContainer, getServiceNameFromTags, resolveImageName)
}

// defaultSourceAndServiceInner implements defaultSourceAndService with function
// callbacks to allow testing.  Its behavior differs slightly depending whether
// we are logging containers or pods.
func defaultSourceAndServiceInner(
	source *sources.LogSource,
	logWhat containersorpods.LogWhat,
	inspectContainer func(containerID string) (types.ContainerJSON, error),
	getServiceNameFromTags func(containerID, containerName string) string,
	resolveImageName func(imageName string) (string, error),
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

	containerJSON, err := inspectContainer(containerID)
	if err != nil {
		log.Warnf("Could not inspect container %s: %v", containerID, err)
		return
	}

	if serviceName == "" {
		serviceName = getServiceNameFromTags(containerID, containerJSON.Name)
	}

	if serviceName != "" && sourceName != "" {
		return
	}

	// determine the "short name" of the image, which is the final default for both values

	imageName, err := resolveImageName(containerJSON.Image)
	if err != nil {
		log.Warnf("Could not resolve image name %s: %v", containerJSON.Image, err)
		return
	}

	var shortName string
	_, shortName, _, err = containers.SplitImageName(imageName)
	if err != nil {
		log.Warnf("Could not parse image name %s: %v", imageName, err)
		return
	}

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
