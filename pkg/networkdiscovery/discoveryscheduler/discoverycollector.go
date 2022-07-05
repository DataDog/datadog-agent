package discoveryscheduler

import (
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/discoverycollector"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DiscoveryScheduler TODO
type DiscoveryScheduler struct {
	flushInterval time.Duration
	sender        aggregator.Sender
	stopChan      chan struct{}
	hostname      string
	collector     *discoverycollector.DiscoveryCollector
}

// NewDiscoveryScheduler TODO
func NewDiscoveryScheduler(sender aggregator.Sender, config *config.NetworkDiscoveryConfig, hostname string) *DiscoveryScheduler {
	return &DiscoveryScheduler{
		flushInterval: time.Duration(config.MinCollectionInterval) * time.Second,
		sender:        sender,
		stopChan:      make(chan struct{}),
		hostname:      hostname,
		collector:     discoverycollector.NewDiscoveryCollector(sender, hostname),
	}
}

// Start will start the DiscoveryScheduler worker
func (ds *DiscoveryScheduler) Start() {
	log.Info("Flow Aggregator started")
	ds.schedulerLoop() // blocking call
}

// Stop will stop running DiscoveryScheduler
func (ds *DiscoveryScheduler) Stop() {
	close(ds.stopChan)
}

func (ds *DiscoveryScheduler) schedulerLoop() {
	var flushTicker <-chan time.Time

	// TODO: handle error when flush interval is <= 0
	flushTicker = time.NewTicker(ds.flushInterval).C

	for {
		select {
		// stop sequence
		case <-ds.stopChan:
			return
		// automatic flush sequence
		case <-flushTicker:
			ds.collect()
		}
	}
}

// Flush flushes the aggregator
func (ds *DiscoveryScheduler) collect() int {
	log.Info("collect network topology data")
	ds.collector.Collect()
	return 0
}
