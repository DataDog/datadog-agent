// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package client

// Destinations holds the main destination and additional ones to send logs to.
type Destinations struct {
	Main        Destination
	Additionals []Destination
}

// NewDestinations returns a new destinations composite.
func NewDestinations(main Destination, additionals []Destination) *Destinations {
	return &Destinations{
		Main:        main,
		Additionals: additionals,
	}
}
