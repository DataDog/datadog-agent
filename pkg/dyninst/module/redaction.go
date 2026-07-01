// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/dyninst/redaction"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Environment variables, read from the target process, that tune dynamic
// instrumentation redaction. They mirror the Java, .NET, and Python tracers.
const (
	envRedactedIdentifiers          = "DD_DYNAMIC_INSTRUMENTATION_REDACTED_IDENTIFIERS"
	envRedactedTypes                = "DD_DYNAMIC_INSTRUMENTATION_REDACTED_TYPES"
	envRedactionExcludedIdentifiers = "DD_DYNAMIC_INSTRUMENTATION_REDACTION_EXCLUDED_IDENTIFIERS"
)

// redactionConfigForPID builds the redaction policy for a target process by
// reading the redaction environment variables from its environment. The
// default keyword set always applies, so a process that sets nothing is still
// protected.
func redactionConfigForPID(pid int32) *redaction.Config {
	return redactionConfig(pid, kernel.ProcFSRoot())
}

func redactionConfig(pid int32, procRoot string) *redaction.Config {
	env := processEnviron(pid, procRoot)
	values := func(name string) []string {
		if v := env[name]; v != "" {
			return strings.Split(v, ",")
		}
		return nil
	}
	return redaction.NewConfig(
		values(envRedactedIdentifiers),
		values(envRedactedTypes),
		values(envRedactionExcludedIdentifiers),
	)
}

// processEnviron reads a target process's environment in a single pass. It
// returns nil when the environ file cannot be read (the process exited or is
// inaccessible), in which case only the default policy applies. The file holds
// NUL-separated KEY=VALUE entries.
func processEnviron(pid int32, procRoot string) map[string]string {
	path := filepath.Join(procRoot, strconv.Itoa(int(pid)), "environ")
	data, err := os.ReadFile(path)
	if err != nil {
		log.Debugf("dynamic instrumentation: reading environ for pid %d: %v", pid, err)
		return nil
	}
	env := make(map[string]string)
	for _, entry := range bytes.Split(data, []byte{0}) {
		if key, value, ok := bytes.Cut(entry, []byte{'='}); ok {
			env[string(key)] = string(value)
		}
	}
	return env
}
