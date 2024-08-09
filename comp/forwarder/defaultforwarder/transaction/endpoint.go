// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transaction

import "strings"

// Endpoint is an endpoint
type Endpoint struct {
	//Subdomain of the endpoint
	Subdomain string
	// Route to hit in the HTTP transaction
	Route string
	// Name of the endpoint for the telemetry metrics
	Name string
}

// String returns the route of the endpoint
func (e Endpoint) String() string {
	return e.Route
}

// GetEndpoint returns the full endpoint URL
func (e Endpoint) GetEndpoint(domain string) string {
	if e.Subdomain == "" {
		e.Subdomain = "app"
	}

	e.Subdomain = strings.TrimSuffix(e.Subdomain, "/")
	e.Subdomain = strings.TrimPrefix(e.Subdomain, "/")

	domain = strings.TrimSuffix(domain, "/")
	e.Route = strings.TrimPrefix(e.Route, "/")

	url := domain + "/" + e.Route

	if !strings.Contains(url, "http://") && !strings.Contains(url, "https://") {
		url = "https://" + url
	}

	url = strings.Replace(url, "app", e.Subdomain, 1)

	return url
}
