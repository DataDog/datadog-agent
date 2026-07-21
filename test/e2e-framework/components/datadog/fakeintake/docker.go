// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fakeintake

import (
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const hostPort = 30080

// DefaultImageURL is the fakeintake image used when no DockerOption overrides it.
const DefaultImageURL = "public.ecr.aws/datadog/fakeintake:latest"

// dockerParams configures the local Docker-based fakeintake container. It mirrors the
// subset of scenarios/aws/fakeintake.Params that's meaningful for a single local
// container (LoadBalancer/CPU don't apply here); it's defined locally rather than
// reusing that package's type to avoid an import cycle (scenarios/aws/fakeintake
// imports this package).
type dockerParams struct {
	imageURL        string
	memory          int
	dddevForwarding bool
	retentionPeriod string
}

// DockerOption configures NewLocalDockerFakeintake.
type DockerOption func(*dockerParams)

// WithDockerImageURL overrides the fakeintake container image.
func WithDockerImageURL(imageURL string) DockerOption {
	return func(p *dockerParams) { p.imageURL = imageURL }
}

// WithDockerMemory sets the container's GOMEMLIMIT, in MiB.
func WithDockerMemory(memoryMiB int) DockerOption {
	return func(p *dockerParams) { p.memory = memoryMiB }
}

// WithDockerDDDevForwarding enables forwarding payloads to the dddev org account.
func WithDockerDDDevForwarding() DockerOption {
	return func(p *dockerParams) { p.dddevForwarding = true }
}

// WithDockerRetentionPeriod sets how long the fakeintake retains received payloads.
func WithDockerRetentionPeriod(retentionPeriod string) DockerOption {
	return func(p *dockerParams) { p.retentionPeriod = retentionPeriod }
}

// NewLocalDockerFakeintake deploys a fakeintake container to the local Docker daemon.
func NewLocalDockerFakeintake(e config.Env, resourceName string, opts ...DockerOption) (*Fakeintake, error) {
	params := dockerParams{imageURL: DefaultImageURL}
	for _, opt := range opts {
		opt(&params)
	}

	return components.NewComponent(e, resourceName, func(comp *Fakeintake) error {
		command := []string{}
		if params.dddevForwarding {
			command = append(command, "--dddev-forward")
		}
		if params.retentionPeriod != "" {
			command = append(command, "-retention-period="+params.retentionPeriod)
		}
		command = append(command, "--rc-key-data="+DefaultRCSigningKeySeed)

		envs := []string{"STORAGE_DRIVER=memory"}
		if params.memory > 0 {
			envs = append(envs, fmt.Sprintf("GOMEMLIMIT=%dMiB", params.memory))
		}

		_, err := docker.NewContainer(e.Ctx(), e.CommonNamer().ResourceName("local-docker-container"), &docker.ContainerArgs{
			Image:   pulumi.String(params.imageURL),
			Command: pulumi.ToStringArray(command),
			Envs:    pulumi.ToStringArray(envs),
			Ports: docker.ContainerPortArray{
				&docker.ContainerPortArgs{
					Internal: pulumi.Int(80),
					External: pulumi.Int(hostPort),
				},
			},
		}, e.WithProviders(config.ProviderDocker))
		if err != nil {
			return err
		}

		localIP, err := getLocalIP()
		if err != nil {
			return err
		}

		comp.Host = pulumi.Sprintf("%s", localIP.String())
		comp.Port = pulumi.Int(hostPort).ToIntOutput()
		comp.Scheme = pulumi.Sprintf("%s", "http")
		comp.URL = pulumi.Sprintf("%s://%s:%d", comp.Scheme, comp.Host, comp.Port)

		return nil
	})
}

func getLocalIP() (net.IP, error) {
	// Open a connection to an external valid URL to
	// get the local address from the connection instance
	// The URL does not need to exist
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP, nil
}
