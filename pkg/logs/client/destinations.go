// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

// Destinations holds the main destination and additional ones to send logs to.
type Destinations struct {
	Reliable    []Destination
	Additionals []Destination
}

// NewDestinations returns a new destinations composite.
func NewDestinations(reliable []Destination, additionals []Destination) *Destinations {
	return &Destinations{
		Reliable:    reliable,
		Additionals: additionals,
	}
}
