// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"net/url"
	"os"
	"strings"

	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
)

// OPMSEndpointURL returns the absolute OPMS URL for path. Passing an empty path
// yields the OPMS origin.
//
// Both the Go OPMS client and the split-mode bootstrap command resolve the
// endpoint through this helper so the Go and Rust runners cannot diverge.
func (c *Config) OPMSEndpointURL(path string) string {
	scheme, host := "https", c.DDApiHost
	if os.Getenv(app.InternalUseDDURLForOPMSEnvVar) == "true" {
		host = c.DDHost
		if strings.HasPrefix(host, "http://") {
			scheme = "http"
		}
		host = strings.TrimPrefix(strings.TrimPrefix(host, "http://"), "https://")
	}
	return (&url.URL{Scheme: scheme, Host: host, Path: path}).String()
}
