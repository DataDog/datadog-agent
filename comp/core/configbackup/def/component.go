// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package configbackup implements backing up the agent configuration files at every agent start.
package configbackup

// team: fleet-automation

// Component is the component type.
//
// The component exposes no public method. Its only purpose is to run a
// one-shot side effect (the OnStart hook that writes a configuration
// snapshot), so nothing declares it as a dependency and the fx module forces
// its construction with an fx.Invoke.
type Component interface{}
