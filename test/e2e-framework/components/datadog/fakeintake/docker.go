// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fakeintake

import (
	"net"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const hostPort = 30080

func NewLocalDockerFakeintake(e config.Env, resourceName string) (*Fakeintake, error) {
	return components.NewComponent(e, resourceName, func(comp *Fakeintake) error {

		_, err := docker.NewContainer(e.Ctx(), e.CommonNamer().ResourceName("local-docker-container"), &docker.ContainerArgs{
			Image: pulumi.String("public.ecr.aws/datadog/fakeintake:latest"),
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
