package integration

import (
	"context"
	"io/ioutil"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/listeners"
)

const (
	redisImage         string = "redis:latest"
	redisContainerName string = "datadog-integration-redis"
)

func ResetContainers() {
	c, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}

	c.ContainerRemove(context.Background(), redisContainerName, types.ContainerRemoveOptions{Force: true})
}

func removeFromImage(image string) {
	c, err := client.NewEnvClient()
	ctx := context.Background()

	if err != nil {
		panic(err)
	}

	l, err := c.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		panic(err)
	}

	for _, ctr := range l {
		if ctr.Image == image {
			c.ContainerRemove(ctx, ctr.ID, types.ContainerRemoveOptions{Force: true})
		}
	}
}

// runFromImage runs a dead-simple container based on
// an image name passed to it, and returns its ID
func runFromImage(image string) (string, error) {
	c, err := client.NewEnvClient()
	ctx := context.Background()

	if err != nil {
		panic(err)
	}

	l, err := c.ImageList(ctx, types.ImageListOptions{})
	if err != nil {
		panic(err)
	}

	match := false
	for _, img := range l {
		if img.RepoTags[0] == image {
			match = true
			break
		}
	}

	if !match {
		resp, err := c.ImagePull(ctx, image, types.ImagePullOptions{})
		if err != nil {
			panic(err)
		}
		_, err = ioutil.ReadAll(resp) // Necessary for image pull to complete
		resp.Close()
		if err != nil {
			panic(err)
		}
	}

	resp, err := c.ContainerCreate(ctx, &container.Config{Image: image}, nil, nil, redisContainerName)
	if err != nil {
		panic(err)
	}

	if err := c.ContainerStart(ctx, redisContainerName, types.ContainerStartOptions{}); err != nil {
		panic(err)
	}

	return resp.ID, nil
}

// this tests getHostsFromPs, getPortsFromPs, and getTagsFromPs as well
func GetCurrentServicesTest(t *testing.T) {
	newSvc, delSvc := make(chan listeners.Service), make(chan listeners.Service)
	stop := make(chan bool)

	// make sure we don't block on writes to channels
	go func() {
		for {
			select {
			case <-newSvc:
			case <-delSvc:
			case <-stop:
			}
		}
	}()

	dl, err := listeners.NewDockerListener(stop, newSvc, delSvc)
	if err != nil {
		panic(err)
	}

	// make sure you stop running containers before running this test
	// this tests a blank run
	res := dl.GetCurrentServices()
	assert.Len(t, res, 0)

	// same test but with a simple redis container this time
	id, _ := runFromImage(redisImage)
	res = dl.GetCurrentServices()

	assert.Len(t, res, 1)
	assert.Equal(t, id, res[id].ID)
	assert.Equal(t, 1, len(res[id].Hosts))
	assert.Equal(t, "redis:latest", res[id].ConfigID)
	assert.Equal(t, 0, len(res[id].Tags))
}

// this tests processEvent, createService and removeService as well
func ListenTest(t *testing.T) {
	newSvc, delSvc := make(chan listeners.Service), make(chan listeners.Service)
	stop := make(chan bool)

	dl, err := listeners.NewDockerListener(stop, newSvc, delSvc)
	if err != nil {
		panic(err)
	}

	dl.Listen()

	id, _ := runFromImage(redisImage)
	createdSvc := <-newSvc

	assert.Equal(t, 1, len(dl.Services))
	assert.Equal(t, createdSvc, dl.Services[id])
	assert.Equal(t, id, createdSvc.ID)

	removeFromImage(redisImage)

	oldSvc := <-delSvc
	assert.Equal(t, 0, len(dl.Services))
	assert.Equal(t, oldSvc, createdSvc)
}

// actually run tests. Make sure to stop running containers on the machine before running this
// otherwise it will block
func TestDockerListener(t *testing.T) {
	t.Run("GetCurrentServicesTest", func(t *testing.T) { GetCurrentServicesTest(t) })
	ResetContainers()
	t.Run("Listen", func(t *testing.T) { ListenTest(t) })
	ResetContainers()
}
