// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// extraEnvVars are the environment variables that change how the collector runs or reaches Datadog
// without being bound to a setting of the Datadog configuration. They are kept by their whole name,
// whatever the rules of filterEnvironment say about a part of it, and their values are scrubbed like
// any other.
var extraEnvVars = []string{
	// Proxy settings. The Agent reads DD_PROXY_* outside the schema, and the core Agent flare lists
	// them too. The scrubber masks the password of a URL.
	"HTTP_PROXY",
	"HTTPS_PROXY",
	"NO_PROXY",
	"DD_PROXY_HTTP",
	"DD_PROXY_HTTPS",
	"DD_PROXY_NO_PROXY",

	// Go runtime
	"GOGC",
	"GODEBUG",
	"GOMAXPROCS",
	"GOMEMLIMIT",
	"GOTRACEBACK",

	// Go TLS trusted root certificate locations
	"SSL_CERT_FILE",
	"SSL_CERT_DIR",

	// Memory allocator preloaded in the container images
	"LD_PRELOAD",
	"MALLOC_CONF",

	// Host paths used in a container
	"HOST_ETC",
	"HOST_PROC",
	"HOST_ROOT",

	// Where the collector reads its auth token: a path, not the token
	"DD_AUTH_TOKEN_FILE_PATH",

	// The name of the pod or of the host
	"HOSTNAME",
}

// filterEnvironment keeps the environment variables that are safe to put in a flare, like the core
// Agent flare does: the ones bound to a setting of the Datadog configuration, except the ones bound
// to sensitive settings, and extraEnvVars. Any other variable could hold a secret that no scrubber
// recognizes, so it is dropped instead of scrubbed.
func filterEnvironment(env map[string]string, cfg pkgconfigmodel.Reader) map[string]string {
	allowed := map[string]struct{}{}
	for _, name := range cfg.FilterSensitiveEnvVars(cfg.GetEnvVars()) {
		allowed[strings.ToUpper(name)] = struct{}{}
	}
	extra := map[string]struct{}{}
	for _, name := range extraEnvVars {
		extra[name] = struct{}{}
	}

	filtered := make(map[string]string, len(env))
	for name, value := range env {
		upper := strings.ToUpper(name)
		if _, ok := extra[upper]; ok {
			filtered[name] = value
			continue
		}
		// Same rule as the core Agent flare, in case a credential isn't declared sensitive in the schema
		if strings.Contains(upper, "_KEY") || strings.Contains(upper, "_AUTH_TOKEN") {
			continue
		}
		if _, ok := allowed[upper]; ok {
			filtered[name] = value
		}
	}
	return filtered
}
