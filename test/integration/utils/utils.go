package utils

import (
	"context"

	"github.com/docker/docker/api/types"
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
