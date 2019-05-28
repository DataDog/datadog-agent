// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package client

import (
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/client/tcp"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// Destinations holds the main destination and additional ones to send logs to.
type Destinations struct {
	Main        Destination
	Additionals []Destination
}

// NewDestinations takes endpoints configuration and returns
func NewDestinations(endpoints *config.Endpoints, destinationsContext *tcp.DestinationsContext) *Destinations {
	destinations := &Destinations{}
	var additionals []Destination

	if endpoints.UseHTTP {
		destinations.Main = http.NewDestination(endpoints.Main)
		for _, endpoint := range endpoints.Additionals {
			additionals = append(additionals, http.NewDestination(endpoint))
		}
		destinations.Additionals = additionals
	} else {
		destinations.Main = tcp.NewDestination(endpoints.Main, endpoints.UseProto, destinationsContext)
		for _, endpoint := range endpoints.Additionals {
			additionals = append(additionals, tcp.NewDestination(endpoint, endpoints.UseProto, destinationsContext))
		}
		destinations.Additionals = additionals
	}

	return destinations
}

// NewDestinationsOld returns a new destinations composite.
// TODO(achntrl): To remove
func NewDestinationsOld(main Destination, additionals []Destination) *Destinations {
	return &Destinations{
		Main:        main,
		Additionals: additionals,
	}
}
