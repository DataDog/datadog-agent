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
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/cloudfoundry/cf_container"
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/cloudfoundry/cf_vm"
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/containerd"
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/docker"
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/ecs"
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/ecsfargate"
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/kubeapiserver"
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/kubelet"
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/kubemetadata"
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/podman"
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/remoteworkloadmeta"
)
