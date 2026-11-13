// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

// ReportingIdentity resolves the tuple a file's Agent Health reports name it by: a
// rotation loss, or files left untailed by the open files limit. The
// IntegrationName fallback keeps integration-only configs off the bare tuple;
// configs setting none of the three collapse onto ("unknown", "unknown").
func ReportingIdentity(cfg *config.LogsConfig) (source string, service string) {
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
