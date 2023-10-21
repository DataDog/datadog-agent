// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || freebsd || netbsd || openbsd || solaris || dragonfly || darwin || aix

package config

// GetSyslogURI returns the configured/default syslog uri.
// Returns an empty string when syslog is disabled.
func GetSyslogURI() string {
	return GetSyslogURIFromConfig(Datadog)
}

// GetSyslogURIFromConfig is like GetSyslogURI but reads from the provided config
func GetSyslogURIFromConfig(cfg Config) string {
	enabled := cfg.GetBool("log_to_syslog")
	uri := cfg.GetString("syslog_uri")

	if !enabled {
		return ""
	}

	if uri == "" {
		return defaultSyslogURI
	}

	return uri
}
