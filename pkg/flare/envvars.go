// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package flare

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var envvarNameWhitelist = []string{
	// Docker client
	"DOCKER_API_VERSION",
	"DOCKER_CONFIG",
	"DOCKER_CERT_PATH",
	"DOCKER_HOST",
	"DOCKER_TLS_VERIFY",

	// Proxy settings
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
	"GOTRACEBACK",

	// Trace agent
	"DD_APM_ENABLED",
	"DD_APM_NON_LOCAL_TRAFFIC",
	"DD_RECEIVER_PORT",   // deprecated
	"DD_IGNORE_RESOURCE", // deprecated
	"DD_APM_DD_URL",
	"DD_APM_ANALYZED_SPANS",
	"DD_CONNECTION_LIMIT", // deprecated
	"DD_APM_CONNECTION_LIMIT",
	"DD_APM_ENV",
	"DD_APM_RECEIVER_PORT",
	"DD_APM_IGNORE_RESOURCES",
	"DD_MAX_EPS ", // deprecated
	"DD_APM_MAX_EPS",
	"DD_APM_TPS", //deprecated
	"DD_APM_MAX_TPS",
	"DD_APM_MAX_MEMORY",
	"DD_APM_MAX_CPU_PERCENT",
	"DD_APM_FEATURES",
	"DD_APM_RECEIVER_SOCKET",
	"DD_APM_REPLACE_TAGS",
	"DD_APM_ADDITIONAL_ENDPOINTS",
	"DD_APM_PROFILING_DD_URL",
	"DD_APM_PROFILING_ADDITIONAL_ENDPOINTS",

	// Process agent
	"DD_PROCESS_AGENT_ENABLED",
	"DD_PROCESS_AGENT_URL",
	"DD_SCRUB_ARGS",
	"DD_CUSTOM_SENSITIVE_WORDS",
	"DD_STRIP_PROCESS_ARGS",
	"DD_LOGS_STDOUT",
	"DD_AGENT_PY",
	"DD_AGENT_PY_ENV",
	"LOG_LEVEL",
	"LOG_TO_CONSOLE",
	"DD_COLLECT_DOCKER_NETWORK",
	"DD_CONTAINER_BLACKLIST",
	"DD_CONTAINER_WHITELIST",
	"DD_CONTAINER_CACHE_DURATION",
	"DD_PROCESS_AGENT_CONTAINER_SOURCE",
	"DD_SYSTEM_PROBE_ENABLED",
	"DD_SYSTEM_PROBE_NETWORK_ENABLED",
}

func getWhitelistedEnvvars() []string {
	envVarWhiteList := append(envvarNameWhitelist, config.Datadog.GetEnvVars()...)
	var found []string
	for _, envvar := range os.Environ() {
		parts := strings.SplitN(envvar, "=", 2)
		key := strings.ToUpper(parts[0])
		if strings.Contains(key, "_KEY") || strings.Contains(key, "_AUTH_TOKEN") {
			// `_key`-suffixed env vars are sensitive: don't track them
			// `_auth_token`-suffixed env vars are sensitive: don't track them
			continue
		}
		for _, whitelisted := range envVarWhiteList {
			if key == whitelisted {
				found = append(found, envvar)
				continue
			}
		}
	}
	return found
}

// zipEnvvars collects whitelisted envvars that can affect the agent's
// behaviour while not being handled by viper
func zipEnvvars(tempDir, hostname string) error {
	envvars := getWhitelistedEnvvars()

	var b bytes.Buffer
	if len(envvars) > 0 {
		fmt.Fprintln(&b, "Found the following envvars:")
		for _, envvar := range envvars {
			fmt.Fprintln(&b, " - ", envvar)
		}
	} else {
		fmt.Fprintln(&b, "Found no whitelisted envvar")
	}

	f := filepath.Join(tempDir, hostname, "envvars.log")
	w, err := NewRedactingWriter(f, os.ModePerm, true)
	if err != nil {
		return err
	}
	defer w.Close()

	_, err = w.Write(b.Bytes())
	return err
}
