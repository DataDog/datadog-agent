package main

import (
	// "fmt"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
)

func main() {
	config.Datadog.AddConfigPath(".")
	err := config.Datadog.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("unable to load Datadog config file: %s", err))
	}

	aggregatorInstance := aggregator.GetAggregator()
	dogstatsd.RunServer(aggregatorInstance.GetChannel())
}
