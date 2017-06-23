// +build dogstatsd

// Package common provides a set of common symbols needed by different packages,
// to avoid circular dependencies.
package common

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
)

var (
	// DSD is the global dogstastd instance
	DSD *dogstatsd.Server
)

// CreateDSD creates a new DSD server and assigns it to DSD global
func CreateDSD(agg *aggregator.BufferedAggregator) error {
	var err error
	DSD, err = dogstatsd.NewServer(agg.GetChannels())

	return err
}

// StopDSD stops the dogstatsd server
func StopDSD() {
	if DSD != nil {
		DSD.Stop()
	}
}
