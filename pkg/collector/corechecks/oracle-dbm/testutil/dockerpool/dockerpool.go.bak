// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dockerpool

import (
	"github.com/ory/dockertest"
	"github.com/ory/dockertest/docker"
	"github.com/sirupsen/logrus"
)

type TeardownFunc func()

// PoolConfig is used to define a basic container and the pool.
//
// Public fields define the required values to create a pool.
// Private fields are optional and can be applied with `func Option...()`
type PoolConfig struct {
	// Repository represents the image repository.
	Repository string
	// ImageTag represents the image tag.
	ImageTag string
	// Expiration sets the max amount of time to try and create a pool before exiting.
	Expiration uint
	// DockerHostConfig contains the container options related to starting a container.
	DockerHostConfig *docker.HostConfig

	// endpoint represents the docker endpoint. By default, this is an empty string which sets the pool's
	// default behavior to take the endpoint from the environment variable DOCKER_HOST and DOCKER_URL, or from
	// docker-machine if the environment variable DOCKER_MACHINE_NAME is set, or if neither is defined a
	// sensible default for the operating system you are on.
	endpoint string
	// env defines the image's environment variables
	env []string
	// exposedPorts defines ports to be exposed from the pool container.
	exposedPorts []string
}

// PoolOption defines an option for a docker pool.
type PoolOption func(*PoolConfig)

// OptionEndpoint sets the docker endpoint.
func OptionEndpoint(endpoint string) PoolOption {
	return func(pc *PoolConfig) {
		pc.endpoint = endpoint
	}
}

// OptionalEnvs applies a list of the image's environment variables.
func OptionEnvs(envs ...string) PoolOption {
	return func(pc *PoolConfig) {
		pc.env = envs
	}
}

// OptionalExposedPort applies a list of ports to be exposed from the container.
func OptionalExposedPorts(ports ...string) PoolOption {
	return func(pc *PoolConfig) {
		pc.exposedPorts = ports
	}
}

// CreatePool creates a new docker pool.
func CreatePool(pc *PoolConfig, options ...PoolOption) (*dockertest.Pool, *dockertest.Resource, TeardownFunc) {
	pc.endpoint = ""
	pc.env = nil
	pc.exposedPorts = nil

	for _, apply := range options {
		apply(pc)
	}

	pool, err := dockertest.NewPool(pc.endpoint)
	if err != nil {
		logrus.Fatalf("Failed to construct pool: %s", err)
	}
	if err := pool.Client.Ping(); err != nil {
		logrus.Fatalf("Failed to connect to docker: %s", err)
	}

	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository:   pc.Repository,
		Tag:          pc.ImageTag,
		Env:          pc.env,
		ExposedPorts: pc.exposedPorts,
	}, func(config *docker.HostConfig) {
		config.AutoRemove = pc.DockerHostConfig.AutoRemove
		config.RestartPolicy = pc.DockerHostConfig.RestartPolicy
		// extensible, update as needed
	})
	if err != nil {
		logrus.Fatalf("Failed to start resource: %s", err)
	}

	teardown := func() {
		logrus.Infof("Tearing down docker pool id=[%s]", resource.Container.ID)
		if err := pool.Purge(resource); err != nil {
			logrus.Errorf("Failed to purge pool resource: %s", err)
		}
	}

	return pool, resource, teardown
}
