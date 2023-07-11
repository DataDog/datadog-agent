// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpftest

import (
	"runtime"
	"testing"

	"github.com/DataDog/gopsutil/host"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var hostinfo *host.InfoStat
var kv kernel.Version

func init() {
	kv, _ = kernel.HostVersion()
	hostinfo, _ = host.Info()
}

func SupportedBuildModes() []BuildMode {
	modes := []BuildMode{Prebuilt, RuntimeCompiled, CORE}
	if runtime.GOARCH == "amd64" && (hostinfo.Platform == "amazon" || hostinfo.Platform == "amzn") && kv.Major() == 5 && kv.Minor() == 10 {
		modes = append(modes, Fentry)
	}
	return modes
}

func TestBuildModes(t *testing.T, modes []BuildMode, name string, fn func(t *testing.T)) {
	for _, mode := range modes {
		TestBuildMode(t, mode, name, fn)
	}
}

func TestBuildMode(t *testing.T, mode BuildMode, name string, fn func(t *testing.T)) {
	t.Run(mode.String(), func(t *testing.T) {
		for k, v := range mode.Env() {
			t.Setenv(k, v)
		}
		if name != "" {
			t.Run(name, fn)
		} else {
			fn(t)
		}
	})
}
