// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package netflow fetch information needed to render the 'netflow' section of the status page.
package netflow

import netflowServer "github.com/DataDog/datadog-agent/comp/netflow/server"

// PopulateStatus populates the status stats
func PopulateStatus(stats map[string]interface{}) {
	if netflowServer.IsEnabled() {
		stats["netflowStats"] = netflowServer.GetStatus()
	}
}
