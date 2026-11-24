// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	_ "embed"
	"strings"
)

//go:embed "config/dockercompose_tcp_congestion.yaml"
var dockerTCPCongestionComposeYaml string

// dockerTCPCongestionCompose returns the compose file running npmToolsImage, which
// pullNPMToolsImage must have stored on the host beforehand.
func dockerTCPCongestionCompose() string {
	return strings.ReplaceAll(dockerTCPCongestionComposeYaml, "{NPM_TOOLS_IMAGE}", npmToolsImage)
}
