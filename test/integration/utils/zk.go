package utils

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

// StartZkContainer downloads the image and starts a Zk container
func StartZkContainer(imageName string, containerName string) {
	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	l, err := cli.ImageList(ctx, types.ImageListOptions{})
	if err != nil {
		panic(err)
	}

	match := false
	for _, img := range l {
		if img.RepoTags[0] == imageName {
			fmt.Printf("Found image %s locally\n", imageName)
			match = true
			break
		}
	}

	if !match {
		fmt.Printf("Image %s not found, pulling\n", imageName)
		resp, err := cli.ImagePull(ctx, imageName, types.ImagePullOptions{})
		if err != nil {
			panic(err)
		}
		_, err = ioutil.ReadAll(resp) // Necessary for image pull to complete
		resp.Close()
		if err != nil {
			panic(err)
		}
	}

	containerConfig := &container.Config{
		Image: imageName,
	}

	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			"2181/tcp": []nat.PortBinding{nat.PortBinding{HostPort: "2181"}},
		},
	}

	_, err = cli.ContainerCreate(ctx, containerConfig, hostConfig, nil, containerName)
	if err != nil {
		// containers already exists
		fmt.Fprintf(os.Stderr, "Error creating container %s: %s\n", containerName, err)
	}

	if err := cli.ContainerStart(ctx, containerName, types.ContainerStartOptions{}); err != nil {
		panic(err)
	}
}
