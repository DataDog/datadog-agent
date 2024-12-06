// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !jmx

package standalone

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger"
	internalAPI "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

// ExecJMXCommandConsole is not supported when the 'jmx' build tag isn't included
func ExecJMXCommandConsole(_ string, _ []string, _ string, _ []integration.Config, _ internalAPI.Component, _ jmxlogger.Component) error {
	return fmt.Errorf("not supported: the Agent is compiled without the 'jmx' build tag")
}

// ExecJmxListWithMetricsJSON is not supported when the 'jmx' build tag isn't included
func ExecJmxListWithMetricsJSON(_ []string, _ string, _ []integration.Config, _ internalAPI.Component, _ jmxlogger.Component) error {
	return fmt.Errorf("not supported: the Agent is compiled without the 'jmx' build tag")
}

// ExecJmxListWithRateMetricsJSON is not supported when the 'jmx' build tag isn't included
func ExecJmxListWithRateMetricsJSON(_ []string, _ string, _ []integration.Config, _ internalAPI.Component, _ jmxlogger.Component) error {
	return fmt.Errorf("not supported: the Agent is compiled without the 'jmx' build tag")
}
