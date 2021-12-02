// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collectors is a wrapper that loads the available workloadmeta
// collectors. It exists as a shorthand for importing all packages manually in
// all of the agents.
package collectors

import (
	// this package only loads the collectors
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/cloudfoundry"
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/containerd"
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/docker"
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/ecs"
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/ecsfargate"
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/kubelet"
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/kubemetadata"
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/podman"
)
