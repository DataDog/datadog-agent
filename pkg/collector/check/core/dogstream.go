package core

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	log "github.com/cihub/seelog"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/dogstream"
	"github.com/DataDog/datadog-agent/pkg/dogstream/tailer"
)

// DogstreamCheck doesn't need additional fields
type DogstreamCheck struct {
	config dogstream.StreamEntry
	tailer *tailer.Tailer
}

func (c *DogstreamCheck) String() string {
	return "DogstreamCheck"
}

// Run executes the check
func (c *DogstreamCheck) Run() error {
	log.Infof("Running dogstream check")
	c.tailer.Run()
	return nil
}

// Configure the Dogstream check from YAML data
func (c *DogstreamCheck) Configure(data check.ConfigData) {
	c.config = dogstream.StreamEntry{}
	err := yaml.Unmarshal(data, &c.config)
	if err != nil {
		log.Error(err)
		return
	}

	var parsers []dogstream.Parser
	for _, parser := range c.config.Parser {
		dsp, err := dogstream.Load(parser)
		if err != nil {
			log.Error(err)
			return
		}
		parsers = append(parsers, dsp)
	}

	c.tailer = tailer.NewTailer()
	err = c.tailer.AddFile(c.config.File, parsers)
	if err != nil {
		log.Error(err)
		return
	}

	log.Infof("Configured Dogstream instance: %s", c.config.Name)
}

// InitSender does nothing for this check
func (c *DogstreamCheck) InitSender() {
}

// Interval returns 0 since this is a long-running check
func (c *DogstreamCheck) Interval() time.Duration {
	return 0
}

// ID FIXME: this should return a real identifier
func (c *DogstreamCheck) ID() string {
	return c.String()
}

// Stop stops the check
func (c *DogstreamCheck) Stop() {
	c.tailer.Stop()
}

func init() {
	RegisterCheck(
		"dogstream",
		&DogstreamCheck{},
	)
}
