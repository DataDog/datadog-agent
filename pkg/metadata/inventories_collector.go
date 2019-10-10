package metadata

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"

	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"

	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util"
)

const (
	minSendInterval = 5 * time.Minute
	maxSendInterval = 10 * time.Minute
)

type schedulerInterface interface {
	AddCollector(name string, interval time.Duration) error
	SendNow(name string, delay time.Duration)
}

type autoConfigInterface interface {
	GetLoadedConfigs() map[string]integration.Config
}

type collectorInterface interface {
	GetAllInstanceIDs(checkName string) []check.ID
}

type inventoriesCollector struct {
	ac   autoConfigInterface
	coll collectorInterface
	sc   schedulerInterface
}

// Send collects the data needed and submits the payload
func (c inventoriesCollector) Send(s *serializer.Serializer) error {
	hostname, err := util.GetHostname()
	if err != nil {
		return fmt.Errorf("unable to submit inventories metadata payload, no hostname: %s", err)
	}

	payload := inventories.GetPayload(hostname, c.ac, c.coll)

	if err := s.SendMetadata(payload); err != nil {
		return fmt.Errorf("unable to submit inventories payload, %s", err)
	}
	return nil
}

// Send collects the data needed and submits the payload
func (c inventoriesCollector) Init() error {
	return inventories.StartSendNowRoutine(c.sc, minSendInterval)
}

// SetupInventories registers the inventories collector into the Scheduler and, if configured, schedules it
func SetupInventories(sc schedulerInterface, ac autoConfigInterface, coll collectorInterface) error {
	ic := inventoriesCollector{
		ac:   ac,
		coll: coll,
		sc:   sc,
	}
	RegisterCollector("inventories", ic)

	if err := sc.AddCollector("inventories", maxSendInterval); err != nil {
		return err
	}

	return nil
}
