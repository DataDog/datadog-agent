// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

// defaultConfFile returns the default location of the Datadog configuration file. It is a variable
// so that the tests can keep off the file of an Agent installed on the machine.
var defaultConfFile = defaultpaths.GetDefaultConfFile

// defaultCoreConfPath returns the Datadog configuration file to load when --core-config isn't given,
// like `otel-agent run` does: the default datadog.yaml of the Agent, when it exists. The flare then
// has the settings of the Agent, and finds its IPC token and certificate next to the file, whatever
// the working directory is. A standalone collector has no Agent, so it loads nothing.
func defaultCoreConfPath() string {
	if standalone, err := strconv.ParseBool(os.Getenv("DD_OTEL_STANDALONE")); err == nil && standalone {
		return ""
	}
	// Stat follows the symlinks that the container entrypoints create
	path := defaultConfFile()
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return ""
	}
	return path
}
