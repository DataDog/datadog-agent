// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Contains BSD-2-Clause code (c) 2015-present Andrea Barberio

// Package dublintraceroute provides the common interface for Dublin Traceroute
package dublintraceroute

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/udp/results"
)

// default values and constants
const (
	DefaultReadTimeout = time.Millisecond * 3000
)

// DublinTraceroute is the common interface that every Dublin Traceroute
// probe type has to implement
type DublinTraceroute interface {
	Validate() error
	Traceroute() (*results.Results, error)
}
