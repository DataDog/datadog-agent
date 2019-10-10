package inventories

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/metadata"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

var (
	metadataUpdatedC = make(chan interface{})
)

var (
	// For testing purposes
	timeNow   = time.Now
	timeSince = time.Since
)

const (
	minSendInterval = 10 * time.Minute
	maxSendInterval = 5 * time.Minute
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
	ac       autoConfigInterface
	coll     collectorInterface
	lastSend time.Time
}

// Send collects the data needed and submits the payload
func (c inventoriesCollector) Send(s *serializer.Serializer) error {
	c.lastSend = timeNow()

	hostname, err := util.GetHostname()
	if err != nil {
		return fmt.Errorf("unable to submit inventories metadata payload, no hostname: %s", err)
	}

	payload := GetPayload(hostname, c.ac, c.coll)

	if err := s.SendMetadata(payload); err != nil {
		return fmt.Errorf("unable to submit inventories payload, %s", err)
	}
	return nil
}

// Setup registers the inventories collector into the Scheduler and, if configured, schedules it
func Setup(sc schedulerInterface, ac autoConfigInterface, coll collectorInterface) error {
	ic := inventoriesCollector{
		ac:       ac,
		coll:     coll,
		lastSend: timeNow(),
	}
	metadata.RegisterCollector("inventories", ic)

	if err := sc.AddCollector("inventories", maxSendInterval); err != nil {
		return err
	}

	// This listens to the metadataUpdatedC signal to run the collector out of its regular interval
	go func() {
		for {
			<-metadataUpdatedC
			delay := minSendInterval - timeSince(ic.lastSend)
			if delay < 0 {
				delay = 0
			}
			sc.SendNow("inventories", delay)
		}
	}()

	return nil
}
