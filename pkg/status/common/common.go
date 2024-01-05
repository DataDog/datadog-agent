// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common provides a function to get the status elements common to all Agent flavors.
package common

import (
	"context"
	"os"
	"runtime"
	"time"

	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/utils"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/aggregator"
	"github.com/DataDog/datadog-agent/pkg/status/collector"
	"github.com/DataDog/datadog-agent/pkg/status/compliance"
	"github.com/DataDog/datadog-agent/pkg/status/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/status/forwarder"
	"github.com/DataDog/datadog-agent/pkg/status/hostname"
	"github.com/DataDog/datadog-agent/pkg/status/netflow"
	"github.com/DataDog/datadog-agent/pkg/status/ntp"
	"github.com/DataDog/datadog-agent/pkg/status/snmptraps"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// GetStatus grabs the status from expvar and puts it into a map.
func GetStatus(invAgent inventoryagent.Component) (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	stats, err := expvarStats(stats, invAgent)
	if err != nil {
		log.Errorf("Error Getting ExpVar Stats: %v", err)
	}

	stats["version"] = version.AgentVersion
	stats["flavor"] = flavor.GetFlavor()
	stats["metadata"] = hostMetadataUtils.GetFromCache(context.TODO(), config.Datadog)
	stats["conf_file"] = config.Datadog.ConfigFileUsed()
	stats["pid"] = os.Getpid()
	stats["go_version"] = runtime.Version()
	stats["agent_start_nano"] = config.StartTime.UnixNano()
	stats["build_arch"] = runtime.GOARCH
	now := time.Now()
	stats["time_nano"] = now.UnixNano()

	return stats, nil
}

func expvarStats(stats map[string]interface{}, invAgent inventoryagent.Component) (map[string]interface{}, error) {
	var err error
	forwarder.PopulateStatus(stats)
	collector.PopulateStatus(stats)
	aggregator.PopulateStatus(stats)
	dogstatsd.PopulateStatus(stats)
	hostname.PopulateStatus(stats)
	err = ntp.PopulateStatus(stats)
	// invAgent can be nil when generating a status page for some agent where inventory is not enabled
	// (clusteragent, security-agent, ...).
	//
	// todo: (component) remove this condition once status is a component.
	if invAgent != nil {
		stats["agent_metadata"] = invAgent.Get()
	} else {
		stats["agent_metadata"] = map[string]string{}
	}

	snmptraps.PopulateStatus(stats)

	netflow.PopulateStatus(stats)

	compliance.PopulateStatus(stats)

	return stats, err
}
