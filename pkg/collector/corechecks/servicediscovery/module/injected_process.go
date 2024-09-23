// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:generate go run github.com/tinylib/msgp -io=false

package module

// InjectedProcess represents the data injected by the auto-injector into the
// process.
type InjectedProcess struct {
	LocalHostname   string   `msgp:"local_hostname"`
	InjectedEnv     [][]byte `msgp:"injected_envs"`
	LanguageName    string   `msgp:"language_name"`
	TracerVersion   string   `msgp:"tracer_version"`
	InjectorVersion string   `msgp:"injector_version"`
}
