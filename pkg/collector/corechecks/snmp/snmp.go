// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package cluster

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/yaml.v2"

	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

const (
	snmpCheckName = "snmp"
)

// Check aggregates metrics from one SnmpCheck instance
type Check struct {
	core.CheckBase
	dcaClient clusteragent.DCAClientInterface
	config    snmpConfig
}

type snmpInstanceConfig struct {
}

type snmpInitConfig struct{}

type snmpConfig struct {
	instance snmpInstanceConfig
	initConf snmpInitConfig
}

// Run executes the check
func (c *Check) Run() error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	nodeName, err := util.GetHostname()
	if err != nil {
		return err
	}

	schedule := types.ConfigsToSchedule{}
	configs, err := c.dcaClient.PostClusterCheckConfigs(nodeName, schedule)
	if err != nil {
		return err
	}

	log.Warnf("config: %#v\n", configs)

	sender.Commit()

	return nil
}

// Configure configures the snmp checks
func (c *Check) Configure(rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	err := c.CommonConfigure(rawInstance, source)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(rawInitConfig, &c.config.initConf)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(rawInstance, &c.config.instance)
	if err != nil {
		return err
	}

	dcaClient, err := clusteragent.GetClusterAgentClient()
	if err != nil {
		return err
	}
	c.dcaClient = dcaClient

	return nil
}

func snmpFactory() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(snmpCheckName),
	}
}

func init() {
	core.RegisterCheck(snmpCheckName, snmpFactory)
}
