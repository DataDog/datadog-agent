// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package sender

// Destinations holds the main destination and additional ones to send logs to.
type Destinations struct {
	Main       *Client
	Additonals []*Client
}

// NewDestinations returns a new destinations composite.
func NewDestinations(main *Client, additionnals []*Client) *Destinations {
	return &Destinations{
		Main:       main,
		Additonals: additionnals,
	}
}
