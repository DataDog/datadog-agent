package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
)

func main() {
	config.Datadog.AddConfigPath(".")
	err := config.Datadog.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("unable to load Datadog config file: %s", err))
	}

	// for now we handle only one key and one domain
	keysPerDomain := map[string][]string{
		config.Datadog.GetString("dd_url"): {
			config.Datadog.GetString("api_key"),
		},
	}
	f := forwarder.NewForwarder(keysPerDomain)
	f.Start()

	aggregatorInstance := aggregator.InitAggregator(f)
	statsd := dogstatsd.NewServer(aggregatorInstance.GetChannel())

	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// Block here until we receive the interrupt signal
	<-signalCh

	statsd.Stop()
	log.Info("See ya!")
	log.Flush()
	os.Exit(0)
}
