// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package flare

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

var envvarPrefixWhitelist = []string{
	// Docker client
	"DOCKER_API_VERSION=",
	"DOCKER_CONFIG=",
	"DOCKER_CERT_PATH=",
	"DOCKER_HOST=",
	"DOCKER_TLS_VERIFY=",
	"HTTP_PROXY=",
	"HTTPS_PROXY=",
	"NO_PROXY=",

	// Go runtime
	"GOGC=",
	"GODEBUG=",
	"GOMAXPROCS=",
	"GOTRACEBACK=",
}

func getWhitelistedEnvvars() []string {
	var vars []string
	for _, envvar := range os.Environ() {
		for _, prefix := range envvarPrefixWhitelist {
			if strings.HasPrefix(envvar, prefix) {
				vars = append(vars, envvar)
				continue
			}
		}
	}
	return vars
}

// zipEnvvars collects whitelisted envvars that can affect the agent's
// behaviour while not being handled by viper
func zipEnvvars(tempDir, hostname string) error {
	envvars := getWhitelistedEnvvars()
	if len(envvars) == 0 {
		// Don't create the file if we have nothing
		return nil
	}

	var b bytes.Buffer
	for _, envvar := range envvars {
		b.WriteString(envvar)
		b.WriteString("\n")
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
