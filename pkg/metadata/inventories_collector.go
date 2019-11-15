package metadata

import (
	"expvar"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util"
)

const (
	minSendInterval = 5 * time.Minute
	maxSendInterval = 10 * time.Minute
)

type inventoriesCollector struct {
	ac   inventories.AutoConfigInterface
	coll inventories.CollectorInterface
	sc   *Scheduler
}

var (
	expvarPayload func() interface{}
)

func (c inventoriesCollector) createPayload() (*inventories.Payload, error) {
	hostname, err := util.GetHostname()
	if err != nil {
		return nil, fmt.Errorf("unable to submit inventories metadata payload, no hostname: %s", err)
	}

	return inventories.GetPayload(hostname, c.ac, c.coll), nil
}

// Send collects the data needed and submits the payload
func (c inventoriesCollector) Send(s *serializer.Serializer) error {
	payload, err := c.createPayload()
	if err != nil {
		return err
	}

	if err := s.SendMetadata(payload); err != nil {
		return fmt.Errorf("unable to submit inventories payload, %s", err)
	}
	return nil
}

// Init initializes the inventory metadata collection
func (c inventoriesCollector) Init() error {
	return inventories.StartMetadataUpdatedGoroutine(c.sc, minSendInterval)
}

// SetupInventories registers the inventories collector into the Scheduler and, if configured, schedules it
func SetupInventories(sc *Scheduler, ac inventories.AutoConfigInterface, coll inventories.CollectorInterface) error {
	ic := inventoriesCollector{
		ac:   ac,
		coll: coll,
		sc:   sc,
	}
	RegisterCollector("inventories", ic)

	if err := sc.AddCollector("inventories", maxSendInterval); err != nil {
		return err
	}

	expvar.Publish("inventories", expvar.Func(func() interface{} {
		log.Debugf("Creating inventory payload for expvar")
		p, err := ic.createPayload()
		if err != nil {
			log.Errorf("Could not create inventory payload for expvar: %s", err)
			return nil
		}
		return p
	}))
	return nil
}
