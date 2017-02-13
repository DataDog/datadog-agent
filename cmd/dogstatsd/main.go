package main

import (
	// "fmt"
	"fmt"

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
	dogstatsd.RunServer(aggregatorInstance.GetChannel())
}
