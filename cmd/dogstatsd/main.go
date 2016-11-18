package main

import (
	// "fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
)

func main() {
	aggregatorInstance := aggregator.GetAggregator(config.NewConfig())
	dogstatsd.RunServer(aggregatorInstance.GetChannel())
}
