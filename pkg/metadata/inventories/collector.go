package inventories

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery"

	"github.com/DataDog/datadog-agent/pkg/collector"

	md "github.com/DataDog/datadog-agent/pkg/metadata"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

var (
	metadataUpdatedC = make(chan interface{})
)

const (
	minSendInterval = 10 * time.Minute
	maxSendInterval = 5 * time.Minute
)

type inventoriesCollector struct {
	ac       getLoadedConfigsInterface
	coll     getAllInstanceIDsInterface
	lastSend time.Time
}

// Send collects the data needed and submits the payload
func (c inventoriesCollector) Send(s *serializer.Serializer) error {
	c.lastSend = time.Now()
	payload := GetPayload(c.ac, c.coll)
	if err := s.SendMetadata(payload); err != nil {
		return fmt.Errorf("unable to submit inventories payload, %s", err)
	}
	return nil
}

// Setup registers the inventories collector into the Scheduler and, if configured, schedules it
func Setup(sc *md.Scheduler, ac *autodiscovery.AutoConfig, coll *collector.Collector) error {
	ic := inventoriesCollector{
		ac:       ac,
		coll:     coll,
		lastSend: time.Now(),
	}
	md.RegisterCollector("inventories", ic)

	if err := sc.AddCollector("inventories", maxSendInterval); err != nil {
		return err
	}

	// This listens to the metadataUpdatedC signal to run the collector out of its regular interval
	go func() {
		for {
			<-metadataUpdatedC
			delay := minSendInterval - time.Since(ic.lastSend)
			if delay < 0 {
				delay = 0
			}
			sc.SendNow("inventories", delay)
		}
	}()

	return nil
}
