// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !jmx

package standalone

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger"
	internalAPI "github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// ExecJMXCommandConsole is not supported when the 'jmx' build tag isn't included
func ExecJMXCommandConsole(command string, selectedChecks []string, logLevel string, configs []integration.Config, wmeta workloadmeta.Component, taggerComp tagger.Component, ac autodiscovery.Component, senderManager sender.DiagnoseSenderManager, agentAPI internalAPI.Component, collector optional.Option[collector.Component], jmxLogger jmxlogger.Component) error { //nolint:revive // TODO fix revive unused-parameter
	return fmt.Errorf("not supported: the Agent is compiled without the 'jmx' build tag")
}

// ExecJmxListWithMetricsJSON is not supported when the 'jmx' build tag isn't included
func ExecJmxListWithMetricsJSON(selectedChecks []string, logLevel string, configs []integration.Config, wmeta workloadmeta.Component, taggerComp tagger.Component, ac autodiscovery.Component, senderManager sender.DiagnoseSenderManager, agentAPI internalAPI.Component, collector optional.Option[collector.Component], jmxLogger jmxlogger.Component) error { //nolint:revive // TODO fix revive unused-parameter
	return fmt.Errorf("not supported: the Agent is compiled without the 'jmx' build tag")
}

// ExecJmxListWithRateMetricsJSON is not supported when the 'jmx' build tag isn't included
func ExecJmxListWithRateMetricsJSON(selectedChecks []string, logLevel string, configs []integration.Config, wmeta workloadmeta.Component, taggerComp tagger.Component, ac autodiscovery.Component, senderManager sender.DiagnoseSenderManager, agentAPI internalAPI.Component, collector optional.Option[collector.Component], jmxLogger jmxlogger.Component) error { //nolint:revive // TODO fix revive unused-parameter
	return fmt.Errorf("not supported: the Agent is compiled without the 'jmx' build tag")
}
