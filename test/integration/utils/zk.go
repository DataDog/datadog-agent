package utils

import (
	"context"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

// StartZkContainer downloads the image and starts a Zk container
func StartZkContainer(imageName string, containerName string) (string, error) {
	healthCheck := &container.HealthConfig{
		Test:     []string{"CMD", "echo", "srvr", "|", "nc", "localhost", "2180", "|", "grep", "Mode"},
		Interval: 1 * time.Second,
		Timeout:  1 * time.Second,
	}

	containerConfig := &container.Config{
		Image:       imageName,
		Healthcheck: healthCheck,
	}

	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			"2181/tcp": []nat.PortBinding{{HostPort: "2181"}},
		},
	}

	cli, err := client.NewEnvClient()
	if err != nil {
		return "", err
	}
	ctx := context.Background()

	err = PullImage(imageName)
	if err != nil {
		return "", err
	}

	containerID, err := StartContainer(containerName, containerConfig, hostConfig)
	if err != nil {
		return "", err
	}

	// wait for the container to start
	err = waitFor(func() bool {
		res, err := cli.ContainerInspect(ctx, containerName)
		return err == nil && res.ContainerJSONBase.State.Health.Status == "healthy"
	}, 5*time.Second)

	return containerID, err
}
