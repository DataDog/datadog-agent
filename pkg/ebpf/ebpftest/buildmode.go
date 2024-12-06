// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpftest

import (
	"fmt"
	"os"
)

// TODO I don't love fentry as a buildmode here...
var (
	Prebuilt        BuildMode
	RuntimeCompiled BuildMode
	CORE            BuildMode
	Fentry          BuildMode
	Ebpfless        BuildMode
)

func init() {
	Prebuilt = prebuiltMode{}
	RuntimeCompiled = runtimeCompiled{}
	CORE = core{}
	Fentry = fentry{}
	Ebpfless = ebpfless{}
}

// BuildMode is an eBPF build mode
type BuildMode interface {
	fmt.Stringer
	Env() map[string]string
}

type prebuiltMode struct{}

func (p prebuiltMode) String() string {
	return "prebuilt"
}

func (p prebuiltMode) Env() map[string]string {
	return map[string]string{
		"NETWORK_TRACER_FENTRY_TESTS":        "false",
		"DD_ENABLE_RUNTIME_COMPILER":         "false",
		"DD_ENABLE_CO_RE":                    "false",
		"DD_ALLOW_RUNTIME_COMPILED_FALLBACK": "false",
		"DD_ALLOW_PREBUILT_FALLBACK":         "false",
	}
}

type runtimeCompiled struct{}

func (r runtimeCompiled) String() string {
	return "runtime compiled"
}

func (r runtimeCompiled) Env() map[string]string {
	return map[string]string{
		"NETWORK_TRACER_FENTRY_TESTS":        "false",
		"DD_ENABLE_RUNTIME_COMPILER":         "true",
		"DD_ENABLE_CO_RE":                    "false",
		"DD_ALLOW_RUNTIME_COMPILED_FALLBACK": "false",
		"DD_ALLOW_PREBUILT_FALLBACK":         "false",
	}
}

type core struct{}

func (c core) String() string {
	return "CO-RE"
}

func (c core) Env() map[string]string {
	return map[string]string{
		"NETWORK_TRACER_FENTRY_TESTS":        "false",
		"DD_ENABLE_RUNTIME_COMPILER":         "false",
		"DD_ENABLE_CO_RE":                    "true",
		"DD_ALLOW_RUNTIME_COMPILED_FALLBACK": "false",
		"DD_ALLOW_PREBUILT_FALLBACK":         "false",
	}
}

type fentry struct{}

func (f fentry) String() string {
	return "fentry"
}

func (f fentry) Env() map[string]string {
	return map[string]string{
		"NETWORK_TRACER_FENTRY_TESTS":        "true",
		"DD_ENABLE_RUNTIME_COMPILER":         "false",
		"DD_ENABLE_CO_RE":                    "true",
		"DD_ALLOW_RUNTIME_COMPILED_FALLBACK": "false",
		"DD_ALLOW_PREBUILT_FALLBACK":         "false",
	}
}

type ebpfless struct{}

func (e ebpfless) String() string {
	return "eBPFless"
}

func (e ebpfless) Env() map[string]string {
	return map[string]string{
		"NETWORK_TRACER_FENTRY_TESTS":        "false",
		"DD_ENABLE_RUNTIME_COMPILER":         "false",
		"DD_ENABLE_CO_RE":                    "false",
		"DD_ALLOW_RUNTIME_COMPILED_FALLBACK": "false",
		"DD_ALLOW_PREBUILT_FALLBACK":         "false",
		"DD_NETWORK_CONFIG_ENABLE_EBPFLESS":  "true",
	}
}

// GetBuildMode returns which build mode the current environment matches, if any
func GetBuildMode() BuildMode {
	for _, mode := range []BuildMode{Prebuilt, RuntimeCompiled, CORE, Fentry} {
		if hasBuildModeEnv(mode) {
			return mode
		}
	}
	return nil
}

func hasBuildModeEnv(mode BuildMode) bool {
	for k, v := range mode.Env() {
		if os.Getenv(k) != v {
			return false
		}
	}
	return true
}
