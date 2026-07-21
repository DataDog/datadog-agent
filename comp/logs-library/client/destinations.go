// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

// Destinations encapsulates a set of log destinations, distinguishing reliable vs unreliable destinations
type Destinations struct {
	Reliable   []Destination
	Unreliable []Destination
}

// NewDestinations returns a new destinations composite.
func NewDestinations(reliable []Destination, unreliable []Destination) *Destinations {
	return &Destinations{
		Reliable:   reliable,
		Unreliable: unreliable,
	}
}
