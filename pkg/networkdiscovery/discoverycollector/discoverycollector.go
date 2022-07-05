package discoverycollector

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DiscoveryCollector TODO
type DiscoveryCollector struct {
	flushInterval time.Duration
	sender        aggregator.Sender
	stopChan      chan struct{}
	hostname      string
}

// NewDiscoveryCollector TODO
func NewDiscoveryCollector(sender aggregator.Sender, config *config.NetworkDiscoveryConfig, hostname string) *DiscoveryCollector {
	return &DiscoveryCollector{
		flushInterval: time.Duration(config.MinCollectionInterval) * time.Second,
		sender:        sender,
		stopChan:      make(chan struct{}),
		hostname:      hostname,
	}
}

// Start will start the DiscoveryCollector worker
func (dc *DiscoveryCollector) Start() {
	log.Info("Flow Aggregator started")
	dc.collectorLoop() // blocking call
}

// Stop will stop running DiscoveryCollector
func (dc *DiscoveryCollector) Stop() {
	close(dc.stopChan)
}

func (dc *DiscoveryCollector) collectorLoop() {
	var flushTicker <-chan time.Time

	// TODO: handle error when dc.flushInterval is <= 0
	flushTicker = time.NewTicker(dc.flushInterval).C

	for {
		select {
		// stop sequence
		case <-dc.stopChan:
			return
		// automatic flush sequence
		case <-flushTicker:
			dc.collect()
		}
	}
}

// Flush flushes the aggregator
func (dc *DiscoveryCollector) collect() int {
	log.Info("collect network topology data")
	return 0
}
