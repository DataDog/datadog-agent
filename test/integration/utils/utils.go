package utils

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"runtime"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// FindDockerImage returns whether an image name was found in the local registry
func FindDockerImage(imageName string) (bool, error) {
	cli, err := client.NewEnvClient()
	if err != nil {
		return false, err
	}

	ctx := context.Background()

	l, err := cli.ImageList(ctx, types.ImageListOptions{})
	if err != nil {
		return false, err
	}

	for _, img := range l {
		for _, tag := range img.RepoTags {
			if tag == imageName {
				return true, nil
			}
		}
	}

	return false, nil
}

func PullImage(imageName string) error {
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

		var resp io.ReadCloser
		var err error
		resp, err = cli.ImagePull(ctx, imageName, types.ImagePullOptions{})
		if err != nil {
			giveup := false
			if err.Error() == "repository name must be canonical" {
				// make the image name canonical
				cname := "docker.io/library/" + imageName
				fmt.Println("Trying with canonical name: " + cname)
				resp, err = cli.ImagePull(ctx, cname, types.ImagePullOptions{})
				if err != nil {
					// bail out
					giveup = true
				}
			}

			if giveup {
				return err
			}
		}

		_, err = ioutil.ReadAll(resp) // Necessary for image pull to complete
		resp.Close()
		if err != nil {
			return err
		}
	} else {
		fmt.Printf("Found image %s locally\n", imageName)
	}

	return nil
}

// StartContainer with given image, name and configuration
func StartContainer(containerName string, containerConfig *container.Config,
	hostConfig *container.HostConfig) (string, error) {

	cli, err := client.NewEnvClient()
	if err != nil {
		return "", err
	}
	ctx := context.Background()

	resp, err := cli.ContainerCreate(ctx, containerConfig, hostConfig, nil, containerName)
	if err != nil {
		// containers already exists
		fmt.Fprintf(os.Stderr, "Error creating container %s: %s\n", containerName, err)
	}

	if err := cli.ContainerStart(ctx, containerName, types.ContainerStartOptions{}); err != nil {
		return "", err
	}

	return string(resp.ID), nil
}

// GetContainerIP inspects the container and returns its IP address
func GetContainerIP(containerID string) (string, error) {
	// docker doesn't support bridge network mode on OSX, fallback to host
	if runtime.GOOS == "darwin" {
		return "127.0.0.1", nil
	}

	cli, err := client.NewEnvClient()
	if err != nil {
		return "", err
	}
	ctx := context.Background()

	resp, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error inspecting container %s: %s\n", containerID, err)
		return "", err
	}

	for _, network := range resp.NetworkSettings.Networks {
		// return first network's IP
		return network.IPAddress, nil
	}

	// No network found
	return "", fmt.Errorf("no IP found for container %s", containerID)
}
