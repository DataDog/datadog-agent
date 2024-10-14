// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package endpoints stores a collection of `transaction.Endpoint` mainly used by the forwarder package to send data to
// Datadog using the right request path for a given type of data.
package transaction

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint Endpoint
		domain   string
		want     string
	}{
		{
			name:     "default subdomain applied when empty",
			endpoint: Endpoint{Route: "docs"},
			domain:   "https://dev.example.com",
			want:     "https://dev.example.com/docs",
		},
		{
			name:     "explicit subdomain replacement",
			endpoint: Endpoint{Subdomain: "admin", Route: "docs"},
			domain:   "app.example.com",
			want:     "https://admin.example.com/docs",
		},
		{
			name:     "no subdomain, app not in domain",
			endpoint: Endpoint{Subdomain: "app/", Route: "docs"},
			domain:   "myappsite.com",
			want:     "https://myappsite.com/docs",
		},
		{
			name:     "subdomain app with app in domain",
			endpoint: Endpoint{Subdomain: "app", Route: "support"},
			domain:   "https://app.company.com",
			want:     "https://app.company.com/support",
		},
		{
			name:     "complex route and domain",
			endpoint: Endpoint{Subdomain: "api", Route: "/v1/users"},
			domain:   "https://app.service.com",
			want:     "https://api.service.com/v1/users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.endpoint.GetEndpoint(tt.domain)
			assert.Equal(t, tt.want, got)
		})
	}
}
