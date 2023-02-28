// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package constants

var (
	// DefaultConfPath points to the folder containing datadog.yaml
	DefaultConfPath = "c:\\programdata\\datadog"
	// DefaultLogFile points to the log file that will be used if not configured
	DefaultLogFile = "c:\\programdata\\datadog\\logs\\agent.log"
	// DefaultDCALogFile points to the log file that will be used if not configured
	DefaultDCALogFile = "c:\\programdata\\datadog\\logs\\cluster-agent.log"
	//DefaultJmxLogFile points to the jmx fetch log file that will be used if not configured
	DefaultJmxLogFile = "c:\\programdata\\datadog\\logs\\jmxfetch.log"
	// DefaultCheckFlareDirectory a flare friendly location for checks to be written
	DefaultCheckFlareDirectory = "c:\\programdata\\datadog\\logs\\checks\\"
	// DefaultJMXFlareDirectory a flare friendly location for jmx command logs to be written
	DefaultJMXFlareDirectory = "c:\\programdata\\datadog\\logs\\jmxinfo\\"
	//DefaultDogstatsDLogFile points to the dogstatsd stats log file that will be used if not configured
	DefaultDogstatsDLogFile = "c:\\programdata\\datadog\\logs\\dogstatsd_info\\dogstatsd-stats.log"
)
