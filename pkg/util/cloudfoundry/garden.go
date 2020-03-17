package cloudfoundry

import (
	"sync"
	"time"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden/client"
	"code.cloudfoundry.org/garden/client/connection"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

const (
	// ContainerNameTagKey tag key for container tags
	ContainerNameTagKey = "container_name"
	// AppInstanceGUIDTagKey tag key for container tags
	AppInstanceGUIDTagKey = "app_instance_guid"
	// AppNameTagKey tag key for container tags
	AppNameTagKey = "app_name"
	// AppInstanceIndexTagKey tag key for container tags
	AppInstanceIndexTagKey = "app_instance_index"
	// AppGUIDTagKey tag key for container tags
	AppGUIDTagKey = "app_guid"
)

var (
	globalGardenUtil     *GardenUtil
	globalGardenUtilLock sync.Mutex
)

// GardenUtilInterface describes a wrapper for collecting local garden containers
type GardenUtilInterface interface {
	GetGardenContainers() ([]garden.Container, error)
	ListContainers() ([]*containers.Container, error)
	UpdateContainerMetrics(cList []*containers.Container) error
}

// GardenUtil wraps interactions with a local garden API.
type GardenUtil struct {
	retrier retry.Retrier
	cli     client.Client
}

// GetGardenUtil returns the global instance of the garden utility and initializes it first if needed
func GetGardenUtil() (*GardenUtil, error) {
	globalGardenUtilLock.Lock()
	defer globalGardenUtilLock.Unlock()
	network := config.Datadog.GetString("cloud_foundry_garden.listen_network")
	address := config.Datadog.GetString("cloud_foundry_garden.listen_address")
	if globalGardenUtil == nil {
		globalGardenUtil = &GardenUtil{
			cli: client.New(connection.New(network, address)),
		}
		globalGardenUtil.retrier.SetupRetrier(&retry.Config{
			Name:          "gardenUtil",
			AttemptMethod: globalGardenUtil.cli.Ping,
			Strategy:      retry.RetryCount,
			RetryCount:    10,
			RetryDelay:    30 * time.Second,
		})
	}
	if err := globalGardenUtil.retrier.TriggerRetry(); err != nil {
		log.Debugf("Could not initiate connection to garden server %s using network %s: %s", address, network, err)
		return nil, err
	}
	return globalGardenUtil, nil
}
