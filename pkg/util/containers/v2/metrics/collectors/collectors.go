// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package collectors

import (
	// Register all the collectors
	_ "github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/containerd"
	_ "github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/docker"
	_ "github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/ecsfargate"
	_ "github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/system"
)
