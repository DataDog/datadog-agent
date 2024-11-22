// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetSyslogURI returns the configured/default syslog uri
func GetSyslogURI(cfg pkgconfigmodel.Reader) string {
	return GetSyslogURIFromConfig(cfg)
}

// GetSyslogURIFromConfig is like GetSyslogURI but reads from the provided config
func GetSyslogURIFromConfig(cfg pkgconfigmodel.Reader) string {
	enabled := cfg.GetBool("log_to_syslog")

	if enabled {
		log.Infof("logging to syslog is not available on windows.")
	}
	return ""
}
