package main

import (
	// "fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
)

func main() {
	dogstatsd.RunServer(aggregator.GetChannel())
}
