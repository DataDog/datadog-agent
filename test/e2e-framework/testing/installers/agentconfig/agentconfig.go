// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agentconfig is the single datadog.yaml generation policy shared by
// the host-side installers: the install script (on a provisioned VM) and the
// binary installer (in a local container). One wiring policy means fakeintake
// routing never forks between installation methods.
package agentconfig

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils/yamlutil"
)

// Endpoint describes where the Agent sends its data.
type Endpoint struct {
	Scheme string
	Host   string
	Port   int
}

// Generate builds the Agent's datadog.yaml: api key, fakeintake wiring when an
// endpoint is given, and the user's extra config merged last. extraConfig may
// override generated keys deliberately (documented escape hatch).
func Generate(apiKey string, endpoint *Endpoint, extraConfig string) (string, error) {
	config := fmt.Sprintf("api_key: %q\n", apiKey)
	if endpoint != nil {
		config += fmt.Sprintf(`dd_url: %s://%s:%d
logs_config.logs_dd_url: %s:%d
logs_config.logs_no_ssl: true
logs_config.force_use_http: true
`, endpoint.Scheme, endpoint.Host, endpoint.Port, endpoint.Host, endpoint.Port)
	}
	if extraConfig == "" {
		return config, nil
	}
	merged, err := yamlutil.MergeYAMLWithSlices(config, extraConfig)
	if err != nil {
		return "", fmt.Errorf("merging Agent config: %w", err)
	}
	return merged, nil
}
