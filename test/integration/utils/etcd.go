package utils

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

// StartEtcdContainer downloads the image and starts an Etcd container
func StartEtcdContainer(imageName string, containerName string) error {
	cli, err := client.NewEnvClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	match, err := FindDockerImage(imageName)
	if err != nil {
		return err
	}

	if !match {
		fmt.Printf("Image %s not found, pulling\n", imageName)
		resp, err := cli.ImagePull(ctx, imageName, types.ImagePullOptions{})
		if err != nil {
			return err
		}
		_, err = ioutil.ReadAll(resp) // Necessary for image pull to complete
		resp.Close()
		if err != nil {
			return err
		}
	} else {
		fmt.Printf("Found image %s locally\n", imageName)
	}

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

	_, err = cli.ContainerCreate(ctx, containerConfig, hostConfig, nil, containerName)
	if err != nil {
		// containers already exists
		fmt.Fprintf(os.Stderr, "Error creating container %s: %s\n", containerName, err)
	}

	if err := cli.ContainerStart(ctx, containerName, types.ContainerStartOptions{}); err != nil {
		return err
	}

	// wait for the container to start
	err = waitFor(func() bool {
		res, err := cli.ContainerInspect(ctx, containerName)
		return err == nil && res.ContainerJSONBase.State.Health.Status == "healthy"
	}, 5*time.Second)

	return err
}
