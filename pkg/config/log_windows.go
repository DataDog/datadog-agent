// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	log "github.com/cihub/seelog"
)

// GetSyslogURI returns the configured/default syslog uri
func GetSyslogURI() string {
	enabled := Datadog.GetBool("log_to_syslog")

	if enabled {
		log.Infof("logging to syslog is not available on windows.")
	}
	return ""
}
