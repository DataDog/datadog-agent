// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package stats provides statistical utilities for the trace package.
package stats

import (
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

// OTLPTracesToConcentratorInputs converts eligible OTLP spans to Concentrator.Input.
// The converted Inputs only have the minimal number of fields for APM stats calculation and are only meant
// to be used in Concentrator.Add(). Do not use them for other purposes.
func OTLPTracesToConcentratorInputs(
	traces ptrace.Traces,
	conf *config.AgentConfig,
	containerTagKeys []string,
	peerTagKeys []string,
) []stats.Input {
	return OTLPTracesToConcentratorInputsWithObfuscation(traces, conf, containerTagKeys, peerTagKeys, nil)
}

// OTLPTracesToConcentratorInputsWithObfuscation converts eligible OTLP spans to Concentrator Input.
// The converted Inputs only have the minimal number of fields for APM stats calculation and are only meant
// to be used in Concentrator.Add(). Do not use them for other purposes.
// This function enables obfuscation of spans prior to stats calculation and datadogconnector will migrate
// to this function once this function is published as part of latest pkg/trace module.
func OTLPTracesToConcentratorInputsWithObfuscation(
	traces ptrace.Traces,
	conf *config.AgentConfig,
	containerTagKeys []string,
	peerTagKeys []string,
	obfuscator *obfuscate.Obfuscator,
) []stats.Input {
	return stats.OTLPTracesToConcentratorInputsWithObfuscation(traces, conf, containerTagKeys, peerTagKeys, obfuscator)
}

// newTestObfuscator creates a new obfuscator for testing
func newTestObfuscator(conf *config.AgentConfig) *obfuscate.Obfuscator {
	oconf := conf.Obfuscation.Export(conf)
	oconf.Redis.Enabled = true
	o := obfuscate.NewObfuscator(oconf)
	return o
}
