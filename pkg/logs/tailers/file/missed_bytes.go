// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

// missedBytesIdentity resolves the tuple to report a rotation loss under. Both
// fields can be empty, so the fallbacks keep unlabelled configs from collapsing
// onto one tuple.
func missedBytesIdentity(cfg *config.LogsConfig) (source string, service string) {
	const unknown = "unknown"
	if cfg == nil {
		return unknown, unknown
	}

	source = unknown
	switch {
	case cfg.Source != "":
		source = cfg.Source
	case cfg.IntegrationName != "":
		source = cfg.IntegrationName
	}

	service = unknown
	switch {
	case cfg.Service != "":
		service = cfg.Service
	case cfg.Source != "":
		service = cfg.Source
	case cfg.IntegrationName != "":
		service = cfg.IntegrationName
	}

	return source, service
}
