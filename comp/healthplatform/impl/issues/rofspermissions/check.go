// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package rofspermissions

import (
	"strings"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Check if all directories agent could write to are writable by the agent.
func Check(cfg config.Component) (*healthplatform.IssueReport, error) {
	writeDirs := []string{
		cfg.GetString("conf_path"),
		cfg.GetString("run_path"),
	}

	var nonWritableDirs []string
	for _, dir := range writeDirs {
		if dir == "" {
			continue
		}

		writable, err := filesystem.IsWritable(dir)
		if err != nil {
			log.Warnf("Write permission check failed for %s: %v", dir, err)
			continue
		}

		if !writable {
			log.Warnf("Directory '%s' is not writable", dir)
			nonWritableDirs = append(nonWritableDirs, dir)
		}
	}
	report := &healthplatform.IssueReport{
		IssueId: "read-only-filesystem-error",
		Context: map[string]string{
			"directories": strings.Join(nonWritableDirs, ","),
		},
	}
	if len(nonWritableDirs) > 0 {
		return nil, nil
	}
	return report, nil
}
