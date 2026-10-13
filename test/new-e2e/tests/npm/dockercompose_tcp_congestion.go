// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	_ "embed"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
)

//go:embed "config/dockercompose_tcp_congestion.yaml"
var dockerTCPCongestionComposeYaml string

// dockerTCPCongestionCompose returns the compose file with the apps image version substituted.
func dockerTCPCongestionCompose() string {
	return strings.ReplaceAll(dockerTCPCongestionComposeYaml, "{APPS_VERSION}", apps.Version)
}
