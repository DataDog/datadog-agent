// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/versions"
	"github.com/docker/docker/client"
)

// Supported versions of the Docker API
const (
	minVersion = "1.18"
	maxVersion = "1.25"
)

// NewClient returns a new Docker client with the right API version to communicate with the docker server
func NewClient() (*client.Client, error) {
	client, err := client.NewEnvClient()
	if err != nil {
		return nil, err
	}
	serverVersion, err := getServerAPIVersion(client)
	if err != nil {
		return nil, err
	}
	clientVersion, err := computeClientAPIVersion(serverVersion)
	if err != nil {
		return nil, err
	}
	client.UpdateClientVersion(clientVersion)
	return client, nil
}

// serverAPIVersion returns the latest version of the docker API supported by the docker server
func getServerAPIVersion(client *client.Client) (string, error) {
	// hit unversioned API first to be able to communicate with the backend
	client.UpdateClientVersion("")
	v, err := client.ServerVersion(context.Background())
	if err != nil {
		return "", err
	}
	return v.APIVersion, nil
}

// computeAPIVersion returns the version of the API that the docker client should use to be able to communicate with the server
func computeClientAPIVersion(serverVersion string) (string, error) {
	if versions.LessThan(serverVersion, minVersion) {
		return "", fmt.Errorf("Docker API versions prior to %s are not supported by logs-agent, the current version is %s", minVersion, serverVersion)
	}
	if versions.LessThan(serverVersion, maxVersion) {
		return serverVersion, nil
	}
	return maxVersion, nil
}

// GetContainer returns the container matching id,
// if it does not exist, returns an error.
func GetContainer(client *client.Client, id string) (types.Container, error) {
	args := filters.NewArgs()
	args.Add("id", id)
	// TODO(achntrl): Call dockerUtil inspect instead
	containers, err := client.ContainerList(context.Background(), types.ContainerListOptions{
		Filters: args,
	})
	if err != nil {
		return types.Container{}, err
	}
	if len(containers) != 1 {
		return types.Container{}, fmt.Errorf("no container matches id: %v", id)
	}
	return containers[0], nil
}
