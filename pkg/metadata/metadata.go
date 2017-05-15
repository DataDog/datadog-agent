package metadata

import (
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metadata/resources"
	"github.com/DataDog/datadog-agent/pkg/metadata/v5"
	log "github.com/cihub/seelog"
)

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
	fwd             *forwarder.Forwarder
	sendHostT       *time.Ticker
	sendExtHostT    *time.Ticker
	sendAgentCheckT *time.Ticker
	sendProcessesT  *time.Ticker
	apikey          string // the api key to use to send metadata
	hostname        string
	stop            chan bool
}

// NewCollector builds and returns a new Metadata Collector
func NewCollector(fwd *forwarder.Forwarder, apikey, hostname string) *Collector {
	collector := &Collector{
		fwd:             fwd,
		sendHostT:       time.NewTicker(config.Datadog.GetDuration("metadata_interval")),
		sendExtHostT:    time.NewTicker(config.Datadog.GetDuration("external_host_tags_interval")),
		sendAgentCheckT: time.NewTicker(config.Datadog.GetDuration("agent_checks_interval")),
		sendProcessesT:  time.NewTicker(config.Datadog.GetDuration("processes_interval")),
		apikey:          apikey,
		hostname:        hostname,
		stop:            make(chan bool),
	}

	collector.firstRun()
	go collector.loop()

	return collector
}

// Stop the metadata collector
func (c *Collector) Stop() {
	c.stop <- true
}

func (c *Collector) loop() {
	for {
		select {
		case <-c.sendHostT.C:
			c.sendHost()
		case <-c.sendExtHostT.C:
			c.sendExtHost()
		case <-c.sendAgentCheckT.C:
			c.sendAgentCheck()
		case <-c.sendProcessesT.C:
			c.sendProcesses()
		case <-c.stop:
			c.sendHostT.Stop()
			c.sendExtHostT.Stop()
			c.sendAgentCheckT.Stop()
			c.sendProcessesT.Stop()
			return
		}
	}
}

func (c *Collector) firstRun() {
	// Alway send host metadata at the first run
	c.sendHost()
}

func (c *Collector) sendHost() {
	payload := v5.GetPayload(c.hostname)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("unable to serialize host metadata payload, %s", err)
		return
	}

	err = c.fwd.SubmitV1Intake(c.apikey, &payloadBytes)
	if err != nil {
		log.Errorf("unable to submit host metadata payload to the forwarder, %s", err)
		return
	}

	log.Infof("Sent host metadata payload, size: %d bytes.", len(payloadBytes))
	log.Debugf("Sent host metadata payload, content: %v", string(payloadBytes))
}

func (c *Collector) sendExtHost() {
	// TODO
	log.Info("Sending external host tags metadata, NYI.")
}

func (c *Collector) sendAgentCheck() {
	// TODO
	log.Info("Sending agent check, NYI.")
}

func (c *Collector) sendProcesses() {

	payload := map[string]interface{}{
		"resources": resources.GetPayload(c.hostname),
	}
	payloadBytes, err := json.Marshal(payload)

	if err != nil {
		log.Errorf("unable to serialize processes metadata payload, %s", err)
		return
	}

	err = c.fwd.SubmitV1Intake(c.apikey, &payloadBytes)
	if err != nil {
		log.Errorf("unable to submit processes metadata payload to the forwarder, %s", err)
		return
	}

	log.Infof("Sent processes metadata payload, size: %d bytes.", len(payloadBytes))
	log.Debugf("Sent processes metadata payload, content: %v", string(payloadBytes))
}
