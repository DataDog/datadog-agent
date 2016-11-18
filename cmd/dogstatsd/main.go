package main

import (
	// "fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
)

func main() {
	aggregatorInstance := aggregator.GetAggregator()
	dogstatsd.RunServer(aggregatorInstance.GetChannel())
}
