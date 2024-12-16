// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package netpath contains e2e tests for Network Path Integration feature
package haagent

import (
	_ "embed"
)

//go:embed fixtures/system-probe.yaml
var sysProbeConfig []byte

//go:embed fixtures/network_path.yaml
var networkPathIntegration []byte

//go:embed fixtures/snmp.yaml
var snmpIntegration []byte
