// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetSyslogURI returns the configured/default syslog uri
func GetSyslogURI() string {
	return GetSyslogURIFromConfig(Datadog)
}

// GetSyslogURIFromConfig is like GetSyslogURI but reads from the provided config
func GetSyslogURIFromConfig(cfg Config) string {
	enabled := cfg.GetBool("log_to_syslog")

	if enabled {
		log.Infof("logging to syslog is not available on windows.")
	}
	return ""
}
