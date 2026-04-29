// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package launchmetrics

import (
	"runtime"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/serverless/tags"
)

// tlmStarted backs datadog.agent.serverless_init.started — emitted once per
// serverless-init process start, scraped by the agent telemetry collector.
var tlmStarted = telemetryimpl.GetCompatComponent().NewGauge(
	"serverless_init",
	"started",
	[]string{"init_mode", "init_version", "init_platform", "arch"},
	"Emitted once per serverless-init process start",
)

// Emit sets the launch gauge to 1 with the resolved tag values. Must be
// called before the agent telemetry collector runs its initial scrape (the
// serverless-init-started profile uses iterations=1).
func Emit(cloudService cloudservice.CloudService, sidecarMode bool) {
	mode := "process_wrapper"
	if sidecarMode {
		mode = "sidecar"
	}
	tlmStarted.Set(1,
		mode,
		tags.GetExtensionVersion(),
		DetectPlatform(cloudService),
		normalizeArch(runtime.GOARCH),
	)
}

// normalizeArch maps runtime.GOARCH (~25 possible values across all of Go's
// supported targets) to a bounded allowlist. In practice serverless-init's
// build matrix only produces "amd64" and "arm64". The "other" arm exists
// so a new build target — or any unexpected value — emits a defined tag
// value rather than expanding the metric's tag value space uncoordinated.
func normalizeArch(arch string) string {
	switch arch {
	case "amd64", "arm64":
		return arch
	default:
		return "other"
	}
}
