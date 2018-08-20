// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package flare

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
}

func getWhitelistedEnvvars() []string {
	var found []string
	for _, envvar := range os.Environ() {
		parts := strings.SplitN(envvar, "=", 2)
		for _, whitelisted := range envvarNameWhitelist {
			if parts[0] == whitelisted {
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

	fmt.Fprintln(&b, "Looked for these whitelisted envvars:")
	for _, envvar := range envvarNameWhitelist {
		fmt.Fprintln(&b, " - ", envvar)
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
