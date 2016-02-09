package main

import (
	"time"

	// project
	"github.com/DataDog/datadog-agent"
	"github.com/DataDog/datadog-agent/aggregator"
	"github.com/DataDog/datadog-agent/checks/system"

	// 3rd party
	"github.com/op/go-logging"
)

const (
	AGENT_VERSION = "6.0.0"
)

var log = logging.MustGetLogger("datadog-agent")

func main() {

	log.Infof("Starting Datadog Agent v%v", AGENT_VERSION)

	// Start python check loop
	go ddagent.StartLoop()

	// Run memory check
	check := system.MemoryCheck{
		Name: "memory",
	}

	agg := new(aggregator.DefaultAggregator)

	ticker := time.NewTicker(time.Millisecond * 10000)
	for t := range ticker.C {
		check.Check(agg)
		log.Infof("Tick at %v", t)
	}

}
