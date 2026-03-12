// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

// RegisterDefaults is a no-op: all flightrecorder defaults are registered in
// pkg/config/setup/config.go via BindEnvAndSetDefault so they are available
// before any component is constructed.
//
// The function is kept as a named symbol so tests can call it explicitly when
// they build a standalone config without the full setup package.
func RegisterDefaults(_ interface{}) {}
