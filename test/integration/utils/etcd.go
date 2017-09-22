package utils

import (
	"context"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

// StartEtcdContainer starts an Etcd container and waits for the healthcheck
func StartEtcdContainer(imageName string, containerName string) (string, error) {
	healthCheck := &container.HealthConfig{
		Test:     []string{"CMD", "wget", "--spider", "http://localhost:2379/health"},
		Interval: 1 * time.Second,
		Timeout:  1 * time.Second,
	}

	containerConfig := &container.Config{
		Image: imageName,
		Cmd: []string{
			"/usr/local/bin/etcd",
			"-advertise-client-urls", "http://127.0.0.1:2379",
			"-listen-client-urls", "http://0.0.0.0:2379",
		},
		Healthcheck: healthCheck,
	}

	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			"2379/tcp": []nat.PortBinding{nat.PortBinding{HostPort: "2379"}},
		},
	}

	err := PullImage(imageName)
	if err != nil {
		return "", err
	}

	id, err := StartContainer(containerName, containerConfig, hostConfig)
	if err != nil {
		return "", err
	}

	cli, err := client.NewEnvClient()
	if err != nil {
		return "", err
	}
	ctx := context.Background()

	// wait for the container to start
	err = waitFor(func() bool {
		res, err := cli.ContainerInspect(ctx, containerName)
		return err == nil && res.ContainerJSONBase.State.Health.Status == "healthy"
	}, 5*time.Second)

	return id, err
}
