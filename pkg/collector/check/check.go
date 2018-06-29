// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package check

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"time"
)

const (
	// DefaultCheckInterval is the interval in seconds the scheduler should apply
	// when no value was provided in Check configuration.
	DefaultCheckInterval time.Duration = 15 * time.Second
)

// Check is an interface for types capable to run checks
type Check interface {
	Run() error                                          // run the check
	Stop()                                               // stop the check if it's running
	String() string                                      // provide a printable version of the check name
	Configure(config, initConfig integration.Data) error // configure the check from the outside
	Interval() time.Duration                             // return the interval time for the check
	ID() ID                                              // provide a unique identifier for every check instance
	GetWarnings() []error                                // return the last warning registered by the check
	GetMetricStats() (map[string]int64, error)           // get metric stats from the sender
	Version() string                                     // return the version of the check if available
}
