package main

import (
	_ "fmt"

	// project
	"github.com/DataDog/datadog-agent"

	// 3rd party
	"github.com/op/go-logging"
)

const (
	AGENT_VERSION = "6.0.0"
)

var log = logging.MustGetLogger("datadog-agent")

func main() {

	log.Infof("Starting Datadog Agent v%v", AGENT_VERSION)

	ddagent.StartLoop()

}
