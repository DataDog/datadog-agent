package metadata

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	log "github.com/cihub/seelog"
)

// Catalog keeps track of Go checks by name
var catalog = make(map[string]Provider)

func init() {
	// define defaults for the Agent
	config.Datadog.SetDefault("metadata_interval", 4*time.Hour)
	config.Datadog.SetDefault("external_host_tags_interval", 5*time.Minute)
	config.Datadog.SetDefault("agent_checks_interval", 10*time.Minute)
	config.Datadog.SetDefault("processes_interval", time.Minute)
}

// Collector takes care of sending metadata at specific
// time intervals
type Collector struct {
	fwd      forwarder.Forwarder
	apikey   string // the api key to use to send metadata
	hostname string
	tickers  []*time.Ticker
}

// NewCollector builds and returns a new Metadata Collector
func NewCollector(fwd forwarder.Forwarder, apikey, hostname string) *Collector {
	collector := &Collector{
		fwd:      fwd,
		apikey:   apikey,
		hostname: hostname,
	}

	err := collector.firstRun()
	if err != nil {
		log.Errorf("Unable to send host metadata at first run: %v", err)
	}

	return collector
}

// Stop the metadata collector
func (c *Collector) Stop() {
	for _, t := range c.tickers {
		t.Stop()
	}
}

// AddProvider TODO docstring
func (c *Collector) AddProvider(name string, interval time.Duration) error {
	p, found := catalog[name]
	if !found {
		return fmt.Errorf("Unable to find metadata provider: %s", name)
	}

	ticker := time.NewTicker(interval)
	go func() {
		for _ = range ticker.C {
			p.Send(c.apikey, c.fwd)
		}
	}()
	c.tickers = append(c.tickers, ticker)

	return nil
}

// Always send host metadata at the first run
func (c *Collector) firstRun() error {
	p, found := catalog["host"]
	if !found {
		panic("Unable to find 'host' metadata provider in the catalog!")
	}
	return p.Send(c.apikey, c.fwd)
}
