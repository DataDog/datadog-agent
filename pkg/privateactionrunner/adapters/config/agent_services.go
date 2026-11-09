// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"net"
	"strconv"

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	confighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	configsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func (c *agentConfig) AgentAPIURL() (string, error) {
	address, err := confighelper.GetIPCAddress(c.source)
	if err != nil {
		return "", err
	}
	return "https://" + net.JoinHostPort(address, strconv.Itoa(c.source.GetInt("cmd_port"))), nil
}
func (c *agentConfig) HasAPIKey() bool { return c.source.IsConfigured("api_key") }
func (c *agentConfig) LogProcessingRules() ([]*logsconfig.ProcessingRule, error) {
	return logsconfig.GlobalProcessingRules(c.source)
}
func (c *agentConfig) CaptureSocketPaths() (string, string) {
	source := configsetup.SystemProbe()
	return source.GetString("packet_capture.socket"), source.GetString("system_probe_config.sysprobe_socket")
}
