// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package env

import (
	"net/http"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// GetHTTPClient returns an HTTP client with the proxy settings loaded from the environment.
func GetHTTPClient() *http.Client {
	// Load proxy settings before creating any HTTP transport
	pkgconfigsetup.LoadProxyFromEnv(pkgconfigsetup.Datadog())
	httpClient := http.DefaultClient
	httpClient.Transport = httputils.CreateHTTPTransport(pkgconfigsetup.Datadog())
	return httpClient
}
