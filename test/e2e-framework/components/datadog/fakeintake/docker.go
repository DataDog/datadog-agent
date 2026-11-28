// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fakeintake

import (
	"net"

	"github.com/pulumi/pulumi-docker/sdk/v5/go/docker"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
)

const hostPort = 30080

func NewLocalDockerFakeintake(e config.Env, resourceName string) (*Fakeintake, error) {
	return components.NewComponent(e, resourceName, func(comp *Fakeintake) error {

		_, err := docker.NewContainer(e.Ctx(), e.CommonNamer().ResourceName("local-docker-container"), &docker.ContainerArgs{
			Image:   pulumi.String(ImageURL(e.DatadogPublicRegistry() + "/fakeintake")),
			Command: pulumi.ToStringArray([]string{"--rc-key-data=" + DefaultRCSigningKeySeed}),
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

// NewLocalKindFakeintake creates a fakeintake container for a local kind cluster, with its port
// published on 127.0.0.1 only. It returns two components for the same container: inCluster, for
// the workloads in the cluster, which reach it by name on kind's Docker network, and onHost, for
// the test client, which reaches it on 127.0.0.1.
func NewLocalKindFakeintake(e config.Env, resourceName string, kindCluster pulumi.Resource) (inCluster *Fakeintake, onHost *Fakeintake, err error) {
	inCluster, err = components.NewComponent(e, resourceName, func(comp *Fakeintake) error {
		container, err := docker.NewContainer(e.Ctx(), e.CommonNamer().ResourceName("local-docker-container"), &docker.ContainerArgs{
			Image:   pulumi.String(ImageURL(e.DatadogPublicRegistry() + "/fakeintake")),
			Command: pulumi.ToStringArray([]string{"--rc-key-data=" + DefaultRCSigningKeySeed}),
			Ports: docker.ContainerPortArray{
				&docker.ContainerPortArgs{
					Internal: pulumi.Int(80),
					External: pulumi.Int(hostPort),
					Ip:       pulumi.String("127.0.0.1"),
				},
			},
			NetworksAdvanced: docker.ContainerNetworksAdvancedArray{
				&docker.ContainerNetworksAdvancedArgs{Name: pulumi.String("kind")},
			},
		},
			e.WithProviders(config.ProviderDocker),
			// kind creates its network with the cluster.
			pulumi.DependsOn([]pulumi.Resource{kindCluster}),
			// The new container can't publish the host port until the old one releases it.
			pulumi.DeleteBeforeReplace(true),
		)
		if err != nil {
			return err
		}

		comp.Host = container.Name
		comp.Port = pulumi.Int(80).ToIntOutput()
		comp.Scheme = pulumi.String("http").ToStringOutput()
		comp.URL = pulumi.Sprintf("%s://%s:%d", comp.Scheme, comp.Host, comp.Port)

		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	onHost, err = components.NewComponent(e, resourceName+"-host", func(comp *Fakeintake) error {
		comp.Host = pulumi.String("127.0.0.1").ToStringOutput()
		comp.Port = pulumi.Int(hostPort).ToIntOutput()
		comp.Scheme = pulumi.String("http").ToStringOutput()
		comp.URL = pulumi.Sprintf("%s://%s:%d", comp.Scheme, comp.Host, comp.Port)

		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return inCluster, onHost, nil
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
