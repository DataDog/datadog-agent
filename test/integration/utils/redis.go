package utils

import (
	"context"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// StartRedisContainer starts a Redis container and waits for the healthcheck
func StartRedisContainer(imageName string, containerName string) (string, error) {
	healthCheck := &container.HealthConfig{
		Test:     []string{"CMD", "redis-cli", "ping"},
		Interval: 1 * time.Second,
		Timeout:  1 * time.Second,
	}

	cfg := &container.Config{
		Image:       imageName,
		Healthcheck: healthCheck,
	}
	id, err := StartContainer(containerName, cfg, nil)
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
