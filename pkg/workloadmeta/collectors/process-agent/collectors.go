// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package process_agent is a meta-package that loads the internal process agent collectors.
// It should _never_ be imported outside `github.com/DataDog/datadog-agent/cmd/process-agent`
package process_agent

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/process"
)

func AnyCollectorsEnabled(reader config.ConfigReader) bool {
	localProcessCollectorEnabled := process.Enabled(reader) != nil
	return localProcessCollectorEnabled
}
