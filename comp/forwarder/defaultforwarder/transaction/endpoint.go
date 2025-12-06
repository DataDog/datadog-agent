// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transaction

import (
	"fmt"
	"net/url"
	"strings"
)

// Endpoint is an endpoint
type Endpoint struct {
	// Route to hit in the HTTP transaction
	Route string
	// Name of the endpoint for the telemetry metrics
	Name string
}

// String returns the route of the endpoint
func (e Endpoint) String() string {
	return e.Route
}

// IsFQDN returns if the route is a Fully Qualified Domain Name
func (e Endpoint) IsFQDN() bool {
	url, err := url.Parse(e.Route)
	if err != nil {
		return false
	}
	return strings.HasSuffix(url.Hostname(), ".")
}

// ToPQDN creates a new Endpoint with a Partially Qualified Domain Name (no trailing dot)
// Requires Route to be a valid URL
func (e Endpoint) ToPQDN() Endpoint {
	url, err := url.Parse(e.Route)
	if err != nil {
		panic("Route is not a valid URL")
	}

	host := strings.TrimSuffix(url.Host, ".")
	if port := url.Port(); port != "" {
		url.Host = fmt.Sprintf("%s:%s", host, port)
	} else {
		url.Host = host
	}

	return Endpoint{
		Route: url.String(),
		Name:  e.Name,
	}
}
