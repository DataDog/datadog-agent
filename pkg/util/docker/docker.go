package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/client"
)

// GetHostname queries Docker for the host name
func GetHostname() (string, error) {
	client, err := client.NewEnvClient()
	if err != nil {
		return "", err
	}

	info, err := client.Info(context.Background())
	if err != nil {
		return "", fmt.Errorf("unable to get Docker info: %s", err)
	}

	return info.Name, nil
}
